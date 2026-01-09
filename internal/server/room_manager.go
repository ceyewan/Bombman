package server

import (
	"context"
	"sync"
)

type RoomManager struct {
	room *Room
}

func NewRoomManager(ctx context.Context, enableAI bool) *RoomManager {
	return &RoomManager{
		room: NewRoom(ctx, enableAI),
	}
}

func (m *RoomManager) Run(wg *sync.WaitGroup) {
	wg.Add(1)
	go m.room.Run(wg)
}

func (m *RoomManager) Join(session Session, req JoinEvent) error {
	return m.room.Join(session, req)
}

func (m *RoomManager) EnqueueInput(playerID int32, input InputEvent) {
	m.room.EnqueueInput(playerID, input)
}

func (m *RoomManager) Leave(playerID int32) {
	m.room.Leave(playerID)
}

func (m *RoomManager) CurrentFrame() int32 {
	return m.room.CurrentFrame()
}

func (m *RoomManager) Shutdown() {
	m.room.Shutdown()
}
