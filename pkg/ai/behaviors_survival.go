package ai

import (
	"bomberman/pkg/ai/bt"
	"bomberman/pkg/core"
)

// 方向常量
const (
	DirNone  = 0
	DirUp    = 1
	DirDown  = 2
	DirLeft  = 3
	DirRight = 4
)

func condInDanger(bb bt.Blackboard) bool {
	board := bb.(*Blackboard)
	pos := getPlayerGrid(board.Player)
	return board.Danger.InDanger(pos.GridX, pos.GridY)
}

func actFindSafe(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	start := getPlayerGrid(board.Player)

	// 如果已有逃生目标，验证它是否仍然可行
	if board.EscapeTo != nil {
		// 检查是否已到达目标
		if *board.EscapeTo == start {
			board.EscapeTo = nil // 已到达，清空目标
		} else {
			// 检查目标是否仍然安全
			_, pathOK := nextStepToward(board.Game, start, *board.EscapeTo)
			targetStillSafe := board.Danger.SafeAtFrame(board.EscapeTo.GridX, board.EscapeTo.GridY, board.Frame+int32(core.BombFuseFrames))
			if pathOK && targetStillSafe {
				return bt.StatusSuccess // 继续使用现有目标
			}
			// 目标不可行，需要重新计算
			board.EscapeTo = nil
		}
	}

	// 计算新的逃生目标
	best := findNearestSafeTimed(board.Game, board.Danger, start, board.Frame)
	if best == nil {
		return bt.StatusFailure
	}
	board.EscapeTo = best
	return bt.StatusSuccess
}

func actMoveToSafe(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	if board.EscapeTo == nil {
		return bt.StatusFailure
	}

	currentPos := getPlayerGrid(board.Player)

	// 如果安全点就是当前位置，不需要移动（原地不动是安全解）
	if *board.EscapeTo == currentPos {
		board.NextInput = core.Input{} // 不动
		board.LastDirection = DirNone
		board.DirectionFrames = 0
		return bt.StatusRunning
	}

	step, ok := nextStepToward(board.Game, currentPos, *board.EscapeTo)
	if !ok {
		return bt.StatusFailure
	}
	board.NextInput = inputTowardWithInertia(board.Player, step, board)
	return bt.StatusRunning
}

// findNearestSafeTimed 使用时间敏感的 BFS 查找安全点
// 考虑 AI 到达时该格是否仍然安全，以及路径上每一步是否安全
func findNearestSafeTimed(game *core.Game, danger *DangerField, start core.GridPos, currentFrame int32) *core.GridPos {
	// 安全余量：到达后还需要一些时间才能离开，所以需要额外的安全时间
	const safetyMargin = framesPerTile

	// 首先检查：原地不动是否安全？
	// 如果当前位置在所有炸弹爆炸后仍然安全，则不需要移动
	if danger.SafeAtFrame(start.GridX, start.GridY, currentFrame+int32(core.BombFuseFrames)+safetyMargin) {
		copyPos := start
		return &copyPos
	}

	type timedNode struct {
		pos   core.GridPos
		frame int32
	}

	queue := []timedNode{{start, currentFrame}}
	visited := make(map[core.GridPos]int32) // 记录到达该位置的最早帧
	visited[start] = currentFrame

	directions := []core.GridPos{
		{GridX: 0, GridY: -1}, // 上
		{GridX: 0, GridY: 1},  // 下
		{GridX: -1, GridY: 0}, // 左
		{GridX: 1, GridY: 0},  // 右
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		// 检查当前节点在到达时是否安全（路径安全检查）
		// 起点特殊处理：即使起点危险也需要从这里开始逃跑
		if cur.pos != start {
			// 必须在到达时安全，且有足够时间离开
			if !danger.SafeAtFrame(cur.pos.GridX, cur.pos.GridY, cur.frame+safetyMargin) {
				continue // 这条路径不安全，不再从这里继续探索
			}
			// 如果这个位置到达时完全安全（不会被任何炸弹波及），可以作为目标
			if danger.SafeAtFrame(cur.pos.GridX, cur.pos.GridY, cur.frame+int32(core.BombFuseFrames)) {
				copyPos := cur.pos
				return &copyPos
			}
		}

		// 探索相邻格子
		for _, d := range directions {
			nx := cur.pos.GridX + d.GridX
			ny := cur.pos.GridY + d.GridY

			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				continue
			}

			if !isWalkable(game, nx, ny) {
				continue
			}

			npos := core.GridPos{GridX: nx, GridY: ny}
			arriveFrame := cur.frame + framesPerTile

			// 关键检查：到达该位置时是否安全（不会在到达前爆炸）
			if !danger.SafeAtFrame(nx, ny, arriveFrame) {
				continue // 到达时已经爆炸了，不能走这条路
			}

			// 只有当我们能更早到达该位置时才重新访问
			if prevFrame, exists := visited[npos]; exists && prevFrame <= arriveFrame {
				continue
			}

			// 限制搜索深度（最多搜索炸弹引爆时间 + 一些余量）
			if arriveFrame-currentFrame > int32(core.BombFuseFrames)*2 {
				continue
			}

			visited[npos] = arriveFrame
			queue = append(queue, timedNode{npos, arriveFrame})
		}
	}

	return nil
}

