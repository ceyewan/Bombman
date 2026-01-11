package ai

import (
	"bomberman/pkg/ai/bt"
	"bomberman/pkg/core"
)

// 游荡方向持续帧数
const wanderDirectionFrames = 30 // 保持同一方向约 0.5 秒

func actWander(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	if board.RNG == nil {
		return bt.StatusFailure
	}

	pos := getPlayerGrid(board.Player)

	// 如果当前方向仍然可行且未超时，继续保持
	if board.WanderFrames > 0 && board.WanderDirection != DirNone {
		board.WanderFrames--
		// 检查当前方向是否仍然可行
		if canWanderInDirection(board.Game, board.Danger, pos, board.WanderDirection) {
			board.NextInput = directionToInput(board.WanderDirection)
			return bt.StatusRunning
		}
		// 方向不可行，需要换方向
		board.WanderDirection = DirNone
		board.WanderFrames = 0
	}

	// 选择一个安全且可行的方向
	safeDirections := getSafeWalkableDirections(board.Game, board.Danger, pos)
	if len(safeDirections) == 0 {
		// 没有安全方向，随机选一个可行方向
		walkable := getWalkableDirections(board.Game, pos)
		if len(walkable) == 0 {
			return bt.StatusRunning // 完全被困，不动
		}
		board.WanderDirection = walkable[board.RNG.Intn(len(walkable))]
	} else {
		board.WanderDirection = safeDirections[board.RNG.Intn(len(safeDirections))]
	}

	board.WanderFrames = wanderDirectionFrames
	board.NextInput = directionToInput(board.WanderDirection)
	return bt.StatusRunning
}

// canWanderInDirection 检查指定方向是否可以移动且安全
func canWanderInDirection(game *core.Game, danger *DangerField, pos core.GridPos, dir int) bool {
	nx, ny := pos.GridX, pos.GridY
	switch dir {
	case DirUp:
		ny--
	case DirDown:
		ny++
	case DirLeft:
		nx--
	case DirRight:
		nx++
	default:
		return false
	}

	if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
		return false
	}

	if !isWalkable(game, nx, ny) {
		return false
	}

	// 检查是否安全
	if danger.InDanger(nx, ny) {
		return false
	}

	return true
}

// getSafeWalkableDirections 获取所有安全且可行走的方向
func getSafeWalkableDirections(game *core.Game, danger *DangerField, pos core.GridPos) []int {
	directions := []int{DirUp, DirDown, DirLeft, DirRight}
	result := make([]int, 0, 4)

	for _, dir := range directions {
		if canWanderInDirection(game, danger, pos, dir) {
			result = append(result, dir)
		}
	}

	return result
}

// getWalkableDirections 获取所有可行走的方向（不考虑危险）
func getWalkableDirections(game *core.Game, pos core.GridPos) []int {
	directions := []int{DirUp, DirDown, DirLeft, DirRight}
	deltas := []core.GridPos{
		{GridX: 0, GridY: -1}, // 上
		{GridX: 0, GridY: 1},  // 下
		{GridX: -1, GridY: 0}, // 左
		{GridX: 1, GridY: 0},  // 右
	}
	result := make([]int, 0, 4)

	for i, d := range deltas {
		nx := pos.GridX + d.GridX
		ny := pos.GridY + d.GridY

		if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
			continue
		}

		if isWalkable(game, nx, ny) {
			result = append(result, directions[i])
		}
	}

	return result
}
