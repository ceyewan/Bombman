package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
	"bomberman/pkg/protocol"
)

type Room struct {
	ctx    context.Context
	cancel context.CancelFunc

	game    *core.Game
	frameID int32
	state   GameState
	resetAt time.Time

	connections     map[int32]*Connection
	nextPlayerID    int32
	inputQueue      map[int32]*gamev1.ClientInput
	sendQueueFullAt map[int32]time.Time

	// 客户端预测支持：记录每个玩家最后处理的输入序号
	lastProcessedInputSeq map[int32]int32

	joinCh  chan joinRequest
	inputCh chan inputEvent
	leaveCh chan int32
}

type joinRequest struct {
	conn   *Connection
	req    *gamev1.JoinRequest
	respCh chan error
}

type inputEvent struct {
	playerID int32
	input    *gamev1.ClientInput
}

func NewRoom(parent context.Context) *Room {
	ctx, cancel := context.WithCancel(parent)

	return &Room{
		ctx:                   ctx,
		cancel:                cancel,
		game:                  core.NewGame(),
		frameID:               0,
		state:                 StateWaiting,
		connections:           make(map[int32]*Connection),
		nextPlayerID:          1,
		inputQueue:            make(map[int32]*gamev1.ClientInput),
		sendQueueFullAt:       make(map[int32]time.Time),
		lastProcessedInputSeq: make(map[int32]int32),
		joinCh:                make(chan joinRequest),
		inputCh:               make(chan inputEvent, 256),
		leaveCh:               make(chan int32, 256),
	}
}

func (r *Room) Run(wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(TickDuration)
	defer ticker.Stop()

	log.Printf("房间循环启动: %d TPS", ServerTPS)

	for {
		select {
		case <-r.ctx.Done():
			r.closeAllConnections(false)
			log.Println("房间循环停止")
			return

		case req := <-r.joinCh:
			r.handleJoin(req)

		case ev := <-r.inputCh:
			r.handleInput(ev)

		case playerID := <-r.leaveCh:
			r.handleLeave(playerID)

		case <-ticker.C:
			r.tick()
		}
	}
}

func (r *Room) Shutdown() {
	r.cancel()
}

func (r *Room) Join(conn *Connection, req *gamev1.JoinRequest) error {
	respCh := make(chan error, 1)

	select {
	case <-r.ctx.Done():
		return fmt.Errorf("房间已关闭")
	case r.joinCh <- joinRequest{conn: conn, req: req, respCh: respCh}:
	}

	select {
	case <-r.ctx.Done():
		return fmt.Errorf("房间已关闭")
	case err := <-respCh:
		return err
	}
}

func (r *Room) EnqueueInput(playerID int32, input *gamev1.ClientInput) {
	select {
	case <-r.ctx.Done():
		return
	case r.inputCh <- inputEvent{playerID: playerID, input: input}:
	}
}

func (r *Room) Leave(playerID int32) {
	select {
	case <-r.ctx.Done():
		return
	case r.leaveCh <- playerID:
	}
}

func (r *Room) tick() {
	now := time.Now()

	if r.state == StateEnding && !r.resetAt.IsZero() && now.After(r.resetAt) {
		r.resetRoom()
		return
	}

	if r.state != StateRunning {
		return
	}

	r.applyInputs()

	// 更新核心游戏逻辑
	r.game.Update(1.0 / ServerTPS)

	// 增加帧 ID
	r.frameID++

	if shouldEnd, winnerID := r.checkGameOver(); shouldEnd {
		r.handleGameOver(winnerID)
	}

	if r.state == StateRunning {
		r.broadcastState()
	}
}

func (r *Room) applyInputs() {
	if len(r.inputQueue) == 0 {
		return
	}

	inputs := r.inputQueue
	r.inputQueue = make(map[int32]*gamev1.ClientInput, len(inputs))

	for playerID, input := range inputs {
		r.applyInput(playerID, input)
		// 记录已处理的输入序号
		if input.Seq > r.lastProcessedInputSeq[playerID] {
			r.lastProcessedInputSeq[playerID] = input.Seq
		}
	}
}

