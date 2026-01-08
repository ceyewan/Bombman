package ai

import (
	"math"
	"math/rand"
	"time"

	"bomberman/pkg/core"
)

// ===== 核心数据结构 =====

// DangerGrid 危险热力图（事件驱动更新）
type DangerGrid struct {
	Cells         [core.MapHeight][core.MapWidth]float64 // 危险值 0.0-1.0
	bombSnapshots map[*core.Bomb]BombSnapshot            // 炸弹快照
}

// BombSnapshot 炸弹快照（用于增量更新）
type BombSnapshot struct {
	FramesUntilExplode int // 改为帧
	GridX, GridY        int
}

// GridPos 格子坐标
type GridPos struct {
	X, Y int
}

// Node 用于路径搜索
type Node struct {
	Pos    GridPos
	Parent *Node
}

// AIController AI 控制器（智能版）
type AIController struct {
	PlayerID int
	rnd      *rand.Rand

	// 危险热力图
	dangerGrid DangerGrid

	// 思考频率控制（性能优化）
	thinkIntervalFrames int // 思考间隔（帧）
	thinkCounter        int // 思考计数器
	cachedInput         core.Input

	// 门的发现状态
	doorRevealed bool
	doorPos      GridPos

	// 随机游走状态（兜底行为）
	changeDirTickerFrames int // 换方向倒计时（帧）
	randomInput           core.Input

	// 卡住检测
	lastPos        GridPos
	stuckCounter   int
	stuckThreshold int // 连续 N 帧位置不变视为卡住

	// 逃跑目标缓存（防止抖动）
	escapeTarget *GridPos
}

// ===== 构造函数 =====

// NewAIController 创建新的 AI 控制器
func NewAIController(playerID int) *AIController {
	// 初始化随机方向，避免第一次随机游走时选择停止
	randomInput := core.Input{}
	choice := rand.Intn(4) // 0-3，不包含4（停止）
	switch choice {
	case 0:
		randomInput.Up = true
	case 1:
		randomInput.Down = true
	case 2:
		randomInput.Left = true
	case 3:
		randomInput.Right = true
	}

	return &AIController{
		PlayerID:             playerID,
		rnd:                  rand.New(rand.NewSource(time.Now().UnixNano() + int64(playerID))),
		thinkIntervalFrames:  core.AIThinkIntervalFrames, // 6 帧
		thinkCounter:         0,
		dangerGrid: DangerGrid{
			bombSnapshots: make(map[*core.Bomb]BombSnapshot),
		},
		changeDirTickerFrames: 30 + rand.Intn(60), // 0.5-1.5秒 = 30-90帧
		randomInput:           randomInput,
		stuckThreshold:        core.AIStuckThresholdFrames,
	}
}

// ===== 主决策入口 =====

// Decide 根据当前游戏状态决定下一帧的输入
func (c *AIController) Decide(game *core.Game) core.Input {
	player := getPlayerByID(game, c.PlayerID)
	if player == nil || player.Dead {
		return core.Input{}
	}

	// 如果上一帧放了炸弹，立即强制更新危险图并重新思考
	if c.cachedInput.Bomb {
		c.dangerGrid.UpdateIfNeeded(game) // 立即刷新
		c.thinkCounter = 0
		c.cachedInput = core.Input{} // 清除炸弹指令，防止重复放
	}

	// 思考频率控制（性能优化）
	c.thinkCounter++

	if c.thinkCounter < c.thinkIntervalFrames {
		return c.cachedInput // 返回缓存的决策
	}

	// 重新思考
	c.thinkCounter = 0
	newInput := c.thinkLogic(game, player)

	// 如果决定放炸弹，不缓存移动指令（下一帧专心逃跑）
	if newInput.Bomb {
		c.cachedInput = core.Input{Bomb: true}
		return c.cachedInput
	}

	c.cachedInput = newInput
	return c.cachedInput
}

