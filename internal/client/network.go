package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
	"bomberman/pkg/protocol"

	kcp "github.com/xtaci/kcp-go/v5"
)

const (
	MaxPacketSize = 4096
)

// NetworkClient 网络客户端

type NetworkClient struct {
	conn       net.Conn
	serverAddr string
	proto      string

	// 玩家信息
	playerID  int32
	character core.CharacterType
	gameSeed  int64
	tps       int32

	// 网络
	connected bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// 消息队列
	stateChan    chan *gamev1.GameState
	eventChan    chan *gamev1.GameEvent
	joinRespChan chan *gamev1.JoinResponse

	// 发送队列
	inputSeq        int32
	sendChan        chan []byte
	lastServerFrame int32

	// 时间同步（毫秒）
	timeOffsetMs        int64
	lastServerTimeMs    int64
	lastServerFramePong int32
	lastRTTMs           int64

	// 错误
	errChan chan error
}

// NewNetworkClient 创建网络客户端
func NewNetworkClient(serverAddr, proto string, character core.CharacterType) *NetworkClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &NetworkClient{
		serverAddr:   serverAddr,
		proto:        proto,
		character:    character,
		ctx:          ctx,
		cancel:       cancel,
		stateChan:    make(chan *gamev1.GameState, 256),
		eventChan:    make(chan *gamev1.GameEvent, 64),
		joinRespChan: make(chan *gamev1.JoinResponse, 1),
		sendChan:     make(chan []byte, 256),
		errChan:      make(chan error, 1),
	}
}

// Connect 连接到服务器
func (nc *NetworkClient) Connect() error {
	log.Printf("连接到服务器: %s (%s)", nc.serverAddr, nc.proto)

	// 建立连接
	conn, err := nc.dial()
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}

	nc.conn = conn
	nc.connected = true

	log.Printf("已连接到服务器: %s", conn.RemoteAddr())

	// 启动接收循环
	nc.wg.Add(1)
	go nc.receiveLoop()

	// 启动发送循环
	nc.wg.Add(1)
	go nc.sendLoop()

	// 启动 Ping 循环
	nc.wg.Add(1)
	go nc.pingLoop()

	// 发送加入请求
	if err := nc.sendJoinRequest(); err != nil {
		nc.Close()
		return fmt.Errorf("发送加入请求失败: %w", err)
	}

	// 等待加入响应
	select {
	case resp := <-nc.joinRespChan:
		if resp == nil {
			nc.Close()
			return errors.New("加入响应为空")
		}
		if !resp.Success {
			nc.Close()
			return fmt.Errorf("加入失败: %s", resp.ErrorMessage)
		}
		nc.playerID = resp.PlayerId
		nc.gameSeed = resp.GameSeed
		nc.tps = resp.Tps
		log.Printf("玩家 ID: %d", nc.playerID)
		return nil

	case err := <-nc.errChan:
		nc.Close()
		return err

	case <-time.After(10 * time.Second):
		nc.Close()
		return errors.New("等待加入响应超时")
	}
}

func (nc *NetworkClient) dial() (net.Conn, error) {
	switch nc.proto {
	case "", "tcp":
		conn, err := net.DialTimeout("tcp", nc.serverAddr, 5*time.Second)
		if err != nil {
			return nil, err
		}
		// 开启 TCP_NODELAY，禁用 Nagle 算法以减少延迟
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetNoDelay(true)
		}
		return conn, nil
	case "kcp":
		conn, err := kcp.DialWithOptions(nc.serverAddr, nil, 0, 0)
		if err != nil {
			return nil, err
		}
		// 不需要 SetStreamMode，我们使用长度前缀协议处理消息边界
		return conn, nil
	default:
		return nil, fmt.Errorf("不支持的协议: %s", nc.proto)
	}
}

// Close 关闭连接
func (nc *NetworkClient) Close() {
	if !nc.connected {
		return
	}

	nc.connected = false
	nc.cancel()

	// 关闭网络连接
	if nc.conn != nil {
		nc.conn.Close()
	}

	// 等待所有 goroutine 结束
	nc.wg.Wait()

	// 关闭通道
	close(nc.stateChan)
	close(nc.eventChan)
	close(nc.joinRespChan)
	close(nc.sendChan)
	close(nc.errChan)

	log.Printf("网络客户端已关闭")
}

// GetPlayerID 获取玩家 ID
func (nc *NetworkClient) GetPlayerID() int32 {
	return nc.playerID
}

func (nc *NetworkClient) GetGameSeed() int64 {
	return nc.gameSeed
}

func (nc *NetworkClient) GetTPS() int32 {
	return nc.tps
}

