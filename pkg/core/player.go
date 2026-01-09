package core

import "math"

// Player 玩家（纯逻辑，不包含渲染）
// 时间单位改为帧（整数）
type Player struct {
	ID        int           // 玩家ID
	X, Y      float64       // 玩家当前位置（左上角）
	Width     int           // 玩家宽度
	Height    int           // 玩家高度
	Direction DirectionType // 朝向
	IsMoving  bool          // 是否在移动
	Character CharacterType // 角色类型
	Dead      bool          // 是否死亡

	NextPlacementFrame int32 // 下一次可放置炸弹的帧号

	Speed float64 // 移动速度（像素/帧）

	BombIgnoreGridX  int  // 放置炸弹时忽略碰撞的格子X
	BombIgnoreGridY  int  // 放置炸弹时忽略碰撞的格子Y
	BombIgnoreActive bool // 是否忽略碰撞（放置炸弹后直到离开该格子）

	MaxBombs  int // 最大同时炸弹数
	BombRange int // 炸弹爆炸范围
}

// NewPlayer 创建新玩家
func NewPlayer(id int, x, y int, charType CharacterType) *Player {
	return &Player{
		ID:                 id,
		X:                  float64(x),
		Y:                  float64(y),
		Width:              PlayerWidth,
		Height:             PlayerHeight,
		Speed:              PlayerSpeedPerFrame, // 像素/帧
		Direction:          DirDown,
		IsMoving:           false,
		Character:          charType,
		Dead:               false,
		NextPlacementFrame: 0,
		BombIgnoreGridX:    0,
		BombIgnoreGridY:    0,
		BombIgnoreActive:   false,
		MaxBombs:           1,
		BombRange:          3,
	}
}

// Update 每帧更新玩家状态
func (p *Player) Update(game *Game) {
	if p.Dead {
		return
	}
}

// Move 移动玩家（返回是否成功移动）
// 参数 dx, dy 是像素/帧的移动距离
func (p *Player) Move(dx, dy float64, game *Game) bool {
	if p.Dead {
		return false
	}

	if p.BombIgnoreActive && !p.overlapsGrid(p.BombIgnoreGridX, p.BombIgnoreGridY) {
		p.BombIgnoreActive = false
	}

	newX := p.X + dx
	newY := p.Y + dy

	// 收集爆炸格子（用于碰撞检测）
	explosionCells := collectExplosionCells(game.Explosions)

	// 检查碰撞
	bombPositions := getBombGridPositions(game.Bombs, p.BombIgnoreActive, p.BombIgnoreGridX, p.BombIgnoreGridY)
	if !game.Map.CanMoveTo(int(newX), int(newY), p.Width, p.Height, bombPositions, explosionCells) {
		correctedX, correctedY, ok := p.tryCornerCorrection(dx, dy, game, bombPositions, explosionCells)
		if !ok {
			return false
		}
		newX = correctedX
		newY = correctedY
	}

	p.X = newX
	p.Y = newY
	p.IsMoving = (dx != 0 || dy != 0)

	p.applySoftAlign(dx, dy, game, bombPositions, explosionCells)

	// 更新方向
	if dx > 0 {
		p.Direction = DirRight
	} else if dx < 0 {
		p.Direction = DirLeft
	} else if dy > 0 {
		p.Direction = DirDown
	} else if dy < 0 {
		p.Direction = DirUp
	}

	if p.BombIgnoreActive && !p.overlapsGrid(p.BombIgnoreGridX, p.BombIgnoreGridY) {
		p.BombIgnoreActive = false
	}

	return true
}

// collectExplosionCells 收集所有爆炸影响的格子
func collectExplosionCells(explosions []*Explosion) []GridPos {
	cells := make([]GridPos, 0)
	for _, exp := range explosions {
		cells = append(cells, exp.Cells...)
	}
	return cells
}

// PlaceBomb 放置炸弹（返回是否成功）
func (p *Player) PlaceBomb(game *Game, currentFrame int32) *Bomb {
	if p.Dead {
		return nil
	}

	// 检查放置防抖（帧为单位）
	if p.NextPlacementFrame > currentFrame {
		return nil
	}

	// 检查当前活跃炸弹数量
	activeBombs := 0
	for _, bomb := range game.Bombs {
		if bomb.OwnerID == p.ID && !bomb.Exploded {
			activeBombs++
		}
	}
	if activeBombs >= p.MaxBombs {
		return nil
	}

	// 获取玩家所在格子
	gridX, gridY := p.GetGridPosition()

	// 只能在空地放置炸弹
	if game.Map.GetTile(gridX, gridY) != TileEmpty {
		return nil
	}

	// 检查该格子是否已有炸弹
	for _, bomb := range game.Bombs {
		bombGridX, bombGridY := bomb.GetGridPosition()
		if bombGridX == gridX && bombGridY == gridY {
			return nil // 已有炸弹
		}
	}

	p.NextPlacementFrame = currentFrame + BombPlacementDelayFrames
	p.BombIgnoreGridX = gridX
	p.BombIgnoreGridY = gridY
	p.BombIgnoreActive = true
	bomb := NewBomb(gridX, gridY, p.ID, currentFrame)
	bomb.ExplosionRange = p.BombRange
	return bomb
}

