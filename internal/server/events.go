package server

type EventKind int

const (
	EventUnknown EventKind = iota
	EventJoin
	EventInput
	EventPing
	EventPong
)

type InputData struct {
	FrameID int32
	Up      bool
	Down    bool
	Left    bool
	Right   bool
	Bomb    bool
}

type JoinEvent struct {
	PlayerName  string
	CharacterID int32
	RoomID      string // 房间 ID，空字符串表示自动分配到默认房间
}

type InputEvent struct {
	PlayerID int32
	RoomID   string // 房间 ID
	Seq      int32
	Inputs   []InputData
}

type PingEvent struct {
	ClientTime int64
}

type PongEvent struct {
	ClientTime  int64
	ServerTime  int64
	ServerFrame int32
}

type ServerEvent struct {
	Kind  EventKind
	Join  *JoinEvent
	Input *InputEvent
	Ping  *PingEvent
	Pong  *PongEvent
}
