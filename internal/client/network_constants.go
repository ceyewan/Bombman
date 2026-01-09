package client

// ===== 网络插值与预测配置（客户端专用）=====
const (
	// 默认插值缓冲延迟（毫秒）：当自适应未启用时使用
	// 值越大越平滑，但延迟感越强；通常 100ms 是较好的折中
	DefaultInterpolationDelayMs int64 = 100

	// 插值缓冲延迟范围（毫秒）
	MinInterpolationDelayMs int64 = 50   // 最小插值延迟
	MaxInterpolationDelayMs int64 = 500  // 最大插值延迟

	// 插值缓冲区大小：存储最近 N 个状态快照
	InterpolationBufferSize = 30

	// 航位推测最大时长（毫秒）：超过此时间未收到新状态则停止预测
	DeadReckoningMaxMs int64 = 250

	// 客户端预测：平滑纠错阈值（像素）
	// 误差小于此值时使用 Lerp 平滑纠正，大于时直接拉回
	ReconciliationSmoothThreshold = 8.0

	// 客户端预测：平滑纠错速度（每帧修正比例）
	ReconciliationSmoothFactor = 0.2

	// 输入缓冲区大小：存储未确认的输入用于重放
	InputBufferSize = 128

	// 默认输入提前帧数：当自适应未启用时使用
	DefaultInputLeadFrames = 2

	// 输入提前帧数范围
	MinInputLeadFrames = 1
	MaxInputLeadFrames = 6

	// 每次发送的输入条数
	InputSendWindow = 4
)
