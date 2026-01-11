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
func NewJoinRequestPacket(playerName string, characterType gamev1.CharacterType, roomID string) (*gamev1.Packet, error) {
	req := &gamev1.JoinRequest{
		PlayerName: playerName,
		Character:  characterType,
		RoomId:     roomID,
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

// NewRoomListRequestPacket 构造房间列表请求消息包
func NewRoomListRequestPacket(page, pageSize int32) (*gamev1.Packet, error) {
	req := &gamev1.RoomListRequest{
		Page:     page,
		PageSize: pageSize,
	}

	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_ROOM_LIST_REQUEST,
		Payload: payload,
	}, nil
}

// NewRoomActionPacket 构造房间操作消息包
func NewRoomActionPacket(action *gamev1.RoomAction) (*gamev1.Packet, error) {
	payload, err := proto.Marshal(action)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_ROOM_ACTION,
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
func NewJoinResponsePacket(success bool, playerId int32, errorMessage string, gameSeed int64, tps int32, sessionToken string, roomID string, roomState *gamev1.RoomStateUpdate) (*gamev1.Packet, error) {
	resp := &gamev1.JoinResponse{
		Success:      success,
		PlayerId:     playerId,
		ErrorMessage: errorMessage,
		GameSeed:     gameSeed,
		Tps:          tps,
		SessionToken: sessionToken,
		RoomId:       roomID,
		RoomState:    roomState,
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

// NewRoomListResponsePacket 构造房间列表响应消息包
func NewRoomListResponsePacket(rooms []*gamev1.RoomInfo, total int32) (*gamev1.Packet, error) {
	resp := &gamev1.RoomListResponse{
		Rooms: rooms,
		Total: total,
	}

	payload, err := proto.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_ROOM_LIST_RESPONSE,
		Payload: payload,
	}, nil
}

// NewRoomActionResponsePacket 构造房间操作响应消息包
func NewRoomActionResponsePacket(success bool, errorMessage string, sessionToken string, roomID string) (*gamev1.Packet, error) {
	resp := &gamev1.RoomActionResponse{
		Success:      success,
		ErrorMessage: errorMessage,
		SessionToken: sessionToken,
		RoomId:       roomID,
	}

	payload, err := proto.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_ROOM_ACTION_RESPONSE,
		Payload: payload,
	}, nil
}

// NewRoomStateUpdatePacket 构造房间状态更新消息包
func NewRoomStateUpdatePacket(update *gamev1.RoomStateUpdate) (*gamev1.Packet, error) {
	payload, err := proto.Marshal(update)
	if err != nil {
		return nil, err
	}

	return &gamev1.Packet{
		Type:    gamev1.MessageType_MESSAGE_TYPE_ROOM_STATE_UPDATE,
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
	matchEndFrame int32,
) (*gamev1.Packet, error) {
	state := &gamev1.GameState{
		FrameId:          frameId,
		Phase:            phase,
		Players:          players,
		Bombs:            bombs,
		Explosions:       explosions,
		TileChanges:      tileChanges,
		LastProcessedSeq: lastProcessedSeq,
		MatchEndFrame:    matchEndFrame,
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

// ParseRoomListRequest 从 Packet 中解析 RoomListRequest
func ParseRoomListRequest(pkt *gamev1.Packet) (*gamev1.RoomListRequest, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_ROOM_LIST_REQUEST {
		return nil, errors.New("not a room list request message")
	}

	req := &gamev1.RoomListRequest{}
	err := proto.Unmarshal(pkt.Payload, req)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// ParseRoomAction 从 Packet 中解析 RoomAction
func ParseRoomAction(pkt *gamev1.Packet) (*gamev1.RoomAction, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_ROOM_ACTION {
		return nil, errors.New("not a room action message")
	}

	action := &gamev1.RoomAction{}
	err := proto.Unmarshal(pkt.Payload, action)
	if err != nil {
		return nil, err
	}
	return action, nil
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

// ParseRoomListResponse 从 Packet 中解析 RoomListResponse
func ParseRoomListResponse(pkt *gamev1.Packet) (*gamev1.RoomListResponse, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_ROOM_LIST_RESPONSE {
		return nil, errors.New("not a room list response message")
	}

	resp := &gamev1.RoomListResponse{}
	err := proto.Unmarshal(pkt.Payload, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ParseRoomActionResponse 从 Packet 中解析 RoomActionResponse
func ParseRoomActionResponse(pkt *gamev1.Packet) (*gamev1.RoomActionResponse, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_ROOM_ACTION_RESPONSE {
		return nil, errors.New("not a room action response message")
	}

	resp := &gamev1.RoomActionResponse{}
	err := proto.Unmarshal(pkt.Payload, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ParseRoomStateUpdate 从 Packet 中解析 RoomStateUpdate
func ParseRoomStateUpdate(pkt *gamev1.Packet) (*gamev1.RoomStateUpdate, error) {
	if pkt.Type != gamev1.MessageType_MESSAGE_TYPE_ROOM_STATE_UPDATE {
		return nil, errors.New("not a room state update message")
	}

	update := &gamev1.RoomStateUpdate{}
	err := proto.Unmarshal(pkt.Payload, update)
	if err != nil {
		return nil, err
	}
	return update, nil
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
