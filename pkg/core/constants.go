package core

import "time"

// ===== 基础时间常量 =====
const (
	// 服务器 Tick 频率
	TPS           = 60                // Ticks Per Second
	FrameDuration = time.Second / TPS // 每帧时长 ≈ 16.67ms
	FrameSeconds  = 1.0 / TPS         // 每帧秒数（仅用于渲染等需要秒的场景）
)

// ===== 屏幕和地图配置 =====
const (
	ScreenWidth  = 640
	ScreenHeight = 480
	TileSize     = 32
	MapWidth     = 20
	MapHeight    = 15
)

// ===== 游戏参数（帧为单位）=====
const (
	// 炸弹相关（帧 = 秒数 × TPS）
	BombFuseFrames      = 120 // 2秒 × 60 = 120帧
	BombExplosionFrames = 30  // 0.5秒 × 60 = 30帧
	BombCooldownFrames  = 12  // 0.2秒 × 60 = 12帧

	// 玩家相关
	PlayerSpeedPerFrame = 2.0 // 像素/帧 = 120像素/秒 ÷ 60

	// AI 相关
	AIThinkIntervalFrames     = 6  // 100ms × 60 ≈ 6帧
	AIThinkIntervalDangerFrames = 2 // 危险时思考间隔 ≈ 2帧
	AIStuckThresholdFrames    = 60 // 卡住判定 = 1秒 × 60
)

// ===== 玩家碰撞配置 =====
const (
	PlayerWidth               = TileSize - 6 // 碰撞盒宽度（留3像素边距）
	PlayerHeight              = TileSize - 6 // 碰撞盒高度
	PlayerMargin              = 1            // 碰撞检测内边距
	CornerCorrectionTolerance = 4            // 拐角修正容错（像素）
	SoftAlignFactor           = 0.6          // 软对齐比例（相对本帧移动距离）
)

// ===== 网络插值配置 =====
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

// ===== 游戏流程常量（帧为单位）=====
const (
	GameStartCountdownFrames = 180 // 3秒 × 60
	GameOverDelayFrames      = 300 // 5秒 × 60
)

// ===== 辅助函数 =====

// SecondsToFrames 秒转帧（用于配置转换）
func SecondsToFrames(seconds float64) int {
	return int(seconds * TPS)
}

// FramesToSeconds 帧转秒（用于显示）
func FramesToSeconds(frames int) float64 {
	return float64(frames) / float64(TPS)
}

// FramesToMillis 帧转毫秒（用于网络延迟计算）
func FramesToMillis(frames int) int64 {
	return int64(frames) * 1000 / TPS
}

// ===== 向后兼容的常量（逐步弃用）=====
const (
	// @deprecated 请使用 BombFuseFrames * FrameSeconds
	DefaultTimeToBombSeconds = 2.0
	// @deprecated 请使用 BombExplosionFrames * FrameSeconds
	DefaultExplosionDurationSeconds = 0.5
	// @deprecated 请使用 BombCooldownFrames * FrameSeconds
	BombCooldownSeconds = 0.2
	// @deprecated 请使用 PlayerSpeedPerFrame / FrameSeconds
	PlayerDefaultSpeed = 120.0
	// @deprecated 请使用 TPS
	FPS = 60
	// @deprecated 请使用 FrameSeconds
	FixedDeltaTime = 1.0 / 60
	// @deprecated 请使用常量 DefaultExplosionRange
	DefaultExplosionRange = 3
)
