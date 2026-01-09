package client

import (
	"image/color"
	"math"

	"bomberman/pkg/core"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// BombRenderer 炸弹渲染器
type BombRenderer struct {
	Bomb *core.Bomb
}

// NewBombRenderer 创建炸弹渲染器
func NewBombRenderer(bomb *core.Bomb) *BombRenderer {
	return &BombRenderer{Bomb: bomb}
}

// Draw 绘制炸弹
func (b *BombRenderer) Draw(screen *ebiten.Image, currentFrame int32) {
	bomb := b.Bomb
	// 格子坐标转像素坐标
	centerOffset := float32(core.TileSize) / 2
	cx := float32(bomb.GridX*core.TileSize) + centerOffset
	cy := float32(bomb.GridY*core.TileSize) + centerOffset

	// 计算闪烁效果（使用帧）
	elapsedFrames := int(bomb.ExplodeAtFrame - currentFrame)
	elapsedFrames = core.BombFuseFrames - elapsedFrames
	if elapsedFrames < 0 {
		elapsedFrames = 0
	}
	ratio := 0.0
	if core.BombFuseFrames > 0 {
		ratio = float64(elapsedFrames) / float64(core.BombFuseFrames)
	}
	if ratio > 1 {
		ratio = 1
	}

	// 炸弹半径
	radius := float32(12)

	// 根据时间闪烁
	blink := math.Sin(float64(elapsedFrames) * 0.1) // 快速闪烁
	alpha := uint8(200 + 55*blink)

	// 炸弹主体（黑色）
	bombColor := color.RGBA{0, 0, 0, alpha}
	vector.FillCircle(screen, cx, cy, radius, bombColor, false)

	// 炸弹轮廓
	vector.StrokeCircle(screen, cx, cy, radius, 2,
		color.RGBA{50, 50, 50, 255}, false)

	// 引线（根据时间变短）
	fuseLength := float32(15 * (1 - ratio))
	if fuseLength > 0 {
		fuseX := cx - radius*0.5
		fuseY := cy - radius

		// 引线（棕色）
		vector.StrokeLine(screen, fuseX, fuseY, fuseX-fuseLength*0.5, fuseY-fuseLength,
			2, color.RGBA{139, 69, 19, 255}, false)

		// 引线火花（红色，闪烁）
		if blink > 0 {
			sparkX := fuseX - fuseLength*0.5
			sparkY := fuseY - fuseLength
			sparkColor := color.RGBA{255, uint8(100 + 155*blink), 0, 255}
			vector.DrawFilledCircle(screen, sparkX, sparkY, 3, sparkColor, false)
		}
	}

	// 如果接近爆炸，添加警告效果
	if ratio > 0.7 {
		warningAlpha := uint8((ratio - 0.7) / 0.3 * 100)
		warningRadius := radius + float32(10*(ratio-0.7)/0.3)
		vector.StrokeCircle(screen, cx, cy, warningRadius, 2,
			color.RGBA{255, 0, 0, warningAlpha}, false)
	}
}

// ExplosionRenderer 爆炸渲染器
type ExplosionRenderer struct {
	Explosion *core.Explosion
}

// NewExplosionRenderer 创建爆炸渲染器
func NewExplosionRenderer(explosion *core.Explosion) *ExplosionRenderer {
	return &ExplosionRenderer{Explosion: explosion}
}

// Draw 绘制爆炸效果
func (e *ExplosionRenderer) Draw(screen *ebiten.Image, currentFrame int32) {
	explosion := e.Explosion
	ratio := 0.0
	totalFrames := int(explosion.ExpiresAtFrame - explosion.CreatedAtFrame)
	if totalFrames > 0 {
		elapsed := int(currentFrame - explosion.CreatedAtFrame)
		ratio = float64(elapsed) / float64(totalFrames)
	}
	if ratio > 1 {
		ratio = 1
	}

	// 爆炸逐渐消失
	alpha := uint8(255 * (1 - ratio))

	for _, cell := range explosion.Cells {
		px := float32(cell.GridX * core.TileSize)
		py := float32(cell.GridY * core.TileSize)

		// 爆炸动画：从中心扩散
		scale := float32(0.3 + 0.7*math.Min(ratio*2, 1.0))
		offset := float32(core.TileSize) * (1 - scale) / 2

		// 火焰效果：黄色到红色渐变
		var explosionColor color.RGBA
		if ratio < 0.3 {
			// 初期：亮黄色
			explosionColor = color.RGBA{255, 255, 0, alpha}
		} else if ratio < 0.6 {
			// 中期：橙色
			explosionColor = color.RGBA{255, 165, 0, alpha}
		} else {
			// 后期：红色
			explosionColor = color.RGBA{255, 0, 0, alpha}
		}

		// 绘制爆炸主体
		vector.DrawFilledRect(screen, px+offset, py+offset,
			float32(core.TileSize)*scale, float32(core.TileSize)*scale,
			explosionColor, false)

		// 添加内部高亮（白色中心）
		if ratio < 0.5 {
			innerAlpha := uint8(200 * (1 - ratio*2))
			innerScale := scale * 0.6
			innerOffset := float32(core.TileSize) * (1 - innerScale) / 2
			vector.DrawFilledRect(screen, px+innerOffset, py+innerOffset,
				float32(core.TileSize)*innerScale, float32(core.TileSize)*innerScale,
				color.RGBA{255, 255, 255, innerAlpha}, false)
		}

		// 爆炸边缘效果
		vector.StrokeRect(screen, px+offset, py+offset,
			float32(core.TileSize)*scale, float32(core.TileSize)*scale,
			2, color.RGBA{255, 100, 0, alpha}, false)
	}
}
