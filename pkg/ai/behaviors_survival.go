package ai

import (
	"bomberman/pkg/ai/bt"
	"bomberman/pkg/core"
)

func condInDanger(bb bt.Blackboard) bool {
	board := bb.(*Blackboard)
	pos := getPlayerGrid(board.Player)
	return board.Danger.InDanger(pos.GridX, pos.GridY)
}

func actFindSafe(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	start := getPlayerGrid(board.Player)
	best := findNearestSafe(board.Game, board.Danger, start)
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
	step, ok := nextStepToward(board.Game, getPlayerGrid(board.Player), *board.EscapeTo)
	if !ok {
		return bt.StatusFailure
	}
	board.NextInput = inputToward(board.Player, step)
	return bt.StatusRunning
}

func findNearestSafe(game *core.Game, danger *DangerField, start core.GridPos) *core.GridPos {
	queue := []core.GridPos{start}
	visited := make(map[core.GridPos]bool)
	visited[start] = true

	directions := []core.GridPos{{GridX: 0, GridY: -1}, {GridX: 0, GridY: 1}, {GridX: -1, GridY: 0}, {GridX: 1, GridY: 0}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if !(cur == start) && !danger.InDanger(cur.GridX, cur.GridY) {
			return &cur
		}

		for _, d := range directions {
			nx := cur.GridX + d.GridX
			ny := cur.GridY + d.GridY
			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				continue
			}
			npos := core.GridPos{GridX: nx, GridY: ny}
			if visited[npos] {
				continue
			}
			if !isWalkable(game, nx, ny) {
				continue
			}
			visited[npos] = true
			queue = append(queue, npos)
		}
	}

	return nil
}

func inputToward(player *core.Player, next core.GridPos) core.Input {
	pos := getPlayerGrid(player)
	input := core.Input{}
	dx := next.GridX - pos.GridX
	dy := next.GridY - pos.GridY

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
