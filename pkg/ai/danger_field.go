package ai

import (
	"math"

	"bomberman/pkg/core"
)

type DangerField struct {
	Earliest [core.MapHeight][core.MapWidth]int32
	Level    [core.MapHeight][core.MapWidth]float64
}

const maxFrame = int32(math.MaxInt32)

func (df *DangerField) Update(game *core.Game) {
	current := game.CurrentFrame
	for y := 0; y < core.MapHeight; y++ {
		for x := 0; x < core.MapWidth; x++ {
			df.Earliest[y][x] = maxFrame
			df.Level[y][x] = 0.0
		}
	}

	bombs := game.Bombs
	actual := make(map[*core.Bomb]int32, len(bombs))
	for _, b := range bombs {
		actual[b] = b.ExplodeAtFrame
	}

	// Propagate chain explosions until stable.
	changed := true
	for changed {
		changed = false
		for _, b := range bombs {
			bFrame := actual[b]
			cells := b.GetExplosionCells(game.Map)
			for _, cell := range cells {
				for _, other := range bombs {
					if other == b {
						continue
					}
					if other.GridX == cell.GridX && other.GridY == cell.GridY {
						if actual[other] > bFrame {
							actual[other] = bFrame
							changed = true
						}
					}
				}
			}
		}
	}

	// Apply bomb danger to cells.
	for _, b := range bombs {
		cells := b.GetExplosionCells(game.Map)
		when := actual[b]
		for _, cell := range cells {
			if cell.GridX < 0 || cell.GridX >= core.MapWidth || cell.GridY < 0 || cell.GridY >= core.MapHeight {
				continue
			}
			if when < df.Earliest[cell.GridY][cell.GridX] {
				df.Earliest[cell.GridY][cell.GridX] = when
			}
		}
	}

	// Existing explosions are immediate danger.
	for _, exp := range game.Explosions {
		for _, cell := range exp.Cells {
			if cell.GridX < 0 || cell.GridX >= core.MapWidth || cell.GridY < 0 || cell.GridY >= core.MapHeight {
				continue
			}
			df.Earliest[cell.GridY][cell.GridX] = current
		}
	}

	// Convert earliest frames to danger level.
	for y := 0; y < core.MapHeight; y++ {
		for x := 0; x < core.MapWidth; x++ {
			earliest := df.Earliest[y][x]
			if earliest == maxFrame {
				df.Level[y][x] = 0.0
				continue
			}
			remaining := float64(earliest - current)
			if remaining <= 0 {
				df.Level[y][x] = 1.0
				continue
			}
			if remaining >= float64(core.BombFuseFrames) {
				df.Level[y][x] = 0.0
				continue
			}
			df.Level[y][x] = 1.0 - remaining/float64(core.BombFuseFrames)
		}
	}
}

func (df *DangerField) InDanger(x, y int) bool {
	if x < 0 || x >= core.MapWidth || y < 0 || y >= core.MapHeight {
		return true
	}
	return df.Level[y][x] > 0.05
}

func (df *DangerField) SafeAtFrame(x, y int, frame int32) bool {
	if x < 0 || x >= core.MapWidth || y < 0 || y >= core.MapHeight {
		return false
	}
	return frame < df.Earliest[y][x]
}
