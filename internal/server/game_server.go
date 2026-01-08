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
room *Room

// 配置
enableAI bool

// 网络
listener ServerListener
addr     string
proto    string

// 控制
ctx      context.Context
cancel   context.CancelFunc
wg       sync.WaitGroup
shutdown chan struct{}
}

// NewGameServer 创建新的游戏服务器
func NewGameServer(addr, proto string, enableAI bool) *GameServer {
ctx, cancel := context.WithCancel(context.Background())

return &GameServer{
addr:     addr,                // 监听地址
proto:    proto,               // 监听协议
enableAI: enableAI,            // 是否启用 AI
ctx:      ctx,                 // 上下文
cancel:   cancel,              // 取消函数
shutdown: make(chan struct{}), // 关闭信号
}
}

// Start 启动服务器
func (s *GameServer) Start() error {
	log.Printf("启动游戏服务器: %s", s.addr)

	// 监听 TCP 端口
	listener, err := newListener(s.proto, s.addr)
	if err != nil {
		return fmt.Errorf("监听失败: %w", err)
	}
	s.listener = listener

log.Printf("服务器监听中: %s", s.addr)

s.room = NewRoom(s.ctx, s.enableAI)

// 启动房间循环
s.wg.Add(1)
go s.room.Run(&s.wg)

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

	if s.room != nil {
		s.room.Shutdown()
	}

	// 关闭监听器
	if s.listener != nil {
		s.listener.Close()
	}

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

// handleJoinRequest 处理加入请求
func (s *GameServer) handleJoinRequest(conn *Connection, req *gamev1.JoinRequest) error {
	if s.room == nil {
		return fmt.Errorf("房间未初始化")
	}
	return s.room.Join(conn, req)
}

// handleClientInput 处理客户端输入
func (s *GameServer) handleClientInput(playerID int32, input *gamev1.ClientInput) {
	if s.room == nil {
		return
	}
	s.room.EnqueueInput(playerID, input)
}

// removePlayer 移除玩家
func (s *GameServer) removePlayer(playerID int32) {
	if s.room == nil {
		return
	}
	s.room.Leave(playerID)
}
