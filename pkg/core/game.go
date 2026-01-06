package core

// Game 游戏状态（纯逻辑，不包含渲染）
type Game struct {
	Map        *GameMap
	Players    []*Player
	Bombs      []*Bomb
	Explosions []*Explosion
}

// NewGame 创建新游戏
func NewGame() *Game {
	return &Game{
		Map:        NewGameMap(),
		Players:    make([]*Player, 0),
		Bombs:      make([]*Bomb, 0),
		Explosions: make([]*Explosion, 0),
	}
}

// AddPlayer 添加玩家
func (g *Game) AddPlayer(player *Player) {
	g.Players = append(g.Players, player)
}

// AddBomb 添加炸弹
func (g *Game) AddBomb(bomb *Bomb) {
	g.Bombs = append(g.Bombs, bomb)
}

// Update 更新游戏状态
func (g *Game) Update(deltaTime float64) {
	// 更新玩家
	for _, player := range g.Players {
		player.Update(deltaTime, g)
	}

	// 更新炸弹
	for i := len(g.Bombs) - 1; i >= 0; i-- {
		bomb := g.Bombs[i]
		if bomb.IsExploded() {
			// 炸弹爆炸
			g.createExplosion(bomb)
			// 移除炸弹
			g.Bombs = append(g.Bombs[:i], g.Bombs[i+1:]...)
		}
	}

	// 更新爆炸效果
	for i := len(g.Explosions) - 1; i >= 0; i-- {
		explosion := g.Explosions[i]
		if explosion.IsExpired() {
			// 移除爆炸效果
			g.Explosions = append(g.Explosions[:i], g.Explosions[i+1:]...)
		}
	}
}

// createExplosion 创建爆炸效果
func (g *Game) createExplosion(bomb *Bomb) {
	gridX, gridY := bomb.GetGridPosition()

	explosion := NewExplosion(gridX, gridY, bomb.ExplosionRange)
	explosion.Cells = explosion.CalculateExplosionCells(g.Map)

	// 炸毁砖块
	for _, cell := range explosion.Cells {
		if g.Map.GetTile(cell.GridX, cell.GridY) == TileBrick {
			g.Map.SetTile(cell.GridX, cell.GridY, TileEmpty)
		}
	}

	// 检查玩家是否被炸到
	for _, player := range g.Players {
		playerGridX := int(player.X) / TileSize
		playerGridY := int(player.Y) / TileSize

		for _, cell := range explosion.Cells {
			if cell.GridX == playerGridX && cell.GridY == playerGridY {
				player.Dead = true
				break
			}
		}
	}

	g.Explosions = append(g.Explosions, explosion)
}

// IsGameOver 检查游戏是否结束
func (g *Game) IsGameOver() bool {
	if len(g.Players) == 0 {
		return false
	}

	aliveCount := 0
	for _, player := range g.Players {
		if !player.Dead {
			aliveCount++
		}
	}

	// 只有一个或零个玩家存活时游戏结束
	return aliveCount <= 1
}
