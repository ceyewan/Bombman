package protocol

import (
	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
)

// ========== Direction 转换 ==========

// CoreDirectionToProto 将 core.Direction 转换为 gamev1.Direction
// Core: DirDown=0, DirUp=1, DirLeft=2, DirRight=3
// Proto: DIRECTION_DOWN=1, DIRECTION_UP=2, DIRECTION_LEFT=3, DIRECTION_RIGHT=4
func CoreDirectionToProto(dir core.DirectionType) gamev1.Direction {
	switch dir {
	case core.DirDown:
		return gamev1.Direction_DIRECTION_DOWN
	case core.DirUp:
		return gamev1.Direction_DIRECTION_UP
	case core.DirLeft:
		return gamev1.Direction_DIRECTION_LEFT
	case core.DirRight:
		return gamev1.Direction_DIRECTION_RIGHT
	default:
		return gamev1.Direction_DIRECTION_UNSPECIFIED
	}
}

// ProtoDirectionToCore 将 gamev1.Direction 转换为 core.Direction
func ProtoDirectionToCore(dir gamev1.Direction) core.DirectionType {
	switch dir {
	case gamev1.Direction_DIRECTION_DOWN:
		return core.DirDown
	case gamev1.Direction_DIRECTION_UP:
		return core.DirUp
	case gamev1.Direction_DIRECTION_LEFT:
		return core.DirLeft
	case gamev1.Direction_DIRECTION_RIGHT:
		return core.DirRight
	default:
		return core.DirDown // 默认向下
	}
}

// ========== CharacterType 转换 ==========

// CoreCharacterTypeToProto 将 core.CharacterType 转换为 gamev1.CharacterType
// Core: White=0, Black=1, Red=2, Blue=3
// Proto: WHITE=1, BLACK=2, RED=3, BLUE=4
func CoreCharacterTypeToProto(char core.CharacterType) gamev1.CharacterType {
	return gamev1.CharacterType(char + 1)
}

// ProtoCharacterTypeToCore 将 gamev1.CharacterType 转换为 core.CharacterType
func ProtoCharacterTypeToCore(char gamev1.CharacterType) core.CharacterType {
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
		Id:                 int32(p.ID),
		X:                  p.X,
		Y:                  p.Y,
		Direction:          CoreDirectionToProto(p.Direction),
		IsMoving:           p.IsMoving,
		Dead:               p.Dead,
		Character:          CoreCharacterTypeToProto(p.Character),
		NextPlacementFrame: int32(p.NextPlacementFrame),
		CurrentBombs:       0, // 核心中不跟踪当前炸弹数，由 Game 层管理
		MaxBombs:           int32(p.MaxBombs),
	}
}

// ProtoPlayerToCore 将 gamev1.PlayerState 转换为 core.Player
// 注意：这不会创建完整的 Player 对象，主要用于同步状态
func ProtoPlayerToCore(p *gamev1.PlayerState) *core.Player {
	if p == nil {
		return nil
	}

	player := core.NewPlayer(int(p.Id), int(p.X), int(p.Y), ProtoCharacterTypeToCore(p.Character))
	player.X = p.X // 修正浮点位置
	player.Y = p.Y
	player.Direction = ProtoDirectionToCore(p.Direction)
	player.IsMoving = p.IsMoving
	player.Dead = p.Dead
	player.NextPlacementFrame = int(p.NextPlacementFrame)
	player.MaxBombs = int(p.MaxBombs)
	return player
}

// ========== Bomb 转换 ==========

// CoreBombToProto 将 core.Bomb 转换为 gamev1.BombState
// 关键：直接使用帧，不再转换为毫秒！
func CoreBombToProto(b *core.Bomb, id int32) *gamev1.BombState {
	if b == nil {
		return nil
	}

	return &gamev1.BombState{
		Id:             id,
		GridX:          int32(b.GridX),
		GridY:          int32(b.GridY),
		ExplodeAtFrame: b.ExplodeAtFrame,
		ExplosionRange: int32(b.ExplosionRange),
		OwnerId:        int32(b.OwnerID),
		PlacedAtFrame:  b.PlacedAtFrame,
	}
}

// ProtoBombToCore 将 gamev1.BombState 转换为 core.Bomb
// 关键：直接使用帧，不再转换毫秒！
func ProtoBombToCore(b *gamev1.BombState) *core.Bomb {
	if b == nil {
		return nil
	}

	return &core.Bomb{
		GridX:          int(b.GridX),
		GridY:          int(b.GridY),
		ExplodeAtFrame: b.ExplodeAtFrame,
		PlacedAtFrame:  b.PlacedAtFrame,
		ExplosionRange: int(b.ExplosionRange),
		OwnerID:        int(b.OwnerId),
		Exploded:       false,
	}
}

// ========== Explosion 转换 ==========