// thinkLogic 核心决策逻辑（行为树）
func (c *AIController) thinkLogic(game *core.Game, player *core.Player) core.Input {
	// 更新危险热力图
	c.dangerGrid.UpdateIfNeeded(game)

	// 卡住检测
	currentPos := GridPos{
		X: int(player.X) / core.TileSize,
		Y: int(player.Y) / core.TileSize,
	}

	if currentPos == c.lastPos {
		c.stuckCounter++
	} else {
		c.stuckCounter = 0
		c.lastPos = currentPos
	}

	// 如果卡住太久，强制随机移动脱困
	if c.stuckCounter > c.stuckThreshold {
		c.stuckCounter = 0
		c.pickNewRandomDirection()
		return c.randomInput
	}

	// 更新门的发现状态
	c.updateDoorStatus(game)

	// 行为树（优先级从高到低）

	// 1. 生存优先：如果当前位置危险，逃跑！
	// 注意：如果刚放了炸弹，dangerGrid 可能还没更新（炸弹还没生成），
	// 但 thinkLogic 是在放炸弹后的下一帧调用的（因为 thinkTimer=0），
	// 所以只要游戏状态更新了，这里就能检测到危险。
	if c.isInDanger(player) {
		return c.escape(player, game)
	}

	// 脱离危险后，清除逃跑目标缓存
	c.escapeTarget = nil

	// 2. 寻找门：如果门已被发现且幸存者只剩自己，前往门
	if c.doorRevealed && c.shouldGoToDoor(game) {
		return c.goToDoor(player, game)
	}

	// 3. 攻击模式：寻找敌人或可破坏的砖块
	if target := c.findAttackTarget(player, game); target != nil {
		if c.isAtTarget(player, target) && c.canPlaceBombSafely(player, game) {
			// 到达目标位置，放置炸弹
			return core.Input{Bomb: true}
		}
		// 移动到目标
		return c.moveToTarget(player, target, game)
	}

	// 4. 探索模式：破坏砖块寻找门
	if target := c.findNearestBrick(player, game); target != nil {
		if c.isAtTarget(player, target) && c.canPlaceBombSafely(player, game) {
			return core.Input{Bomb: true}
		}
		return c.moveToTarget(player, target, game)
	}

	// 5. 兜底：随机游走
	return c.randomWalk()
}

// ===== 危险热力图更新 =====

// UpdateIfNeeded 事件驱动更新危险热力图
func (dg *DangerGrid) UpdateIfNeeded(game *core.Game) {
	// 清空所有危险值
	for y := 0; y < core.MapHeight; y++ {
		for x := 0; x < core.MapWidth; x++ {
			dg.Cells[y][x] = 0.0
		}
	}

	// 1. 标记所有炸弹的危险区（即将爆炸）
	currentBombs := make(map[*core.Bomb]bool)
	for _, bomb := range game.Bombs {
		currentBombs[bomb] = true
		dg.addBombDanger(bomb, game.Map)
		gridX, gridY := bomb.GetGridPosition()
		dg.bombSnapshots[bomb] = BombSnapshot{
			FramesUntilExplode: bomb.FramesUntilExplode,
			GridX:              gridX,
			GridY:              gridY,
		}
	}

	// 2. 标记所有正在爆炸的火焰区域（致命！）
	for _, explosion := range game.Explosions {
		dg.addExplosionDanger(explosion)
	}

	// 清理已爆炸的炸弹
	for bomb := range dg.bombSnapshots {
		if !currentBombs[bomb] {
			delete(dg.bombSnapshots, bomb)
		}
	}
}

// addBombDanger 添加炸弹的危险区（改进的分段危险值计算）
func (dg *DangerGrid) addBombDanger(bomb *core.Bomb, gameMap *core.GameMap) {
	gridX, gridY := bomb.GetGridPosition()

	// 分段危险值（基于帧）：
	// - 剩余 > 90帧(1.5s): 低危 (0.2)
	// - 剩余 60-90帧(1.0-1.5s): 中危 (0.5)
	// - 剩余 < 60帧(1.0s): 高危 (0.9)
	var dangerValue float64
	switch {
	case bomb.FramesUntilExplode > 90:
		dangerValue = 0.2
	case bomb.FramesUntilExplode > 60:
		dangerValue = 0.5
	default:
		dangerValue = 0.9
	}

	// 计算爆炸范围
	explosion := core.NewExplosion(gridX, gridY, bomb.ExplosionRange, 0, bomb.OwnerID)
	cells := explosion.CalculateExplosionCells(gameMap)

	// 标记危险格子
	for _, cell := range cells {
		if cell.GridY >= 0 && cell.GridY < core.MapHeight &&
			cell.GridX >= 0 && cell.GridX < core.MapWidth {
			dg.Cells[cell.GridY][cell.GridX] += dangerValue
			if dg.Cells[cell.GridY][cell.GridX] > 1.0 {
				dg.Cells[cell.GridY][cell.GridX] = 1.0
			}
		}
	}
}

