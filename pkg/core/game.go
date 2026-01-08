package core

// Game 游戏状态（纯逻辑，不包含渲染）
type Game struct {
	Map             *GameMap
	Players         []*Player
	Bombs           []*Bomb
	Explosions      []*Explosion
	IsAuthoritative bool // 是否由于权威逻辑（控制爆炸、伤害判定等）
	CurrentFrame    int32 // 当前帧号
	Seed            int64 // 随机种子（用于确定性）
}

// NewGame 创建新游戏
func NewGame() *Game {
	return &Game{
		Map:             NewGameMap(),
		Players:         make([]*Player, 0),
		Bombs:           make([]*Bomb, 0),
		Explosions:      make([]*Explosion, 0),
		IsAuthoritative: true, // 默认开启权威逻辑（单机模式）
		CurrentFrame:    0,
		Seed:            0,
	}
}

// NewGameWithSeed 使用指定种子创建新游戏
func NewGameWithSeed(seed int64) *Game {
	return &Game{
		Map:             NewGameMapWithSeed(seed),
		Players:         make([]*Player, 0),
		Bombs:           make([]*Bomb, 0),
		Explosions:      make([]*Explosion, 0),
		IsAuthoritative: true,
		CurrentFrame:    0,
		Seed:            seed,
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

// Update 每帧更新游戏状态（不再需要 deltaTime）
func (g *Game) Update() {
	g.CurrentFrame++

	// 1. 更新玩家
	for _, player := range g.Players {
		player.Update(g)
	}

	// 2. 更新炸弹
	g.updateBombs()

	// 3. 更新爆炸
	g.updateExplosions()
}

// updateBombs 更新所有炸弹
// 注意：在非权威模式（网络客户端）下，炸弹由服务器同步控制，不应本地更新
func (g *Game) updateBombs() {
	// 非权威模式：炸弹生命周期由服务器完全控制
	if !g.IsAuthoritative {
		return
	}

	// 收集需要爆炸的炸弹
	explodingBombs := make([]*Bomb, 0)

	for _, bomb := range g.Bombs {
		if bomb.Update() {
			explodingBombs = append(explodingBombs, bomb)
		}
	}

	// 处理爆炸（可能触发连锁）
	for _, bomb := range explodingBombs {
		g.explodeBomb(bomb)
	}

	// 移除已爆炸的炸弹
	newBombs := make([]*Bomb, 0, len(g.Bombs))
	for _, bomb := range g.Bombs {
		if !bomb.Exploded {
			newBombs = append(newBombs, bomb)
		}
	}
	g.Bombs = newBombs
}

// explodeBomb 处理单个炸弹爆炸
func (g *Game) explodeBomb(bomb *Bomb) {
	if bomb.Exploded {
		return
	}
	bomb.Exploded = true

	// 获取爆炸格子
	cells := bomb.GetExplosionCells(g.Map)

	// 创建爆炸效果
	explosion := NewExplosion(bomb.X, bomb.Y, bomb.ExplosionRange, g.CurrentFrame, bomb.OwnerID)
	// 将 GridPos 转换为内部 Cells 格式
	explosion.Cells = make([]GridPos, len(cells))
	copy(explosion.Cells, cells)
	// 预分配地图变化数组
	explosion.TileChanges = make([]TileChange, 0, len(cells))
	g.Explosions = append(g.Explosions, explosion)

	// 炸毁砖块，记录变化
	for _, cell := range cells {
		if g.Map.GetTile(cell.X, cell.Y) == TileBrick {
			oldTile := TileBrick
			var newTile TileType

			// 检查是否是隐藏门
			if cell.X == g.Map.HiddenDoorPos.X && cell.Y == g.Map.HiddenDoorPos.Y {
				newTile = TileDoor
			} else {
				newTile = TileEmpty
			}

			// 记录变化（用于客户端同步）
			explosion.TileChanges = append(explosion.TileChanges, TileChange{
				X:       cell.X,
				Y:       cell.Y,
				OldType: oldTile,
				NewType: newTile,
			})

			// 应用变化
			g.Map.SetTile(cell.X, cell.Y, newTile)
		}
	}

	// 检查连锁爆炸
	for _, otherBomb := range g.Bombs {
		if otherBomb.Exploded {
			continue
		}
		for _, cell := range cells {
			if otherBomb.X == cell.X && otherBomb.Y == cell.Y {
				g.explodeBomb(otherBomb) // 递归触发
				break
			}
		}
	}

	// 检查玩家伤害
	g.checkDamage(explosion)
}

// updateExplosions 更新所有爆炸
// 注意：在非权威模式（网络客户端）下，爆炸由服务器同步控制，不应本地更新
func (g *Game) updateExplosions() {
	// 非权威模式：爆炸生命周期由服务器完全控制，不本地递减
	if !g.IsAuthoritative {
		return
	}

	newExplosions := make([]*Explosion, 0, len(g.Explosions))
	for _, exp := range g.Explosions {
		if !exp.Update() {
			newExplosions = append(newExplosions, exp)
		}
	}
	g.Explosions = newExplosions
}

// checkDamage 检查玩家伤害
func (g *Game) checkDamage(explosion *Explosion) {
	for _, player := range g.Players {
		if player.Dead {
			continue
		}

		// 计算玩家中心点所在的格子
		centerX := player.X + float64(player.Width)/2
		centerY := player.Y + float64(player.Height)/2
		pGridX := int(centerX) / TileSize
		pGridY := int(centerY) / TileSize

		// 检查是否在爆炸范围内
		for _, cell := range explosion.Cells {
			if cell.X == pGridX && cell.Y == pGridY {
				player.Dead = true
				break
			}
		}
	}
}

// GetPlayer 获取玩家
func (g *Game) GetPlayer(id int) *Player {
	for _, p := range g.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// GetAlivePlayers 获取存活玩家
func (g *Game) GetAlivePlayers() []*Player {
	alive := make([]*Player, 0)
	for _, p := range g.Players {
		if !p.Dead {
			alive = append(alive, p)
		}
	}
	return alive
}

// IsGameOver 检查游戏是否结束
func (g *Game) IsGameOver() bool {
	if len(g.Players) == 0 {
		return false
	}

	aliveCount := 0
	var survivor *Player

	for _, player := range g.Players {
		if !player.Dead {
			aliveCount++
			survivor = player
		}
	}

	// 条件1：所有人死亡 -> 游戏结束 (失败)
	if aliveCount == 0 {
		return true
	}

	// 条件2：幸存者只有1人 (满足PvP胜利前置条件)
	if aliveCount == 1 {
		// 检查是否到达门 (满足Stage 1/2 通关条件)
		pGridX, pGridY := PlayerXYToGrid(int(survivor.X), int(survivor.Y))
		if g.Map.GetTile(pGridX, pGridY) == TileDoor {
			return true // 胜利！
		}
		// 只有1人幸存，但还没进门 -> 游戏继续
		return false
	}

	// 超过1人存活 -> 战斗继续
	return false
}
