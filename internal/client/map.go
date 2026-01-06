package client

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"bomberman/pkg/core"
)

// MapRenderer 地图渲染器
type MapRenderer struct {
	GameMap *core.GameMap
}

// NewMapRenderer 创建地图渲染器
func NewMapRenderer(gameMap *core.GameMap) *MapRenderer {
	return &MapRenderer{GameMap: gameMap}
}

// Draw 绘制地图
func (m *MapRenderer) Draw(screen *ebiten.Image) {
	for y := 0; y < core.MapHeight; y++ {
		for x := 0; x < core.MapWidth; x++ {
			px := float32(x * core.TileSize)
			py := float32(y * core.TileSize)

			tile := m.GameMap.GetTile(x, y)
			var c color.Color
			switch tile {
			case core.TileEmpty:
				c = color.RGBA{34, 139, 34, 255} // 草地绿
			case core.TileWall:
				c = color.RGBA{80, 80, 80, 255} // 灰色墙
			case core.TileBrick:
				c = color.RGBA{205, 133, 63, 255} // 砖块棕色
			}

			// 绘制方块
			vector.DrawFilledRect(screen, px, py, core.TileSize, core.TileSize, c, false)

			// 绘制边框
			vector.StrokeRect(screen, px, py, core.TileSize, core.TileSize, 1, color.RGBA{0, 0, 0, 100}, false)

			// 为砖块添加纹理效果
			if tile == core.TileBrick {
				// 简单的横线模拟砖块纹理
				for i := 0; i < 3; i++ {
					lineY := py + float32(i*10+5)
					vector.StrokeLine(screen, px+2, lineY, px+core.TileSize-2, lineY, 1,
						color.RGBA{180, 118, 53, 255}, false)
				}
			}

			// 为墙壁添加纹理
			if tile == core.TileWall {
				// 十字纹理
				vector.StrokeLine(screen, px+core.TileSize/2, py+5, px+core.TileSize/2, py+core.TileSize-5,
					2, color.RGBA{60, 60, 60, 255}, false)
				vector.StrokeLine(screen, px+5, py+core.TileSize/2, px+core.TileSize-5, py+core.TileSize/2,
					2, color.RGBA{60, 60, 60, 255}, false)
			}
		}
	}
}

// GridToPlayerXY 格子位置转换为玩家所在的位置（保留此函数供外部使用）
func GridToPlayerXY(gridX, gridY int) (int, int) {
	return core.GridToPlayerXY(gridX, gridY)
}
