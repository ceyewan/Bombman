package client

import (
	"image/color"
	"time"

	"bomberman/pkg/core"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
)

// Direction 重新导出
type Direction = core.DirectionType

// 常量重新导出
const (
	DirDown  = core.DirDown
	DirUp    = core.DirUp
	DirLeft  = core.DirLeft
	DirRight = core.DirRight
)

// ControlScheme 按键方案
type ControlScheme int

const (
	ControlWASD  ControlScheme = iota // WASD + 空格键
	ControlArrow                      // 方向键+回车键
)

func (c ControlScheme) String() string {
	switch c {
	case ControlWASD:
		return "WASD+空格"
	case ControlArrow:
		return "方向键+回车"
	}
	return "未知"
}

// 常量重新导出
const (
	ScreenWidth  = core.ScreenWidth
	ScreenHeight = core.ScreenHeight
	FPS          = core.TPS
	TileSize     = core.TileSize
	MapWidth     = core.MapWidth
	MapHeight    = core.MapHeight
)

// Game 游戏主结构（Ebiten 游戏循环）
type Game struct {
	coreGame           *core.Game
	players            []*Player
	bombRenderers      []*BombRenderer
	explosionRenderers []*ExplosionRenderer
	mapRenderer        *MapRenderer
	gameOver           bool
	gameOverMessage    string
	lastUpdateTime     time.Time
	controlScheme      ControlScheme
}

// NewGame 创建新游戏
func NewGame() *Game {
	selectedControl := ControlWASD

	coreGame := core.NewGame(time.Now().UnixNano())

	g := &Game{
		coreGame:           coreGame,
		players:            make([]*Player, 0),
		bombRenderers:      make([]*BombRenderer, 0),
		explosionRenderers: make([]*ExplosionRenderer, 0),
		lastUpdateTime:     time.Now(),
		controlScheme:      selectedControl,
	}

	g.mapRenderer = NewMapRenderer(coreGame.Map)

	return g
}

func NewGameWithSeed(seed int64) *Game {
	selectedControl := ControlWASD

	coreGame := core.NewGame(seed)

	g := &Game{
		coreGame:           coreGame,
		players:            make([]*Player, 0),
		bombRenderers:      make([]*BombRenderer, 0),
		explosionRenderers: make([]*ExplosionRenderer, 0),
		lastUpdateTime:     time.Now(),
		controlScheme:      selectedControl,
	}

	g.mapRenderer = NewMapRenderer(coreGame.Map)

	return g
}

func (g *Game) AddPlayer(player *Player) {
	g.players = append(g.players, player)
	g.coreGame.AddPlayer(player.corePlayer)
}

func (g *Game) AddBomb(bomb *core.Bomb) {
	g.coreGame.AddBomb(bomb)
	g.bombRenderers = append(g.bombRenderers, NewBombRenderer(bomb))
}

// SetControlScheme 设置控制方案
func (g *Game) SetControlScheme(scheme ControlScheme) {
	g.controlScheme = scheme
}

// Update 更新游戏状态
func (g *Game) Update() error {
	if g.gameOver {
		return nil
	}

	// 更新核心游戏逻辑（不再需要 deltaTime）
	g.coreGame.Update()

	// 检查游戏是否结束
	if g.coreGame.IsGameOver() {
		g.gameOver = true
	}

	// 同步渲染器列表
	g.syncRenderers()

	// 更新玩家动画和输入
	for _, player := range g.players {
		player.Update(g.controlScheme, g.coreGame, g.coreGame.CurrentFrame)
	}

	return nil
}

// syncRenderers 同步渲染器列表
func (g *Game) syncRenderers() {
	// 同步炸弹渲染器
	g.bombRenderers = g.bombRenderers[:0]
	for _, bomb := range g.coreGame.Bombs {
		g.bombRenderers = append(g.bombRenderers, NewBombRenderer(bomb))
	}

	// 同步爆炸渲染器
	g.explosionRenderers = g.explosionRenderers[:0]
	for _, explosion := range g.coreGame.Explosions {
		g.explosionRenderers = append(g.explosionRenderers, NewExplosionRenderer(explosion))
	}
}

