package core

// Bomb 炸弹（纯逻辑结构，不包含渲染）
// 时间单位全部改为帧（整数），确保确定性
type Bomb struct {
	// 位置（格子坐标）
	X int
	Y int

	// 时间（帧为单位）
	FramesUntilExplode int   // 剩余引爆帧数
	PlacedAtFrame      int32 // 放置时的帧号（用于同步校验）

	// 属性
	ExplosionRange int // 爆炸范围（格子数）
	OwnerID        int // 放置者 ID

	// 状态
	Exploded bool // 是否已爆炸（用于连锁爆炸）
}

// GridPos 格子坐标（通用类型）
type GridPos struct {
	X, Y int
}

// NewBomb 创建新炸弹
func NewBomb(gridX, gridY int, ownerID int, currentFrame int32) *Bomb {
	return &Bomb{
		X:                  gridX, // 直接存储格子坐标
		Y:                  gridY,
		FramesUntilExplode: BombFuseFrames,
		PlacedAtFrame:      currentFrame,
		ExplosionRange:     3, // 默认范围
		OwnerID:            ownerID,
		Exploded:           false,
	}
}

// IsExploded 检查炸弹是否已爆炸
func (b *Bomb) IsExploded() bool {
	return b.FramesUntilExplode <= 0
}

// Update 每帧更新炸弹，返回是否应该爆炸
func (b *Bomb) Update() bool {
	if b.Exploded {
		return false // 已处理
	}
	b.FramesUntilExplode--
	return b.FramesUntilExplode <= 0
}

// TriggerExplode 触发爆炸（用于连锁爆炸）
func (b *Bomb) TriggerExplode() {
	b.FramesUntilExplode = 0
	b.Exploded = true
}

// GetGridPosition 获取炸弹的格子坐标
func (b *Bomb) GetGridPosition() (int, int) {
	return b.X, b.Y
}

// GetDangerLevel 获取危险等级（供 AI 使用）
// 返回值 0.0~1.0，越大越危险
func (b *Bomb) GetDangerLevel() float64 {
	switch {
	case b.FramesUntilExplode <= 30: // 0.5秒内
		return 0.95
	case b.FramesUntilExplode <= 60: // 1秒内
		return 0.8
	case b.FramesUntilExplode <= 90: // 1.5秒内
		return 0.5
	case b.FramesUntilExplode <= 120: // 2秒内
		return 0.3
	default:
		return 0.15
	}
}

// GetExplosionCells 获取爆炸影响的格子（缓存友好）
func (b *Bomb) GetExplosionCells(gameMap *GameMap) []GridPos {
	cells := make([]GridPos, 0, 1+4*b.ExplosionRange)
	cells = append(cells, GridPos{X: b.X, Y: b.Y}) // 中心点

	directions := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for _, dir := range directions {
		for i := 1; i <= b.ExplosionRange; i++ {
			nx, ny := b.X+dir[0]*i, b.Y+dir[1]*i

			// 边界检查
			if nx < 0 || nx >= MapWidth || ny < 0 || ny >= MapHeight {
				break
			}

			tile := gameMap.GetTile(nx, ny)

			// 不可破坏墙阻挡
			if tile == TileWall {
				break
			}

			cells = append(cells, GridPos{X: nx, Y: ny})

			// 可破坏砖块阻挡（但砖块本身会被炸）
			if tile == TileBrick {
				break
			}
		}
	}

	return cells
}

// Explosion 爆炸效果（纯逻辑）
// 时间单位改为帧（整数）
type Explosion struct {
	CenterX         int       // 中心格子X
	CenterY         int       // 中心格子Y
	Range           int       // 爆炸范围
	FramesRemaining int       // 剩余持续帧数
	CreatedAtFrame  int32     // 创建时的帧号
	Cells           []GridPos // 影响的格子
	OwnerID         int       // 来源炸弹的所有者

	// 地图变化（用于客户端同步）
	TileChanges []TileChange // 爆炸导致的地图变化
}

// TileChange 地图变化记录
type TileChange struct {
	X, Y   int         // 格子坐标
	OldType TileType    // 变化前的类型
	NewType TileType    // 变化后的类型
}

// ExplosionCell 爆炸影响的格子（向后兼容）
type ExplosionCell struct {
	GridX, GridY int
}

// NewExplosion 创建新爆炸
func NewExplosion(centerX, centerY, rangeVal int, currentFrame int32, ownerID int) *Explosion {
	return &Explosion{
		CenterX:         centerX,
		CenterY:         centerY,
		Range:           rangeVal,
		FramesRemaining: BombExplosionFrames,
		CreatedAtFrame:  currentFrame,
		Cells:           []GridPos{},
		OwnerID:         ownerID,
	}
}

// IsExpired 检查爆炸是否已结束
func (e *Explosion) IsExpired() bool {
	return e.FramesRemaining <= 0
}

// Update 每帧更新爆炸，返回是否应该移除
func (e *Explosion) Update() bool {
	e.FramesRemaining--
	return e.FramesRemaining <= 0
}

// CalculateExplosionCells 计算爆炸影响的所有格子（向后兼容）
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

// ContainsCell 检查爆炸是否包含指定格子
func (e *Explosion) ContainsCell(x, y int) bool {
	for _, cell := range e.Cells {
		if cell.X == x && cell.Y == y {
			return true
		}
	}
	return false
}
