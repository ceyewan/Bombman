package core

import (
	"math/rand"
)

// GameMap 游戏地图（核心逻辑，不包含渲染）
type GameMap struct {
	Tiles         [][]TileType
	Width         int
	Height        int
	HiddenDoorPos struct{ X, Y int } // 隐藏门的坐标
}

// GridPos 格子坐标（通用类型）
type GridPos struct {
	GridX, GridY int
}

// NewGameMapWithSeed 使用指定种子创建新地图（用于确定性）
func NewGameMap(seed int64) *GameMap {
	m := &GameMap{
		Tiles:  make([][]TileType, MapHeight),
		Width:  MapWidth,
		Height: MapHeight,
	}

	// 使用地图模板（带种子）
	m.loadMapTemplateWithSeed(int64(seed))

	return m
}

// loadMapTemplateWithSeed 加载地图模板（带种子，用于确定性）
func (m *GameMap) loadMapTemplateWithSeed(seed int64) {
	// 地图模板：W=墙壁, B=砖块, .=空地
	template := []string{
		"..B.B.W.W.W.W.B.B...",
		"..W.W.B...B...W.W...",
		".W.W.W.W.W.W.W.W.W..",
		"B..B..BBB.BBB..B..B.",
		".W.W.WBW.W.WBW.W.W..",
		"B....B...B...B....B.",
		".WBWBW.W.W.W.WBWBW..",
		"W.B..B...B...B..B.WW",
		".WBWBW.W.W.W.WBWBW..",
		"B....B...B...B....B.",
		".W.W.WBW.W.WBW.W.W..",
		"B..B..BBB.BBB..B..B.",
		".W.W.W.W.W.W.W.W.W..",
		"..W.W.B...B...W.W...",
		"..B.B.W.W.W.W.B.B...",
	}

	// 解析模板
	for y := 0; y < MapHeight; y++ {
		m.Tiles[y] = make([]TileType, MapWidth)
		for x := 0; x < MapWidth; x++ {
			switch template[y][x] {
			case 'W':
				m.Tiles[y][x] = TileWall
			case 'B':
				m.Tiles[y][x] = TileBrick
			case '.':
				m.Tiles[y][x] = TileEmpty
			}
		}
	}

	// 随机选择一个砖块放置隐藏门
	brickPositions := []struct{ X, Y int }{}
	for y := 0; y < MapHeight; y++ {
		for x := 0; x < MapWidth; x++ {
			if m.Tiles[y][x] == TileBrick {
				brickPositions = append(brickPositions, struct{ X, Y int }{X: x, Y: y})
			}
		}
	}

	r := rand.New(rand.NewSource(seed))
	if len(brickPositions) > 0 {
		idx := r.Intn(len(brickPositions))
		m.HiddenDoorPos = brickPositions[idx]
	}
}

// GetTile 获取指定位置的地图块
func (m *GameMap) GetTile(x, y int) TileType {
	if x < 0 || x >= MapWidth || y < 0 || y >= MapHeight {
		return TileWall
	}
	return m.Tiles[y][x]
}

// SetTile 设置指定位置的地图块
func (m *GameMap) SetTile(x, y int, tile TileType) {
	if x >= 0 && x < MapWidth && y >= 0 && y < MapHeight {
		m.Tiles[y][x] = tile
	}
}

// CanMoveTo 检查是否可以移动到指定像素位置
// x, y: 目标位置左上角坐标
// width, height: 目标宽高
// bombGridPositions: 当前地图上所有炸弹的格子位置
// explosionCells: 当前地图上所有爆炸影响的格子位置
func (m *GameMap) CanMoveTo(x, y, width, height int, bombGridPositions []struct{ X, Y int }, explosionCells []GridPos) bool {
	// 玩家碰撞盒（Hitbox），稍微内缩以避免边缘穿模
	hitboxX := x + PlayerMargin
	hitboxY := y + PlayerMargin
	hitboxW := PlayerWidth - PlayerMargin*2
	hitboxHeight := PlayerHeight - PlayerMargin*2

	// 边界检查
	if hitboxX < 0 || hitboxY < 0 || hitboxX+hitboxW > MapWidth*TileSize || hitboxY+hitboxHeight > MapHeight*TileSize {
		return false
	}

	// 计算碰撞盒覆盖的格子范围
	startGridX := hitboxX / TileSize
	endGridX := (hitboxX + hitboxW - 1) / TileSize
	startGridY := hitboxY / TileSize
	endGridY := (hitboxY + hitboxHeight - 1) / TileSize

	for gy := startGridY; gy <= endGridY; gy++ {
		for gx := startGridX; gx <= endGridX; gx++ {
			tile := m.GetTile(gx, gy)
			if tile == TileWall || tile == TileBrick {
				return false // 碰撞墙或砖块
			}

			// 检查炸弹碰撞
			for _, bPos := range bombGridPositions {
				if bPos.X == gx && bPos.Y == gy {
					// 忽略玩家自己放置的炸弹
					if gx == PlayerXYToGrid(x, y).GridX && gy == PlayerXYToGrid(x, y).GridY {
						continue
					}
					return false // 碰撞炸弹
				}
			}

			// 检查爆炸碰撞
			for _, eCell := range explosionCells {
				if eCell.GridX == gx && eCell.GridY == gy {
					return false // 碰撞爆炸区域
				}
			}
		}
	}
	return true
}

// GridToPlayerXY 格子位置转换为玩家所在的位置，居中放置
// 地图格子坐标x轴是横向，正方向向右，y轴纵向，正方向向下，0点在左上角
func GridToPlayerXY(gridX, gridY int) (int, int) {
	return gridX*TileSize + (TileSize-PlayerWidth)/2, gridY*TileSize + (TileSize-PlayerHeight)/2
}

// PlayerXYToGrid 玩家像素位置转换为格子坐标
func PlayerXYToGrid(x, y int) GridPos {
	return GridPos{
		GridX: (x + PlayerWidth/2) / TileSize,
		GridY: (y + PlayerHeight/2) / TileSize,
	}
}