// addExplosionDanger 添加爆炸火焰的危险区（危险值 = 1.0，绝对致命）
func (dg *DangerGrid) addExplosionDanger(explosion *core.Explosion) {
	for _, cell := range explosion.Cells {
		if cell.Y >= 0 && cell.Y < core.MapHeight &&
			cell.X >= 0 && cell.X < core.MapWidth {
			dg.Cells[cell.Y][cell.X] = 1.0 // 火焰 = 必死
		}
	}
}

// ===== 生存逻辑 =====

// isInDanger 检查玩家是否处于危险中
func (c *AIController) isInDanger(player *core.Player) bool {
	// 使用左上角坐标计算格子位置（与其他函数保持一致）
	gridX, gridY := core.PlayerXYToGrid(int(player.X), int(player.Y))

	// 降低危险阈值，更敏感地躲避炸弹
	if gridX >= 0 && gridX < core.MapWidth && gridY >= 0 && gridY < core.MapHeight {
		return c.dangerGrid.Cells[gridY][gridX] > 0.05
	}
	return false
}

// escape 逃跑（BFS 寻找最近安全格子）
func (c *AIController) escape(player *core.Player, game *core.Game) core.Input {
	playerGridX, playerGridY := core.PlayerXYToGrid(int(player.X), int(player.Y))

	// 检查缓存的逃跑目标是否仍然有效
	if c.escapeTarget != nil {
		// 验证目标仍然安全
		if c.escapeTarget.X >= 0 && c.escapeTarget.X < core.MapWidth &&
			c.escapeTarget.Y >= 0 && c.escapeTarget.Y < core.MapHeight &&
			c.dangerGrid.Cells[c.escapeTarget.Y][c.escapeTarget.X] < 0.01 {
			// 目标仍然安全，继续往那里跑
			nextStep, found := c.getNextStep(player, *c.escapeTarget, game)
			if found {
				return c.moveToCell(player, nextStep)
			}
		}
		// 目标无效了，清除缓存
		c.escapeTarget = nil
	}

	// BFS 搜索最近的安全格子
	safeGrid := c.findNearestSafeCell(playerGridX, playerGridY, game)
	if safeGrid == nil {
		// 没有安全格子，随机移动（听天由命）
		return c.randomWalk()
	}

	// 缓存逃跑目标
	c.escapeTarget = safeGrid

	// 使用 BFS 寻路移动到安全格子
	nextStep, found := c.getNextStep(player, *safeGrid, game)
	if found {
		return c.moveToCell(player, nextStep)
	}

	// 找不到路径，随机移动
	return c.randomWalk()
}

