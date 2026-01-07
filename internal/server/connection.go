package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/protocol"
)

const (
	MaxPacketSize = 4096 // 最大消息大小
)

// Connection 表示一个客户端连接
type Connection struct {
	conn     net.Conn
	server   *GameServer
	playerID int32

	// 发送队列
	sendChan chan []byte
	closeCh  chan struct{}
	closed   bool
	closeMu  sync.Mutex
}

// NewConnection 创建新连接，连接到服务器上
func NewConnection(conn net.Conn, server *GameServer) *Connection {
	return &Connection{
		conn:     conn,
		server:   server,
		playerID: -1,                     // -1 表示未分配
		sendChan: make(chan []byte, 256), // 发送队列缓冲区
		closeCh:  make(chan struct{}),
		closed:   false,
	}
}

// Handle 处理连接
func (c *Connection) Handle(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("玩家 %d: 连接处理开始", c.getPlayerID())

	// 启动发送循环
	wg.Add(1)
	go c.sendLoop(ctx, wg)

	// 启动接收循环
	wg.Add(1)
	go c.receiveLoop(ctx, wg)

	// 等待上下文取消或连接关闭
	select {
	case <-ctx.Done():
	case <-c.closeCh:
	}

	c.Close()
}

// Close 关闭连接
func (c *Connection) Close() {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.closeCh)

	// 关闭网络连接
	if c.conn != nil {
		c.conn.Close()
	}

	// 关闭发送通道
	close(c.sendChan)

	// 从服务器移除玩家
	if playerID := c.getPlayerID(); playerID >= 0 {
		c.server.removePlayer(playerID)
	}

	log.Printf("玩家 %d: 连接已关闭", c.getPlayerID())
}

// Send 发送数据（异步）
func (c *Connection) Send(data []byte) error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return fmt.Errorf("连接已关闭")
	}
	defer c.closeMu.Unlock()

	select {
	case c.sendChan <- data:
		return nil
	default:
		return fmt.Errorf("发送队列满")
	}
}

// sendLoop 发送循环
func (c *Connection) sendLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("玩家 %d: 发送循环启动", c.getPlayerID())

	for {
		select {
		case <-ctx.Done():
			log.Printf("玩家 %d: 发送循环停止", c.getPlayerID())
			return

		case data, ok := <-c.sendChan:
			if !ok {
				// 通道已关闭
				return
			}

			// 发送数据长度前缀（4 字节）
			length := uint32(len(data))
			if err := binary.Write(c.conn, binary.BigEndian, length); err != nil {
				log.Printf("玩家 %d: 发送长度失败: %v", c.getPlayerID(), err)
				c.Close()
				return
			}

			// 发送数据体
			if _, err := c.conn.Write(data); err != nil {
				log.Printf("玩家 %d: 发送数据失败: %v", c.getPlayerID(), err)
				c.Close()
				return
			}
		}
	}
}

// receiveLoop 接收循环
func (c *Connection) receiveLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("玩家 %d: 接收循环启动", c.getPlayerID())

	for {
		select {
		case <-ctx.Done():
			log.Printf("玩家 %d: 接收循环停止", c.getPlayerID())
			return

		default:
			// 读取消息长度（4 字节）
			var length uint32
			if err := binary.Read(c.conn, binary.BigEndian, &length); err != nil {
				if err != io.EOF {
					log.Printf("玩家 %d: 读取长度失败: %v", c.getPlayerID(), err)
				}
				c.Close()
				return
			}

			// 检查消息大小
			if length > MaxPacketSize {
				log.Printf("玩家 %d: 消息过大 (%d bytes)", c.getPlayerID(), length)
				c.Close()
				return
			}

			if length == 0 {
				log.Printf("玩家 %d: 收到空消息", c.getPlayerID())
				continue
			}

			// 读取消息体
			data := make([]byte, length)
			if _, err := io.ReadFull(c.conn, data); err != nil {
				log.Printf("玩家 %d: 读取数据失败: %v", c.getPlayerID(), err)
				c.Close()
				return
			}

			// 处理消息
			if err := c.handleMessage(data); err != nil {
				log.Printf("玩家 %d: 处理消息失败: %v", c.getPlayerID(), err)
			}
		}
	}
}

// handleMessage 处理接收到的消息
func (c *Connection) handleMessage(data []byte) error {
	// 反序列化
	packet, err := protocol.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("反序列化失败: %w", err)
	}

	// 根据消息类型分发
	switch payload := packet.Payload.(type) {
	case *gamev1.GamePacket_Input:
		// 客户端输入
		c.server.handleClientInput(c.getPlayerID(), payload.Input)

	case *gamev1.GamePacket_JoinReq:
		// 加入请求
		if c.getPlayerID() >= 0 {
			log.Printf("玩家 %d: 重复加入请求", c.getPlayerID())
			return fmt.Errorf("玩家已加入")
		}
		if err := c.server.handleJoinRequest(c, payload.JoinReq); err != nil {
			return fmt.Errorf("处理加入请求失败: %w", err)
		}
		log.Printf("玩家 %d: 加入成功", c.getPlayerID())

	default:
		return fmt.Errorf("未知消息类型: %T", payload)
	}

	return nil
}

// String 返回连接的字符串表示
func (c *Connection) String() string {
	if c.getPlayerID() >= 0 {
		return fmt.Sprintf("Connection{%d, %s}", c.getPlayerID(), c.conn.RemoteAddr())
	}
	return fmt.Sprintf("Connection{%s}", c.conn.RemoteAddr())
}

func (c *Connection) getPlayerID() int32 {
	return atomic.LoadInt32(&c.playerID)
}

func (c *Connection) setPlayerID(playerID int32) {
	atomic.StoreInt32(&c.playerID, playerID)
}
