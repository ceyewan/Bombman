package client

import (
	"image/color"

	"bomberman/pkg/ai"
	"bomberman/pkg/core"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// PlayerRenderer 玩家渲染器
type PlayerRenderer struct {
	corePlayer *core.Player
	CharInfo   CharacterInfo
	AnimFrame  int
	AnimTime   float64
}

// Player 玩家（包含渲染和输入）
type Player struct {
	corePlayer   *core.Player
	renderer     *PlayerRenderer
	aiController *ai.AIController
	isLocal      bool
	smoother     *RemoteSmoother

	// 本地玩家渲染/模拟分离
	renderX, renderY  float64 // 渲染位置（平滑跟随模拟位置）
	renderInitialized bool    // 是否已初始化渲染位置
}

// NewPlayer 创建新玩家
func NewPlayer(g *Game, id int, x, y int, charType core.CharacterType, useAI bool) *Player {
	corePlayer := core.NewPlayer(id, x, y, charType)

	renderer := &PlayerRenderer{
		corePlayer: corePlayer,
		CharInfo:   GetCharacterInfo(charType),
		AnimFrame:  0,
		AnimTime:   0,
	}

	p := &Player{
		corePlayer: corePlayer,
		renderer:   renderer,
		isLocal:    !useAI,
	}

	if useAI {
		p.aiController = ai.NewAIController(id)
	}

	return p
}

// Update 更新玩家状态（输入处理）
func (p *Player) Update(controlScheme ControlScheme, coreGame *core.Game, currentFrame int32) {
	// 处理输入
	if !p.corePlayer.Dead {
		if p.isLocal {
			p.handleInput(controlScheme, coreGame, currentFrame)
		} else if p.aiController != nil {
			// AI 控制
			input := p.aiController.Decide(coreGame)
			core.ApplyInput(coreGame, p.corePlayer.ID, input, currentFrame)
		}
	}

	// 更新动画（使用固定时间步长）
	p.renderer.updateAnimation(core.FrameSeconds)
}

// UpdateAnimation 仅更新动画（用于网络客户端）
func (p *Player) UpdateAnimation(deltaTime float64) {
	p.renderer.updateAnimation(deltaTime)

	// 本地玩家更新渲染位置（平滑跟随模拟位置）
	if p.isLocal {
		p.updateRenderPosition()
	}
}

// updateRenderPosition 更新渲染位置，平滑跟随模拟位置
func (p *Player) updateRenderPosition() {
	if !p.renderInitialized {
		p.renderX = p.corePlayer.X
		p.renderY = p.corePlayer.Y
		p.renderInitialized = true
		return
	}

	// 计算误差
	dx := p.corePlayer.X - p.renderX
	dy := p.corePlayer.Y - p.renderY
	distSq := dx*dx + dy*dy

	// 阈值（平方）：超过此值则瞬间拉回，否则平滑跟随
	const snapThresholdSq = 64.0 // 8 像素

	if distSq > snapThresholdSq {
		// 大误差：瞬间跳到模拟位置
		p.renderX = p.corePlayer.X
		p.renderY = p.corePlayer.Y
	} else if distSq > 0.01 { // 避免微小抖动
		// 小误差：平滑跟随（每帧移动 30% 差距）
		const followFactor = 0.3
		p.renderX += dx * followFactor
		p.renderY += dy * followFactor
	}
}

// GetRenderPosition 获取渲染位置
func (p *Player) GetRenderPosition() (float64, float64) {
	if p.isLocal && p.renderInitialized {
		return p.renderX, p.renderY
	}
	return p.corePlayer.X, p.corePlayer.Y
}

// handleInput 处理键盘输入
func (p *Player) handleInput(controlScheme ControlScheme, coreGame *core.Game, currentFrame int32) {
	// 炸弹按键
	var bombKeyPressed bool
	if controlScheme == ControlWASD {
		bombKeyPressed = ebiten.IsKeyPressed(ebiten.KeySpace)
	} else {
		bombKeyPressed = ebiten.IsKeyPressed(ebiten.KeyEnter)
	}

	if bombKeyPressed {
		bomb := p.corePlayer.PlaceBomb(coreGame, currentFrame)
		if bomb != nil {
			coreGame.AddBomb(bomb)
		}
	}

	// 移动距离（现在是像素/帧）
	moveDistance := p.corePlayer.Speed

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

	// 获取渲染位置（本地玩家使用平滑位置）
	renderX, renderY := p.GetRenderPosition()

	// 玩家尺寸
	size := float32(player.Width)
	offset := float32(6) / 2 // (TileSize - PlayerWidth) / 2

	px := float32(renderX) - offset
	py := float32(renderY) - offset

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
func (p *Player) ID() int                       { return p.corePlayer.ID }
func (p *Player) X() float64                    { return p.corePlayer.X }
func (p *Player) Y() float64                    { return p.corePlayer.Y }
func (p *Player) Character() core.CharacterType { return p.corePlayer.Character }

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
		isLocal:    false,
	}
}