// CoreExplosionToProto 将 core.Explosion 转换为 gamev1.ExplosionState
// 关键：直接使用帧，不再转换为毫秒！
func CoreExplosionToProto(e *core.Explosion, id int32) *gamev1.ExplosionState {
	if e == nil {
		return nil
	}

	cells := make([]*gamev1.GridCell, len(e.Cells))
	for i, cell := range e.Cells {
		cells[i] = &gamev1.GridCell{
			X: int32(cell.GridX),
			Y: int32(cell.GridY),
		}
	}

	return &gamev1.ExplosionState{
		Id:             id,
		Cells:          cells,
		ExpiresAtFrame: e.ExpiresAtFrame,
		CreatedAtFrame: e.CreatedAtFrame,
	}
}

// ProtoExplosionToCore 将 gamev1.ExplosionState 转换为 core.Explosion
// 关键：直接使用帧，不再转换毫秒！
func ProtoExplosionToCore(e *gamev1.ExplosionState) *core.Explosion {
	if e == nil {
		return nil
	}

	cells := make([]core.GridPos, len(e.Cells))
	for i, cell := range e.Cells {
		cells[i] = core.GridPos{GridX: int(cell.X), GridY: int(cell.Y)}
	}

	return &core.Explosion{
		Cells:          cells,
		ExpiresAtFrame: e.ExpiresAtFrame,
		CreatedAtFrame: e.CreatedAtFrame,
		OwnerID:        0, // 从 proto 无法获取
	}
}

// ========== TileChange 转换 ==========

// CoreTileTypeToProto 将 core.TileType 转换为 gamev1.TileType
func CoreTileTypeToProto(tileType core.TileType) gamev1.TileType {
	switch tileType {
	case core.TileEmpty:
		return gamev1.TileType_TILE_TYPE_EMPTY
	case core.TileWall:
		return gamev1.TileType_TILE_TYPE_WALL
	case core.TileBrick:
		return gamev1.TileType_TILE_TYPE_BRICK
	case core.TileDoor:
		return gamev1.TileType_TILE_TYPE_DOOR
	default:
		return gamev1.TileType_TILE_TYPE_UNSPECIFIED
	}
}

// ProtoTileTypeToCore 将 gamev1.TileType 转换为 core.TileType
func ProtoTileTypeToCore(tileType gamev1.TileType) core.TileType {
	switch tileType {
	case gamev1.TileType_TILE_TYPE_EMPTY:
		return core.TileEmpty
	case gamev1.TileType_TILE_TYPE_WALL:
		return core.TileWall
	case gamev1.TileType_TILE_TYPE_BRICK:
		return core.TileBrick
	case gamev1.TileType_TILE_TYPE_DOOR:
		return core.TileDoor
	default:
		return core.TileEmpty
	}
}

// CoreTileChangeToProto 将 core.TileChange 转换为 gamev1.TileChange
func CoreTileChangeToProto(tc core.TileChange) *gamev1.TileChange {
	return &gamev1.TileChange{
		X:       int32(tc.GridX),
		Y:       int32(tc.GridY),
		NewType: CoreTileTypeToProto(tc.NewType),
	}
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
	for i, b := range bombs {
		if b != nil {
			protoBombs = append(protoBombs, CoreBombToProto(b, int32(i)))
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
	for i, e := range explosions {
		if e != nil {
			protoExplosions = append(protoExplosions, CoreExplosionToProto(e, int32(i)))
		}
	}
	return protoExplosions
}

// CoreTileChangesToProto 批量转换 TileChange 列表
func CoreTileChangesToProto(changes []core.TileChange) []*gamev1.TileChange {
	if changes == nil {
		return nil
	}

	protoChanges := make([]*gamev1.TileChange, 0, len(changes))
	for _, tc := range changes {
		protoChanges = append(protoChanges, CoreTileChangeToProto(tc))
	}
	return protoChanges
}

// ========== GamePhase 转换 ==========

// CoreGameStateToProto 将 server.GameState 转换为 gamev1.GamePhase
func CoreGameStateToProto(state int) gamev1.GamePhase {
	switch state {
	case 0: // StateWaiting
		return gamev1.GamePhase_GAME_PHASE_WAITING
	case 1: // StateCountdown
		return gamev1.GamePhase_GAME_PHASE_COUNTDOWN
	case 2: // StatePlaying
		return gamev1.GamePhase_GAME_PHASE_PLAYING
	case 3: // StateGameOver
		return gamev1.GamePhase_GAME_PHASE_GAME_OVER
	default:
		return gamev1.GamePhase_GAME_PHASE_UNSPECIFIED
	}
}

// ProtoGamePhaseToCore 将 gamev1.GamePhase 转换为 server.GameState
func ProtoGamePhaseToCore(phase gamev1.GamePhase) int {
	switch phase {
	case gamev1.GamePhase_GAME_PHASE_WAITING:
		return 0 // StateWaiting
	case gamev1.GamePhase_GAME_PHASE_COUNTDOWN:
		return 1 // StateCountdown
	case gamev1.GamePhase_GAME_PHASE_PLAYING:
		return 2 // StatePlaying
	case gamev1.GamePhase_GAME_PHASE_GAME_OVER:
		return 3 // StateGameOver
	default:
		return 0 // 默认等待
	}
}