// IsConnected 检查是否已连接
func (nc *NetworkClient) IsConnected() bool {
	return nc.connected
}

// ========== 消息接收 ==========

// receiveLoop 接收循环
func (nc *NetworkClient) receiveLoop() {
	defer nc.wg.Done()

	for {
		select {
		case <-nc.ctx.Done():
			return

		default:
			// 读取消息长度（4 字节）
			var length uint32
			if err := binary.Read(nc.conn, binary.BigEndian, &length); err != nil {
				if !errors.Is(err, io.EOF) {
					nc.errChan <- fmt.Errorf("读取长度失败: %w", err)
				}
				return
			}

			// 检查消息大小
			if length > MaxPacketSize {
				nc.errChan <- fmt.Errorf("消息过大 (%d bytes)", length)
				return
			}

			if length == 0 {
				continue
			}

			// 读取消息体
			data := make([]byte, length)
			if _, err := io.ReadFull(nc.conn, data); err != nil {
				nc.errChan <- fmt.Errorf("读取数据失败: %w", err)
				return
			}

			// 处理消息
			if err := nc.handleMessage(data); err != nil {
				log.Printf("处理消息失败: %v", err)
			}
		}
	}
}

// handleMessage 处理接收到的消息
func (nc *NetworkClient) handleMessage(data []byte) error {
	pkt, err := protocol.UnmarshalPacket(data)
	if err != nil {
		return fmt.Errorf("反序列化失败: %w", err)
	}

	switch pkt.Type {
	case gamev1.MessageType_MESSAGE_TYPE_JOIN_RESPONSE:
		resp, err := protocol.ParseJoinResponse(pkt)
		if err != nil {
			return fmt.Errorf("解析加入响应失败: %w", err)
		}
		select {
		case nc.joinRespChan <- resp:
		default:
		}

	case gamev1.MessageType_MESSAGE_TYPE_GAME_STATE:
		state, err := protocol.ParseGameState(pkt)
		if err != nil {
			return fmt.Errorf("解析状态失败: %w", err)
		}
		nc.lastServerFrame = state.FrameId
		select {
		case nc.stateChan <- state:
		default:
		}

	case gamev1.MessageType_MESSAGE_TYPE_GAME_EVENT:
		event, err := protocol.ParseGameEvent(pkt)
		if err != nil {
			return fmt.Errorf("解析事件失败: %w", err)
		}
		select {
		case nc.eventChan <- event:
		default:
		}

	case gamev1.MessageType_MESSAGE_TYPE_PING:
		ping, err := protocol.ParsePing(pkt)
		if err != nil {
			return fmt.Errorf("解析 Ping 失败: %w", err)
		}
		return nc.sendPong(ping.ClientTime)

	case gamev1.MessageType_MESSAGE_TYPE_PONG:
		pong, err := protocol.ParsePong(pkt)
		if err != nil {
			return fmt.Errorf("解析 Pong 失败: %w", err)
		}
		nc.handlePong(pong)
		return nil

	default:
		return fmt.Errorf("未知消息类型: %v", pkt.Type)
	}

	return nil
}

// ========== 消息发送 ==========

// sendLoop 发送循环
func (nc *NetworkClient) sendLoop() {
	defer nc.wg.Done()

	for {
		select {
		case <-nc.ctx.Done():
			return

		case data, ok := <-nc.sendChan:
			if !ok {
				return
			}

			// 发送长度前缀（4 字节）
			length := uint32(len(data))
			if err := binary.Write(nc.conn, binary.BigEndian, length); err != nil {
				log.Printf("发送长度失败: %v", err)
				return
			}

			// 发送数据体
			if _, err := nc.conn.Write(data); err != nil {
				log.Printf("发送数据失败: %v", err)
				return
			}
		}
	}
}

// sendJoinRequest 发送加入请求
func (nc *NetworkClient) sendJoinRequest() error {
	protoCharType := protocol.CoreCharacterTypeToProto(nc.character)
	packet, err := protocol.NewJoinRequestPacket("Player", protoCharType)
	if err != nil {
		return err
	}

	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		return err
	}

	return nc.sendMessage(data)
}

func (nc *NetworkClient) sendPong(clientTime int64) error {
	packet, err := protocol.NewPongPacket(clientTime, time.Now().UnixMilli(), nc.lastServerFrame)
	if err != nil {
		return err
	}
	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		return err
	}
	return nc.sendMessage(data)
}

// sendMessage 发送消息
func (nc *NetworkClient) sendMessage(data []byte) error {
	select {
	case nc.sendChan <- data:
		return nil
	default:
		return errors.New("发送队列满")
	}
}

