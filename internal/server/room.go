package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/ai"
	"bomberman/pkg/core"
	"bomberman/pkg/protocol"
)

type Room struct {
	ctx    context.Context
	cancel context.CancelFunc
	id     string // 房间 ID
	seed   int64

	legacyMode bool

	game    *core.Game
	frameID int32
	state   GameState
	resetAt time.Time

	enableAI      bool
	aiControllers map[int32]*ai.AIController

	connections     map[int32]Session
	nextPlayerID    int32
	inputQueue      map[int32]map[int32]InputData
	sendQueueFullAt map[int32]time.Time
	lastInput       map[int32]InputData

	// 客户端预测支持：记录每个玩家最后处理的输入序号
	lastProcessedInputSeq map[int32]int32

	// 记录上一帧玩家死亡状态，用于检测变化
	lastPlayerDeadState map[int32]bool

	// 房间大厅状态
	hostID           int32
	readyStatus      map[int32]bool
	playerNames      map[int32]string
	playerCharacters map[int32]core.CharacterType
	roomName         string

	joinCh   chan joinRequest
	inputCh  chan inputEvent
	leaveCh  chan int32
	actionCh chan roomActionRequest
}

type joinRequest struct {
	conn   Session
	req    JoinEvent
	respCh chan error
}

type inputEvent struct {
	playerID int32
	input    InputEvent
}

type roomActionRequest struct {
	playerID int32
	action   *gamev1.RoomAction
	respCh   chan error
}

func NewRoom(parent context.Context, roomID string, seed int64, enableAI bool, legacyMode bool) *Room {
	ctx, cancel := context.WithCancel(parent)

	return &Room{
		ctx:                   ctx,
		cancel:                cancel,
		id:                    roomID,
		seed:                  seed,
		legacyMode:            legacyMode,
		game:                  core.NewGame(seed),
		frameID:               0,
		state:                 StateWaiting,
		enableAI:              enableAI,
		aiControllers:         make(map[int32]*ai.AIController),
		connections:           make(map[int32]Session),
		nextPlayerID:          1,
		inputQueue:            make(map[int32]map[int32]InputData),
		sendQueueFullAt:       make(map[int32]time.Time),
		lastInput:             make(map[int32]InputData),
		lastProcessedInputSeq: make(map[int32]int32),
		lastPlayerDeadState:   make(map[int32]bool),
		readyStatus:           make(map[int32]bool),
		playerNames:           make(map[int32]string),
		playerCharacters:      make(map[int32]core.CharacterType),
		joinCh:                make(chan joinRequest),
		inputCh:               make(chan inputEvent, 256),
		leaveCh:               make(chan int32, 256),
		actionCh:              make(chan roomActionRequest, 64),
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

		case req := <-r.actionCh:
			r.handleRoomAction(req)

		case <-ticker.C:
			r.tick()
		}
	}
}

func (r *Room) Shutdown() {
	r.cancel()
}

func (r *Room) CurrentFrame() int32 {
	return r.frameID
}