// GetGridPosition 获取玩家所在格子
func (p *Player) GetGridPosition() (int, int) {
	gridPos := PlayerXYToGrid(int(p.X), int(p.Y))
	return gridPos.GridX, gridPos.GridY
}

// 辅助函数：获取炸弹的格子坐标列表
func getBombGridPositions(bombs []*Bomb, ignoreActive bool, ignoreX, ignoreY int) []struct{ X, Y int } {
	positions := make([]struct{ X, Y int }, 0, len(bombs))
	for _, bomb := range bombs {
		x, y := bomb.GetGridPosition()
		if ignoreActive && x == ignoreX && y == ignoreY {
			continue
		}
		positions = append(positions, struct{ X, Y int }{X: x, Y: y})
	}
	return positions
}

func (p *Player) overlapsGrid(gridX, gridY int) bool {
	margin := PlayerMargin
	width := p.Width - margin*2
	height := p.Height - margin*2
	if width <= 0 || height <= 0 {
		return false
	}

	px := p.X + float64(margin)
	py := p.Y + float64(margin)
	pw := float64(width)
	ph := float64(height)

	tileX := float64(gridX * TileSize)
	tileY := float64(gridY * TileSize)
	tileSize := float64(TileSize)

	return px < tileX+tileSize && px+pw > tileX && py < tileY+tileSize && py+ph > tileY
}

func (p *Player) tryCornerCorrection(dx, dy float64, game *Game, bombPositions []struct{ X, Y int }, explosionCells []GridPos) (float64, float64, bool) {
	if dx == 0 && dy == 0 {
		return 0, 0, false
	}
	if dx != 0 && dy != 0 {
		return 0, 0, false
	}

	if dx != 0 {
		targetY := p.nearestAlignedY()
		offset := targetY - p.Y
		if math.Abs(offset) > CornerCorrectionTolerance {
			return 0, 0, false
		}
		step := math.Min(math.Abs(offset), math.Abs(dx))
		if step == 0 {
			return 0, 0, false
		}
		if offset < 0 {
			step = -step
		}
		newY := p.Y + step
		newX := p.X + dx
		if game.Map.CanMoveTo(int(newX), int(newY), p.Width, p.Height, bombPositions, explosionCells) {
			return newX, newY, true
		}
		return 0, 0, false
	}

	targetX := p.nearestAlignedX()
	offset := targetX - p.X
	if math.Abs(offset) > CornerCorrectionTolerance {
		return 0, 0, false
	}
	step := math.Min(math.Abs(offset), math.Abs(dy))
	if step == 0 {
		return 0, 0, false
	}
	if offset < 0 {
		step = -step
	}
	newX := p.X + step
	newY := p.Y + dy
	if game.Map.CanMoveTo(int(newX), int(newY), p.Width, p.Height, bombPositions, explosionCells) {
		return newX, newY, true
	}
	return 0, 0, false
}

func (p *Player) applySoftAlign(dx, dy float64, game *Game, bombPositions []struct{ X, Y int }, explosionCells []GridPos) {
	if dx == 0 && dy == 0 {
		return
	}
	if dx != 0 && dy != 0 {
		return
	}

	if dx != 0 {
		targetY := p.nearestAlignedY()
		offset := targetY - p.Y
		if math.Abs(offset) > CornerCorrectionTolerance {
			return
		}
		step := math.Min(math.Abs(offset), math.Abs(dx)*SoftAlignFactor)
		if step == 0 {
			return
		}
		if offset < 0 {
			step = -step
		}
		newY := p.Y + step
		if game.Map.CanMoveTo(int(p.X), int(newY), p.Width, p.Height, bombPositions, explosionCells) {
			p.Y = newY
		}
		return
	}

	targetX := p.nearestAlignedX()
	offset := targetX - p.X
	if math.Abs(offset) > CornerCorrectionTolerance {
		return
	}
	step := math.Min(math.Abs(offset), math.Abs(dy)*SoftAlignFactor)
	if step == 0 {
		return
	}
	if offset < 0 {
		step = -step
	}
	newX := p.X + step
	if game.Map.CanMoveTo(int(newX), int(p.Y), p.Width, p.Height, bombPositions, explosionCells) {
		p.X = newX
	}
}

func (p *Player) nearestAlignedX() float64 {
	centerX := p.X + float64(p.Width)/2
	gridX := int(math.Floor(centerX/float64(TileSize) + 0.5))
	if gridX < 0 {
		gridX = 0
	} else if gridX >= MapWidth {
		gridX = MapWidth - 1
	}
	offset := float64(TileSize-p.Width) / 2
	return float64(gridX*TileSize) + offset
}

func (p *Player) nearestAlignedY() float64 {
	centerY := p.Y + float64(p.Height)/2
	gridY := int(math.Floor(centerY/float64(TileSize) + 0.5))
	if gridY < 0 {
		gridY = 0
	} else if gridY >= MapHeight {
		gridY = MapHeight - 1
	}
	offset := float64(TileSize-p.Height) / 2
	return float64(gridY*TileSize) + offset
}
