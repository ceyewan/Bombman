package ai2

import (
	"bomberman/pkg/core"
)

// DangerField 危险场：记录每个格子的危险等级
type DangerField struct {
	Level [core.MapHeight][core.MapWidth]float64 // 危险等级 0~1，0=安全，1=必死
}

// Update 更新危险场
func (df *DangerField) Update(game *core.Game) {
	// 1. 清空
	for y := 0; y < core.MapHeight; y++ {
		for x := 0; x < core.MapWidth; x++ {
			df.Level[y][x] = 0.0
		}
	}

	// 2. 标记炸弹危险区域
	for _, bomb := range game.Bombs {
		// 计算炸弹当前的危险度 (时间越近越危险)
		// 简单起见，只要有炸弹覆盖，就设为危险
		cells := bomb.GetExplosionCells(game.Map)
		for _, cell := range cells {
			if isValid(cell.GridX, cell.GridY) {
				// 标记为危险
				df.Level[cell.GridY][cell.GridX] = 1.0
			}
		}
	}

	// 3. 标记正在爆炸的区域 (绝对危险)
	for _, exp := range game.Explosions {
		for _, cell := range exp.Cells {
			if isValid(cell.GridX, cell.GridY) {
				df.Level[cell.GridY][cell.GridX] = 1.0
			}
		}
	}
}

// InDanger 检查某位置是否危险
func (df *DangerField) InDanger(x, y int) bool {
	if !isValid(x, y) {
		return true // 越界视为危险
	}
	return df.Level[y][x] > 0.0
}

// IsSafe 检查某位置是否安全
func (df *DangerField) IsSafe(x, y int) bool {
	return !df.InDanger(x, y)
}

func isValid(x, y int) bool {
	return x >= 0 && x < core.MapWidth && y >= 0 && y < core.MapHeight
}
