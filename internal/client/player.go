package client

import (
"image/color"

"github.com/hajimehoshi/ebiten/v2"
"github.com/hajimehoshi/ebiten/v2/vector"
"bomberman/pkg/ai"
"bomberman/pkg/core"
)

// PlayerRenderer 玩家渲染器
type PlayerRenderer struct {
	corePlayer   *core.Player
	CharInfo     CharacterInfo
	AnimFrame    int
	AnimTime     float64
}

// Player 玩家（包含渲染和输入）
type Player struct {
corePlayer   *core.Player
renderer     *PlayerRenderer
aiController *ai.AIController
}

// NewPlayer 创建新玩家
func NewPlayer(g *Game, id int, x, y int, charType core.CharacterType, isSimulated bool) *Player {
corePlayer := core.NewPlayer(id, x, y, charType)
corePlayer.IsSimulated = isSimulated

renderer := &PlayerRenderer{
corePlayer: corePlayer,
CharInfo:   GetCharacterInfo(charType),
AnimFrame:  0,
AnimTime:   0,
}

p := &Player{
corePlayer: corePlayer,
renderer:   renderer,
}

if isSimulated {
p.aiController = ai.NewAIController(id)
}

return p
}

// Update 更新玩家状态（输入处理）
func (p *Player) Update(deltaTime float64, controlScheme ControlScheme, coreGame *core.Game) {
// 处理输入
if !p.corePlayer.Dead {
if !p.corePlayer.IsSimulated {
p.handleInput(deltaTime, controlScheme, coreGame)
} else if p.aiController != nil {
// AI 控制
input := p.aiController.Decide(coreGame, deltaTime)
core.ApplyInput(coreGame, p.corePlayer.ID, input, deltaTime)
}
}

// 更新动画
p.renderer.updateAnimation(deltaTime)
}

// handleInput 处理键盘输入
func (p *Player) handleInput(deltaTime float64, controlScheme ControlScheme, coreGame *core.Game) {
	// 炸弹按键
	var bombKeyPressed bool
	if controlScheme == ControlWASD {
		bombKeyPressed = ebiten.IsKeyPressed(ebiten.KeySpace)
	} else {
		bombKeyPressed = ebiten.IsKeyPressed(ebiten.KeyEnter)
	}

	if bombKeyPressed {
		bomb := p.corePlayer.PlaceBomb(coreGame)
		if bomb != nil {
			coreGame.AddBomb(bomb)
		}
	}

	// 移动距离
	moveDistance := p.corePlayer.Speed * deltaTime

	// 移动按键
	var upPressed, downPressed, leftPressed, rightPressed bool
	if controlScheme == ControlWASD {
		upPressed = ebiten.IsKeyPressed(ebiten.KeyW)
		downPressed = ebiten.IsKeyPressed(ebiten.KeyS)
		leftPressed = ebiten.IsKeyPressed(ebiten.KeyA)
		rightPressed = ebiten.IsKeyPressed(ebiten.KeyD)
	} else {
		upPressed = ebiten.IsKeyPressed(ebiten.KeyArrowUp)
		downPressed = ebiten.IsKeyPressed(ebiten.KeyArrowDown)
		leftPressed = ebiten.IsKeyPressed(ebiten.KeyArrowLeft)
		rightPressed = ebiten.IsKeyPressed(ebiten.KeyArrowRight)
	}

	// 尝试移动
	if upPressed {
		p.corePlayer.Move(0, -moveDistance, coreGame)
		p.corePlayer.Direction = core.DirUp
	}
	if downPressed {
		p.corePlayer.Move(0, moveDistance, coreGame)
		p.corePlayer.Direction = core.DirDown
	}
	if leftPressed {
		p.corePlayer.Move(-moveDistance, 0, coreGame)
		p.corePlayer.Direction = core.DirLeft
	}
	if rightPressed {
		p.corePlayer.Move(moveDistance, 0, coreGame)
		p.corePlayer.Direction = core.DirRight
	}
}

