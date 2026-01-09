package client

import "bomberman/pkg/core"

// stateSnapshot 远端玩家状态快照（客户端插值缓冲）
type stateSnapshot struct {
	timestamp int64
	x, y      float64
	direction core.DirectionType
	isMoving  bool
}

// RemoteSmoother 远端玩家插值与航位推测
type RemoteSmoother struct {
	buffer              []stateSnapshot
	renderTimestamp     int64
	lastVelocityX       float64
	lastVelocityY       float64
	lastUpdateTimestamp int64
	interpolationDelayMs int64 // 当前插值延迟（可动态调整）
}

// NewRemoteSmoother 创建插值缓冲器
func NewRemoteSmoother() *RemoteSmoother {
	return &RemoteSmoother{
		buffer:              make([]stateSnapshot, 0, InterpolationBufferSize),
		interpolationDelayMs: DefaultInterpolationDelayMs,
	}
}

// SetInterpolationDelay 设置插值延迟（毫秒）
func (s *RemoteSmoother) SetInterpolationDelay(delayMs int64) {
	if delayMs < MinInterpolationDelayMs {
		delayMs = MinInterpolationDelayMs
	}
	if delayMs > MaxInterpolationDelayMs {
		delayMs = MaxInterpolationDelayMs
	}
	s.interpolationDelayMs = delayMs
}

// GetInterpolationDelay 获取当前插值延迟（毫秒）
func (s *RemoteSmoother) GetInterpolationDelay() int64 {
	return s.interpolationDelayMs
}

// AddStateSnapshot 添加状态快照到缓冲区
func (s *RemoteSmoother) AddStateSnapshot(timestamp int64, x, y float64, dir core.DirectionType, isMoving bool) {
	snapshot := stateSnapshot{
		timestamp: timestamp,
		x:         x,
		y:         y,
		direction: dir,
		isMoving:  isMoving,
	}

	// 计算速度（用于航位推测）
	if len(s.buffer) > 0 {
		last := s.buffer[len(s.buffer)-1]
		dt := float64(timestamp - last.timestamp)
		if dt > 0 {
			s.lastVelocityX = (x - last.x) / dt
			s.lastVelocityY = (y - last.y) / dt
		}
	}
	s.lastUpdateTimestamp = timestamp

	// 添加到缓冲区
	s.buffer = append(s.buffer, snapshot)

	// 限制缓冲区大小
	if len(s.buffer) > InterpolationBufferSize {
		s.buffer = s.buffer[1:]
	}
}

// UpdateInterpolation 更新插值位置（远端玩家每帧调用）
func (s *RemoteSmoother) UpdateInterpolation(serverTimeMs int64, corePlayer *core.Player) {
	if corePlayer == nil || len(s.buffer) == 0 {
		return
	}

	// 渲染时间 = 服务器时间 - 插值延迟（使用动态值）
	renderTime := serverTimeMs - s.interpolationDelayMs
	s.renderTimestamp = renderTime

	// 在缓冲区中找到 renderTime 两侧的快照
	var prev, next *stateSnapshot
	for i := 0; i < len(s.buffer)-1; i++ {
		if s.buffer[i].timestamp <= renderTime && s.buffer[i+1].timestamp >= renderTime {
			prev = &s.buffer[i]
			next = &s.buffer[i+1]
			break
		}
	}

	if prev != nil && next != nil {
		// 正常插值
		totalTime := float64(next.timestamp - prev.timestamp)
		if totalTime > 0 {
			alpha := float64(renderTime-prev.timestamp) / totalTime
			corePlayer.X = prev.x + (next.x-prev.x)*alpha
			corePlayer.Y = prev.y + (next.y-prev.y)*alpha
			corePlayer.Direction = next.direction
			corePlayer.IsMoving = next.isMoving
		}
	} else if len(s.buffer) > 0 {
		// 缓冲区不足或渲染时间超出范围，使用航位推测
		last := s.buffer[len(s.buffer)-1]
		timeSinceLast := serverTimeMs - last.timestamp

		if timeSinceLast <= DeadReckoningMaxMs {
			// 航位推测：基于最后速度预测位置
			corePlayer.X = last.x + s.lastVelocityX*float64(timeSinceLast)
			corePlayer.Y = last.y + s.lastVelocityY*float64(timeSinceLast)
			corePlayer.Direction = last.direction
			corePlayer.IsMoving = last.isMoving
		} else {
			// 超时，停止在最后已知位置
			corePlayer.X = last.x
			corePlayer.Y = last.y
			corePlayer.Direction = last.direction
			corePlayer.IsMoving = false
		}
	}

	// 清理过期快照（保留 renderTime 之前的最后一个）
	s.cleanupOldSnapshots(renderTime)
}

func (s *RemoteSmoother) cleanupOldSnapshots(renderTime int64) {
	// 找到最后一个 <= renderTime 的快照索引
	cutoff := -1
	for i := 0; i < len(s.buffer); i++ {
		if s.buffer[i].timestamp <= renderTime {
			cutoff = i
		} else {
			break
		}
	}

	// 保留 cutoff 及之后的快照（cutoff 用于插值的 prev）
	if cutoff > 0 {
		s.buffer = s.buffer[cutoff:]
	}
}