func (r *Room) applyInput(playerID int32, input *gamev1.ClientInput) {
	if input == nil {
		return
	}
	ci := core.Input{
		Up:    input.Up,
		Down:  input.Down,
		Left:  input.Left,
		Right: input.Right,
		Bomb:  input.Bomb,
	}

	placed := core.ApplyInput(r.game, int(playerID), ci, 1.0/ServerTPS)
	if placed {
		log.Printf("玩家 %d 放置炸弹", playerID)
	}
}

func (r *Room) handleJoin(req joinRequest) {
	if r.state == StateEnding {
		req.respCh <- fmt.Errorf("房间结算中，暂时无法加入")
		return
	}

	if len(r.connections) >= MaxPlayers {
		req.respCh <- fmt.Errorf("服务器已满 (%d/%d)", len(r.connections), MaxPlayers)
		return
	}

	// 转换角色类型
	characterType := protocol.ProtoCharacterTypeToCore(req.req.CharacterType)

	// 分配玩家 ID
	playerID := r.nextPlayerID
	r.nextPlayerID++

	// 获取出生点
	x, y := getSpawnPosition(int(playerID))

	// 创建玩家
	player := core.NewPlayer(int(playerID), x, y, characterType)
	player.IsSimulated = false // 服务器直接驱动

	// 添加到游戏
	r.game.AddPlayer(player)

	// 保存连接
	req.conn.setPlayerID(playerID)
	r.connections[playerID] = req.conn

	// 构造游戏开始消息
	initialMap := protocol.CoreMapToProto(r.game.Map)
	packet := protocol.NewGameStart(playerID, initialMap)
	data, err := protocol.Marshal(packet)
	if err != nil {
		r.removePlayerByID(playerID)
		delete(r.connections, playerID)
		req.conn.setPlayerID(-1)
		req.respCh <- fmt.Errorf("序列化游戏开始消息失败: %w", err)
		return
	}

	// 发送给客户端
	if err := req.conn.Send(data); err != nil {
		r.removePlayerByID(playerID)
		delete(r.connections, playerID)
		req.conn.setPlayerID(-1)
		req.respCh <- fmt.Errorf("发送游戏开始消息失败: %w", err)
		return
	}

	log.Printf("玩家 %d 加入，角色: %s, 出生点: (%d, %d)", playerID, characterType, x, y)
	log.Printf("玩家 %d 游戏开始消息已发送", playerID)

	if r.state != StateRunning {
		r.state = StateRunning
	}

	req.respCh <- nil
}

func (r *Room) handleInput(ev inputEvent) {
	if r.state != StateRunning {
		return
	}

	if _, exists := r.connections[ev.playerID]; !exists {
		return
	}

	r.inputQueue[ev.playerID] = ev.input
}

func (r *Room) handleLeave(playerID int32) {
	if _, exists := r.connections[playerID]; !exists {
		return
	}

	delete(r.connections, playerID)
	delete(r.inputQueue, playerID)
	delete(r.sendQueueFullAt, playerID)
	delete(r.lastProcessedInputSeq, playerID)

	r.removePlayerByID(playerID)

	log.Printf("玩家 %d 离开，当前玩家数: %d", playerID, len(r.connections))

	// 广播玩家离开消息
	packet := protocol.NewPlayerLeave(playerID)
	data, _ := protocol.Marshal(packet)

	for _, c := range r.connections {
		c.Send(data)
	}

	if len(r.game.Players) == 0 && r.state == StateRunning {
		r.handleGameOver(-1)
	}
}

func (r *Room) handleGameOver(winnerID int32) {
	if r.state == StateEnding {
		return
	}

	r.state = StateEnding
	r.resetAt = time.Now().Add(3 * time.Second)

	log.Printf("游戏结束，获胜者: %d", winnerID)

	r.broadcastGameOver(winnerID)
}

func (r *Room) resetRoom() {
	r.closeAllConnections(false)
	r.game = core.NewGame()
	r.frameID = 0
	r.state = StateWaiting
	r.resetAt = time.Time{}
	r.nextPlayerID = 1
	r.connections = make(map[int32]*Connection)
	r.inputQueue = make(map[int32]*gamev1.ClientInput)
	r.sendQueueFullAt = make(map[int32]time.Time)
	r.lastProcessedInputSeq = make(map[int32]int32)
}

func (r *Room) closeAllConnections(notify bool) {
	for _, conn := range r.connections {
		if notify {
			conn.Close()
		} else {
			conn.CloseWithoutNotify()
		}
	}
}

