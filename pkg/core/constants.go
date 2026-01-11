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
	MapWidth     = ScreenWidth / TileSize  // 20
	MapHeight    = ScreenHeight / TileSize // 15
)

type DirectionType int

// ===== 方向 =====
const (
	DirUp DirectionType = iota
	DirDown
	DirLeft
	DirRight
)

// TileType 地图块类型
type TileType int

const (
	TileEmpty TileType = iota
	TileWall           // 不可破坏的墙
	TileBrick          // 可破坏的砖块
	TileDoor           // 门 (隐藏在砖块下，炸开后出现)
)

// ===== 游戏参数（帧为单位）=====
const (
	// 炸弹相关（帧 = 秒数 × TPS）
	BombFuseFrames           = 180 // 炸弹引爆时间：3秒 × 60 = 180帧
	BombExplosionFrames      = 30  // 爆炸持续时间：0.5秒 × 60 = 30帧
	BombPlacementDelayFrames = 12  // 炸弹放置防抖：0.2秒 × 60 = 12帧
	BombExplosionRange       = 2   // 默认爆炸范围：2格
	BombMaxCountDefault      = 2   // 默认可同时放置炸弹数

	// 玩家相关
	PlayerSpeedPerFrame = 2.0 // 像素/帧 = 120像素/秒 ÷ 60

	// AI 相关
	AIThinkIntervalFrames       = 6  // 100ms × 60 ≈ 6帧
	AIThinkIntervalDangerFrames = 2  // 危险时思考间隔 ≈ 2帧
	AIStuckThresholdFrames      = 60 // 卡住判定 = 1秒 × 60

	// 游戏流程
	GameStartCountdownFrames = 180       // 开始倒计时：3秒
	GameOverDelayFrames      = 300       // 结束延时：5秒
	MatchDurationFrames      = 120 * TPS // 对局时长：2分钟（<=0 关闭限时）
)

// ===== 玩家碰撞配置 =====
const (
	PlayerWidth               = TileSize - 6 // 碰撞盒宽度（留3像素边距）
	PlayerHeight              = TileSize - 6 // 碰撞盒高度
	PlayerMargin              = 2            // 碰撞检测内边距
	CornerCorrectionTolerance = 4            // 拐角修正容错（像素）
	SoftAlignFactor           = 0.6          // 软对齐比例（相对本帧移动距离）
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
