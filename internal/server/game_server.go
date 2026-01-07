package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
	"bomberman/pkg/protocol"
)

const (
	MaxPlayers   = 4  // 最大玩家数
	ServerTPS    = 60 // 服务器每秒更新次数
	TickDuration = time.Second / ServerTPS
)

// GameState 服务端房间状态
type GameState int

const (
	StateWaiting GameState = iota
	StateRunning
	StateEnding
)

// GameServer 游戏服务器
type GameServer struct {
	// 游戏状态
	game    *core.Game
	frameId int32
	gameMu  sync.RWMutex

	// 连接管理
	connections  map[int32]*Connection // playerID -> Connection
	nextPlayerID int32
	connMutex    sync.RWMutex

	// 房间状态
	state   GameState
	stateMu sync.RWMutex

	// 网络
	listener net.Listener
	addr     string

	// 控制
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	shutdown chan struct{}
}

// NewGameServer 创建新的游戏服务器
func NewGameServer(addr string) *GameServer {
	ctx, cancel := context.WithCancel(context.Background())

	return &GameServer{
		game:         core.NewGame(),              // 初始化游戏状态
		frameId:      0,                           // 初始帧 ID
		connections:  make(map[int32]*Connection), // 初始化连接映射
		nextPlayerID: 1,                           // 玩家 ID 从 1 开始
		state:        StateWaiting,                // 初始状态为等待
		addr:         addr,                        // 监听地址
		ctx:          ctx,                         // 上下文
		cancel:       cancel,                      // 取消函数
		shutdown:     make(chan struct{}),         // 关闭信号
	}
}

// Start 启动服务器
func (s *GameServer) Start() error {
	log.Printf("启动游戏服务器: %s", s.addr)

	// 监听 TCP 端口
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("监听失败: %w", err)
	}
	s.listener = listener

	log.Printf("服务器监听中: %s", s.addr)

	// 启动游戏循环
	s.wg.Add(1)
	go s.gameLoop()

	// 启动连接接受循环
	s.wg.Add(1)
	go s.acceptLoop()

	// 等待关闭信号
	<-s.shutdown

	log.Println("服务器正在关闭...")
	return nil
}

// Shutdown 优雅关闭服务器
func (s *GameServer) Shutdown() {
	log.Println("正在关闭服务器...")

	// 取消上下文
	s.cancel()

	// 关闭监听器
	if s.listener != nil {
		s.listener.Close()
	}

	// 关闭所有连接
	s.closeAllConnections()

	// 关闭 shutdown 通道
	close(s.shutdown)

	// 等待所有 goroutine 结束
	s.wg.Wait()

	log.Println("服务器已关闭")
}

// acceptLoop 接受客户端连接
func (s *GameServer) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			log.Println("停止接受新连接")
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Printf("接受连接失败: %v", err)
				continue
			}
		}

		log.Printf("新连接来自: %s", conn.RemoteAddr())

		// 创建连接对象
		connection := NewConnection(conn, s)

		// 启动连接处理
		s.wg.Add(1)
		go connection.Handle(s.ctx, &s.wg)
	}
}

// gameLoop 游戏主循环（60 TPS）
func (s *GameServer) gameLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(TickDuration)
	defer ticker.Stop()

	log.Printf("游戏循环启动: %d TPS", ServerTPS)

	for {
		select {
		case <-s.ctx.Done():
			log.Println("游戏循环停止")
			return

		case <-ticker.C:
			// 更新游戏逻辑
			s.updateGame()

			// 广播状态
			s.broadcastState()
		}
	}
}

// updateGame 更新游戏状态
func (s *GameServer) updateGame() {
	if s.getState() != StateRunning {
		return
	}

	// 固定时间步长
	deltaTime := 1.0 / ServerTPS

	var shouldEnd bool
	var winnerID int32

	s.gameMu.Lock()
	// 更新核心游戏逻辑
	s.game.Update(deltaTime)

	// 增加帧 ID
	s.frameId++

	shouldEnd, winnerID = s.checkGameOverLocked()
	s.gameMu.Unlock()

	if shouldEnd {
		s.handleGameOver(winnerID)
	}
}

