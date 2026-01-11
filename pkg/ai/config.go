package ai

// AIConfig 定义 AI 的行为参数，用于控制 AI 的智力水平
type AIConfig struct {
	// ThinkIntervalFrames 思考间隔（帧），值越小 AI 反应越快
	ThinkIntervalFrames int

	// MistakeRate 随机失误率 (0.0-1.0)，值越高 AI 越容易犯错
	MistakeRate float64

	// FullChainRecursion 是否启用完整连锁爆炸计算
	// 开启时 AI 会精确计算连锁爆炸，关闭时只计算一层
	FullChainRecursion bool

	// PreferBricks 是否优先炸砖块而非追击敌人
	PreferBricks bool
}

// 预设配置：普通难度
var AIConfigNormal = AIConfig{
	ThinkIntervalFrames: 120,  // 2s
	MistakeRate:         0.05, // 5% 失误率
	FullChainRecursion:  false,
	PreferBricks:        true, // 优先炸砖块开路
}

// 预设配置：困难难度
var AIConfigHard = AIConfig{
	ThinkIntervalFrames: 60,  // 1s
	MistakeRate:         0.0, // 无失误
	FullChainRecursion:  true,
	PreferBricks:        false, // 敌人优先
}
