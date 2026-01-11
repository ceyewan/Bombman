package ai

import (
	"math"

	"bomberman/pkg/core"
)

// === 条件节点 ===

// condIsInDanger 检查是否在危险区
func condIsInDanger(bb *Blackboard) bool {
	gridX, gridY := bb.Player.GetGridPosition()
	// 如果当前位置在危险热力图中，返回 true
	return bb.Danger.InDanger(gridX, gridY)
}

// condCanPlaceBomb 检查是否可以放置炸弹
func condCanPlaceBomb(bb *Blackboard) bool {
	// 检查冷却时间
	if bb.Player.NextPlacementFrame > bb.Frame {
		return false
	}

	// 检查当前已放置数量（AI 只允许同时存在 1 个炸弹）
	activeBombs := 0
	for _, b := range bb.Game.Bombs {
		if b.OwnerID == bb.Player.ID && !b.Exploded {
			activeBombs++
		}
	}
	return activeBombs < 1
}

// === 动作节点 ===

// actEscape 逃生行动
func actEscape(bb *Blackboard) Status {
	// 1. 如果当前位置安全
	gridX, gridY := bb.Player.GetGridPosition()
	if bb.Danger.IsSafe(gridX, gridY) {
		// 已经在安全区，但必须确保位于格子中心（躲避炸弹的关键）
		idealX := float64(gridX*core.TileSize + (core.TileSize-bb.Player.Width)/2)
		idealY := float64(gridY*core.TileSize + (core.TileSize-bb.Player.Height)/2)

		// 如果未对齐中心（容差 2.0 像素）
		if math.Abs(bb.Player.X-idealX) > 2.0 || math.Abs(bb.Player.Y-idealY) > 2.0 {
			// 微调对齐
			dx := idealX - bb.Player.X
			dy := idealY - bb.Player.Y

			// 简单的轴向移动：优先移动距离大的轴
			if math.Abs(dx) > math.Abs(dy) {
				if dx > 0 {
					bb.NextInput.Right = true
				} else {
					bb.NextInput.Left = true
				}
			} else {
				if dy > 0 {
					bb.NextInput.Down = true
				} else {
					bb.NextInput.Up = true
				}
			}
			return StatusRunning // 还在调整位置，视为 Running
		}

		// 安全且对齐中心
		bb.Path = nil
		return StatusSuccess
	}

	// 2. 如果已有逃生路径，且目标仍然安全，继续执行
	if len(bb.Path) > 0 {
		target := bb.Path[len(bb.Path)-1]
		if bb.Danger.IsSafe(target.GridX, target.GridY) {
			input, remainPath := MoveAlongPath(bb.Player, bb.Path)
			bb.Path = remainPath
			bb.NextInput = input
			return StatusRunning
		}
	}

	// 3. 寻找最近的安全点
	safePos := findNearestSafePos(bb)
	if safePos == nil {
		return StatusFailure // 无路可逃
	}

	// 4. 规划路径
	start := core.PlayerXYToGrid(int(bb.Player.X), int(bb.Player.Y))
	path := FindPath(bb.Game, start, *safePos)
	if path == nil {
		return StatusFailure
	}

	bb.Path = path
	bb.CurrentTarget = safePos

	// 5. 执行第一步
	input, remainPath := MoveAlongPath(bb.Player, bb.Path)
	bb.Path = remainPath
	bb.NextInput = input
	return StatusRunning
}