// broadcastState 广播游戏状态到所有客户端
func (s *GameServer) broadcastState() {
	if s.getState() != StateRunning {
		return
	}

	s.gameMu.RLock()
	frameID := s.frameId
	// 转换玩家列表
	protoPlayers := protocol.CorePlayersToProto(s.game.Players)

	// 转换炸弹列表
	protoBombs := protocol.CoreBombsToProto(s.game.Bombs)

	// 转换爆炸列表
	protoExplosions := protocol.CoreExplosionsToProto(s.game.Explosions)
	s.gameMu.RUnlock()

	// 构造 ServerState 消息
	packet := protocol.NewServerState(frameID, protoPlayers, protoBombs, protoExplosions)

	// 序列化
	data, err := protocol.Marshal(packet)
	if err != nil {
		log.Printf("序列化状态失败: %v", err)
		return
	}

	// 发送到所有连接
	conns := s.snapshotConnections()
	for _, conn := range conns {
		if err := conn.Send(data); err != nil {
			log.Printf("发送状态到玩家 %d 失败: %v", conn.getPlayerID(), err)
		}
	}
}

// handleGameOver 处理游戏结束
func (s *GameServer) handleGameOver(winnerID int32) {
	if !s.transitionToEnding() {
		return
	}

	log.Printf("游戏结束，获胜者: %d", winnerID)

	s.broadcastGameOver(winnerID)

	go s.resetRoomAfterDelay(3 * time.Second)
}

// ========== 玩家管理 ==========

// addPlayer 添加玩家
func (s *GameServer) addPlayer(conn *Connection, characterType core.CharacterType) (int32, error) {
	s.connMutex.Lock()

	// 检查是否已满
	if len(s.connections) >= MaxPlayers {
		s.connMutex.Unlock()
		return 0, fmt.Errorf("服务器已满 (%d/%d)", len(s.connections), MaxPlayers)
	}

	// 分配玩家 ID
	playerID := s.nextPlayerID
	s.nextPlayerID++

	// 获取出生点
	x, y := getSpawnPosition(int(playerID))

	// 创建玩家
	player := core.NewPlayer(int(playerID), x, y, characterType)
	player.IsSimulated = false // 服务器直接驱动

	// 保存连接
	conn.setPlayerID(playerID)
	s.connections[playerID] = conn
	s.connMutex.Unlock()

	// 添加到游戏
	s.gameMu.Lock()
	s.game.AddPlayer(player)
	s.gameMu.Unlock()

	log.Printf("玩家 %d 加入，角色: %s, 出生点: (%d, %d)", playerID, characterType, x, y)

	return playerID, nil
}

// removePlayer 移除玩家
func (s *GameServer) removePlayer(playerID int32) {
	var conns []*Connection
	var remainingConnections int

	s.connMutex.Lock()

	_, exists := s.connections[playerID]
	if !exists {
		s.connMutex.Unlock()
		return
	}

	// 从地图中删除连接
	delete(s.connections, playerID)
	remainingConnections = len(s.connections)

	conns = make([]*Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		conns = append(conns, conn)
	}
	s.connMutex.Unlock()

	// 从游戏中删除玩家
	var remainingPlayers int
	s.gameMu.Lock()
	for i, player := range s.game.Players {
		if player.ID == int(playerID) {
			s.game.Players = append(s.game.Players[:i], s.game.Players[i+1:]...)
			break
		}
	}
	remainingPlayers = len(s.game.Players)
	s.gameMu.Unlock()

	log.Printf("玩家 %d 离开，当前玩家数: %d", playerID, remainingConnections)

	// 广播玩家离开消息
	packet := protocol.NewPlayerLeave(playerID)
	data, _ := protocol.Marshal(packet)

	for _, c := range conns {
		c.Send(data)
	}

	if remainingPlayers == 0 && s.getState() == StateRunning && s.ctx.Err() == nil {
		s.handleGameOver(-1)
	}
}

// ========== 消息处理 ==========

// handleJoinRequest 处理加入请求
func (s *GameServer) handleJoinRequest(conn *Connection, req *gamev1.JoinRequest) error {
	if s.getState() == StateEnding {
		return fmt.Errorf("房间结算中，暂时无法加入")
	}

	// 转换角色类型
	characterType := protocol.ProtoCharacterTypeToCore(req.CharacterType)

	// 添加玩家
	playerID, err := s.addPlayer(conn, characterType)
	if err != nil {
		return err
	}

	// 构造游戏开始消息
	s.gameMu.RLock()
	initialMap := protocol.CoreMapToProto(s.game.Map)
	s.gameMu.RUnlock()
	packet := protocol.NewGameStart(playerID, initialMap)
	data, err := protocol.Marshal(packet)
	if err != nil {
		return fmt.Errorf("序列化游戏开始消息失败: %w", err)
	}

	// 发送给客户端
	if err := conn.Send(data); err != nil {
		return fmt.Errorf("发送游戏开始消息失败: %w", err)
	}

	log.Printf("玩家 %d 游戏开始消息已发送", playerID)

	s.setState(StateRunning)

	return nil
}

