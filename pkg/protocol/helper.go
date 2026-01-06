package protocol

import (
	"errors"
	"fmt"

	// 引入生成的 proto 包
	gamev1 "bomberman/api/gen/bomberman/v1"

	"google.golang.org/protobuf/proto"
)

// ========== 辅助构造方法 (让代码更干净) ==========

// NewClientInput 构造输入消息
func NewClientInput(seq int32, up, down, left, right, bomb bool) *gamev1.GamePacket {
	return &gamev1.GamePacket{
		Payload: &gamev1.GamePacket_Input{
			Input: &gamev1.ClientInput{
				Seq:   seq,
				Up:    up,
				Down:  down,
				Left:  left,
				Right: right,
				Bomb:  bomb,
			},
		},
	}
}

// NewServerState 构造状态消息
func NewServerState(frame int32, players []*gamev1.PlayerState, bombs []*gamev1.BombState) *gamev1.GamePacket {
	return &gamev1.GamePacket{
		Payload: &gamev1.GamePacket_State{
			State: &gamev1.ServerState{
				FrameId: frame,
				Players: players,
				Bombs:   bombs,
				// Map 建议单独处理，或者只在变化时填充
			},
		},
	}
}

// ========== 序列化与反序列化封装 ==========

// Marshal 将消息对象转换为字节切片 (用于 KCP 发送)
func Marshal(pkt *gamev1.GamePacket) ([]byte, error) {
	return proto.Marshal(pkt)
}

// Unmarshal 将字节切片转换为消息对象 (用于 KCP 接收)
func Unmarshal(data []byte) (*gamev1.GamePacket, error) {
	pkt := &gamev1.GamePacket{}
	err := proto.Unmarshal(data, pkt)
	if err != nil {
		return nil, err
	}
	return pkt, nil
}

// ========== 地图辅助工具 (处理一维/二维转换) ==========

// FlattenMap 将二维地图转为一维 (用于发送)
func FlattenMap(grid [][]int32) *gamev1.MapState {
	if len(grid) == 0 {
		return &gamev1.MapState{}
	}
	height := len(grid)
	width := len(grid[0])
	tiles := make([]int32, 0, width*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			tiles = append(tiles, grid[y][x])
		}
	}
	return &gamev1.MapState{
		Width:  int32(width),
		Height: int32(height),
		Tiles:  tiles,
	}
}

// InflateMap 将一维地图还原为二维 (用于接收)
func InflateMap(m *gamev1.MapState) ([][]int32, error) {
	if m == nil {
		return nil, errors.New("map state is nil")
	}
	width := int(m.Width)
	height := int(m.Height)
	if len(m.Tiles) != width*height {
		return nil, fmt.Errorf("map data size mismatch: expected %d, got %d", width*height, len(m.Tiles))
	}

	grid := make([][]int32, height)
	for y := 0; y < height; y++ {
		grid[y] = make([]int32, width)
		for x := 0; x < width; x++ {
			grid[y][x] = m.Tiles[y*width+x]
		}
	}
	return grid, nil
}
