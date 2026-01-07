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

// 网络插值配置
const (
	// 插值缓冲延迟（毫秒）：远端玩家渲染时间滞后于服务器时间
	// 值越大越平滑，但延迟感越强；通常 100ms 是较好的折中
	InterpolationDelayMs = 100

	// 插值缓冲区大小：存储最近 N 个状态快照
	InterpolationBufferSize = 30

	// 航位推测最大时长（毫秒）：超过此时间未收到新状态则停止预测
	DeadReckoningMaxMs = 250

	// 客户端预测：平滑纠错阈值（像素）
	// 误差小于此值时使用 Lerp 平滑纠正，大于时直接拉回
	ReconciliationSmoothThreshold = 8.0

	// 客户端预测：平滑纠错速度（每帧修正比例）
	ReconciliationSmoothFactor = 0.2

	// 输入缓冲区大小：存储未确认的输入用于重放
	InputBufferSize = 128
)

// 炸弹配置（秒）
const (
	DefaultTimeToBombSeconds        = 2.0 // 2秒
	DefaultExplosionRange           = 3   // 爆炸范围（格子数）
	DefaultExplosionDurationSeconds = 0.5 // 500毫秒
	BombCooldownSeconds             = 0.2 // 200毫秒
)