// findNearestSafeCell BFS 搜索最近的安全格子
func (c *AIController) findNearestSafeCell(startX, startY int, game *core.Game) *GridPos {
	type node struct {
		pos   GridPos
		depth int
	}

	queue := []node{{pos: GridPos{X: startX, Y: startY}, depth: 0}}
	visited := make(map[GridPos]bool)
	maxDepth := 20 // 增加搜索深度

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// 超过深度限制
		if current.depth > maxDepth {
			continue
		}

		if visited[current.pos] {
			continue
		}
		visited[current.pos] = true

		// 检查是否安全（危险值 < 0.01，几乎完全安全）
		if current.pos.X >= 0 && current.pos.X < core.MapWidth &&
			current.pos.Y >= 0 && current.pos.Y < core.MapHeight {
			danger := c.dangerGrid.Cells[current.pos.Y][current.pos.X]
			// 如果不是起点且非常安全，返回
			if (current.pos.X != startX || current.pos.Y != startY) && danger < 0.01 {
				return &current.pos
			}
		}

		// 搜索四个方向
		for _, dir := range []struct{ dx, dy int }{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			nx, ny := current.pos.X+dir.dx, current.pos.Y+dir.dy
			if nx >= 0 && nx < core.MapWidth && ny >= 0 && ny < core.MapHeight {
				tile := game.Map.GetTile(nx, ny)

				// 关键修复：路径本身也不能太危险
				pathDanger := c.dangerGrid.Cells[ny][nx]

				// 允许穿越轻微危险区（< 0.3），但禁止穿越高危区
				// 这样 AI 可以在炸弹快爆炸前"擦边"逃跑
				if tile != core.TileWall && tile != core.TileBrick && pathDanger < 0.3 {
					queue = append(queue, node{
						pos:   GridPos{X: nx, Y: ny},
						depth: current.depth + 1,
					})
				}
			}
		}
	}

	// 如果找不到完全安全的地方，找一个危险值最小的
	minDanger := 1.0
	var safestPos *GridPos
	for y := 0; y < core.MapHeight; y++ {
		for x := 0; x < core.MapWidth; x++ {
			if game.Map.GetTile(x, y) == core.TileEmpty || game.Map.GetTile(x, y) == core.TileDoor {
				if c.dangerGrid.Cells[y][x] < minDanger {
					minDanger = c.dangerGrid.Cells[y][x]
					pos := GridPos{X: x, Y: y}
					safestPos = &pos
				}
			}
		}
	}

	return safestPos // 返回最安全的位置
}

// ===== BFS 寻路算法 =====

// getNextStep 使用 BFS 寻找通往目标的下一步
// 返回: 下一步的格子坐标, 是否找到路径
func (c *AIController) getNextStep(player *core.Player, target GridPos, game *core.Game) (GridPos, bool) {
	startGridX, startGridY := core.PlayerXYToGrid(int(player.X), int(player.Y))
	start := GridPos{X: startGridX, Y: startGridY}

	if start == target {
		return start, true
	}

	queue := []Node{{Pos: start, Parent: nil}}
	visited := make(map[GridPos]bool)
	visited[start] = true

	// 记录终点节点以便回溯
	var endNode *Node

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.Pos == target {
			endNode = &curr
			break
		}

		// 搜索 4 个方向（固定顺序，避免抖动）
		dirs := []GridPos{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

		for _, d := range dirs {
			nextPos := GridPos{X: curr.Pos.X + d.X, Y: curr.Pos.Y + d.Y}

			// 越界检查
			if nextPos.X < 0 || nextPos.X >= core.MapWidth || nextPos.Y < 0 || nextPos.Y >= core.MapHeight {
				continue
			}
			if visited[nextPos] {
				continue
			}

			// 障碍物检查 (墙、砖块)
			tile := game.Map.GetTile(nextPos.X, nextPos.Y)
			if tile == core.TileWall || tile == core.TileBrick {
				continue
			}

			// 检查是否有炸弹阻挡
			hasBomb := false
			for _, b := range game.Bombs {
				bx, by := b.GetGridPosition()
				if bx == nextPos.X && by == nextPos.Y {
					hasBomb = true
					break
				}
			}
			if hasBomb {
				continue
			}

			// 危险检查：逃跑时用更低的阈值，避免穿越危险区
			// 与 findNearestSafeCell 保持一致，使用 0.3 作为阈值
			if c.dangerGrid.Cells[nextPos.Y][nextPos.X] > 0.3 {
				continue
			}

			visited[nextPos] = true
			queue = append(queue, Node{Pos: nextPos, Parent: &curr})
		}
	}

	// 如果没找到路径
	if endNode == nil {
		return GridPos{}, false
	}

	// 回溯找到出发后的第一步
	curr := endNode
	for curr.Parent != nil && curr.Parent.Pos != start {
		curr = curr.Parent
	}

	return curr.Pos, true
}

