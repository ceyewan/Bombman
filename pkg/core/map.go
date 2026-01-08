package core

import (
	"math/rand"
	"time"
)

// TileType 地图块类型
type TileType int

const (
	TileEmpty TileType = iota
	TileWall           // 不可破坏的墙
	TileBrick          // 可破坏的砖块
	TileDoor           // 门 (隐藏在砖块下，炸开后出现)
)

// GameMap 游戏地图（核心逻辑，不包含渲染）
type GameMap struct {
	Tiles         [][]TileType
	Width         int
	Height        int
	HiddenDoorPos struct{ X, Y int } // 隐藏门的坐标
}

// NewGameMap 创建新地图
func NewGameMap() *GameMap {
	m := &GameMap{
		Tiles: make([][]TileType, MapHeight),
		Width:  MapWidth,
		Height: MapHeight,
	}

	// 使用地图模板
	m.loadMapTemplate()

	return m
}

// loadMapTemplate 加载地图模板 - 有趣的对称设计
func (m *GameMap) loadMapTemplate() {
	rand.Seed(time.Now().UnixNano())

	// 地图模板：W=墙壁, B=砖块, .=空地
	// 设计一个更开阔的地图，边缘也可以活动 (20x15)
	// 每行必须正好20个字符
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

	// 左上角留出玩家出生安全区域（确保无砖块）
	m.Tiles[1][1] = TileEmpty
	m.Tiles[1][2] = TileEmpty
	m.Tiles[1][3] = TileEmpty
	m.Tiles[2][1] = TileEmpty
	m.Tiles[3][1] = TileEmpty

// 随机选择一个砖块放置隐藏门
brickPositions := []struct{ X, Y int }{}
for y := 0; y < MapHeight; y++ {
for x := 0; x < MapWidth; x++ {
if m.Tiles[y][x] == TileBrick {
brickPositions = append(brickPositions, struct{ X, Y int }{X: x, Y: y})
}
}
}

if len(brickPositions) > 0 {
idx := rand.Intn(len(brickPositions))
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
// bombs: 当前地图上的炸弹列表（使用格子坐标）
func (m *GameMap) CanMoveTo(x, y, width, height int, bombGridPositions []struct{ X, Y int }) bool {
	// 内缩1像素检测，防止边界穿模
	margin := PlayerMargin
	x += margin
	y += margin
	width -= margin * 2
	height -= margin * 2

	if width <= 0 || height <= 0 {
		return false
	}

	// 像素边界检查，避免负坐标被截断成 0
	maxX := MapWidth * TileSize
	maxY := MapHeight * TileSize
	if x < 0 || y < 0 || x+width >= maxX || y+height >= maxY {
		return false
	}

	// 检查多个点（四角 + 边中点）
	checkPoints := []struct{ px, py int }{
		// 四个角
		{x, y},
		{x + width, y},
		{x, y + height},
		{x + width, y + height},
		// 上下边的中点
		{x + width/2, y},
		{x + width/2, y + height},
		// 左右边的中点
		{x, y + height/2},
		{x + width, y + height/2},
	}

	for _, point := range checkPoints {
		gridX := point.px / TileSize
		gridY := point.py / TileSize

		// 检查地图障碍物
		tile := m.GetTile(gridX, gridY)
		if tile == TileWall || tile == TileBrick {
			return false
		}

		// 检查炸弹碰撞
		for _, bombPos := range bombGridPositions {
			if gridX == bombPos.X && gridY == bombPos.Y {
				return false
			}
		}
	}

	return true
}

// GridToPlayerXY 格子位置转换为玩家所在的位置
// 地图格子坐标x轴是横向，正方向向右，y轴纵向，正方向向下，0点在左上角
func GridToPlayerXY(gridX, gridY int) (int, int) {
	if gridX < 0 {
		gridX = 0
	}
	if gridX >= MapWidth {
		gridX = MapWidth - 1
	}
	if gridY < 0 {
		gridY = 0
	}
	if gridY >= MapHeight {
		gridY = MapHeight - 1
	}
	// 上下左右预留3像素
	playerSize := TileSize - 6
	offset := (TileSize - playerSize) / 2
	return gridX*TileSize + offset, gridY*TileSize + offset
}

// PlayerXYToGrid 玩家像素位置转换为格子坐标
func PlayerXYToGrid(x, y int) (int, int) {
	return (x + 3) / TileSize, (y + 3) / TileSize
}
