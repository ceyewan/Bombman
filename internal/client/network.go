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

	// 网络
	connected bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// 消息队列
	stateChan       chan *gamev1.ServerState
	gameStartChan   chan *gamev1.GameStart
	gameOverChan    chan *gamev1.GameOver
	playerJoinChan  chan *gamev1.PlayerJoin
	playerLeaveChan chan int32

	// 发送队列
	inputSeq int32
	sendChan chan []byte

	// 初始地图
	initialMap *gamev1.MapState

	// 错误
	errChan chan error
}

// NewNetworkClient 创建网络客户端
func NewNetworkClient(serverAddr, proto string, character core.CharacterType) *NetworkClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &NetworkClient{
		serverAddr:      serverAddr,
		proto:           proto,
		character:       character,
		ctx:             ctx,
		cancel:          cancel,
		stateChan:       make(chan *gamev1.ServerState, 256),
		gameStartChan:   make(chan *gamev1.GameStart, 1),
		gameOverChan:    make(chan *gamev1.GameOver, 1),
		playerJoinChan:  make(chan *gamev1.PlayerJoin, 16),
		playerLeaveChan: make(chan int32, 16),
		sendChan:        make(chan []byte, 256),
		errChan:         make(chan error, 1),
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

	// 发送加入请求
	if err := nc.sendJoinRequest(); err != nil {
		nc.Close()
		return fmt.Errorf("发送加入请求失败: %w", err)
	}

	// 等待游戏开始消息
	select {
	case gameStart := <-nc.gameStartChan:
		nc.playerID = gameStart.YourPlayerId
		nc.initialMap = gameStart.InitialMap
		log.Printf("玩家 ID: %d", nc.playerID)
		return nil

	case err := <-nc.errChan:
		nc.Close()
		return err

	case <-time.After(10 * time.Second):
		nc.Close()
		return errors.New("等待游戏开始超时")
	}
}

// GetInitialMap 获取初始地图（由服务器下发）
func (nc *NetworkClient) GetInitialMap() *gamev1.MapState {
	return nc.initialMap
}

func (nc *NetworkClient) dial() (net.Conn, error) {
	switch nc.proto {
	case "", "tcp":
		return net.DialTimeout("tcp", nc.serverAddr, 5*time.Second)
	case "kcp":
		conn, err := kcp.DialWithOptions(nc.serverAddr, nil, 0, 0)
		if err != nil {
			return nil, err
		}
		conn.SetStreamMode(true)
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
	close(nc.gameStartChan)
	close(nc.gameOverChan)
	close(nc.playerJoinChan)
	close(nc.playerLeaveChan)
	close(nc.sendChan)
	close(nc.errChan)

	log.Printf("网络客户端已关闭")
}

// GetPlayerID 获取玩家 ID
func (nc *NetworkClient) GetPlayerID() int32 {
	return nc.playerID
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
	// 反序列化
	packet, err := protocol.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("反序列化失败: %w", err)
	}

	// 根据消息类型分发
	switch payload := packet.Payload.(type) {
	case *gamev1.GamePacket_State:
		// 游戏状态
		select {
		case nc.stateChan <- payload.State:
		default:
			// 队列满，丢弃旧状态
		}

	case *gamev1.GamePacket_GameStart:
		// 游戏开始
		select {
		case nc.gameStartChan <- payload.GameStart:
		default:
		}

	case *gamev1.GamePacket_GameOver:
		// 游戏结束
		select {
		case nc.gameOverChan <- payload.GameOver:
		default:
		}

	case *gamev1.GamePacket_PlayerJoin:
		// 玩家加入
		select {
		case nc.playerJoinChan <- payload.PlayerJoin:
		default:
		}

	case *gamev1.GamePacket_PlayerLeave:
		// 玩家离开
		select {
		case nc.playerLeaveChan <- payload.PlayerLeave.PlayerId:
		default:
		}

	default:
		return fmt.Errorf("未知消息类型: %T", payload)
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
	packet := protocol.NewJoinRequest(protoCharType)

	data, err := protocol.Marshal(packet)
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

// ========== 输入 ==========

// SendInput 发送玩家输入
func (nc *NetworkClient) SendInput(up, down, left, right, bomb bool) {
	nc.SendInputWithSeq(up, down, left, right, bomb)
}

// SendInputWithSeq 发送玩家输入并返回序号
func (nc *NetworkClient) SendInputWithSeq(up, down, left, right, bomb bool) int32 {
	if !nc.connected {
		return 0
	}

	nc.inputSeq++
	seq := nc.inputSeq

	packet := protocol.NewClientInput(seq, up, down, left, right, bomb)

	data, err := protocol.Marshal(packet)
	if err != nil {
		log.Printf("序列化输入失败: %v", err)
		return seq
	}

	if err := nc.sendMessage(data); err != nil {
		log.Printf("发送输入失败: %v", err)
	}

	return seq
}

// ========== 状态接收 ==========

// ReceiveState 接收游戏状态（非阻塞）
func (nc *NetworkClient) ReceiveState() *gamev1.ServerState {
	select {
	case state := <-nc.stateChan:
		return state
	default:
		return nil
	}
}

// ReceiveGameStart 接收游戏开始（非阻塞）
func (nc *NetworkClient) ReceiveGameStart() *gamev1.GameStart {
	select {
	case gameStart := <-nc.gameStartChan:
		return gameStart
	default:
		return nil
	}
}

// ReceiveGameOver 接收游戏结束（非阻塞）
func (nc *NetworkClient) ReceiveGameOver() *gamev1.GameOver {
	select {
	case gameOver := <-nc.gameOverChan:
		return gameOver
	default:
		return nil
	}
}

// ReceivePlayerJoin 接收玩家加入（非阻塞）
func (nc *NetworkClient) ReceivePlayerJoin() *gamev1.PlayerJoin {
	select {
	case playerJoin := <-nc.playerJoinChan:
		return playerJoin
	default:
		return nil
	}
}

// ReceivePlayerLeave 接收玩家离开（非阻塞）
func (nc *NetworkClient) ReceivePlayerLeave() int32 {
	select {
	case playerID := <-nc.playerLeaveChan:
		return playerID
	default:
		return -1
	}
}
