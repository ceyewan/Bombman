package server

type Session interface {
	ID() int32
	Send(data []byte) error
	Close()
	CloseWithoutNotify()
	SetPlayerID(id int32)
}