func (r *Room) broadcastState() {
	// 转换玩家列表
	protoPlayers := protocol.CorePlayersToProto(r.game.Players)

	// 转换炸弹列表
	protoBombs := protocol.CoreBombsToProto(r.game.Bombs)

	// 转换爆炸列表
	protoExplosions := protocol.CoreExplosionsToProto(r.game.Explosions)

	// 服务器当前时间（毫秒）
	serverTimeMs := time.Now().UnixMilli()

	// 构造 ServerState 消息（带客户端预测支持）
	packet := protocol.NewServerStateWithMeta(
		r.frameID,
		protoPlayers,
		protoBombs,
		protoExplosions,
		r.lastProcessedInputSeq,
		serverTimeMs,
	)

	// 序列化
	data, err := protocol.Marshal(packet)
	if err != nil {
		log.Printf("序列化状态失败: %v", err)
		return
	}

	// 发送到所有连接
	for _, conn := range r.connections {
		if err := conn.Send(data); err != nil {
			if errors.Is(err, ErrSendQueueFull) {
				r.handleSendQueueFull(conn)
				continue
			}
			log.Printf("发送状态到玩家 %d 失败: %v", conn.getPlayerID(), err)
			conn.Close()
			continue
		}
		delete(r.sendQueueFullAt, conn.getPlayerID())
	}
}

const sendQueueFullGrace = 2 * time.Second

func (r *Room) handleSendQueueFull(conn *Connection) {
	playerID := conn.getPlayerID()
	if playerID < 0 {
		return
	}

	now := time.Now()
	if since, ok := r.sendQueueFullAt[playerID]; !ok {
		r.sendQueueFullAt[playerID] = now
		log.Printf("玩家 %d 发送队列满，进入宽限期", playerID)
		return
	} else if now.Sub(since) < sendQueueFullGrace {
		return
	}

	delete(r.sendQueueFullAt, playerID)
	log.Printf("玩家 %d 发送队列持续满超过 %s，断开连接", playerID, sendQueueFullGrace)
	conn.Close()
}

func (r *Room) broadcastGameOver(winnerID int32) {
	packet := protocol.NewGameOver(winnerID)
	data, err := protocol.Marshal(packet)
	if err != nil {
		log.Printf("序列化游戏结束消息失败: %v", err)
		return
	}

	for _, conn := range r.connections {
		if err := conn.Send(data); err != nil {
			log.Printf("发送游戏结束到玩家 %d 失败: %v", conn.getPlayerID(), err)
		}
	}
}

func (r *Room) checkGameOver() (bool, int32) {
	// 使用核心逻辑的 IsGameOver() 判定
	// 这包含了新的"进门"胜利条件
	if !r.game.IsGameOver() {
		return false, -1
	}

	// 游戏结束，找出获胜者
	total, alive, winnerID := r.countPlayersAlive()
	if total == 0 {
		return true, -1
	}

	// 如果只剩1人且该人站在门上，则该人获胜
	if alive == 1 {
		return true, winnerID
	}

	// 所有人都死了
	if alive == 0 {
		return true, -1
	}

	// 其他情况不应该发生（IsGameOver返回true但多人存活）
	return false, -1
}

func (r *Room) countPlayersAlive() (total int, alive int, winnerID int32) {
	winnerID = -1
	for _, player := range r.game.Players {
		total++
		if !player.Dead {
			alive++
			winnerID = int32(player.ID)
		}
	}

	if alive != 1 {
		winnerID = -1
	}
	return total, alive, winnerID
}

func (r *Room) removePlayerByID(playerID int32) {
	for i, player := range r.game.Players {
		if player.ID == int(playerID) {
			r.game.Players = append(r.game.Players[:i], r.game.Players[i+1:]...)
			return
		}
	}
}

// getSpawnPosition 根据玩家 ID 获取出生点
func getSpawnPosition(playerID int) (int, int) {
	// 4 个角落的位置
	spawns := []struct{ x, y int }{
		{0, 0},     // 玩家 1: 左上角
		{608, 0},   // 玩家 2: 右上角
		{0, 448},   // 玩家 3: 左下角
		{608, 448}, // 玩家 4: 右下角
	}

	// 取模，支持任意数量的玩家
	index := (playerID - 1) % len(spawns)
	return spawns[index].x, spawns[index].y
}
