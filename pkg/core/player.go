package core

import "math"

// Direction 移动方向
type Direction int

const (
	DirDown Direction = iota
	DirUp
	DirLeft
	DirRight
)

// StateSnapshot 状态快照（用于插值缓冲）
type StateSnapshot struct {
	Timestamp int64   // 服务器时间戳（毫秒）
	X, Y      float64 // 位置
	Direction Direction
	IsMoving  bool
}

// Player 玩家（纯逻辑，不包含渲染）
// 时间单位改为帧（整数）
type Player struct {
	ID        int
	X, Y      float64 // 玩家当前位置（渲染位置，保留浮点用于平滑移动）
	Width     int
	Height    int
	Direction Direction
	IsMoving  bool
	Character CharacterType
	Dead      bool

	// 冷却时间（帧为单位）
	BombCooldownFrames int // 炸弹冷却剩余帧数

	// 速度（像素/帧）
	Speed float64

	// 炸弹穿越（刚放置的炸弹可以走出去）
	BombIgnoreGridX  int
	BombIgnoreGridY  int
	BombIgnoreActive bool

	// 炸弹属性
	MaxBombs  int // 最大同时炸弹数
	BombRange int // 炸弹爆炸范围

	// ========== 网络同步相关 ==========

	// 插值缓冲区（远端玩家使用）
	StateBuffer     []StateSnapshot // 状态快照队列
	RenderTimestamp int64           // 当前渲染时间点（服务器时间 - 延迟）

	// 航位推测
	LastVelocityX, LastVelocityY float64 // 最后已知速度（像素/毫秒）
	LastUpdateTimestamp          int64   // 最后更新时间戳

	// 旧字段（保留向后兼容，但将逐步弃用）
	NetworkX, NetworkY         float64 // 服务器同步过来的位置
	LastNetworkX, LastNetworkY float64
	LerpProgress               float64 // 插值进度 (0.0 ~ 1.0)
	LerpSpeed                  float64 // 插值速度
	IsSimulated                bool    // true表示由服务器控制（远端玩家）
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
		BombCooldownFrames: 0,
		BombIgnoreGridX:    0,
		BombIgnoreGridY:    0,
		BombIgnoreActive:   false,
		MaxBombs:           1,
		BombRange:          3,
		// 插值缓冲区
		StateBuffer:         make([]StateSnapshot, 0, InterpolationBufferSize),
		RenderTimestamp:     0,
		LastVelocityX:       0,
		LastVelocityY:       0,
		LastUpdateTimestamp: 0,
		// 旧字段（兼容）
		NetworkX:     float64(x),
		NetworkY:     float64(y),
		LastNetworkX: float64(x),
		LastNetworkY: float64(y),
		LerpProgress: 1.0,
		LerpSpeed:    PlayerSpeedPerFrame,
		IsSimulated:  false,
	}
}

// Update 每帧更新玩家状态
func (p *Player) Update(game *Game) {
	if p.Dead {
		return
	}

	// 更新冷却（帧为单位，无需 deltaTime）
	if p.BombCooldownFrames > 0 {
		p.BombCooldownFrames--
	}
}

// ========== 插值缓冲系统（远端玩家）==========

// AddStateSnapshot 添加状态快照到缓冲区
func (p *Player) AddStateSnapshot(timestamp int64, x, y float64, dir Direction, isMoving bool) {
	snapshot := StateSnapshot{
		Timestamp: timestamp,
		X:         x,
		Y:         y,
		Direction: dir,
		IsMoving:  isMoving,
	}

	// 计算速度（用于航位推测）
	if len(p.StateBuffer) > 0 {
		last := p.StateBuffer[len(p.StateBuffer)-1]
		dt := float64(timestamp - last.Timestamp)
		if dt > 0 {
			p.LastVelocityX = (x - last.X) / dt
			p.LastVelocityY = (y - last.Y) / dt
		}
	}
	p.LastUpdateTimestamp = timestamp

	// 添加到缓冲区
	p.StateBuffer = append(p.StateBuffer, snapshot)

	// 限制缓冲区大小
	if len(p.StateBuffer) > InterpolationBufferSize {
		p.StateBuffer = p.StateBuffer[1:]
	}
}

// UpdateInterpolation 更新插值位置（远端玩家每帧调用）
// serverTimeMs: 当前服务器时间（毫秒）
func (p *Player) UpdateInterpolation(serverTimeMs int64) {
	if !p.IsSimulated || len(p.StateBuffer) == 0 {
		return
	}

	// 渲染时间 = 服务器时间 - 插值延迟
	renderTime := serverTimeMs - InterpolationDelayMs
	p.RenderTimestamp = renderTime

	// 在缓冲区中找到 renderTime 两侧的快照
	var prev, next *StateSnapshot
	for i := 0; i < len(p.StateBuffer)-1; i++ {
		if p.StateBuffer[i].Timestamp <= renderTime && p.StateBuffer[i+1].Timestamp >= renderTime {
			prev = &p.StateBuffer[i]
			next = &p.StateBuffer[i+1]
			break
		}
	}

	if prev != nil && next != nil {
		// 正常插值
		totalTime := float64(next.Timestamp - prev.Timestamp)
		if totalTime > 0 {
			alpha := float64(renderTime-prev.Timestamp) / totalTime
			p.X = prev.X + (next.X-prev.X)*alpha
			p.Y = prev.Y + (next.Y-prev.Y)*alpha
			p.Direction = next.Direction
			p.IsMoving = next.IsMoving
		}
	} else if len(p.StateBuffer) > 0 {
		// 缓冲区不足或渲染时间超出范围，使用航位推测
		last := p.StateBuffer[len(p.StateBuffer)-1]
		timeSinceLast := serverTimeMs - last.Timestamp

		if timeSinceLast <= DeadReckoningMaxMs {
			// 航位推测：基于最后速度预测位置
			p.X = last.X + p.LastVelocityX*float64(timeSinceLast)
			p.Y = last.Y + p.LastVelocityY*float64(timeSinceLast)
			p.Direction = last.Direction
			p.IsMoving = last.IsMoving
		} else {
			// 超时，停止在最后已知位置
			p.X = last.X
			p.Y = last.Y
			p.Direction = last.Direction
			p.IsMoving = false
		}
	}

	// 清理过期快照（保留 renderTime 之前的最后一个）
	p.cleanupOldSnapshots(renderTime)
}

