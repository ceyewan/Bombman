package client

// ===== 网络插值与预测配置（客户端专用）=====
const (
	// 插值缓冲延迟（毫秒）：远端玩家渲染时间滞后于服务器时间
	// 值越大越平滑，但延迟感越强；通常 100ms 是较好的折中
	InterpolationDelayMs int64 = 100

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
)
