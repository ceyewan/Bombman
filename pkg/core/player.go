package core

// Direction 移动方向
type Direction int

const (
	DirDown Direction = iota
	DirUp
	DirLeft
	DirRight
)

// Player 玩家（纯逻辑，不包含渲染）
type Player struct {
	ID                    int
	X, Y                  float64 // 玩家当前位置
	Width                 int
	Height                int
	Speed                 float64 // 像素/秒
	Direction             Direction
	IsMoving              bool
	Character             CharacterType
	Dead                  bool
	BombCooldownSeconds   float64
	BombCooldownRemaining float64
	BombIgnoreGridX       int
	BombIgnoreGridY       int
	BombIgnoreActive      bool

	// 服务器同步相关（网络版本使用）
	NetworkX, NetworkY         float64 // 服务器同步过来的位置
	LastNetworkX, LastNetworkY float64
	LerpProgress               float64 // 插值进度 (0.0 ~ 1.0)
	LerpSpeed                  float64 // 插值速度
	IsSimulated                bool    // true表示由服务器控制
}

// NewPlayer 创建新玩家
func NewPlayer(id int, x, y int, charType CharacterType) *Player {
	return &Player{
		ID:                    id,
		X:                     float64(x),
		Y:                     float64(y),
		Width:                 PlayerWidth,
		Height:                PlayerHeight,
		Speed:                 PlayerDefaultSpeed,
		Direction:             DirDown,
		IsMoving:              false,
		Character:             charType,
		Dead:                  false,
		BombCooldownSeconds:   BombCooldownSeconds,
		BombCooldownRemaining: 0,
		BombIgnoreGridX:       0,
		BombIgnoreGridY:       0,
		BombIgnoreActive:      false,
		NetworkX:              float64(x),
		NetworkY:              float64(y),
		LastNetworkX:          float64(x),
		LastNetworkY:          float64(y),
		LerpProgress:          1.0,
		LerpSpeed:             PlayerDefaultSpeed,
		IsSimulated:           false,
	}
}

// Update 更新玩家状态
func (p *Player) Update(deltaTime float64, game *Game) {
	if p.Dead {
		return
	}

	if p.BombCooldownRemaining > 0 {
		p.BombCooldownRemaining -= deltaTime
		if p.BombCooldownRemaining < 0 {
			p.BombCooldownRemaining = 0
		}
	}

	if p.IsSimulated {
		// 模拟玩家（服务器控制），使用插值
		p.updateLerp(deltaTime)
	} else {
		// 本地玩家，由客户端控制移动
		// 实际移动逻辑由客户端处理
	}
}

// updateLerp 更新插值位置
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

// SetNetworkPosition 设置服务器同步的位置
func (p *Player) SetNetworkPosition(x, y float64) {
	p.LastNetworkX = p.NetworkX
	p.LastNetworkY = p.NetworkY
	p.NetworkX = x
	p.NetworkY = y
	p.LerpProgress = 0.0
}

// Move 移动玩家（返回是否成功移动）
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
		return false
	}

	p.X = newX
	p.Y = newY
	p.IsMoving = (dx != 0 || dy != 0)

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
func (p *Player) PlaceBomb(game *Game) *Bomb {
	if p.Dead {
		return nil
	}

	// 检查冷却时间
	if p.BombCooldownRemaining > 0 {
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

	p.BombCooldownRemaining = p.BombCooldownSeconds
	p.BombIgnoreGridX = gridX
	p.BombIgnoreGridY = gridY
	p.BombIgnoreActive = true
	bomb := NewBomb(gridX, gridY)
	return bomb
}

// GetGridPosition 获取玩家所在格子
func (p *Player) GetGridPosition() (int, int) {
	return int(p.X) / TileSize, int(p.Y) / TileSize
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
