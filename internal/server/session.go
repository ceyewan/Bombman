package server

type Session interface {
	ID() int32
	GetRoomID() string
	SetRoomID(roomID string)
	Send(data []byte) error
	Close()
	CloseWithoutNotify()
	SetPlayerID(id int32)
}