// moveToCell [重构]：先对齐轴线，再移动
func (c *AIController) moveToCell(player *core.Player, targetGrid GridPos) core.Input {
	input := core.Input{}

	// 目标中心像素坐标（修正：使用 GridToPlayerXY 获取中心对齐的坐标）
	tx, ty := core.GridToPlayerXY(targetGrid.X, targetGrid.Y)
	targetPixelX := float64(tx)
	targetPixelY := float64(ty)
	currentGridX, currentGridY := core.PlayerXYToGrid(int(player.X), int(player.Y))

	// 容差：越小越精准，建议 2.0 或更小以确保顺利进洞
	const alignTolerance = 2.0

	dx := targetPixelX - player.X
	dy := targetPixelY - player.Y

	// 策略：如果要跨越格子，必须先确保垂直轴对齐

	// 情况 1: 水平移动 (目标在左右)
	if targetGrid.X != currentGridX {
		// 必须先对齐 Y 轴
		if math.Abs(dy) > alignTolerance {
			if dy > 0 {
				input.Down = true
			} else {
				input.Up = true
			}
			return input // ⛔ 关键：Y 轴未对齐前，禁止水平移动
		}
		// Y 轴已对齐，允许水平移动
		if dx > 0 {
			input.Right = true
		} else {
			input.Left = true
		}
		return input
	}

	// 情况 2: 垂直移动 (目标在上下)
	if targetGrid.Y != currentGridY {
		// 必须先对齐 X 轴
		if math.Abs(dx) > alignTolerance {
			if dx > 0 {
				input.Right = true
			} else {
				input.Left = true
			}
			return input // ⛔ 关键：X 轴未对齐前，禁止垂直移动
		}
		// X 轴已对齐，允许垂直移动
		if dy > 0 {
			input.Down = true
		} else {
			input.Up = true
		}
		return input
	}

	// 情况 3: 格子内微调 (走到中心)
	if math.Abs(dx) > alignTolerance {
		if dx > 0 {
			input.Right = true
		} else {
			input.Left = true
		}
	}
	if math.Abs(dy) > alignTolerance {
		if dy > 0 {
			input.Down = true
		} else {
			input.Up = true
		}
	}

	return input
}

// ===== 炸弹放置逻辑 =====

// canPlaceBombSafely 检查是否可以安全放置炸弹（使用 BFS 验证逃生路径）
func (c *AIController) canPlaceBombSafely(player *core.Player, game *core.Game) bool {
	gridX, gridY := core.PlayerXYToGrid(int(player.X), int(player.Y))

	// 1. 检查当前位置是否已有炸弹
	for _, b := range game.Bombs {
		bx, by := b.GetGridPosition()
		if bx == gridX && by == gridY {
			return false
		}
	}

	// 2. 模拟爆炸范围
	simulatedRange := core.DefaultExplosionRange
	dangerMap := make(map[GridPos]bool)

	// 简单计算十字范围
	dangerMap[GridPos{gridX, gridY}] = true
	dirs := []GridPos{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	for _, d := range dirs {
		for i := 1; i <= simulatedRange; i++ {
			nx, ny := gridX+d.X*i, gridY+d.Y*i
			// 遇到墙停止
			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				break
			}
			tile := game.Map.GetTile(nx, ny)
			if tile == core.TileWall {
				break
			}

			dangerMap[GridPos{nx, ny}] = true

			// 遇到砖块停止（但在这一格也会炸）
			if tile == core.TileBrick {
				break
			}
		}
	}

	// 3. 使用 BFS 寻找最近的安全点（不在 dangerMap 中的点）
	type NodeState struct {
		Pos   GridPos
		Depth int
	}
	qState := []NodeState{{GridPos{gridX, gridY}, 0}}
	visited := make(map[GridPos]bool)
	visited[GridPos{gridX, gridY}] = true

	foundSafeSpot := false
	maxSearchDepth := 10 // 只要能在 10 步内找到安全点就行

	for len(qState) > 0 {
		curr := qState[0]
		qState = qState[1:]

		// 如果当前点不在模拟的危险区内，且本身是安全的（没有其他炸弹威胁）
		if !dangerMap[curr.Pos] && c.dangerGrid.Cells[curr.Pos.Y][curr.Pos.X] < 0.1 {
			foundSafeSpot = true
			break
		}

		if curr.Depth >= maxSearchDepth {
			continue
		}

		for _, d := range dirs {
			nx, ny := curr.Pos.X+d.X, curr.Pos.Y+d.Y
			nextPos := GridPos{nx, ny}

			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				continue
			}
			if visited[nextPos] {
				continue
			}

			tile := game.Map.GetTile(nx, ny)
			if tile == core.TileWall || tile == core.TileBrick {
				continue
			}

			// 关键：逃跑路径不能被现有的其他炸弹阻挡
			blockedByBomb := false
			for _, b := range game.Bombs {
				bx, by := b.GetGridPosition()
				if bx == nx && by == ny {
					blockedByBomb = true
					break
				}
			}
			if blockedByBomb {
				continue
			}

			visited[nextPos] = true
			qState = append(qState, NodeState{nextPos, curr.Depth + 1})
		}
	}

	return foundSafeSpot
}

