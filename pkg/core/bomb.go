package core

// Bomb 炸弹
type Bomb struct {
	GridX int // 格子坐标X
	GridY int // 格子坐标Y

	// 时间（帧为单位）
	ExplodeAtFrame int32 // 引爆帧号
	PlacedAtFrame  int32 // 放置时的帧号

	// 属性
	ExplosionRange int // 爆炸范围（格子数）
	OwnerID        int // 放置者 ID

	// 状态
	Exploded bool // 是否已爆炸（用于连锁爆炸）
}

// NewBomb 创建新炸弹
func NewBomb(gridX, gridY int, ownerID int, currentFrame int32) *Bomb {
	return &Bomb{
		GridX:          gridX, // 直接存储格子坐标
		GridY:          gridY,
		ExplodeAtFrame: currentFrame + BombFuseFrames,
		PlacedAtFrame:  currentFrame,
		ExplosionRange: BombExplosionRange, // 默认范围
		OwnerID:        ownerID,
		Exploded:       false,
	}
}

// IsExploded 检查炸弹是否已爆炸
func (b *Bomb) IsExploded(frameId int32) bool {
	return frameId >= b.ExplodeAtFrame
}

// Update 每帧更新炸弹，返回是否应该爆炸
func (b *Bomb) Update(frameId int32) bool {
	if b.Exploded {
		return false // 已处理
	}
	return frameId >= b.ExplodeAtFrame
}

// TriggerExplode 触发爆炸（用于连锁爆炸）
func (b *Bomb) TriggerExplode() {
	b.Exploded = true
}

// GetGridPosition 获取炸弹的格子坐标
func (b *Bomb) GetGridPosition() (int, int) {
	return b.GridX, b.GridY
}

// GetDangerLevel 获取危险等级, 越接近爆炸等级越高，0 表示绝对安全
func (b *Bomb) GetDangerLevel(frameId int32) float64 {
	remaining := float64(b.ExplodeAtFrame - frameId)
	if remaining <= 0 {
		return 1.0
	}
	if remaining >= float64(BombFuseFrames) {
		return 0.0
	}
	return 1.0 - remaining/float64(BombFuseFrames)
}

// GetExplosionCells 获取爆炸影响的格子（缓存友好）
func (b *Bomb) GetExplosionCells(gameMap *GameMap) []GridPos {
	cells := make([]GridPos, 0, 1+4*b.ExplosionRange)
	cells = append(cells, GridPos{GridX: b.GridX, GridY: b.GridY}) // 中心点

	directions := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for _, dir := range directions {
		for i := 1; i <= b.ExplosionRange; i++ {
			nx, ny := b.GridX+dir[0]*i, b.GridY+dir[1]*i

			// 边界检查
			if nx < 0 || nx >= MapWidth || ny < 0 || ny >= MapHeight {
				break
			}

			tile := gameMap.GetTile(nx, ny)

			// 墙阻挡
			if tile == TileWall || tile == TileDoor {
				break
			}

			cells = append(cells, GridPos{GridX: nx, GridY: ny})

			// 可破坏砖块阻挡（但砖块本身会被炸）
			if tile == TileBrick {
				break
			}
		}
	}

	return cells
}
