package ai

import (
	"bomberman/pkg/ai/bt"
	"bomberman/pkg/core"
)

func condHasBombCapacity(bb bt.Blackboard) bool {
	board := bb.(*Blackboard)
	active := 0
	for _, b := range board.Game.Bombs {
		if b.OwnerID == board.Player.ID && !b.Exploded {
			active++
		}
	}
	return active < board.Player.MaxBombs
}

func actFindTarget(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	start := getPlayerGrid(board.Player)

	if target := findEnemyTarget(board.Game, board.Player, start); target != nil {
		board.Target = target
		return bt.StatusSuccess
	}

	if target := findBrickTarget(board.Game, start); target != nil {
		board.Target = target
		return bt.StatusSuccess
	}

	return bt.StatusFailure
}

func actPreCheckEscape(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	if board.Target == nil {
		return bt.StatusFailure
	}
	// If not at target yet, allow movement first.
	if *board.Target != getPlayerGrid(board.Player) {
		return bt.StatusSuccess
	}

	// Build a temporary danger field with a new bomb at current position.
	temp := DangerField{}
	temp.Update(board.Game)
	pos := getPlayerGrid(board.Player)
	bomb := core.NewBomb(pos.GridX, pos.GridY, board.Player.ID, board.Frame)
	bomb.ExplosionRange = board.Player.BombRange

	// Apply the hypothetical bomb impact.
	cells := bomb.GetExplosionCells(board.Game.Map)
	for _, cell := range cells {
		if cell.GridX < 0 || cell.GridX >= core.MapWidth || cell.GridY < 0 || cell.GridY >= core.MapHeight {
			continue
		}
		if bomb.ExplodeAtFrame < temp.Earliest[cell.GridY][cell.GridX] {
			temp.Earliest[cell.GridY][cell.GridX] = bomb.ExplodeAtFrame
		}
	}
	// Chain: if the new bomb triggers existing bombs earlier, apply their cells too.
	for _, other := range board.Game.Bombs {
		if other.Exploded {
			continue
		}
		for _, cell := range cells {
			if other.GridX == cell.GridX && other.GridY == cell.GridY {
				otherCells := other.GetExplosionCells(board.Game.Map)
				for _, oc := range otherCells {
					if oc.GridX < 0 || oc.GridX >= core.MapWidth || oc.GridY < 0 || oc.GridY >= core.MapHeight {
						continue
					}
					if bomb.ExplodeAtFrame < temp.Earliest[oc.GridY][oc.GridX] {
						temp.Earliest[oc.GridY][oc.GridX] = bomb.ExplodeAtFrame
					}
				}
				break
			}
		}
	}

	if !canEscapeAfterPlacement(board.Game, &temp, pos, board.Frame) {
		return bt.StatusFailure
	}

	return bt.StatusSuccess
}

func actMoveToTarget(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	if board.Target == nil {
		return bt.StatusFailure
	}
	pos := getPlayerGrid(board.Player)
	if *board.Target == pos {
		return bt.StatusSuccess
	}
	step, ok := nextStepToward(board.Game, pos, *board.Target)
	if !ok {
		return bt.StatusFailure
	}
	board.NextInput = inputToward(board.Player, step)
	return bt.StatusRunning
}

func actPlaceBomb(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	pos := getPlayerGrid(board.Player)
	if board.Target == nil || *board.Target != pos {
		return bt.StatusFailure
	}
	board.NextInput = core.Input{Bomb: true}
	return bt.StatusSuccess
}

func findEnemyTarget(game *core.Game, self *core.Player, start core.GridPos) *core.GridPos {
	bestDist := 9999
	var best *core.GridPos

	for _, p := range game.Players {
		if p.ID == self.ID || p.Dead {
			continue
		}
		enemyPos := getPlayerGrid(p)
		if alignedAndClear(game, start, enemyPos, self.BombRange) {
			// If already aligned, place here.
			copyPos := start
			return &copyPos
		}

		// Try to move to a cell aligned with enemy within bomb range.
		candidates := alignedCells(game, enemyPos, self.BombRange)
		for _, c := range candidates {
			d := manhattan(start, c)
			if d < bestDist {
				bestDist = d
				copyPos := c
				best = &copyPos
			}
		}
	}

	return best
}

func findBrickTarget(game *core.Game, start core.GridPos) *core.GridPos {
	queue := []core.GridPos{start}
	visited := make(map[core.GridPos]bool)
	visited[start] = true

	directions := []core.GridPos{{GridX: 0, GridY: -1}, {GridX: 0, GridY: 1}, {GridX: -1, GridY: 0}, {GridX: 1, GridY: 0}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		// Check nearby bricks to decide target cell.
		for _, d := range directions {
			nx := cur.GridX + d.GridX
			ny := cur.GridY + d.GridY
			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				continue
			}
			if game.Map.GetTile(nx, ny) == core.TileBrick {
				copyPos := cur
				return &copyPos
			}
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

func alignedAndClear(game *core.Game, from, to core.GridPos, rng int) bool {
	if from.GridX == to.GridX {
		dist := absInt(from.GridY - to.GridY)
		if dist > rng {
			return false
		}
		step := 1
		if to.GridY < from.GridY {
			step = -1
		}
		for y := from.GridY + step; y != to.GridY; y += step {
			tile := game.Map.GetTile(from.GridX, y)
			if tile == core.TileWall || tile == core.TileBrick {
				return false
			}
		}
		return true
	}

	if from.GridY == to.GridY {
		dist := absInt(from.GridX - to.GridX)
		if dist > rng {
			return false
		}
		step := 1
		if to.GridX < from.GridX {
			step = -1
		}
		for x := from.GridX + step; x != to.GridX; x += step {
			tile := game.Map.GetTile(x, from.GridY)
			if tile == core.TileWall || tile == core.TileBrick {
				return false
			}
		}
		return true
	}

	return false
}

func alignedCells(game *core.Game, target core.GridPos, rng int) []core.GridPos {
	cells := make([]core.GridPos, 0, rng*4)
	for i := 1; i <= rng; i++ {
		candidates := []core.GridPos{{GridX: target.GridX + i, GridY: target.GridY}, {GridX: target.GridX - i, GridY: target.GridY}, {GridX: target.GridX, GridY: target.GridY + i}, {GridX: target.GridX, GridY: target.GridY - i}}
		for _, c := range candidates {
			if c.GridX < 0 || c.GridX >= core.MapWidth || c.GridY < 0 || c.GridY >= core.MapHeight {
				continue
			}
			if !isWalkable(game, c.GridX, c.GridY) {
				continue
			}
			cells = append(cells, c)
		}
	}
	return cells
}

func manhattan(a, b core.GridPos) int {
	return absInt(a.GridX-b.GridX) + absInt(a.GridY-b.GridY)
}