// ===== 门的逻辑 =====

// updateDoorStatus 更新门的发现状态
func (c *AIController) updateDoorStatus(game *core.Game) {
	if c.doorRevealed {
		return // 已发现
	}

	// 检查隐藏门是否已被炸开
	doorX, doorY := game.Map.HiddenDoorPos.X, game.Map.HiddenDoorPos.Y
	if game.Map.GetTile(doorX, doorY) == core.TileDoor {
		c.doorRevealed = true
		c.doorPos = GridPos{X: doorX, Y: doorY}
	}
}

// shouldGoToDoor 判断是否应该前往门
func (c *AIController) shouldGoToDoor(game *core.Game) bool {
	// 统计存活玩家数
	aliveCount := 0
	for _, p := range game.Players {
		if !p.Dead {
			aliveCount++
		}
	}

	// 只有自己存活时才去门
	return aliveCount == 1
}

// goToDoor 前往门
func (c *AIController) goToDoor(player *core.Player, game *core.Game) core.Input {
	return c.moveToTarget(player, &c.doorPos, game)
}

// ===== 攻击逻辑 =====

// findAttackTarget 寻找攻击目标（优先玩家 > 砖块）
func (c *AIController) findAttackTarget(player *core.Player, game *core.Game) *GridPos {
	playerGridX, playerGridY := core.PlayerXYToGrid(int(player.X), int(player.Y))

	// 1. 优先寻找敌人
	closestEnemy := (*GridPos)(nil)
	minEnemyDist := math.MaxFloat64

	for _, enemy := range game.Players {
		if enemy.ID == c.PlayerID || enemy.Dead {
			continue
		}

		enemyGridX, enemyGridY := core.PlayerXYToGrid(int(enemy.X), int(enemy.Y))
		dist := manhattanDistance(playerGridX, playerGridY, enemyGridX, enemyGridY)

		if dist < minEnemyDist {
			minEnemyDist = dist
			closestEnemy = &GridPos{X: enemyGridX, Y: enemyGridY}
		}
	}

	// 如果敌人很近（< 5 格），追击
	if closestEnemy != nil && minEnemyDist < 5 {
		return closestEnemy
	}

	// 2. 否则寻找最近的砖块
	return c.findNearestBrick(player, game)
}

