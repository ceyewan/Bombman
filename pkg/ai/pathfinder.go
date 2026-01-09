package ai

import (
	"container/list"

	"bomberman/pkg/core"
)

type stepNode struct {
	Pos   core.GridPos
	Prev  *stepNode
	Frame int32
}

func getPlayerGrid(player *core.Player) core.GridPos {
	return core.PlayerXYToGrid(int(player.X), int(player.Y))
}

func isWalkable(game *core.Game, x, y int) bool {
	tile := game.Map.GetTile(x, y)
	if tile == core.TileWall || tile == core.TileBrick {
		return false
	}
	for _, b := range game.Bombs {
		if b.GridX == x && b.GridY == y && !b.Exploded {
			return false
		}
	}
	return true
}

func nextStepToward(game *core.Game, start, target core.GridPos) (core.GridPos, bool) {
	if start == target {
		return start, true
	}
	queue := list.New()
	visited := make(map[core.GridPos]bool)
	queue.PushBack(&stepNode{Pos: start})
	visited[start] = true

	directions := []core.GridPos{{GridX: 0, GridY: -1}, {GridX: 0, GridY: 1}, {GridX: -1, GridY: 0}, {GridX: 1, GridY: 0}}

	var targetNode *stepNode
	for queue.Len() > 0 {
		n := queue.Remove(queue.Front()).(*stepNode)
		if n.Pos == target {
			targetNode = n
			break
		}
		for _, d := range directions {
			nx := n.Pos.GridX + d.GridX
			ny := n.Pos.GridY + d.GridY
			npos := core.GridPos{GridX: nx, GridY: ny}
			if visited[npos] {
				continue
			}
			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				continue
			}
			if !isWalkable(game, nx, ny) {
				continue
			}
			visited[npos] = true
			queue.PushBack(&stepNode{Pos: npos, Prev: n})
		}
	}

	if targetNode == nil {
		return core.GridPos{}, false
	}

	for targetNode.Prev != nil && targetNode.Prev.Pos != start {
		targetNode = targetNode.Prev
	}
	return targetNode.Pos, true
}

func canEscapeAfterPlacement(game *core.Game, danger *DangerField, start core.GridPos, currentFrame int32) bool {
	queue := list.New()
	visited := make(map[core.GridPos]bool)
	queue.PushBack(&stepNode{Pos: start, Frame: currentFrame})

	directions := []core.GridPos{{GridX: 0, GridY: -1}, {GridX: 0, GridY: 1}, {GridX: -1, GridY: 0}, {GridX: 1, GridY: 0}}

	for queue.Len() > 0 {
		n := queue.Remove(queue.Front()).(*stepNode)
		if n.Frame-currentFrame > int32(core.BombFuseFrames) {
			continue
		}
		if visited[n.Pos] {
			continue
		}
		visited[n.Pos] = true

		// If the position is safe when we arrive, accept.
		if danger.SafeAtFrame(n.Pos.GridX, n.Pos.GridY, n.Frame) {
			return true
		}

		for _, d := range directions {
			nx := n.Pos.GridX + d.GridX
			ny := n.Pos.GridY + d.GridY
			nextFrame := n.Frame + 1
			if nextFrame-currentFrame > int32(core.BombFuseFrames) {
				continue
			}
			if nx < 0 || nx >= core.MapWidth || ny < 0 || ny >= core.MapHeight {
				continue
			}
			if !isWalkable(game, nx, ny) {
				continue
			}
			npos := core.GridPos{GridX: nx, GridY: ny}
			queue.PushBack(&stepNode{Pos: npos, Frame: nextFrame})
		}
	}

	return false
}