func (r *Room) Join(conn Session, req JoinEvent) error {
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

func (r *Room) EnqueueInput(playerID int32, input InputEvent) {
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

func (r *Room) HandleRoomAction(playerID int32, action *gamev1.RoomAction) error {
	if action == nil {
		return fmt.Errorf("房间操作为空")
	}

	respCh := make(chan error, 1)
	select {
	case <-r.ctx.Done():
		return fmt.Errorf("房间已关闭")
	case r.actionCh <- roomActionRequest{
		playerID: playerID,
		action:   action,
		respCh:   respCh,
	}:
	}

	select {
	case <-r.ctx.Done():
		return fmt.Errorf("房间已关闭")
	case err := <-respCh:
		return err
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
	r.updateAI()

	// 更新核心游戏逻辑（帧递增在 Update 内部）
	r.game.Update()

	// 增加帧 ID（game.CurrentFrame 已在 Update 中递增）
	r.frameID = r.game.CurrentFrame

	// 检测玩家死亡状态变化并广播
	r.checkAndBroadcastPlayerDeaths()

	if shouldEnd, winnerID := r.checkGameOver(); shouldEnd {
		r.handleGameOver(winnerID)
	}

	if r.state == StateRunning {
		r.broadcastState()
	}
}

func (r *Room) checkAndBroadcastPlayerDeaths() {
	for _, player := range r.game.Players {
		playerID := int32(player.ID)
		wasDead := r.lastPlayerDeadState[playerID]
		isDead := player.Dead

		// 初始化状态（第一次看到这个玩家）
		if !wasDead && !isDead {
			r.lastPlayerDeadState[playerID] = false
			continue
		}

		// 检测死亡事件：从存活变为死亡
		if !wasDead && isDead {
			r.lastPlayerDeadState[playerID] = true
			log.Printf("玩家 %d 被炸死", playerID)

			// 广播玩家死亡事件
			event := &gamev1.GameEvent{
				Event: &gamev1.GameEvent_PlayerDied{
					PlayerDied: &gamev1.PlayerDiedEvent{
						PlayerId: playerID,
					},
				},
			}
			packet, err := protocol.NewGameEventPacket(r.frameID, event)
			if err != nil {
				log.Printf("构造玩家死亡事件失败: %v", err)
				continue
			}

			data, err := protocol.MarshalPacket(packet)
			if err != nil {
				log.Printf("序列化玩家死亡事件失败: %v", err)
				continue
			}

			for _, conn := range r.connections {
				if err := conn.Send(data); err != nil {
					log.Printf("发送玩家死亡事件到玩家 %d 失败: %v", conn.ID(), err)
				}
			}
		}
	}
}

func (r *Room) applyInputs() {
	if len(r.connections) == 0 {
		return
	}

	for playerID := range r.connections {
		inputData, ok := r.popInputForFrame(playerID, r.frameID)
		if !ok {
			last, hasLast := r.lastInput[playerID]
			if !hasLast {
				continue
			}
			inputData = last
		} else {
			r.lastInput[playerID] = inputData
		}

		r.applyInputData(playerID, inputData)
	}
}

func (r *Room) applyInputData(playerID int32, input InputData) {
	ci := core.Input{
		Up:    input.Up,
		Down:  input.Down,
		Left:  input.Left,
		Right: input.Right,
		Bomb:  input.Bomb,
	}

	// ApplyInput 现在需要帧号而不是 deltaTime
	placed := core.ApplyInput(r.game, int(playerID), ci, r.frameID)
	if placed {
		log.Printf("玩家 %d 放置炸弹", playerID)
	}
}

func (r *Room) handleJoin(req joinRequest) {
	if r.state == StateEnding {
		req.respCh <- fmt.Errorf("房间结算中，暂时无法加入")
		return
	}

	if len(r.connections)+len(r.aiControllers) >= MaxPlayers {
		req.respCh <- fmt.Errorf("服务器已满 (%d/%d)", len(r.connections)+len(r.aiControllers), MaxPlayers)
		return
	}

	if !r.legacyMode && r.state != StateWaiting {
		req.respCh <- fmt.Errorf("房间游戏中，暂时无法加入")
		return
	}

	// 转换角色类型
	characterType := protocol.ProtoCharacterTypeToCore(req.req.Character)

	// 分配玩家 ID
	playerID := r.nextPlayerID
	r.nextPlayerID++

	// 获取出生点
	x, y := getSpawnPosition(int(playerID))

	// 创建玩家
	player := core.NewPlayer(int(playerID), x, y, characterType)

	// 添加到游戏
	r.game.AddPlayer(player)

	// 保存连接
	req.conn.SetPlayerID(playerID)
	req.conn.SetRoomID(r.id)
	r.connections[playerID] = req.conn
	r.playerNames[playerID] = req.req.PlayerName
	r.playerCharacters[playerID] = characterType
	r.readyStatus[playerID] = false

	if r.hostID == 0 {
		r.hostID = playerID
		if r.roomName == "" {
			name := req.req.PlayerName
			if name == "" {
				name = fmt.Sprintf("Player%d", playerID)
			}
			r.roomName = fmt.Sprintf("%s's room", name)
		}
	}

	// 生成会话 Token（用于重连）
	sessionToken, err := GenerateSessionToken(playerID, r.id)
	if err != nil {
		req.respCh <- fmt.Errorf("生成会话 Token 失败: %w", err)
		return
	}

	// 发送 JoinResponse（包含玩家ID和游戏配置）
	roomState := r.buildRoomState()
	packet, err := protocol.NewJoinResponsePacket(
		true,
		playerID,
		"",
		r.game.Seed,
		int32(core.TPS),
		sessionToken,
		r.id,
		roomState,
	)
	if err != nil {
		req.respCh <- fmt.Errorf("构造加入响应失败: %w", err)
		return
	}

	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		req.respCh <- fmt.Errorf("序列化加入响应失败: %w", err)
		return
	}

	// 发送给客户端
	if err := req.conn.Send(data); err != nil {
		r.removePlayerByID(playerID)
		delete(r.connections, playerID)
		req.conn.SetPlayerID(-1)
		req.conn.SetRoomID("")
		req.respCh <- fmt.Errorf("发送游戏开始消息失败: %w", err)
		return
	}

	log.Printf("玩家 %d 加入，角色: %s, 出生点: (%d, %d)", playerID, characterType, x, y)
	log.Printf("玩家 %d 加入响应已发送", playerID)

	if r.legacyMode {
		if r.state != StateRunning {
			r.state = StateRunning
			r.broadcastRoomState()
			r.broadcastGameStart(0)
		}
		if r.enableAI {
			r.tryFillWithAI()
		}
	} else {
		r.broadcastRoomState()
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

	if len(ev.input.Inputs) == 0 {
		return
	}

	queue, ok := r.inputQueue[ev.playerID]
	if !ok {
		queue = make(map[int32]InputData)
		r.inputQueue[ev.playerID] = queue
	}

	for _, in := range ev.input.Inputs {
		if in.FrameID < r.frameID-InputBufferFrames {
			continue
		}
		queue[in.FrameID] = in
	}

	if ev.input.Seq > r.lastProcessedInputSeq[ev.playerID] {
		r.lastProcessedInputSeq[ev.playerID] = ev.input.Seq
	}
}

func (r *Room) handleLeave(playerID int32) {
	conn, isHuman := r.connections[playerID]
	_, isAI := r.aiControllers[playerID]
	if !isHuman && !isAI {
		return
	}

	if isHuman {
		delete(r.connections, playerID)
		delete(r.inputQueue, playerID)
		delete(r.sendQueueFullAt, playerID)
		delete(r.lastProcessedInputSeq, playerID)
		delete(r.lastInput, playerID)
		conn.SetPlayerID(-1)
		conn.SetRoomID("")
	}

	if isAI {
		delete(r.aiControllers, playerID)
	}

	delete(r.readyStatus, playerID)
	delete(r.playerNames, playerID)
	delete(r.playerCharacters, playerID)

	r.removePlayerByID(playerID)

	log.Printf("玩家 %d 离开，当前玩家数: %d", playerID, len(r.connections))

	if isHuman {
		// 广播玩家离开事件
		event := &gamev1.GameEvent{
			Event: &gamev1.GameEvent_PlayerLeft{
				PlayerLeft: &gamev1.PlayerLeftEvent{
					PlayerId: playerID,
				},
			},
		}
		packet, err := protocol.NewGameEventPacket(r.frameID, event)
		if err == nil {
			data, _ := protocol.MarshalPacket(packet)
			for _, c := range r.connections {
				c.Send(data)
			}
		}
	}

	if playerID == r.hostID {
		r.transferHost()
	}

	if !r.legacyMode {
		r.broadcastRoomState()
	}

	if len(r.game.Players) == 0 && r.state == StateRunning {
		r.handleGameOver(-1)
	}
}

func (r *Room) handleRoomAction(req roomActionRequest) {
	if req.action == nil {
		req.respCh <- errors.New("房间操作为空")
		return
	}

	if r.legacyMode && req.action.Type != gamev1.RoomActionType_ROOM_ACTION_LEAVE {
		req.respCh <- errors.New("兼容房间不支持该操作")
		return
	}

	switch req.action.Type {
	case gamev1.RoomActionType_ROOM_ACTION_READY:
		if r.state != StateWaiting {
			req.respCh <- errors.New("游戏中无法准备")
			return
		}
		if _, ok := r.connections[req.playerID]; !ok {
			req.respCh <- errors.New("玩家不在房间中")
			return
		}
		r.readyStatus[req.playerID] = req.action.Ready
		r.broadcastRoomState()

	case gamev1.RoomActionType_ROOM_ACTION_START:
		if ok, msg := r.CanStart(req.playerID); !ok {
			req.respCh <- errors.New(msg)
			return
		}
		r.startGame()

	case gamev1.RoomActionType_ROOM_ACTION_ADD_AI:
		if req.playerID != r.hostID {
			req.respCh <- errors.New("只有房主可以添加 AI")
			return
		}
		if r.state != StateWaiting {
			req.respCh <- errors.New("游戏中无法添加 AI")
			return
		}
		if !r.enableAI {
			req.respCh <- errors.New("服务器未启用 AI")
			return
		}
		if err := r.addAI(int(req.action.AiCount)); err != nil {
			req.respCh <- err
			return
		}
		r.broadcastRoomState()

	case gamev1.RoomActionType_ROOM_ACTION_LEAVE:
		if _, ok := r.connections[req.playerID]; !ok {
			req.respCh <- errors.New("玩家不在房间中")
			return
		}
		r.handleLeave(req.playerID)

	case gamev1.RoomActionType_ROOM_ACTION_KICK:
		if req.playerID != r.hostID {
			req.respCh <- errors.New("只有房主可以踢人")
			return
		}
		if err := r.kickPlayer(req.action.TargetPlayer); err != nil {
			req.respCh <- err
			return
		}

	default:
		req.respCh <- errors.New("未知房间操作")
		return
	}

	req.respCh <- nil
}

// CanStart 检查是否可以开始游戏
func (r *Room) CanStart(requestorID int32) (bool, string) {
	if r.state != StateWaiting {
		return false, "Game already started"
	}
	if requestorID != r.hostID {
		return false, "Only host can start game"
	}

	totalPlayers := len(r.connections) + len(r.aiControllers)
	if totalPlayers < 2 {
		return false, "Need at least 2 players"
	}

	for playerID := range r.connections {
		if playerID == r.hostID {
			continue
		}
		if !r.readyStatus[playerID] {
			return false, "Some players not ready"
		}
	}

	return true, ""
}

func (r *Room) startGame() {
	if r.state == StateRunning {
		return
	}
	r.state = StateRunning
	r.inputQueue = make(map[int32]map[int32]InputData)
	r.lastInput = make(map[int32]InputData)
	r.lastProcessedInputSeq = make(map[int32]int32)
	r.lastPlayerDeadState = make(map[int32]bool)

	r.broadcastRoomState()
	r.broadcastGameStart(0)
}

func (r *Room) transferHost() {
	r.hostID = 0
	for playerID := range r.connections {
		r.hostID = playerID
		break
	}
}

func (r *Room) addAI(count int) error {
	if count <= 0 {
		return nil
	}
	availableChars := []core.CharacterType{
		core.CharacterWhite,
		core.CharacterBlack,
		core.CharacterBlue,
		core.CharacterRed,
	}

	for count > 0 && len(r.game.Players) < MaxPlayers {
		playerID := r.nextPlayerID
		r.nextPlayerID++

		x, y := getSpawnPosition(int(playerID))
		charType := availableChars[(playerID-1)%int32(len(availableChars))]

		player := core.NewPlayer(int(playerID), x, y, charType)
		r.game.AddPlayer(player)

		r.aiControllers[playerID] = ai.NewAIController(int(playerID))
		r.playerNames[playerID] = fmt.Sprintf("AI-%d", playerID)
		r.playerCharacters[playerID] = charType
		r.readyStatus[playerID] = true

		count--
	}

	if count > 0 {
		return errors.New("房间已满，无法继续添加 AI")
	}
	return nil
}

func (r *Room) kickPlayer(targetID int32) error {
	conn, ok := r.connections[targetID]
	if !ok {
		return errors.New("目标玩家不在房间中")
	}

	sessionToken, err := GenerateSessionToken(0, "")
	if err == nil {
		packet, err := protocol.NewRoomActionResponsePacket(false, "你已被踢出房间", sessionToken, "")
		if err == nil {
			if data, err := protocol.MarshalPacket(packet); err == nil {
				_ = conn.Send(data)
			}
		}
	}

	r.handleLeave(targetID)
	return nil
}

func (r *Room) buildRoomState() *gamev1.RoomStateUpdate {
	status := gamev1.RoomStatus_ROOM_STATUS_WAITING
	if r.state == StateRunning || r.state == StateEnding {
		status = gamev1.RoomStatus_ROOM_STATUS_PLAYING
	}

	playerIDs := make([]int, 0, len(r.connections)+len(r.aiControllers))
	for playerID := range r.connections {
		playerIDs = append(playerIDs, int(playerID))
	}
	for playerID := range r.aiControllers {
		playerIDs = append(playerIDs, int(playerID))
	}
	sort.Ints(playerIDs)

	players := make([]*gamev1.RoomPlayer, 0, len(playerIDs))
	for _, id := range playerIDs {
		playerID := int32(id)
		_, isAI := r.aiControllers[playerID]
		name := r.playerNames[playerID]
		if name == "" {
			if isAI {
				name = fmt.Sprintf("AI-%d", playerID)
			} else {
				name = fmt.Sprintf("Player%d", playerID)
			}
		}
		ready := r.readyStatus[playerID]
		if isAI {
			ready = true
		}
		charType := protocol.CoreCharacterTypeToProto(r.playerCharacters[playerID])
		players = append(players, &gamev1.RoomPlayer{
			Id:        playerID,
			Name:      name,
			Character: charType,
			IsReady:   ready,
			IsHost:    playerID == r.hostID,
			IsAi:      isAI,
		})
	}

	return &gamev1.RoomStateUpdate{
		RoomId:  r.id,
		Status:  status,
		Players: players,
		HostId:  r.hostID,
	}
}

func (r *Room) broadcastRoomState() {
	update := r.buildRoomState()
	packet, err := protocol.NewRoomStateUpdatePacket(update)
	if err != nil {
		log.Printf("构造房间状态失败: %v", err)
		return
	}
	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		log.Printf("序列化房间状态失败: %v", err)
		return
	}
	for _, conn := range r.connections {
		if err := conn.Send(data); err != nil {
			log.Printf("发送房间状态到玩家 %d 失败: %v", conn.ID(), err)
		}
	}
}

func (r *Room) broadcastGameStart(countdownFrames int32) {
	event := &gamev1.GameEvent{
		Event: &gamev1.GameEvent_GameStart{
			GameStart: &gamev1.GameStartEvent{
				CountdownFrames: countdownFrames,
			},
		},
	}
	packet, err := protocol.NewGameEventPacket(r.frameID, event)
	if err != nil {
		log.Printf("构造游戏开始事件失败: %v", err)
		return
	}
	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		log.Printf("序列化游戏开始事件失败: %v", err)
		return
	}
	for _, conn := range r.connections {
		if err := conn.Send(data); err != nil {
			log.Printf("发送游戏开始到玩家 %d 失败: %v", conn.ID(), err)
		}
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
	if r.legacyMode {
		// 关闭所有连接并通知客户端游戏结束
		r.closeAllConnections(true)

		r.game = core.NewGame(r.seed)
		r.frameID = 0
		r.state = StateWaiting
		r.resetAt = time.Time{}
		r.nextPlayerID = 1
		r.aiControllers = make(map[int32]*ai.AIController)
		r.connections = make(map[int32]Session)
		r.inputQueue = make(map[int32]map[int32]InputData)
		r.sendQueueFullAt = make(map[int32]time.Time)
		r.lastProcessedInputSeq = make(map[int32]int32)
		r.lastInput = make(map[int32]InputData)
		r.lastPlayerDeadState = make(map[int32]bool)
		r.readyStatus = make(map[int32]bool)
		r.playerNames = make(map[int32]string)
		r.playerCharacters = make(map[int32]core.CharacterType)
		r.hostID = 0
		r.roomName = ""

		log.Println("房间已重置，等待新玩家加入")
		return
	}

	// 非兼容房间：保留连接，重置游戏状态
	oldAI := r.aiControllers
	r.aiControllers = make(map[int32]*ai.AIController)
	r.game = core.NewGame(r.seed)
	r.frameID = 0
	r.state = StateWaiting
	r.resetAt = time.Time{}
	r.inputQueue = make(map[int32]map[int32]InputData)
	r.sendQueueFullAt = make(map[int32]time.Time)
	r.lastProcessedInputSeq = make(map[int32]int32)
	r.lastInput = make(map[int32]InputData)
	r.lastPlayerDeadState = make(map[int32]bool)

	for playerID := range r.connections {
		charType := r.playerCharacters[playerID]
		x, y := getSpawnPosition(int(playerID))
		player := core.NewPlayer(int(playerID), x, y, charType)
		r.game.AddPlayer(player)
		r.readyStatus[playerID] = false
	}

	for playerID := range oldAI {
		charType := r.playerCharacters[playerID]
		x, y := getSpawnPosition(int(playerID))
		player := core.NewPlayer(int(playerID), x, y, charType)
		r.game.AddPlayer(player)
		r.aiControllers[playerID] = ai.NewAIController(int(playerID))
		r.readyStatus[playerID] = true
	}

	if r.hostID != 0 {
		if _, ok := r.connections[r.hostID]; !ok {
			r.transferHost()
		}
	}

	r.broadcastRoomState()
	log.Println("房间已重置，返回等待状态")
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

	// 收集地图变化（从爆炸中收集）
	var tileChanges []*gamev1.TileChange
	for _, exp := range r.game.Explosions {
		for _, tc := range exp.TileChanges {
			tileChanges = append(tileChanges, &gamev1.TileChange{
				X:       int32(tc.GridX),
				Y:       int32(tc.GridY),
				NewType: gamev1.TileType(tc.NewType),
			})
		}
	}

	// 构造 GameState 消息（使用帧！）
	packet, err := protocol.NewGameStatePacket(
		r.frameID,
		protocol.CoreGameStateToProto(int(r.state)),
		protoPlayers,
		protoBombs,
		protoExplosions,
		tileChanges,
		r.lastProcessedInputSeq,
	)
	if err != nil {
		log.Printf("构造游戏状态失败: %v", err)
		return
	}

	// 序列化
	data, err := protocol.MarshalPacket(packet)
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
			log.Printf("发送状态到玩家 %d 失败: %v", conn.ID(), err)
			conn.Close()
			continue
		}
		delete(r.sendQueueFullAt, conn.ID())
	}
}