// cleanupOldSnapshots 清理过期的快照
func (p *Player) cleanupOldSnapshots(renderTime int64) {
	// 找到最后一个 <= renderTime 的快照索引
	cutoff := -1
	for i := 0; i < len(p.StateBuffer); i++ {
		if p.StateBuffer[i].Timestamp <= renderTime {
			cutoff = i
		} else {
			break
		}
	}

	// 保留 cutoff 及之后的快照（cutoff 用于插值的 prev）
	if cutoff > 0 {
		p.StateBuffer = p.StateBuffer[cutoff:]
	}
}

// ========== 旧版插值（兼容，将逐步弃用）==========

// updateLerp 更新插值位置（旧版本，保留兼容）
func (p *Player) updateLerp(deltaTime float64) {
	if p.LerpProgress < 1.0 {
		p.LerpProgress += deltaTime * p.LerpSpeed
		if p.LerpProgress > 1.0 {
			p.LerpProgress = 1.0
		}

		// 线性插值
		t := p.LerpProgress
		p.X = p.LastNetworkX + (p.NetworkX-p.LastNetworkX)*t
		p.Y = p.LastNetworkY + (p.NetworkY-p.LastNetworkY)*t

		p.IsMoving = true
	} else {
		p.X = p.NetworkX
		p.Y = p.NetworkY
		p.IsMoving = false
	}
}

// SetNetworkPosition 设置服务器同步的位置（旧版本，保留兼容）
func (p *Player) SetNetworkPosition(x, y float64) {
	p.LastNetworkX = p.NetworkX
	p.LastNetworkY = p.NetworkY
	p.NetworkX = x
	p.NetworkY = y
	p.LerpProgress = 0.0
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

	// 检查碰撞
	bombPositions := getBombGridPositions(game.Bombs, p.BombIgnoreActive, p.BombIgnoreGridX, p.BombIgnoreGridY)
	if !game.Map.CanMoveTo(int(newX), int(newY), p.Width, p.Height, bombPositions) {
		correctedX, correctedY, ok := p.tryCornerCorrection(dx, dy, game, bombPositions)
		if !ok {
			return false
		}
		newX = correctedX
		newY = correctedY
	}

	p.X = newX
	p.Y = newY
	p.IsMoving = (dx != 0 || dy != 0)

	p.applySoftAlign(dx, dy, game, bombPositions)

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

// PlaceBomb 放置炸弹（返回是否成功）
func (p *Player) PlaceBomb(game *Game, currentFrame int32) *Bomb {
	if p.Dead {
		return nil
	}

	// 检查冷却时间（帧为单位）
	if p.BombCooldownFrames > 0 {
		return nil
	}

	// 获取玩家所在格子
	centerX := int(p.X + float64(p.Width)/2)
	centerY := int(p.Y + float64(p.Height)/2)
	gridX := centerX / TileSize
	gridY := centerY / TileSize

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

	p.BombCooldownFrames = BombCooldownFrames
	p.BombIgnoreGridX = gridX
	p.BombIgnoreGridY = gridY
	p.BombIgnoreActive = true
	bomb := NewBomb(gridX, gridY, p.ID, currentFrame)
	bomb.ExplosionRange = p.BombRange
	return bomb
}

// GetGridPosition 获取玩家所在格子
func (p *Player) GetGridPosition() (int, int) {
	return int(p.X) / TileSize, int(p.Y) / TileSize
}

// CanPlaceBomb 检查是否可以放置炸弹
func (p *Player) CanPlaceBomb() bool {
	return !p.Dead && p.BombCooldownFrames <= 0
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

func (p *Player) tryCornerCorrection(dx, dy float64, game *Game, bombPositions []struct{ X, Y int }) (float64, float64, bool) {
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
		if game.Map.CanMoveTo(int(newX), int(newY), p.Width, p.Height, bombPositions) {
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
	if game.Map.CanMoveTo(int(newX), int(newY), p.Width, p.Height, bombPositions) {
		return newX, newY, true
	}
	return 0, 0, false
}

func (p *Player) applySoftAlign(dx, dy float64, game *Game, bombPositions []struct{ X, Y int }) {
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
		if game.Map.CanMoveTo(int(p.X), int(newY), p.Width, p.Height, bombPositions) {
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
	if game.Map.CanMoveTo(int(newX), int(p.Y), p.Width, p.Height, bombPositions) {
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
