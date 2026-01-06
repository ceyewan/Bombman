package core

// 屏幕和地图配置
const (
	ScreenWidth  = 640
	ScreenHeight = 480
	TileSize     = 32
	MapWidth     = 20
	MapHeight    = 15
)

// 游戏帧率
const (
	FPS             = 60
	FixedDeltaTime  = 1.0 / FPS
)

// 玩家配置
const (
	PlayerDefaultSpeed = 120.0 // 像素/秒
	PlayerWidth        = TileSize - 6 // 碰撞盒宽度（留3像素边距）
	PlayerHeight       = TileSize - 6 // 碰撞盒高度
	PlayerMargin       = 1            // 碰撞检测内边距
)

// 炸弹配置
const (
	DefaultTimeToBomb       = 2 * 1000 * 1000 * 1000 // 2秒（纳秒）
	DefaultExplosionRange   = 3                       // 爆炸范围（格子数）
	DefaultExplosionDuration = 500 * 1000 * 1000     // 500毫秒（纳秒）
	BombCooldown            = 200 * 1000 * 1000       // 200毫秒（纳秒）
)
