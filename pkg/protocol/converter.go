package protocol

import (
	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
)

// ========== Direction 转换 ==========

// Proto 方向索引：UP=1, DOWN=2, LEFT=3, RIGHT=4
// Core 方向索引：DirUp=1, DirDown=0, DirLeft=2, DirRight=3

// CoreDirectionToProto 将 core.Direction 转换为 gamev1.Direction
func CoreDirectionToProto(dir core.Direction) gamev1.Direction {
	switch dir {
	case core.DirUp:
		return gamev1.Direction_DIRECTION_UP
	case core.DirDown:
		return gamev1.Direction_DIRECTION_DOWN
	case core.DirLeft:
		return gamev1.Direction_DIRECTION_LEFT
	case core.DirRight:
		return gamev1.Direction_DIRECTION_RIGHT
	default:
		return gamev1.Direction_DIRECTION_UNSPECIFIED
	}
}

// ProtoDirectionToCore 将 gamev1.Direction 转换为 core.Direction
func ProtoDirectionToCore(dir gamev1.Direction) core.Direction {
	switch dir {
	case gamev1.Direction_DIRECTION_UP:
		return core.DirUp
	case gamev1.Direction_DIRECTION_DOWN:
		return core.DirDown
	case gamev1.Direction_DIRECTION_LEFT:
		return core.DirLeft
	case gamev1.Direction_DIRECTION_RIGHT:
		return core.DirRight
	default:
		return core.DirDown // 默认向下
	}
}

// ========== CharacterType 转换 ==========

// Proto: WHITE=1, BLACK=2, RED=3, BLUE=4
// Core: White=0, Black=1, Red=2, Blue=3
// 需要 -1 转换

// CoreCharacterTypeToProto 将 core.CharacterType 转换为 gamev1.CharacterType
func CoreCharacterTypeToProto(char core.CharacterType) gamev1.CharacterType {
	// core 从 0 开始，proto 从 1 开始，需要 +1
	return gamev1.CharacterType(char + 1)
}

// ProtoCharacterTypeToCore 将 gamev1.CharacterType 转换为 core.CharacterType
func ProtoCharacterTypeToCore(char gamev1.CharacterType) core.CharacterType {
	// proto 从 1 开始，core 从 0 开始，需要 -1
	// 如果是 UNSPECIFIED (0)，则默认为 White
	if char == gamev1.CharacterType_CHARACTER_TYPE_UNSPECIFIED {
		return core.CharacterWhite
	}
	return core.CharacterType(char - 1)
}

// ========== Player 转换 ==========

// CorePlayerToProto 将 core.Player 转换为 gamev1.PlayerState
func CorePlayerToProto(p *core.Player) *gamev1.PlayerState {
	if p == nil {
		return nil
	}

	return &gamev1.PlayerState{
		Id:        int32(p.ID),
		X:         p.X,
		Y:         p.Y,
		Direction: CoreDirectionToProto(p.Direction),
		IsMoving:  p.IsMoving,
		Dead:      p.Dead,
		Character: CoreCharacterTypeToProto(p.Character),
	}
}

// ProtoPlayerToCore 将 gamev1.PlayerState 转换为 core.Player
// 注意：这不会创建完整的 Player 对象，主要用于同步状态
func ProtoPlayerToCore(p *gamev1.PlayerState) *core.Player {
	if p == nil {
		return nil
	}

	player := core.NewPlayer(int(p.Id), int(p.X), int(p.Y), ProtoCharacterTypeToCore(p.Character))
	player.X = p.X
	player.Y = p.Y
	player.Direction = ProtoDirectionToCore(p.Direction)
	player.IsMoving = p.IsMoving
	player.Dead = p.Dead
	player.NetworkX = p.X
	player.NetworkY = p.Y
	player.LastNetworkX = p.X
	player.LastNetworkY = p.Y
	player.LerpProgress = 1.0
	return player
}

// ========== Bomb 转换 ==========

// CoreBombToProto 将 core.Bomb 转换为 gamev1.BombState
// 使用帧为单位的时间，转换为毫秒传输
func CoreBombToProto(b *core.Bomb) *gamev1.BombState {
	if b == nil {
		return nil
	}

	// 将帧转换为毫秒（用于网络传输）
	timeLeftMs := int32(b.FramesUntilExplode) * 1000 / core.TPS
	if timeLeftMs < 0 {
		timeLeftMs = 0
	}

	return &gamev1.BombState{
		X:              float64(b.X * core.TileSize), // 格子坐标转像素
		Y:              float64(b.Y * core.TileSize),
		TimeLeftMs:     timeLeftMs,
		ExplosionRange: int32(b.ExplosionRange),
	}
}

// ProtoBombToCore 将 gamev1.BombState 转换为 core.Bomb
// 将毫秒转换为帧
func ProtoBombToCore(b *gamev1.BombState) *core.Bomb {
	if b == nil {
		return nil
	}

	// 将毫秒转换为帧
	framesUntilExplode := int(b.TimeLeftMs) * core.TPS / 1000

	// 像素坐标转格子坐标
	gridX := int(b.X) / core.TileSize
	gridY := int(b.Y) / core.TileSize

	return &core.Bomb{
		X:                  gridX,
		Y:                  gridY,
		FramesUntilExplode: framesUntilExplode,
		PlacedAtFrame:      0, // 从 proto 无法获取
		ExplosionRange:     int(b.ExplosionRange),
		OwnerID:            0, // 从 proto 无法获取
		Exploded:           false,
	}
}