// findNearestSafePos BFS 寻找最近的安全格子
func findNearestSafePos(bb *Blackboard) *core.GridPos {
	start := core.PlayerXYToGrid(int(bb.Player.X), int(bb.Player.Y))
	queue := []core.GridPos{start}
	visited := make(map[core.GridPos]bool)
	visited[start] = true

	directions := []core.GridPos{{GridX: 0, GridY: -1}, {GridX: 0, GridY: 1}, {GridX: -1, GridY: 0}, {GridX: 1, GridY: 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// 如果该点安全，就是目标
		if bb.Danger.IsSafe(current.GridX, current.GridY) {
			return &current
		}

		for _, d := range directions {
			next := core.GridPos{GridX: current.GridX + d.GridX, GridY: current.GridY + d.GridY}

			if !isValid(next.GridX, next.GridY) {
				continue
			}
			// 只搜索可行走的区域
			if !isWalkable(bb.Game, next) {
				continue
			}
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return nil
}

// actFindBrick 寻找可炸的砖块目标
func actFindBrick(bb *Blackboard) Status {
	// 如果已有目标，先验证目标是否仍有效
	if bb.CurrentTarget != nil {
		if isBrickAttackPosition(bb, *bb.CurrentTarget) {
			// 检查是否到达
			pos := core.PlayerXYToGrid(int(bb.Player.X), int(bb.Player.Y))
			if pos == *bb.CurrentTarget {
				return StatusSuccess
			}
			// 否则继续寻路（或者直接交给 MoveTo）
			return StatusSuccess
		}
		// 目标已失效（附近无砖/不可走/不安全），清空重新找
		bb.CurrentTarget = nil
		bb.Path = nil
	}

	// 搜索新的目标
	target := findBrickAttackPosition(bb)
	if target == nil {
		return StatusFailure // 没砖了？
	}

	bb.CurrentTarget = target
	return StatusSuccess
}

// findBrickAttackPosition 寻找一个"旁边有砖块的空地"
func findBrickAttackPosition(bb *Blackboard) *core.GridPos {
	start := core.PlayerXYToGrid(int(bb.Player.X), int(bb.Player.Y))

	// BFS 找最近的砖块攻击点
	queue := []core.GridPos{start}
	visited := make(map[core.GridPos]bool)
	visited[start] = true

	directions := []core.GridPos{{GridX: 0, GridY: -1}, {GridX: 0, GridY: 1}, {GridX: -1, GridY: 0}, {GridX: 1, GridY: 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if isBrickAttackPosition(bb, current) {
			result := current
			return &result
		}

		// 继续扩散
		for _, d := range directions {
			next := core.GridPos{GridX: current.GridX + d.GridX, GridY: current.GridY + d.GridY}
			if !isValid(next.GridX, next.GridY) {
				continue
			}
			if !isWalkable(bb.Game, next) {
				continue
			}

			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return nil
}

// isBrickAttackPosition 检查是否为有效的炸砖位置
func isBrickAttackPosition(bb *Blackboard, pos core.GridPos) bool {
	if !isValid(pos.GridX, pos.GridY) {
		return false
	}
	if !isWalkable(bb.Game, pos) {
		return false
	}
	if !bb.Danger.IsSafe(pos.GridX, pos.GridY) {
		return false
	}
	directions := []core.GridPos{{GridX: 0, GridY: -1}, {GridX: 0, GridY: 1}, {GridX: -1, GridY: 0}, {GridX: 1, GridY: 0}}
	for _, d := range directions {
		nx, ny := pos.GridX+d.GridX, pos.GridY+d.GridY
		if bb.Game.Map.GetTile(nx, ny) == core.TileBrick {
			return true
		}
	}
	return false
}

// actMoveToTarget 移动到当前目标
func actMoveToTarget(bb *Blackboard) Status {
	if bb.CurrentTarget == nil {
		return StatusFailure
	}

	currentPos := core.PlayerXYToGrid(int(bb.Player.X), int(bb.Player.Y))
	if currentPos == *bb.CurrentTarget {
		// 到了，确保对齐中心（稍微修正一下，防止放炸弹被卡住）
		idealX := float64(currentPos.GridX*core.TileSize + (core.TileSize-bb.Player.Width)/2)
		idealY := float64(currentPos.GridY*core.TileSize + (core.TileSize-bb.Player.Height)/2)

		if math.Abs(bb.Player.X-idealX) > 2.0 || math.Abs(bb.Player.Y-idealY) > 2.0 {
			// 微调对齐
			dx := idealX - bb.Player.X
			dy := idealY - bb.Player.Y
			if math.Abs(dx) > math.Abs(dy) {
				if dx > 0 {
					bb.NextInput.Right = true
				} else {
					bb.NextInput.Left = true
				}
			} else {
				if dy > 0 {
					bb.NextInput.Down = true
				} else {
					bb.NextInput.Up = true
				}
			}
			return StatusRunning
		}

		return StatusSuccess
	}

	// 如果没有路径，规划路径
	if len(bb.Path) == 0 {
		bb.Path = FindPath(bb.Game, currentPos, *bb.CurrentTarget)
		if len(bb.Path) == 0 {
			// 无法到达
			bb.CurrentTarget = nil // 放弃这个目标
			return StatusFailure
		}
	}

	// 执行路径
	input, remainPath := MoveAlongPath(bb.Player, bb.Path)
	bb.Path = remainPath
	bb.NextInput = input
	return StatusRunning
}

// actPlaceBomb 放置炸弹
func actPlaceBomb(bb *Blackboard) Status {
	// 确保在目标位置
	if bb.CurrentTarget == nil {
		return StatusFailure
	}
	currentPos := core.PlayerXYToGrid(int(bb.Player.X), int(bb.Player.Y))
	if currentPos != *bb.CurrentTarget {
		return StatusFailure
	}
	if !isBrickAttackPosition(bb, *bb.CurrentTarget) {
		bb.CurrentTarget = nil
		bb.Path = nil
		return StatusFailure
	}

	// 放置炸弹
	bb.NextInput.Bomb = true
	// 注意：不需要手动设置 Danger，下一帧 DangerField 会自动检测到新炸弹并触发 condIsInDanger -> actEscape

	bb.CurrentTarget = nil // 目标达成
	bb.Path = nil          // 清空路径
	return StatusSuccess
}