// handleClientInput 处理客户端输入
func (s *GameServer) handleClientInput(playerID int32, input *gamev1.ClientInput) {
	if s.getState() != StateRunning {
		return
	}

	s.gameMu.Lock()
	player := s.getPlayerLocked(playerID)
	if player == nil || player.Dead {
		s.gameMu.Unlock()
		return
	}

	// 计算移动距离
	speed := player.Speed * (1.0 / ServerTPS)

	// 处理移动
	if input.Up {
		player.Move(0, -speed, s.game)
	}
	if input.Down {
		player.Move(0, speed, s.game)
	}
	if input.Left {
		player.Move(-speed, 0, s.game)
	}
	if input.Right {
		player.Move(speed, 0, s.game)
	}

	// 处理炸弹
	if input.Bomb {
		bomb := player.PlaceBomb(s.game)
		if bomb != nil {
			s.game.AddBomb(bomb)
			log.Printf("玩家 %d 放置炸弹", playerID)
		}
	}
	s.gameMu.Unlock()
}

// ========== 辅助函数 ==========

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

func (s *GameServer) getPlayerLocked(playerID int32) *core.Player {
	for _, player := range s.game.Players {
		if player.ID == int(playerID) {
			return player
		}
	}
	return nil
}

func (s *GameServer) checkGameOverLocked() (bool, int32) {
	total, alive, winnerID := s.countPlayersAliveLocked()
	if total == 0 {
		return false, -1
	}

	// 单人训练：只有死亡才结束
	if total == 1 {
		if alive == 0 {
			return true, -1
		}
		return false, -1
	}

	// 多人对战：只剩 0/1 人时结束
	if alive <= 1 {
		return true, winnerID
	}

	return false, -1
}

// ========== 房间状态与重置 ==========

func (s *GameServer) getState() GameState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

func (s *GameServer) setState(state GameState) {
	s.stateMu.Lock()
	s.state = state
	s.stateMu.Unlock()
}

func (s *GameServer) transitionToEnding() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.state == StateEnding {
		return false
	}
	s.state = StateEnding
	return true
}

func (s *GameServer) resetRoomAfterDelay(delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-s.ctx.Done():
		return
	case <-timer.C:
	}

	s.closeAllConnections()
	s.resetGame()
}

func (s *GameServer) snapshotConnections() []*Connection {
	s.connMutex.RLock()
	conns := make([]*Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		conns = append(conns, conn)
	}
	s.connMutex.RUnlock()
	return conns
}

func (s *GameServer) closeAllConnections() {
	s.connMutex.RLock()
	conns := make([]*Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		conns = append(conns, conn)
	}
	s.connMutex.RUnlock()

	for _, conn := range conns {
		conn.Close()
	}

	s.connMutex.Lock()
	s.connections = make(map[int32]*Connection)
	s.connMutex.Unlock()
}

func (s *GameServer) resetGame() {
	s.gameMu.Lock()
	s.game = core.NewGame()
	s.frameId = 0
	s.gameMu.Unlock()

	s.connMutex.Lock()
	s.nextPlayerID = 1
	s.connections = make(map[int32]*Connection)
	s.connMutex.Unlock()

	s.setState(StateWaiting)
}

func (s *GameServer) countPlayersAlive() (total int, alive int, winnerID int32) {
	s.gameMu.RLock()
	defer s.gameMu.RUnlock()
	return s.countPlayersAliveLocked()
}

func (s *GameServer) countPlayersAliveLocked() (total int, alive int, winnerID int32) {
	winnerID = -1
	for _, player := range s.game.Players {
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

func (s *GameServer) broadcastGameOver(winnerID int32) {
	packet := protocol.NewGameOver(winnerID)
	data, err := protocol.Marshal(packet)
	if err != nil {
		log.Printf("序列化游戏结束消息失败: %v", err)
		return
	}

	s.connMutex.RLock()
	conns := make([]*Connection, 0, len(s.connections))
	for _, conn := range s.connections {
		conns = append(conns, conn)
	}
	s.connMutex.RUnlock()

	for _, conn := range conns {
		if err := conn.Send(data); err != nil {
			log.Printf("发送游戏结束到玩家 %d 失败: %v", conn.getPlayerID(), err)
		}
	}
}