// BuildGameState 构建当前游戏状态（用于重连）
func (r *Room) BuildGameState() *gamev1.GameState {
	// 转换玩家列表
	protoPlayers := protocol.CorePlayersToProto(r.game.Players)

	// 转换炸弹列表
	protoBombs := protocol.CoreBombsToProto(r.game.Bombs)

	// 转换爆炸列表
	protoExplosions := protocol.CoreExplosionsToProto(r.game.Explosions)

	// 收集地图变化
	var tileChanges []*gamev1.TileChange
	for _, exp := range r.game.Explosions {
		for _, tc := range exp.TileChanges {
			tileChanges = append(tileChanges, &gamev1.TileChange{
				X:       int32(tc.GridX),
				Y:       int32(tc.GridY),
				NewType: gamev1.TileType(tc.NewType),
			})
		}
	}

	// 复制 lastProcessedInputSeq
	lastProcessedSeq := make(map[int32]int32, len(r.lastProcessedInputSeq))
	for k, v := range r.lastProcessedInputSeq {
		lastProcessedSeq[k] = v
	}

	return &gamev1.GameState{
		FrameId:          r.frameID,
		Phase:            protocol.CoreGameStateToProto(int(r.state)),
		Players:          protoPlayers,
		Bombs:            protoBombs,
		Explosions:       protoExplosions,
		TileChanges:      tileChanges,
		LastProcessedSeq: lastProcessedSeq,
	}
}

