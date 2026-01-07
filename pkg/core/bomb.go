package core

// Bomb 炸弹（纯逻辑结构，不包含渲染）
type Bomb struct {
	X                 int // 格子坐标X
	Y                 int // 格子坐标Y
	TimeToExplode     float64
	ExplosionRange    int
	ExplosionDuration float64
}

// NewBomb 创建新炸弹
func NewBomb(gridX, gridY int) *Bomb {
	return &Bomb{
		X:                 gridX * TileSize, // 转换为像素坐标
		Y:                 gridY * TileSize,
		TimeToExplode:     DefaultTimeToBombSeconds,
		ExplosionRange:    DefaultExplosionRange,
		ExplosionDuration: DefaultExplosionDurationSeconds,
	}
}

// IsExploded 检查炸弹是否已爆炸
func (b *Bomb) IsExploded() bool {
	return b.TimeToExplode <= 0
}

// Update 更新炸弹计时
func (b *Bomb) Update(deltaTime float64) {
	b.TimeToExplode -= deltaTime
}

// GetGridPosition 获取炸弹的格子坐标
func (b *Bomb) GetGridPosition() (int, int) {
	return b.X / TileSize, b.Y / TileSize
}

// Explosion 爆炸效果（纯逻辑）
type Explosion struct {
	CenterX  int
	CenterY  int
	Range    int
	Elapsed  float64
	Duration float64
	Cells    []ExplosionCell
}

// ExplosionCell 爆炸影响的格子
type ExplosionCell struct {
	GridX, GridY int
}

// NewExplosion 创建新爆炸
func NewExplosion(centerX, centerY, rangeVal int) *Explosion {
	return &Explosion{
		CenterX:  centerX,
		CenterY:  centerY,
		Range:    rangeVal,
		Elapsed:  0,
		Duration: DefaultExplosionDurationSeconds,
		Cells:    []ExplosionCell{},
	}
}

// IsExpired 检查爆炸是否已结束
func (e *Explosion) IsExpired() bool {
	return e.Elapsed >= e.Duration
}

// Update 更新爆炸计时
func (e *Explosion) Update(deltaTime float64) {
	e.Elapsed += deltaTime
}

// CalculateExplosionCells 计算爆炸影响的所有格子
func (e *Explosion) CalculateExplosionCells(gameMap *GameMap) []ExplosionCell {
	cells := []ExplosionCell{
		{GridX: e.CenterX, GridY: e.CenterY}, // 中心点
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
			nx := e.CenterX + dir.dx*i
			ny := e.CenterY + dir.dy*i

			// 检查边界
			if nx < 0 || nx >= MapWidth || ny < 0 || ny >= MapHeight {
				break
			}

			tile := gameMap.GetTile(nx, ny)
			if tile == TileWall {
				// 墙壁阻挡爆炸
				break
			}

			cells = append(cells, ExplosionCell{GridX: nx, GridY: ny})

			if tile == TileBrick {
				// 炸毁砖块后停止该方向
				break
			}
		}
	}

	return cells
}
