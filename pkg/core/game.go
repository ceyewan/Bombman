package core

// Game 游戏状态（纯逻辑，不包含渲染）
type Game struct {
	Map             *GameMap
	Players         []*Player
	Bombs           []*Bomb
	Explosions      []*Explosion
	IsAuthoritative bool // 是否由于权威逻辑（控制爆炸、伤害判定等）
}

// NewGame 创建新游戏
func NewGame() *Game {
	return &Game{
		Map:             NewGameMap(),
		Players:         make([]*Player, 0),
		Bombs:           make([]*Bomb, 0),
		Explosions:      make([]*Explosion, 0),
		IsAuthoritative: true, // 默认开启权威逻辑（单机模式）
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
		bomb.Update(deltaTime)
		// 只有权威模式（单机或服务器）才处理爆炸逻辑
		// 客户端联机模式只负责渲染，不处理逻辑
		if g.IsAuthoritative && bomb.IsExploded() {
			// 炸弹爆炸
			g.createExplosion(bomb)
			// 移除炸弹
			g.Bombs = append(g.Bombs[:i], g.Bombs[i+1:]...)
		}
	}

	// 更新爆炸效果
	for i := len(g.Explosions) - 1; i >= 0; i-- {
		explosion := g.Explosions[i]
		explosion.Update(deltaTime)
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
	explosion.Duration = bomb.ExplosionDuration
	explosion.Cells = explosion.CalculateExplosionCells(g.Map)

// 炸毁砖块
for _, cell := range explosion.Cells {
if g.Map.GetTile(cell.GridX, cell.GridY) == TileBrick {
// 检查是否是隐藏门
if cell.GridX == g.Map.HiddenDoorPos.X && cell.GridY == g.Map.HiddenDoorPos.Y {
g.Map.SetTile(cell.GridX, cell.GridY, TileDoor)
} else {
g.Map.SetTile(cell.GridX, cell.GridY, TileEmpty)
}
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