// Draw 绘制玩家
func (p *Player) Draw(screen *ebiten.Image) {
	if p.corePlayer.Dead {
		return
	}

	renderer := p.renderer
	player := p.corePlayer

	// 玩家尺寸
	size := float32(player.Width)
	offset := float32(6) / 2 // (TileSize - PlayerWidth) / 2

	px := float32(player.X) - offset
	py := float32(player.Y) - offset

	// 根据方向调整绘制
	var drawX, drawY float32
	var bodyWidth, bodyHeight float32

	// 身体尺寸（略小于碰撞盒）
	bodyWidth = size * 0.7
	bodyHeight = size * 0.7

	// 居中
	drawX = px + (size-bodyWidth)/2
	drawY = py + (size-bodyHeight)/2

	// 绘制身体
	vector.DrawFilledRect(screen, drawX, drawY, bodyWidth, bodyHeight, renderer.CharInfo.BodyColor, false)

	// 绘制轮廓（2像素宽）
	vector.StrokeRect(screen, drawX, drawY, bodyWidth, bodyHeight, 2, renderer.CharInfo.OutlineColor, false)

	// 绘制手部（根据动画帧）
	handSize := bodyWidth * 0.25
	handOffset := float32(0.0)
	if renderer.AnimFrame == 1 {
		handOffset = 2.0
	}

	// 左手
	vector.DrawFilledCircle(screen, drawX-handOffset-2, drawY+bodyHeight*0.6, handSize, renderer.CharInfo.HandColor, false)

	// 右手
	vector.DrawFilledCircle(screen, drawX+bodyWidth+handOffset+2, drawY+bodyHeight*0.6, handSize, renderer.CharInfo.HandColor, false)

	// 绘制脚（根据动画帧）
	footSize := bodyWidth * 0.3
	footOffset := float32(0.0)
	if renderer.AnimFrame == 1 {
		footOffset = 2.0
	}

	// 左脚
	vector.DrawFilledRect(screen, drawX+bodyWidth*0.2-footOffset, drawY+bodyHeight, footSize, footSize*0.6, renderer.CharInfo.ShoeColor, false)

	// 右脚
	vector.DrawFilledRect(screen, drawX+bodyWidth*0.6+footOffset, drawY+bodyHeight, footSize, footSize*0.6, renderer.CharInfo.ShoeColor, false)

	// 绘制眼睛（根据方向）
	eyeSize := bodyWidth * 0.15
	var eyeLeftX, eyeLeftY, eyeRightX, eyeRightY float32

	eyeY := drawY + bodyHeight*0.3
	eyeSpacing := bodyWidth * 0.2

	switch player.Direction {
	case core.DirUp:
		eyeLeftX = drawX + bodyWidth*0.3
		eyeLeftY = eyeY - 2
		eyeRightX = drawX + bodyWidth*0.7
		eyeRightY = eyeY - 2
	case core.DirDown:
		eyeLeftX = drawX + bodyWidth*0.3
		eyeLeftY = eyeY + 2
		eyeRightX = drawX + bodyWidth*0.7
		eyeRightY = eyeY + 2
	case core.DirLeft:
		eyeLeftX = drawX + bodyWidth*0.3 - eyeSpacing/2
		eyeLeftY = eyeY
		eyeRightX = drawX + bodyWidth*0.5 - eyeSpacing/2
		eyeRightY = eyeY
	case core.DirRight:
		eyeLeftX = drawX + bodyWidth*0.5 + eyeSpacing/2
		eyeLeftY = eyeY
		eyeRightX = drawX + bodyWidth*0.7 + eyeSpacing/2
		eyeRightY = eyeY
	}

	// 绘制眼睛（白色）
	vector.DrawFilledCircle(screen, eyeLeftX, eyeLeftY, eyeSize, color.RGBA{255, 255, 255, 255}, false)
	vector.DrawFilledCircle(screen, eyeRightX, eyeRightY, eyeSize, color.RGBA{255, 255, 255, 255}, false)

	// 绘制瞳孔（黑色）
	pupilSize := eyeSize * 0.5
	vector.DrawFilledCircle(screen, eyeLeftX, eyeLeftY, pupilSize, color.RGBA{0, 0, 0, 255}, false)
	vector.DrawFilledCircle(screen, eyeRightX, eyeRightY, pupilSize, color.RGBA{0, 0, 0, 255}, false)
}

// updateAnimation 更新动画
func (r *PlayerRenderer) updateAnimation(deltaTime float64) {
	if !r.corePlayer.IsMoving {
		r.AnimTime = 0
		r.AnimFrame = 0
		return
	}

	// 动画速度：每0.15秒切换一帧
	r.AnimTime += deltaTime
	if r.AnimTime >= 0.15 {
		r.AnimTime = 0
		r.AnimFrame = (r.AnimFrame + 1) % 2
	}
}

// Getter 方法用于兼容旧代码
func (p *Player) ID() int { return p.corePlayer.ID }
func (p *Player) X() float64 { return p.corePlayer.X }
func (p *Player) Y() float64 { return p.corePlayer.Y }
func (p *Player) Character() core.CharacterType { return p.corePlayer.Character }

// UpdateAnimation 更新动画（不处理输入）
func (p *Player) UpdateAnimation(deltaTime float64) {
	p.renderer.updateAnimation(deltaTime)
}

// NewPlayerFromCore 从 core.Player 创建 Player
func NewPlayerFromCore(corePlayer *core.Player) *Player {
	renderer := &PlayerRenderer{
		corePlayer: corePlayer,
		CharInfo:   GetCharacterInfo(corePlayer.Character),
		AnimFrame:  0,
		AnimTime:   0,
	}

	return &Player{
		corePlayer: corePlayer,
		renderer:   renderer,
	}
}
