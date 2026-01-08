package client

import (
	"image/color"
	"time"

	"bomberman/pkg/core"

	"github.com/hajimehoshi/ebiten/v2"
)

// Direction 重新导出
type Direction = core.Direction

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
	FPS          = core.FPS
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
	lastUpdateTime     time.Time
	controlScheme      ControlScheme
}

// NewGame 创建新游戏
func NewGame() *Game {
	selectedControl := ControlWASD

	coreGame := core.NewGame()

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

	// 计算delta time
	now := time.Now()
	deltaTime := now.Sub(g.lastUpdateTime).Seconds()
	g.lastUpdateTime = now

	// 更新核心游戏逻辑
	g.coreGame.Update(deltaTime)

	// 检查游戏是否结束
	if g.coreGame.IsGameOver() {
		g.gameOver = true
	}

	// 同步渲染器列表
	g.syncRenderers()

	// 更新玩家动画和输入
	for _, player := range g.players {
		player.Update(deltaTime, g.controlScheme, g.coreGame)
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
		renderer.Draw(screen)
	}

	// 绘制炸弹
	for _, renderer := range g.bombRenderers {
		renderer.Draw(screen)
	}

	// 绘制玩家
	for _, player := range g.players {
		player.Draw(screen)
	}

	// 游戏结束提示
	if g.gameOver {
		overlay := ebiten.NewImage(ScreenWidth, ScreenHeight)
		overlay.Fill(color.RGBA{0, 0, 0, 128})
		screen.DrawImage(overlay, nil)
	}
}

// Layout 设置屏幕布局
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ScreenWidth, ScreenHeight
}
