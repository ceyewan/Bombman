package protocol

import (
	"errors"

	gamev1 "bomberman/api/gen/bomberman/v1"

	"google.golang.org/protobuf/proto"
)

// ========== 辅助构造方法 ==========

// NewClientInputPacket 构造输入消息包
func NewClientInputPacket(seq int32, frameId int32, up, down, left, right, bomb bool) (*gamev1.Packet, error) {
	inputs := []*gamev1.InputData{
		{
			FrameId: frameId,
			Up:      up,
			Down:    down,
			Left:    left,
			Right:   right,
			Bomb:    bomb,
		},
	}
	return NewClientInputPacketWithInputs(seq, inputs)
}

// NewClientInputPacketWithInputs 构造批量输入消息包
func NewClientInputPacketWithInputs(seq int32, inputs []*gamev1.InputData) (*gamev1.Packet, error) {
	input := &gamev1.ClientInput{
		Seq:    seq,
		Inputs: inputs,
	}

	payload, err := proto.Marshal(input)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_CLIENT_INPUT,
		Payload: payload,
	}, nil
}

// NewJoinRequestPacket 构造加入请求消息包
func NewJoinRequestPacket(playerName string, characterType gamev1.CharacterType) (*gamev1.Packet, error) {
	req := &gamev1.JoinRequest{
		PlayerName: playerName,
		Character:  characterType,
	}

	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_JOIN_REQUEST,
		Payload: payload,
	}, nil
}

// NewPingPacket 构造心跳消息包
func NewPingPacket(clientTime int64) (*gamev1.Packet, error) {
	ping := &gamev1.Ping{
		ClientTime: clientTime,
	}

	payload, err := proto.Marshal(ping)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_PING,
		Payload: payload,
	}, nil
}

// ========== 服务器消息构造 ==========

// NewJoinResponsePacket 构造加入响应消息包
func NewJoinResponsePacket(success bool, playerId int32, errorMessage string, gameSeed int64, tps int32, sessionToken string) (*gamev1.Packet, error) {
	resp := &gamev1.JoinResponse{
		Success:      success,
		PlayerId:     playerId,
		ErrorMessage: errorMessage,
		GameSeed:     gameSeed,
		Tps:          tps,
		SessionToken: sessionToken,
	}

	payload, err := proto.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_JOIN_RESPONSE,
		Payload: payload,
	}, nil
}

// NewGameStatePacket 构造游戏状态消息包
func NewGameStatePacket(
	frameId int32,
	phase gamev1.GamePhase,
	players []*gamev1.PlayerState,
	bombs []*gamev1.BombState,
	explosions []*gamev1.ExplosionState,
	tileChanges []*gamev1.TileChange,
	lastProcessedSeq map[int32]int32,
) (*gamev1.Packet, error) {
	state := &gamev1.GameState{
		FrameId:            frameId,
		Phase:              phase,
		Players:            players,
		Bombs:              bombs,
		Explosions:         explosions,
		TileChanges:        tileChanges,
		LastProcessedSeq:   lastProcessedSeq,
	}

	payload, err := proto.Marshal(state)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_GAME_STATE,
		Payload: payload,
	}, nil
}

// NewGameEventPacket 构造游戏事件消息包
func NewGameEventPacket(frameId int32, event *gamev1.GameEvent) (*gamev1.Packet, error) {
	event.FrameId = frameId

	payload, err := proto.Marshal(event)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_GAME_EVENT,
		Payload: payload,
	}, nil
}

// NewPongPacket 构造心跳响应消息包
func NewPongPacket(clientTime, serverTime int64, serverFrame int32) (*gamev1.Packet, error) {
	pong := &gamev1.Pong{
		ClientTime:  clientTime,
		ServerTime:  serverTime,
		ServerFrame: serverFrame,
	}

	payload, err := proto.Marshal(pong)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_PONG,
		Payload: payload,
	}, nil
}

// NewReconnectResponsePacket 构造重连响应消息包
func NewReconnectResponsePacket(success bool, errorMessage string, currentState *gamev1.GameState) (*gamev1.Packet, error) {
	resp := &gamev1.ReconnectResponse{
		Success:      success,
		ErrorMessage: errorMessage,
		CurrentState: currentState,
	}

	payload, err := proto.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_RECONNECT_RESPONSE,
		Payload: payload,
	}, nil
}

