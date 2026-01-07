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
	FPS            = 60
	FixedDeltaTime = 1.0 / FPS
)

// 玩家配置
const (
	PlayerDefaultSpeed        = 120.0        // 像素/秒
	PlayerWidth               = TileSize - 6 // 碰撞盒宽度（留3像素边距）
	PlayerHeight              = TileSize - 6 // 碰撞盒高度
	PlayerMargin              = 1            // 碰撞检测内边距
	CornerCorrectionTolerance = 4            // 拐角修正容错（像素）
	SoftAlignFactor           = 0.6          // 软对齐比例（相对本帧移动距离）
)

// 炸弹配置（秒）
const (
	DefaultTimeToBombSeconds        = 2.0 // 2秒
	DefaultExplosionRange           = 3   // 爆炸范围（格子数）
	DefaultExplosionDurationSeconds = 0.5 // 500毫秒
	BombCooldownSeconds             = 0.2 // 200毫秒
)
