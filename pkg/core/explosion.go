package core

// Explosion 爆炸效果
type Explosion struct {
	GridX          int       // 中心格子X
	GridY          int       // 中心格子Y
	Range          int       // 爆炸范围
	ExpiresAtFrame int32     // 结束帧号
	CreatedAtFrame int32     // 创建帧号
	Cells          []GridPos // 影响的格子
	OwnerID        int       // 来源炸弹的所有者
	// 地图变化（用于客户端同步）
	TileChanges []TileChange // 爆炸导致的地图变化
}

// TileChange 地图变化记录
type TileChange struct {
	GridX, GridY int      // 格子坐标
	OldType      TileType // 变化前的类型
	NewType      TileType // 变化后的类型
}

// NewExplosion 创建新爆炸
func NewExplosion(bomb *Bomb, currentFrame int32) *Explosion {
	return &Explosion{
		GridX:          bomb.GridX,
		GridY:          bomb.GridY,
		Range:          bomb.ExplosionRange,
		ExpiresAtFrame: currentFrame + BombExplosionFrames,
		CreatedAtFrame: currentFrame,
		Cells:          []GridPos{},
		OwnerID:        bomb.OwnerID,
	}
}

// IsExpired 检查爆炸是否已结束
func (e *Explosion) IsExpired(frameId int32) bool {
	return frameId >= e.ExpiresAtFrame
}

// Update 每帧更新爆炸，返回是否应该移除
func (e *Explosion) Update(frameId int32) bool {
	return frameId >= e.ExpiresAtFrame
}

// CalculateExplosionCells 计算爆炸影响的所有格子
func (e *Explosion) CalculateExplosionCells(gameMap *GameMap) []GridPos {
	cells := []GridPos{
		{GridX: e.GridX, GridY: e.GridY}, // 中心点
	}

	// 四个方向扩散
	directions := []struct{ dx, dy int }{
		{0, -1}, // 上
		{0, 1},  // 下
		{-1, 0}, // 左
		{1, 0},  // 右
	}

	for _, dir := range directions {
		for i := 1; i <= e.Range; i++ {
			nx := e.GridX + dir.dx*i
			ny := e.GridY + dir.dy*i

			// 检查边界
			if nx < 0 || nx >= MapWidth || ny < 0 || ny >= MapHeight {
				break
			}

			tile := gameMap.GetTile(nx, ny)
			if tile == TileWall || tile == TileDoor {
				// 墙壁阻挡爆炸
				break
			}

			cells = append(cells, GridPos{GridX: nx, GridY: ny})

			if tile == TileBrick {
				// 炸毁砖块后停止该方向
				break
			}
		}
	}

	return cells
}

// ContainsCell 检查爆炸是否包含指定格子
func (e *Explosion) ContainsCell(x, y int) bool {
	for _, cell := range e.Cells {
		if cell.GridX == x && cell.GridY == y {
			return true
		}
	}
	return false
}
