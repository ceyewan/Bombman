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
		return net.Listen("tcp", addr)
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

type kcpListener struct {
	listener *kcp.Listener
}

func (l *kcpListener) Accept() (net.Conn, error) {
	session, err := l.listener.AcceptKCP()
	if err != nil {
		return nil, err
	}
	session.SetStreamMode(true)
	return session, nil
}

func (l *kcpListener) Close() error {
	return l.listener.Close()
}

func (l *kcpListener) Addr() net.Addr {
	return l.listener.Addr()
}
