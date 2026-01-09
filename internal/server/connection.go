package server

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

	"bomberman/pkg/protocol"
)

const (
	MaxPacketSize = 4096            // 最大消息大小
	readTimeout   = 5 * time.Second // 读取超时
	writeTimeout  = 1 * time.Second // 写入超时
)

var ErrSendQueueFull = errors.New("发送队列满")

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

	lastRecvTime atomic.Value
	lastPingTime atomic.Value
	rtt          atomic.Int64
}

// NewConnection 创建新连接，连接到服务器上
func NewConnection(conn net.Conn, server *GameServer) *Connection {
	c := &Connection{
		conn:     conn,
		server:   server,
		playerID: -1,                     // -1 表示未分配
		sendChan: make(chan []byte, 256), // 发送队列缓冲区
		closeCh:  make(chan struct{}),
		closed:   false,
	}
	c.lastRecvTime.Store(time.Now())
	c.lastPingTime.Store(time.Time{})
	return c
}

// Handle 处理连接
func (c *Connection) Handle(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("玩家 %d: 连接处理开始", c.getPlayerID())

	wg.Add(1)
	go c.startHeartbeat(ctx, wg)

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
	c.closeWithNotify(true)
}

// CloseWithoutNotify 关闭连接但不触发移除玩家逻辑
func (c *Connection) CloseWithoutNotify() {
	c.closeWithNotify(false)
}

func (c *Connection) closeWithNotify(notify bool) {
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
	if notify {
		if playerID := c.getPlayerID(); playerID >= 0 {
			c.server.removePlayer(playerID)
		}
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
		return ErrSendQueueFull
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
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := binary.Write(c.conn, binary.BigEndian, length); err != nil {
				log.Printf("玩家 %d: 发送长度失败: %v", c.getPlayerID(), err)
				c.Close()
				return
			}

			// 发送数据体
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
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
			_ = c.conn.SetReadDeadline(time.Now().Add(readTimeout))
			if err := binary.Read(c.conn, binary.BigEndian, &length); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Printf("玩家 %d: 读取超时", c.getPlayerID())
				} else if err != io.EOF {
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
			_ = c.conn.SetReadDeadline(time.Now().Add(readTimeout))
			if _, err := io.ReadFull(c.conn, data); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Printf("玩家 %d: 读取超时", c.getPlayerID())
				} else {
					log.Printf("玩家 %d: 读取数据失败: %v", c.getPlayerID(), err)
				}
				c.Close()
				return
			}

			// 处理消息
			c.onMessageReceived()
			if err := c.handleMessage(data); err != nil {
				log.Printf("玩家 %d: 处理消息失败: %v", c.getPlayerID(), err)
			}
		}
	}
}

// handleMessage 处理接收到的消息
func (c *Connection) handleMessage(data []byte) error {
	event, err := DecodePacket(data)
	if err != nil {
		return fmt.Errorf("反序列化失败: %w", err)
	}

	switch event.Kind {
	case EventJoin:
		if c.getPlayerID() >= 0 {
			log.Printf("玩家 %d: 重复加入请求", c.getPlayerID())
			return fmt.Errorf("玩家已加入")
		}
		if err := c.server.handleJoinRequest(c, event.Join); err != nil {
			return fmt.Errorf("处理加入请求失败: %w", err)
		}
		log.Printf("玩家 %d: 加入成功", c.getPlayerID())

	case EventInput:
		event.Input.PlayerID = c.getPlayerID()
		c.server.handleClientInput(c.getPlayerID(), event.Input)

	case EventPing:
		c.server.handlePing(c, event.Ping)

	case EventPong:
		c.handlePong(event.Pong)

	default:
		return fmt.Errorf("未知消息类型")
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

func (c *Connection) ID() int32 {
	return c.getPlayerID()
}

func (c *Connection) SetPlayerID(playerID int32) {
	c.setPlayerID(playerID)
}

const (
	heartbeatInterval = 5 * time.Second
	heartbeatTimeout  = 15 * time.Second
)

func (c *Connection) startHeartbeat(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closeCh:
			return
		case <-ticker.C:
			lastRecv, _ := c.lastRecvTime.Load().(time.Time)
			if !lastRecv.IsZero() && time.Since(lastRecv) > heartbeatTimeout {
				log.Printf("玩家 %d: 心跳超时", c.getPlayerID())
				c.Close()
				return
			}
			c.sendPing()
		}
	}
}

func (c *Connection) sendPing() {
	packet, err := protocol.NewPingPacket(time.Now().UnixMilli())
	if err != nil {
		return
	}
	data, err := protocol.MarshalPacket(packet)
	if err != nil {
		return
	}
	c.lastPingTime.Store(time.Now())
	_ = c.Send(data)
}

func (c *Connection) handlePong(pong *PongEvent) {
	c.lastRecvTime.Store(time.Now())
	if pong == nil || pong.ClientTime <= 0 {
		return
	}
	rtt := time.Now().UnixMilli() - pong.ClientTime
	c.rtt.Store(rtt)
}

func (c *Connection) onMessageReceived() {
	c.lastRecvTime.Store(time.Now())
}