// findNearestBrick [重构]：寻找最近的"最佳投弹点"
// 逻辑：BFS 遍历可达的空地，检查该空地是否能炸到砖块
func (c *AIController) findNearestBrick(player *core.Player, game *core.Game) *GridPos {
	startGridX, startGridY := core.PlayerXYToGrid(int(player.X), int(player.Y))
	startNode := GridPos{X: startGridX, Y: startGridY}

	// 1. 检查原地是否就是最佳攻击点
	if c.hasBrickInRange(startNode, game) && c.canPlaceBombSafely(player, game) {
		return &startNode
	}

	// 2. BFS 泛洪搜索
	queue := []GridPos{startNode}
	visited := make(map[GridPos]bool)
	visited[startNode] = true

	// 限制搜索步数，防止地图过大导致卡顿
	maxSteps := 100
	steps := 0

	for len(queue) > 0 {
		if steps > maxSteps {
			break
		}

		curr := queue[0]
		queue = queue[1:]
		steps++

		// 检查：如果站在这里放炸弹，能炸到砖块吗？
		// 且该位置安全（不是火海，且放了炸弹能跑掉）
		if c.hasBrickInRange(curr, game) {
			if c.dangerGrid.Cells[curr.Y][curr.X] < 0.1 && c.canPlaceBombSafelyAt(curr, game) {
				return &curr // 找到了最近的可达攻击位
			}
		}

		// 拓展邻居
		dirs := []GridPos{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
		// 随机打乱顺序，增加行为自然度
		rand.Shuffle(len(dirs), func(i, j int) {
			dirs[i], dirs[j] = dirs[j], dirs[i]
		})

		for _, d := range dirs {
			next := GridPos{X: curr.X + d.X, Y: curr.Y + d.Y}

			// 越界检查
			if next.X < 0 || next.X >= core.MapWidth || next.Y < 0 || next.Y >= core.MapHeight {
				continue
			}
			if visited[next] {
				continue
			}

			// 障碍物检查：不能穿墙，也不能穿砖
			tile := game.Map.GetTile(next.X, next.Y)
			if tile == core.TileWall || tile == core.TileBrick {
				continue
			}

			// 炸弹阻挡检查：不能穿过现有的炸弹
			if c.isBlockedByBomb(next, game) {
				continue
			}

			// 危险区域回避：不要为了找砖块冲进火海
			if c.dangerGrid.Cells[next.Y][next.X] > 0.5 {
				continue
			}

			visited[next] = true
			queue = append(queue, next)
		}
	}

	return nil
}

// ===== 移动逻辑 =====

// moveToTarget 移动到目标（使用 BFS 寻路）
func (c *AIController) moveToTarget(player *core.Player, target *GridPos, game *core.Game) core.Input {
	if target == nil {
		return core.Input{}
	}

	playerGridX, playerGridY := core.PlayerXYToGrid(int(player.X), int(player.Y))

	// 已经到达目标
	if playerGridX == target.X && playerGridY == target.Y {
		return core.Input{}
	}

	// 使用 BFS 寻找下一步
	nextStep, found := c.getNextStep(player, *target, game)
	if found {
		return c.moveToCell(player, nextStep)
	}

	// 找不到路径，随机移动
	return c.randomWalk()
}

// isAtTarget 检查是否到达目标
func (c *AIController) isAtTarget(player *core.Player, target *GridPos) bool {
	if target == nil {
		return false
	}

	playerGridX, playerGridY := core.PlayerXYToGrid(int(player.X), int(player.Y))
	return playerGridX == target.X && playerGridY == target.Y
}

// ===== 随机游走（兜底行为） =====

// randomWalk 随机游走
func (c *AIController) randomWalk() core.Input {
	c.changeDirTickerFrames--
	if c.changeDirTickerFrames <= 0 {
		c.changeDirTickerFrames = 30 + c.rnd.Intn(60) // 0.5 ~ 1.5 秒改变一次 (30-90帧)
		c.pickNewRandomDirection()
	}
	return c.randomInput
}

// pickNewRandomDirection 随机选择新方向
func (c *AIController) pickNewRandomDirection() {
	c.randomInput = core.Input{}

	choice := c.rnd.Intn(5)
	switch choice {
	case 0:
		c.randomInput.Up = true
	case 1:
		c.randomInput.Down = true
	case 2:
		c.randomInput.Left = true
	case 3:
		c.randomInput.Right = true
	case 4:
		// 停止
	}
}

// ===== 辅助函数 =====

// hasBrickInRange 检查在 pos 位置放置炸弹，能否炸毁任何砖块（范围 3 格）
func (c *AIController) hasBrickInRange(pos GridPos, game *core.Game) bool {
	bombRange := core.DefaultExplosionRange
	dirs := []GridPos{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for _, d := range dirs {
		for i := 1; i <= bombRange; i++ {
			checkX := pos.X + d.X*i
			checkY := pos.Y + d.Y*i

			if checkX < 0 || checkX >= core.MapWidth || checkY < 0 || checkY >= core.MapHeight {
				break
			}

			tile := game.Map.GetTile(checkX, checkY)
			if tile == core.TileWall {
				break // 墙壁阻挡爆炸
			}
			if tile == core.TileBrick {
				return true // 炸到砖块
			}
		}
	}
	return false
}

// isBlockedByBomb 检查位置是否被炸弹阻挡
func (c *AIController) isBlockedByBomb(pos GridPos, game *core.Game) bool {
	for _, b := range game.Bombs {
		bx, by := b.GetGridPosition()
		if bx == pos.X && by == pos.Y {
			return true
		}
	}
	return false
}

// canPlaceBombSafelyAt 检查在指定位置是否可以安全放置炸弹
// 改进版：防止连环自杀，增加障碍物模拟
func (c *AIController) canPlaceBombSafelyAt(pos GridPos, game *core.Game) bool {
	// 🛑 规则 1: 只有在绝对安全的地方才能放炸弹
	// 如果当前位置已经有危险（比如在另一个炸弹的波及范围内），禁止“火上浇油”
	// 这能有效防止 AI 连续放置两个炸弹导致自己无路可逃
	if c.dangerGrid.Cells[pos.Y][pos.X] > 0 {
		return false
	}

	// 🛑 规则 2: 检查位置是否已有炸弹（物理重叠）
	if c.isBlockedByBomb(pos, game) {
		return false
	}

	// === 步骤 A: 模拟新炸弹的爆炸范围 ===
	simulatedRange := core.DefaultExplosionRange // 假设炸弹威力 (建议与 hasBrickInRange 保持一致)
	newBombDangerZone := make(map[GridPos]bool)

	// 标记中心和十字范围
	newBombDangerZone[pos] = true
	dirs := []GridPos{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}

	for _, d := range dirs {
		for i := 1; i <= simulatedRange; i++ {
			nx, ny := pos.X+d.X*i, pos.Y+d.Y*i
			// 越界检查
			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				break
			}

			tile := game.Map.GetTile(nx, ny)
			if tile == core.TileWall {
				break
			} // 墙壁阻挡

			newBombDangerZone[GridPos{nx, ny}] = true

			if tile == core.TileBrick {
				break
			} // 砖块阻挡（但当前格受波及）
		}
	}

	// === 步骤 B: BFS 寻找逃生路径 ===
	// 目标：找到一个既不受旧炸弹威胁，也不受新炸弹威胁的格子
	// 约束：路径不能穿过墙、砖、旧炸弹、以及**新炸弹**

	type NodeState struct {
		Pos   GridPos
		Depth int
	}
	queue := []NodeState{{pos, 0}}
	visited := make(map[GridPos]bool)
	visited[pos] = true

	maxSearchDepth := 6 // 必须在 6 步内逃脱 (约 1.5 秒路程，给自己留 0.5 秒余量)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		// 1. 检查当前点是否是合格的“避难所”
		// 条件：
		// a. 不在新炸弹的爆炸范围内
		// b. 不在旧炸弹的危险区内 (Danger == 0)
		// c. 不是当前放置炸弹的位置（必须移动开）
		if !newBombDangerZone[curr.Pos] &&
			c.dangerGrid.Cells[curr.Pos.Y][curr.Pos.X] == 0 &&
			(curr.Pos != pos) {
			return true // 找到了生路！可以放炸弹。
		}

		if curr.Depth >= maxSearchDepth {
			continue
		}

		// 2. 拓展路径
		for _, d := range dirs {
			nx, ny := curr.Pos.X+d.X, curr.Pos.Y+d.Y
			nextPos := GridPos{nx, ny}

			// 越界与访问检查
			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				continue
			}
			if visited[nextPos] {
				continue
			}

			// 障碍物检查
			tile := game.Map.GetTile(nx, ny)
			if tile == core.TileWall || tile == core.TileBrick {
				continue
			}

			// 🛑 关键修正：将“即将放置的炸弹”视为障碍物
			// 也就是：一旦离开起点，就不能再走回起点（因为那里会有个炸弹）
			if nextPos == pos {
				continue
			}

			// 旧炸弹阻挡
			if c.isBlockedByBomb(nextPos, game) {
				continue
			}

			// 危险路径检查：逃跑路径本身不能太危险
			// 如果路径上的危险值太高，说明我们要穿过火海去安全点，这是不行的
			if c.dangerGrid.Cells[ny][nx] > 0.5 {
				continue
			}

			visited[nextPos] = true
			queue = append(queue, NodeState{nextPos, curr.Depth + 1})
		}
	}

	// 遍历完所有可能路径都没找到安全点
	return false
}

// ===== 工具函数 =====

func getPlayerByID(game *core.Game, playerID int) *core.Player {
	for _, player := range game.Players {
		if player.ID == playerID {
			return player
		}
	}
	return nil
}

func manhattanDistance(x1, y1, x2, y2 int) float64 {
	return math.Abs(float64(x1-x2)) + math.Abs(float64(y1-y2))
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
