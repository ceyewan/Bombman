package server

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"
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
	roomManager *RoomManager

	// 配置
	enableAI bool

	// 网络 - 支持双协议监听
	tcpListener ServerListener
	kcpListener ServerListener
	tcpAddr     string
	kcpAddr     string

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
		tcpAddr:  addr, // TCP 监听地址
		kcpAddr:  addr, // KCP 监听同一地址（不同协议）
		enableAI: enableAI,
		ctx:      ctx,
		cancel:   cancel,
		shutdown: make(chan struct{}),
	}
}

// Start 启动服务器
func (s *GameServer) Start() error {
	log.Printf("启动游戏服务器 (TCP + KCP): %s", s.tcpAddr)

	// 监听 TCP
	tcpListener, err := newListener("tcp", s.tcpAddr)
	if err != nil {
		return fmt.Errorf("监听 TCP 失败: %w", err)
	}
	s.tcpListener = tcpListener

	// 监听 KCP
	kcpListener, err := newListener("kcp", s.kcpAddr)
	if err != nil {
		tcpListener.Close()
		return fmt.Errorf("监听 KCP 失败: %w", err)
	}
	s.kcpListener = kcpListener

	log.Printf("TCP 监听中: %s", s.tcpAddr)
	log.Printf("KCP 监听中: %s", s.kcpAddr)

	s.roomManager = NewRoomManager(s.ctx, s.enableAI)
	s.roomManager.Run(&s.wg)

	// 启动 TCP 连接接受循环
	s.wg.Add(1)
	go s.acceptLoopTCP()

	// 启动 KCP 连接接受循环
	s.wg.Add(1)
	go s.acceptLoopKCP()

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

	if s.roomManager != nil {
		s.roomManager.Shutdown()
	}

	// 关闭监听器
	if s.tcpListener != nil {
		s.tcpListener.Close()
	}
	if s.kcpListener != nil {
		s.kcpListener.Close()
	}

	// 关闭 shutdown 通道
	close(s.shutdown)

	// 等待所有 goroutine 结束
	s.wg.Wait()

	log.Println("服务器已关闭")
}

// acceptLoopTCP 接受 TCP 客户端连接
func (s *GameServer) acceptLoopTCP() {
	defer s.wg.Done()
	s.acceptLoop("TCP", s.tcpListener)
}

// acceptLoopKCP 接受 KCP 客户端连接
func (s *GameServer) acceptLoopKCP() {
	defer s.wg.Done()
	s.acceptLoop("KCP", s.kcpListener)
}

// acceptLoop 通用连接接受循环
func (s *GameServer) acceptLoop(proto string, listener ServerListener) {
	for {
		select {
		case <-s.ctx.Done():
			log.Printf("停止接受新 %s 连接", proto)
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Printf("[%s] 接受连接失败: %v", proto, err)
				continue
			}
		}

		log.Printf("[%s] 新连接来自: %s", proto, conn.RemoteAddr())

		// 创建连接对象
		connection := NewConnection(conn, s)

		// 启动连接处理
		s.wg.Add(1)
		go connection.Handle(s.ctx, &s.wg)
	}
}

// handleJoinRequest 处理加入请求
func (s *GameServer) handleJoinRequest(conn Session, req *JoinEvent) error {
	if s.roomManager == nil {
		return fmt.Errorf("房间未初始化")
	}
	return s.roomManager.Join(conn, *req)
}

// handleClientInput 处理客户端输入
func (s *GameServer) handleClientInput(conn Session, input *InputEvent) {
	if s.roomManager == nil {
		return
	}
	s.roomManager.EnqueueInput(conn.ID(), *input)
}

// removePlayer 移除玩家
func (s *GameServer) removePlayer(conn Session) {
	if s.roomManager == nil {
		return
	}
	s.roomManager.Leave(conn.ID(), conn.GetRoomID())
}

// handlePing 处理客户端 Ping 请求，并回复 Pong，包含服务器时间和当前帧数
func (s *GameServer) handlePing(conn Session, ping *PingEvent) {
	if ping == nil {
		return
	}
	serverFrame := int32(0)
	if s.roomManager != nil {
		serverFrame = s.roomManager.CurrentFrame()
	}

	packet, err := protocol.NewPongPacket(ping.ClientTime, time.Now().UnixMilli(), serverFrame)
	if err != nil {
		return
	}
	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		return
	}
	_ = conn.Send(data)
}

// handleReconnect 处理重连请求
func (s *GameServer) handleReconnect(conn Session, req *ReconnectEvent) {
	if req == nil || req.SessionToken == "" {
		s.sendReconnectResponse(conn, false, "缺少会话令牌", nil)
		return
	}

	// 验证 Token
	playerID, roomID, err := VerifySessionToken(req.SessionToken)
	if err != nil {
		log.Printf("重连失败: Token 验证失败: %v", err)
		s.sendReconnectResponse(conn, false, "会话令牌无效或已过期", nil)
		return
	}

	if s.roomManager == nil {
		s.sendReconnectResponse(conn, false, "房间未初始化", nil)
		return
	}

	// 尝试恢复会话
	currentState, err := s.roomManager.ReconnectPlayer(conn.ID(), playerID, roomID, conn)
	if err != nil {
		log.Printf("重连失败: 玩家 %d: %v", playerID, err)
		s.sendReconnectResponse(conn, false, err.Error(), nil)
		return
	}

	// 设置连接的玩家 ID 和房间 ID
	conn.SetPlayerID(playerID)
	conn.SetRoomID(roomID)

	log.Printf("玩家 %d 重连成功", playerID)
	s.sendReconnectResponse(conn, true, "", currentState)
}

// sendReconnectResponse 发送重连响应
func (s *GameServer) sendReconnectResponse(conn Session, success bool, errMsg string, currentState *gamev1.GameState) {
	packet, err := protocol.NewReconnectResponsePacket(success, errMsg, currentState)
	if err != nil {
		log.Printf("构造重连响应失败: %v", err)
		return
	}
	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		log.Printf("序列化重连响应失败: %v", err)
		return
	}
	if err := conn.Send(data); err != nil {
		log.Printf("发送重连响应失败: %v", err)
	}
}
