package server

import (
	"fmt"
	"net"

	kcp "github.com/xtaci/kcp-go/v5"
)

type ServerListener interface {
	Accept() (net.Conn, error)
	Close() error
	Addr() net.Addr
}

func newListener(proto, addr string) (ServerListener, error) {
	switch proto {
	case "tcp":
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, err
		}
		return &tcpListener{listener: listener}, nil
	case "kcp":
		listener, err := kcp.ListenWithOptions(addr, nil, 0, 0)
		if err != nil {
			return nil, err
		}
		return &kcpListener{listener: listener}, nil
	default:
		return nil, fmt.Errorf("不支持的协议: %s", proto)
	}
}

type tcpListener struct {
	listener net.Listener
}

func (l *tcpListener) Accept() (net.Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}
	// 开启 TCP_NODELAY，禁用 Nagle 算法以减少延迟
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
	}
	return conn, nil
}

func (l *tcpListener) Close() error {
	return l.listener.Close()
}

func (l *tcpListener) Addr() net.Addr {
	return l.listener.Addr()
}

type kcpListener struct {
	listener *kcp.Listener
}

func (l *kcpListener) Accept() (net.Conn, error) {
	session, err := l.listener.AcceptKCP()
	if err != nil {
		return nil, err
	}
	// 不需要 SetStreamMode，我们使用长度前缀协议处理消息边界
	return session, nil
}

func (l *kcpListener) Close() error {
	return l.listener.Close()
}

func (l *kcpListener) Addr() net.Addr {
	return l.listener.Addr()
}