// ReplaceConnection 替换玩家的连接（用于重连）
func (r *Room) ReplaceConnection(playerID int32, newConn Session) {
	r.connections[playerID] = newConn
	log.Printf("玩家 %d 的连接已替换", playerID)
}

const InputBufferFrames = 120

func (r *Room) popInputForFrame(playerID int32, frameID int32) (InputData, bool) {
	queue, ok := r.inputQueue[playerID]
	if !ok {
		return InputData{}, false
	}
	input, ok := queue[frameID]
	if ok {
		delete(queue, frameID)
	}

	if len(queue) > InputBufferFrames*2 {
		for f := range queue {
			if f < frameID-InputBufferFrames {
				delete(queue, f)
			}
		}
	}

	return input, ok
}

const sendQueueFullGrace = 2 * time.Second

func (r *Room) handleSendQueueFull(conn Session) {
	playerID := conn.ID()
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
	// 广播游戏结束事件
	event := &gamev1.GameEvent{
		Event: &gamev1.GameEvent_GameOver{
			GameOver: &gamev1.GameOverEvent{
				WinnerId: winnerID,
			},
		},
	}
	packet, err := protocol.NewGameEventPacket(r.frameID, event)
	if err != nil {
		log.Printf("构造游戏结束事件失败: %v", err)
		return
	}

	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		log.Printf("序列化游戏结束事件失败: %v", err)
		return
	}

	for _, conn := range r.connections {
		if err := conn.Send(data); err != nil {
			log.Printf("发送游戏结束到玩家 %d 失败: %v", conn.ID(), err)
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

// updateAI 更新 AI 玩家
func (r *Room) updateAI() {
	if len(r.aiControllers) == 0 {
		return
	}

	for id, controller := range r.aiControllers {
		input := controller.Decide(r.game)
		core.ApplyInput(r.game, int(id), input, r.frameID)
	}
}

// tryFillWithAI 尝试用 AI 填满房间
func (r *Room) tryFillWithAI() {
	// 只在有真实玩家且房间未满时添加 AI
	if len(r.connections) == 0 || len(r.game.Players) >= MaxPlayers {
		return
	}

	// 可用的角色类型
	availableChars := []core.CharacterType{
		core.CharacterWhite,
		core.CharacterBlack,
		core.CharacterBlue,
		core.CharacterRed,
	}

	for len(r.game.Players) < MaxPlayers {
		playerID := r.nextPlayerID
		r.nextPlayerID++

		x, y := getSpawnPosition(int(playerID))
		charType := availableChars[(playerID-1)%int32(len(availableChars))]

		player := core.NewPlayer(int(playerID), x, y, charType)

		r.game.AddPlayer(player)

		// 创建 AI 控制器
		r.aiControllers[playerID] = ai.NewAIController(int(playerID))

		log.Printf("添加 AI 玩家 %d", playerID)
	}
}

// getSpawnPosition 根据玩家 ID 获取出生点
func getSpawnPosition(playerID int) (int, int) {
	// 4 个角落的格子坐标
	spawns := []struct{ gx, gy int }{
		{0, 0},                                  // 玩家 1: 左上角
		{core.MapWidth - 1, 0},                  // 玩家 2: 右上角
		{0, core.MapHeight - 1},                 // 玩家 3: 左下角
		{core.MapWidth - 1, core.MapHeight - 1}, // 玩家 4: 右下角
	}

	// 取模，支持任意数量的玩家
	index := (playerID - 1) % len(spawns)
	gx, gy := spawns[index].gx, spawns[index].gy

	// 转换为像素坐标（带中心偏移）
	return core.GridToPlayerXY(gx, gy)
}