// inputTowardWithInertia 生成移动输入，带有方向惯性以减少抖动
func inputTowardWithInertia(player *core.Player, next core.GridPos, board *Blackboard) core.Input {
	pos := getPlayerGrid(player)
	dx := next.GridX - pos.GridX
	dy := next.GridY - pos.GridY

	// 计算理想方向
	var idealDir int
	if absInt(dx) > absInt(dy) {
		if dx > 0 {
			idealDir = DirRight
		} else if dx < 0 {
			idealDir = DirLeft
		}
	} else if dy != 0 {
		if dy > 0 {
			idealDir = DirDown
		} else {
			idealDir = DirUp
		}
	} else {
		// dx == 0 && dy == 0，已经到达目标
		idealDir = DirNone
	}

	// 方向惯性：如果上一个方向仍然可行且保持时间不长，继续保持
	const maxInertiaFrames = 8 // 最多保持 8 帧同一方向

	if board.LastDirection != DirNone &&
		board.DirectionFrames < maxInertiaFrames &&
		idealDir != DirNone {
		// 检查上一个方向是否仍然能让我们接近目标
		if canContinueDirection(board.LastDirection, pos, next) {
			board.DirectionFrames++
			return directionToInput(board.LastDirection)
		}
	}

	// 使用新方向
	board.LastDirection = idealDir
	board.DirectionFrames = 1
	return directionToInput(idealDir)
}

// canContinueDirection 检查继续当前方向是否有助于接近目标
func canContinueDirection(dir int, current, target core.GridPos) bool {
	switch dir {
	case DirUp:
		return target.GridY < current.GridY
	case DirDown:
		return target.GridY > current.GridY
	case DirLeft:
		return target.GridX < current.GridX
	case DirRight:
		return target.GridX > current.GridX
	}
	return false
}

// directionToInput 将方向常量转换为输入
func directionToInput(dir int) core.Input {
	switch dir {
	case DirUp:
		return core.Input{Up: true}
	case DirDown:
		return core.Input{Down: true}
	case DirLeft:
		return core.Input{Left: true}
	case DirRight:
		return core.Input{Right: true}
	}
	return core.Input{}
}

// inputToward 兼容旧接口（不带惯性）
func inputToward(player *core.Player, next core.GridPos) core.Input {
	pos := getPlayerGrid(player)
	input := core.Input{}
	dx := next.GridX - pos.GridX
	dy := next.GridY - pos.GridY

	if dx == 0 && dy == 0 {
		return input // 已到达，不移动
	}

	if absInt(dx) > absInt(dy) {
		if dx > 0 {
			input.Right = true
		} else if dx < 0 {
			input.Left = true
		}
	} else {
		if dy > 0 {
			input.Down = true
		} else if dy < 0 {
			input.Up = true
		}
	}

	return input
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