// Draw 绘制游戏画面
func (g *Game) Draw(screen *ebiten.Image) {
	// 绘制地图
	g.mapRenderer.Draw(screen)

	// 绘制爆炸效果
	for _, renderer := range g.explosionRenderers {
		renderer.Draw(screen, g.coreGame.CurrentFrame)
	}

	// 绘制炸弹
	for _, renderer := range g.bombRenderers {
		renderer.Draw(screen, g.coreGame.CurrentFrame)
	}

	// 绘制玩家
	for _, player := range g.players {
		player.Draw(screen)
	}

	// 游戏结束提示
	if g.gameOver {
		drawGameOverOverlay(screen, g.gameOverMessage)
	}
}

// SetGameOverMessage sets the game over message
func (g *Game) SetGameOverMessage(message string) {
	g.gameOverMessage = message
}

// Layout 设置屏幕布局
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ScreenWidth, ScreenHeight
}

// drawGameOverOverlay draws the game over overlay with message
func drawGameOverOverlay(screen *ebiten.Image, message string) {
	// Dim background
	overlay := ebiten.NewImage(ScreenWidth, ScreenHeight)
	overlay.Fill(color.RGBA{0, 0, 0, 160})

	// Draw panel
	panelWidth := 300
	panelHeight := 120
	panelX := (ScreenWidth - panelWidth) / 2
	panelY := (ScreenHeight - panelHeight) / 2

	// Panel background
	panel := ebiten.NewImage(panelWidth, panelHeight)
	panel.Fill(color.RGBA{30, 35, 45, 230})

	// Panel border
	borderColor := color.RGBA{100, 110, 130, 255}
	for i := 0; i < 3; i++ {
		overlayRect := ebiten.NewImage(panelWidth, panelHeight)
		overlayRect.Fill(color.RGBA{0, 0, 0, 0})
	}

	// Draw panel to overlay
	panelOp := &ebiten.DrawImageOptions{}
	panelOp.GeoM.Translate(float64(panelX), float64(panelY))
	overlay.DrawImage(panel, panelOp)

	// Draw overlay to screen
	screen.DrawImage(overlay, nil)

	// Draw panel border using vector
	vector.DrawFilledRect(panel, 0, 0, 2, float32(panelHeight), borderColor, false)
	vector.DrawFilledRect(panel, float32(panelWidth-2), 0, 2, float32(panelHeight), borderColor, false)
	vector.DrawFilledRect(panel, 0, 0, float32(panelWidth), 2, borderColor, false)
	vector.DrawFilledRect(panel, 0, float32(panelHeight-2), float32(panelWidth), 2, borderColor, false)

	// Draw panel again with border
	panelOp.GeoM.Translate(float64(panelX), float64(panelY))
	screen.DrawImage(panel, panelOp)

	// Draw "GAME OVER" title
	titleY := panelY + 24
	drawCenteredText(screen, "GAME OVER", ScreenWidth/2, titleY, color.RGBA{255, 100, 100, 255})

	// Draw message
	messageY := panelY + 56
	if message != "" {
		drawCenteredText(screen, message, ScreenWidth/2, messageY, color.RGBA{220, 230, 240, 255})
	} else {
		drawCenteredText(screen, "Press Enter to Continue", ScreenWidth/2, messageY, color.RGBA{150, 160, 175, 255})
	}
}

// drawCenteredText draws text centered at the given position
func drawCenteredText(screen *ebiten.Image, textStr string, centerX, y int, clr color.Color) {
	font := text.NewGoXFace(basicfont.Face7x13)
	textWidth := len(textStr) * 7
	x := centerX - textWidth/2

	options := &text.DrawOptions{}
	options.GeoM.Translate(float64(x), float64(y))
	options.ColorScale.ScaleWithColor(clr)
	text.Draw(screen, textStr, font, options)
}
