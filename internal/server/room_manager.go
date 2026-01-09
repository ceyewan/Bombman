package server

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"
)

const (
	DefaultRoomID    = "default" // 默认房间 ID
	MaxRooms         = 100       // 最大房间数
	RoomEmptyTimeout = 60        // 房间空置超时（秒）
)

type RoomManager struct {
	ctx       context.Context
	enableAI  bool
	rooms     map[string]*Room // 房间 ID -> 房间
	roomMutex sync.RWMutex     // 保护 rooms map
	wg        sync.WaitGroup   // 等待组
	shutdown  chan struct{}    // 关闭信号
}

// NewRoomManager 创建新的房间管理器
func NewRoomManager(ctx context.Context, enableAI bool) *RoomManager {
	return &RoomManager{
		ctx:      ctx,
		enableAI: enableAI,
		rooms:    make(map[string]*Room),
		shutdown: make(chan struct{}),
	}
}

// Run 启动房间管理器
func (m *RoomManager) Run(wg *sync.WaitGroup) {
	// 启动房间清理协程
	m.wg.Add(1)
	go m.cleanupLoop()

	// 创建默认房间
	m.getOrCreateRoom(DefaultRoomID)
}

// cleanupLoop 定期清理空房间
func (m *RoomManager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.shutdown:
			return
		case <-ticker.C:
			m.cleanupEmptyRooms()
		}
	}
}

// cleanupEmptyRooms 清理空房间（保留默认房间）
func (m *RoomManager) cleanupEmptyRooms() {
	m.roomMutex.Lock()
	defer m.roomMutex.Unlock()

	for roomID, room := range m.rooms {
		if roomID == DefaultRoomID {
			continue // 不清理默认房间
		}

		// 检查房间是否为空且已结束
		if len(room.connections) == 0 && room.state != StateRunning {
			log.Printf("清理空房间: %s", roomID)
			room.Shutdown()
			delete(m.rooms, roomID)
		}
	}
}

// getOrCreateRoom 获取或创建房间
func (m *RoomManager) getOrCreateRoom(roomID string) *Room {
	m.roomMutex.Lock()
	defer m.roomMutex.Unlock()

	if room, exists := m.rooms[roomID]; exists {
		return room
	}

	// 创建新房间
	log.Printf("创建新房间: %s", roomID)
	room := NewRoom(m.ctx, m.enableAI)
	m.rooms[roomID] = room

	// 启动房间循环
	m.wg.Add(1)
	go room.Run(&m.wg)

	return room
}

// Join 玩家加入房间
func (m *RoomManager) Join(session Session, req JoinEvent) error {
	// 确定房间 ID
	roomID := req.RoomID
	if roomID == "" {
		roomID = DefaultRoomID
	}

	// 获取或创建房间
	room := m.getOrCreateRoom(roomID)

	// 检查房间是否已满
	if len(room.connections) >= MaxPlayers {
		return fmt.Errorf("房间 %s 已满 (%d/%d)", roomID, len(room.connections), MaxPlayers)
	}

	// 检查房间状态
	if room.state == StateEnding {
		return fmt.Errorf("房间 %s 结算中，暂时无法加入", roomID)
	}

	// 设置玩家房间 ID
	session.SetRoomID(roomID)

	// 加入房间
	if err := room.Join(session, req); err != nil {
		session.SetRoomID("")
		return err
	}

	log.Printf("玩家 %d 加入房间 %s", session.ID(), roomID)
	return nil
}

// EnqueueInput 将输入放入对应房间的队列
func (m *RoomManager) EnqueueInput(playerID int32, input InputEvent) {
	m.roomMutex.RLock()
	defer m.roomMutex.RUnlock()

	room, exists := m.rooms[input.RoomID]
	if !exists {
		log.Printf("警告: 房间 %s 不存在，玩家 %d 的输入被丢弃", input.RoomID, playerID)
		return
	}

	room.EnqueueInput(playerID, input)
}

// Leave 玩家离开房间
func (m *RoomManager) Leave(playerID int32, roomID string) {
	m.roomMutex.RLock()
	defer m.roomMutex.RUnlock()

	room, exists := m.rooms[roomID]
	if !exists {
		log.Printf("警告: 房间 %s 不存在，玩家 %d 的离开请求被忽略", roomID, playerID)
		return
	}

	if _, exists := room.connections[playerID]; exists {
		log.Printf("玩家 %d 离开房间 %s", playerID, roomID)
		room.Leave(playerID)
		return
	}

	log.Printf("警告: 玩家 %d 不在任何房间中", playerID)
}

// CurrentFrame 获取房间当前帧号（已弃用，多房间模式下不适用）
func (m *RoomManager) CurrentFrame() int32 {
	m.roomMutex.RLock()
	defer m.roomMutex.RUnlock()

	// 返回默认房间的帧号（用于兼容）
	if room, exists := m.rooms[DefaultRoomID]; exists {
		return room.CurrentFrame()
	}
	return 0
}

// Shutdown 关闭所有房间
func (m *RoomManager) Shutdown() {
	close(m.shutdown)

	m.roomMutex.Lock()
	defer m.roomMutex.Unlock()

	log.Printf("关闭 %d 个房间...", len(m.rooms))

	for roomID, room := range m.rooms {
		log.Printf("关闭房间: %s", roomID)
		room.Shutdown()
	}

	// 等待所有房间结束
	m.wg.Wait()

	log.Println("所有房间已关闭")
}

// GetRoomStats 获取房间统计信息
func (m *RoomManager) GetRoomStats() map[string]RoomStats {
	m.roomMutex.RLock()
	defer m.roomMutex.RUnlock()

	stats := make(map[string]RoomStats)
	for roomID, room := range m.rooms {
		stats[roomID] = RoomStats{
			PlayerCount: len(room.connections),
			State:       int(room.state),
			FrameID:     room.frameID,
		}
	}
	return stats
}

// RoomStats 房间统计信息
type RoomStats struct {
	PlayerCount int
	State       int
	FrameID     int32
}

// CreateRoom 创建新房间（返回房间 ID）
func (m *RoomManager) CreateRoom() string {
	roomID := fmt.Sprintf("room_%d", time.Now().UnixNano())
	m.getOrCreateRoom(roomID)
	return roomID
}

// ReconnectPlayer 玩家重连
// 返回当前游戏状态用于恢复
func (m *RoomManager) ReconnectPlayer(newConnID int32, playerID int32, roomID string, newConn Session) (*gamev1.GameState, error) {
	m.roomMutex.RLock()
	defer m.roomMutex.RUnlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return nil, fmt.Errorf("房间 %s 不存在", roomID)
	}

	// 检查玩家是否在这个房间中
	if _, exists := room.connections[playerID]; !exists {
		return nil, fmt.Errorf("玩家 %d 不在房间 %s 中", playerID, roomID)
	}

	// 获取当前游戏状态
	currentState := room.BuildGameState()

	// 替换连接
	room.ReplaceConnection(playerID, newConn)

	log.Printf("玩家 %d 在房间 %s 重连，新连接 ID: %d", playerID, roomID, newConnID)

	return currentState, nil
}