func (nc *NetworkClient) pingLoop() {
	defer nc.wg.Done()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-nc.ctx.Done():
			return
		case <-ticker.C:
			if err := nc.sendPing(); err != nil {
				log.Printf("发送 Ping 失败: %v", err)
			}
		}
	}
}

func (nc *NetworkClient) sendPing() error {
	packet, err := protocol.NewPingPacket(time.Now().UnixMilli())
	if err != nil {
		return err
	}
	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		return err
	}
	return nc.sendMessage(data)
}

func (nc *NetworkClient) handlePong(pong *gamev1.Pong) {
	if pong == nil {
		return
	}

	now := time.Now().UnixMilli()
	rtt := now - pong.ClientTime
	if rtt < 0 {
		return
	}

	measuredOffset := pong.ServerTime - (pong.ClientTime + rtt/2)
	prev := atomic.LoadInt64(&nc.timeOffsetMs)
	if prev == 0 {
		atomic.StoreInt64(&nc.timeOffsetMs, measuredOffset)
	} else {
		smoothed := int64(float64(prev)*0.9 + float64(measuredOffset)*0.1)
		atomic.StoreInt64(&nc.timeOffsetMs, smoothed)
	}

	atomic.StoreInt64(&nc.lastRTTMs, rtt)
	atomic.StoreInt64(&nc.lastServerTimeMs, pong.ServerTime)
	atomic.StoreInt32(&nc.lastServerFramePong, pong.ServerFrame)
}

// ========== 输入 ==========

// SendInput 发送玩家输入
func (nc *NetworkClient) SendInput(frameID int32, up, down, left, right, bomb bool) {
	nc.SendInputWithSeq(frameID, up, down, left, right, bomb)
}

// SendInputWithSeq 发送玩家输入并返回序号
func (nc *NetworkClient) SendInputWithSeq(frameID int32, up, down, left, right, bomb bool) int32 {
	if !nc.connected {
		return 0
	}

	nc.inputSeq++
	seq := nc.inputSeq

	packet, err := protocol.NewClientInputPacket(seq, frameID, up, down, left, right, bomb)
	if err != nil {
		log.Printf("构造输入失败: %v", err)
		return seq
	}

	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		log.Printf("序列化输入失败: %v", err)
		return seq
	}

	if err := nc.sendMessage(data); err != nil {
		log.Printf("发送输入失败: %v", err)
	}

	return seq
}

// SendInputBatch 批量发送玩家输入（用于输入缓冲）
func (nc *NetworkClient) SendInputBatch(inputs []*gamev1.InputData) int32 {
	if !nc.connected || len(inputs) == 0 {
		return 0
	}

	nc.inputSeq++
	seq := nc.inputSeq

	packet, err := protocol.NewClientInputPacketWithInputs(seq, inputs)
	if err != nil {
		log.Printf("构造批量输入失败: %v", err)
		return seq
	}

	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		log.Printf("序列化批量输入失败: %v", err)
		return seq
	}

	if err := nc.sendMessage(data); err != nil {
		log.Printf("发送批量输入失败: %v", err)
	}

	return seq
}

// ========== 状态接收 ==========

// ReceiveState 接收游戏状态（非阻塞）
func (nc *NetworkClient) ReceiveState() *gamev1.GameState {
	select {
	case state := <-nc.stateChan:
		return state
	default:
		return nil
	}
}

// ReceiveEvent 接收游戏事件（非阻塞）
func (nc *NetworkClient) ReceiveEvent() *gamev1.GameEvent {
	select {
	case event := <-nc.eventChan:
		return event
	default:
		return nil
	}
}

// EstimatedServerTimeMs 估算服务器时间（毫秒）
func (nc *NetworkClient) EstimatedServerTimeMs() int64 {
	offset := atomic.LoadInt64(&nc.timeOffsetMs)
	if offset == 0 {
		return 0
	}
	return time.Now().UnixMilli() + offset
}

// EstimatedServerFrame 估算服务器当前帧号
func (nc *NetworkClient) EstimatedServerFrame() int32 {
	lastTime := atomic.LoadInt64(&nc.lastServerTimeMs)
	lastFrame := atomic.LoadInt32(&nc.lastServerFramePong)
	if lastTime == 0 {
		return nc.lastServerFrame
	}

	now := nc.EstimatedServerTimeMs()
	if now == 0 {
		return nc.lastServerFrame
	}

	deltaMs := now - lastTime
	if deltaMs <= 0 {
		return lastFrame
	}

	frameStep := 1000.0 / float64(core.TPS)
	advance := int32(float64(deltaMs) / frameStep)
	return lastFrame + advance
}