// ========== 序列化与反序列化 ==========

// MarshalPacket 将 Packet 对象转换为字节切片
func MarshalPacket(pkt *gamev1.Packet) ([]byte, error) {
	return proto.Marshal(pkt)
}

// UnmarshalPacket 将字节切片转换为 Packet 对象
func UnmarshalPacket(data []byte) (*gamev1.Packet, error) {
	pkt := &gamev1.Packet{}
	err := proto.Unmarshal(data, pkt)
	if err != nil {
		return nil, err
	}
	return pkt, nil
}

// ========== 消息解析辅助 ==========

// ParseClientInput 从 Packet 中解析 ClientInput
func ParseClientInput(pkt *gamev1.Packet) (*gamev1.ClientInput, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_CLIENT_INPUT {
		return nil, errors.New("not a client input message")
	}

	input := &gamev1.ClientInput{}
	err := proto.Unmarshal(pkt.Payload, input)
	if err != nil {
		return nil, err
	}
	return input, nil
}

// ParseJoinRequest 从 Packet 中解析 JoinRequest
func ParseJoinRequest(pkt *gamev1.Packet) (*gamev1.JoinRequest, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_JOIN_REQUEST {
		return nil, errors.New("not a join request message")
	}

	req := &gamev1.JoinRequest{}
	err := proto.Unmarshal(pkt.Payload, req)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// ParsePing 从 Packet 中解析 Ping
func ParsePing(pkt *gamev1.Packet) (*gamev1.Ping, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_PING {
		return nil, errors.New("not a ping message")
	}

	ping := &gamev1.Ping{}
	err := proto.Unmarshal(pkt.Payload, ping)
	if err != nil {
		return nil, err
	}
	return ping, nil
}

// ParseGameState 从 Packet 中解析 GameState
func ParseGameState(pkt *gamev1.Packet) (*gamev1.GameState, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_GAME_STATE {
		return nil, errors.New("not a game state message")
	}

	state := &gamev1.GameState{}
	err := proto.Unmarshal(pkt.Payload, state)
	if err != nil {
		return nil, err
	}
	return state, nil
}

// ParseGameEvent 从 Packet 中解析 GameEvent
func ParseGameEvent(pkt *gamev1.Packet) (*gamev1.GameEvent, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_GAME_EVENT {
		return nil, errors.New("not a game event message")
	}

	event := &gamev1.GameEvent{}
	err := proto.Unmarshal(pkt.Payload, event)
	if err != nil {
		return nil, err
	}
	return event, nil
}

// ParseJoinResponse 从 Packet 中解析 JoinResponse
func ParseJoinResponse(pkt *gamev1.Packet) (*gamev1.JoinResponse, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_JOIN_RESPONSE {
		return nil, errors.New("not a join response message")
	}

	resp := &gamev1.JoinResponse{}
	err := proto.Unmarshal(pkt.Payload, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ParsePong 从 Packet 中解析 Pong
func ParsePong(pkt *gamev1.Packet) (*gamev1.Pong, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_PONG {
		return nil, errors.New("not a pong message")
	}

	pong := &gamev1.Pong{}
	err := proto.Unmarshal(pkt.Payload, pong)
	if err != nil {
		return nil, err
	}
	return pong, nil
}

// ParseReconnectRequest 从 Packet 中解析 ReconnectRequest
func ParseReconnectRequest(pkt *gamev1.Packet) (*gamev1.ReconnectRequest, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_RECONNECT_REQUEST {
		return nil, errors.New("not a reconnect request message")
	}

	req := &gamev1.ReconnectRequest{}
	err := proto.Unmarshal(pkt.Payload, req)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// ParseReconnectResponse 从 Packet 中解析 ReconnectResponse
func ParseReconnectResponse(pkt *gamev1.Packet) (*gamev1.ReconnectResponse, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_RECONNECT_RESPONSE {
		return nil, errors.New("not a reconnect response message")
	}

	resp := &gamev1.ReconnectResponse{}
	err := proto.Unmarshal(pkt.Payload, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