// ========== Explosion 转换 ==========

// CoreExplosionToProto 将 core.Explosion 转换为 gamev1.ExplosionState
func CoreExplosionToProto(e *core.Explosion) *gamev1.ExplosionState {
	if e == nil {
		return nil
	}

	cells := make([]*gamev1.ExplosionCell, len(e.Cells))
	for i, cell := range e.Cells {
		cells[i] = &gamev1.ExplosionCell{
			GridX: int32(cell.X),
			GridY: int32(cell.Y),
		}
	}

	// 将帧转换为毫秒
	elapsedMs := (core.BombExplosionFrames - e.FramesRemaining) * 1000 / core.TPS

	return &gamev1.ExplosionState{
		CenterX:   int32(e.CenterX),
		CenterY:   int32(e.CenterY),
		Range:     int32(e.Range),
		ElapsedMs: int64(elapsedMs),
		Cells:     cells,
	}
}

// ProtoExplosionToCore 将 gamev1.ExplosionState 转换为 core.Explosion
func ProtoExplosionToCore(e *gamev1.ExplosionState) *core.Explosion {
	if e == nil {
		return nil
	}

	// 将 GridPos 转换为 ExplosionCell
	cells := make([]core.ExplosionCell, len(e.Cells))
	for i, cell := range e.Cells {
		cells[i] = core.ExplosionCell{
			GridX: int(cell.GridX),
			GridY: int(cell.GridY),
		}
	}

	// 将毫秒转换为帧（对于 Elapsed）
	elapsedFrames := int(e.ElapsedMs) * core.TPS / 1000
	framesRemaining := core.BombExplosionFrames - elapsedFrames
	if framesRemaining < 0 {
		framesRemaining = 0
	}

	// 同时创建 GridPos 列表
	gridPosCells := make([]core.GridPos, len(e.Cells))
	for i, cell := range e.Cells {
		gridPosCells[i] = core.GridPos{X: int(cell.GridX), Y: int(cell.GridY)}
	}

	return &core.Explosion{
		CenterX:         int(e.CenterX),
		CenterY:         int(e.CenterY),
		Range:           int(e.Range),
		FramesRemaining: framesRemaining,
		CreatedAtFrame:  0, // 从 proto 无法获取
		Cells:           gridPosCells,
		OwnerID:         0, // 从 proto 无法获取
	}
}

// ========== Map 转换 ==========

// CoreMapToProto 将 core.GameMap 转换为 gamev1.MapState
func CoreMapToProto(m *core.GameMap) *gamev1.MapState {
	if m == nil {
		return nil
	}

	// 转换 [][]core.TileType -> [][]int32
	grid := make([][]int32, m.Height)
	for y := 0; y < m.Height; y++ {
		grid[y] = make([]int32, m.Width)
		for x := 0; x < m.Width; x++ {
			grid[y][x] = int32(m.Tiles[y][x])
		}
	}

	return FlattenMap(grid)
}

// ProtoMapToCore 将 gamev1.MapState 转换为 core.GameMap
func ProtoMapToCore(m *gamev1.MapState) (*core.GameMap, error) {
	if m == nil {
		return nil, nil
	}

	grid, err := InflateMap(m)
	if err != nil {
		return nil, err
	}

	gameMap := &core.GameMap{
		Width:  int(m.Width),
		Height: int(m.Height),
		Tiles:  make([][]core.TileType, m.Height),
	}

	for y := 0; y < int(m.Height); y++ {
		gameMap.Tiles[y] = make([]core.TileType, m.Width)
		for x := 0; x < int(m.Width); x++ {
			gameMap.Tiles[y][x] = core.TileType(grid[y][x])
		}
	}

	return gameMap, nil
}

// ========== 批量转换辅助函数 ==========

// CorePlayersToProto 批量转换 Player 列表
func CorePlayersToProto(players []*core.Player) []*gamev1.PlayerState {
	if players == nil {
		return nil
	}

	protoPlayers := make([]*gamev1.PlayerState, 0, len(players))
	for _, p := range players {
		if p != nil {
			protoPlayers = append(protoPlayers, CorePlayerToProto(p))
		}
	}
	return protoPlayers
}

// CoreBombsToProto 批量转换 Bomb 列表
func CoreBombsToProto(bombs []*core.Bomb) []*gamev1.BombState {
	if bombs == nil {
		return nil
	}

	protoBombs := make([]*gamev1.BombState, 0, len(bombs))
	for _, b := range bombs {
		if b != nil {
			protoBombs = append(protoBombs, CoreBombToProto(b))
		}
	}
	return protoBombs
}

// CoreExplosionsToProto 批量转换 Explosion 列表
func CoreExplosionsToProto(explosions []*core.Explosion) []*gamev1.ExplosionState {
	if explosions == nil {
		return nil
	}

	protoExplosions := make([]*gamev1.ExplosionState, 0, len(explosions))
	for _, e := range explosions {
		if e != nil {
			protoExplosions = append(protoExplosions, CoreExplosionToProto(e))
		}
	}
	return protoExplosions
}
