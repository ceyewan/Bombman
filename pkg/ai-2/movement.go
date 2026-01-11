package ai2

import (
	"container/list"
	"math"

	"bomberman/pkg/core"
)

// FindPath BFS 寻路，返回从 start 到 end 的完整路径（不含 start）
func FindPath(game *core.Game, start, end core.GridPos) []core.GridPos {
	if start == end {
		return []core.GridPos{}
	}

	// 简单的 BFS
	queue := list.New()
	queue.PushBack([]core.GridPos{start})
	visited := make(map[core.GridPos]bool)
	visited[start] = true

	directions := []core.GridPos{{GridX: 0, GridY: -1}, {GridX: 0, GridY: 1}, {GridX: -1, GridY: 0}, {GridX: 1, GridY: 0}}

	for queue.Len() > 0 {
		path := queue.Remove(queue.Front()).([]core.GridPos)
		current := path[len(path)-1]

		if current == end {
			return path[1:] // 返回不含起点的路径
		}

		for _, d := range directions {
			next := core.GridPos{GridX: current.GridX + d.GridX, GridY: current.GridY + d.GridY}

			if next.GridX < 0 || next.GridX >= core.MapWidth || next.GridY < 0 || next.GridY >= core.MapHeight {
				continue
			}

			if !isWalkable(game, next) {
				continue
			}

			if !visited[next] {
				visited[next] = true
				newPath := make([]core.GridPos, len(path))
				copy(newPath, path)
				newPath = append(newPath, next)
				queue.PushBack(newPath)
			}
		}
	}

	return nil // 没找到路径
}

// isWalkable 检查格子是否可行走 (无墙、无砖、无未爆炸弹)
func isWalkable(game *core.Game, pos core.GridPos) bool {
	tile := game.Map.GetTile(pos.GridX, pos.GridY)
	if tile == core.TileWall || tile == core.TileBrick {
		return false
	}
	// 检查是否有炸弹（未爆炸的炸弹也是障碍）
	for _, b := range game.Bombs {
		if b.GridX == pos.GridX && b.GridY == pos.GridY && !b.Exploded {
			return false
		}
	}
	return true
}

// MoveAlongPath 根据路径计算当前帧的输入
func MoveAlongPath(player *core.Player, path []core.GridPos) (core.Input, []core.GridPos) {
	if len(path) == 0 {
		return core.Input{}, path
	}

	targetGrid := path[0]
	currentGrid := core.PlayerXYToGrid(int(player.X), int(player.Y))

	// 如果已经到达目标格子（或非常接近），移除该节点
	if currentGrid == targetGrid {
		// 递归处理下一个节点
		return MoveAlongPath(player, path[1:])
	}

	// 计算当前格子的中心点
	// 假设 TileSize=32, Player=26x26
	// 格子中心: Grid*32 + 16
	// 玩家中心: PlayerX + 13
	// 我们希望 PlayerCenter 对齐 GridCenter
	// 即 PlayerX + 13 = GridX*32 + 16 => PlayerX = GridX*32 + 3
	idealX := float64(currentGrid.GridX*core.TileSize + (core.TileSize-player.Width)/2)
	idealY := float64(currentGrid.GridY*core.TileSize + (core.TileSize-player.Height)/2)

	// 对齐容差 (像素)
	const tolerance = 2.0

	dx := targetGrid.GridX - currentGrid.GridX
	dy := targetGrid.GridY - currentGrid.GridY

	input := core.Input{}

	// X 轴移动：需要 Y 轴对齐
	if dx != 0 {
		if math.Abs(player.Y-idealY) > tolerance {
			// Y 不对齐，先修正 Y
			if player.Y < idealY {
				input.Down = true
			} else {
				input.Up = true
			}
			return input, path
		}
		// Y 已对齐，执行 X 轴移动
		if dx > 0 {
			input.Right = true
		} else {
			input.Left = true
		}
		return input, path
	}

	// Y 轴移动：需要 X 轴对齐
	if dy != 0 {
		if math.Abs(player.X-idealX) > tolerance {
			// X 不对齐，先修正 X
			if player.X < idealX {
				input.Right = true
			} else {
				input.Left = true
			}
			return input, path
		}
		// X 已对齐，执行 Y 轴移动
		if dy > 0 {
			input.Down = true
		} else {
			input.Up = true
		}
		return input, path
	}

	// 理论上 dx, dy 不会同时为 0 (前面已经处理了 current==target)
	// 也不应该同时不为 0 (曼哈顿距离移动，除非斜向，但 BFS 只产生上下左右)
	return core.Input{}, path
}
