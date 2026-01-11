package server

import (
	"fmt"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/protocol"
)

// DecodePacket 解析服务器收到的数据包
func DecodePacket(data []byte) (*ServerEvent, error) {
	pkt, err := protocol.UnmarshalPacket(data)
	if err != nil {
		return nil, fmt.Errorf("解析包失败: %w", err)
	}

	switch pkt.Type {
	case gamev1.MessageType_MESSAGE_TYPE_JOIN_REQUEST:
		req, err := protocol.ParseJoinRequest(pkt)
		if err != nil {
			return nil, err
		}
		return &ServerEvent{
			Kind: EventJoin,
			Join: &JoinEvent{
				PlayerName: req.PlayerName,
				Character:  req.Character,
				RoomID:     req.RoomId,
			},
		}, nil

	case gamev1.MessageType_MESSAGE_TYPE_CLIENT_INPUT:
		input, err := protocol.ParseClientInput(pkt)
		if err != nil {
			return nil, err
		}
		items := make([]InputData, 0, len(input.Inputs))
		for _, in := range input.GetInputs() {
			items = append(items, InputData{
				FrameID: in.FrameId,
				Up:      in.Up,
				Down:    in.Down,
				Left:    in.Left,
				Right:   in.Right,
				Bomb:    in.Bomb,
			})
		}
		return &ServerEvent{
			Kind: EventInput,
			Input: &InputEvent{
				Seq:    input.Seq,
				Inputs: items,
			},
		}, nil

	case gamev1.MessageType_MESSAGE_TYPE_PING:
		ping, err := protocol.ParsePing(pkt)
		if err != nil {
			return nil, err
		}
		return &ServerEvent{
			Kind: EventPing,
			Ping: &PingEvent{ClientTime: ping.ClientTime},
		}, nil

	case gamev1.MessageType_MESSAGE_TYPE_PONG:
		pong, err := protocol.ParsePong(pkt)
		if err != nil {
			return nil, err
		}
		return &ServerEvent{
			Kind: EventPong,
			Pong: &PongEvent{ClientTime: pong.ClientTime, ServerTime: pong.ServerTime, ServerFrame: pong.ServerFrame},
		}, nil

	case gamev1.MessageType_MESSAGE_TYPE_RECONNECT_REQUEST:
		req, err := protocol.ParseReconnectRequest(pkt)
		if err != nil {
			return nil, err
		}
		return &ServerEvent{
			Kind:      EventReconnect,
			Reconnect: &ReconnectEvent{SessionToken: req.SessionToken},
		}, nil

	case gamev1.MessageType_MESSAGE_TYPE_ROOM_LIST_REQUEST:
		req, err := protocol.ParseRoomListRequest(pkt)
		if err != nil {
			return nil, err
		}
		return &ServerEvent{
			Kind: EventRoomList,
			RoomList: &RoomListEvent{
				Page:     req.Page,
				PageSize: req.PageSize,
			},
		}, nil

	case gamev1.MessageType_MESSAGE_TYPE_ROOM_ACTION:
		action, err := protocol.ParseRoomAction(pkt)
		if err != nil {
			return nil, err
		}
		return &ServerEvent{
			Kind: EventRoomAction,
			RoomAction: &RoomActionEvent{
				Action: action,
			},
		}, nil

	default:
		return &ServerEvent{Kind: EventUnknown}, nil
	}
}
