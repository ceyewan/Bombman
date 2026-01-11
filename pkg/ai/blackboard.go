package ai

import (
	"math/rand"

	"bomberman/pkg/ai/bt"
	"bomberman/pkg/core"
)

type Blackboard struct {
	Game   *core.Game
	Player *core.Player
	RNG    *rand.Rand
	Danger *DangerField
	Config *AIConfig

	Frame int32

	Target       *core.GridPos
	EscapeTo     *core.GridPos
	NextInput    core.Input
	ForceThink   bool
	LastInDanger bool
	LastBombs    int

	// 方向惯性：减少抖动
	LastDirection   int // 上一帧方向：0=无, 1=上, 2=下, 3=左, 4=右
	DirectionFrames int // 保持该方向的帧数

	// 游荡方向
	WanderDirection int // 游荡时的方向
	WanderFrames    int // 游荡该方向的帧数
}

func (bb *Blackboard) ResetFrame(game *core.Game, player *core.Player) {
	bb.Game = game
	bb.Player = player
	bb.Frame = game.CurrentFrame
	bb.Target = nil
	// 注意：EscapeTo 不在这里清空，保持逃生目标的连续性
	// 只有在到达目标或目标不可行时才清空（在 actFindSafe 中处理）
	bb.NextInput = core.Input{}
	bb.ForceThink = false
	// 注意：LastDirection、DirectionFrames、WanderDirection、WanderFrames 不重置，保持跨帧连续性
}

func (bb *Blackboard) AsBT() bt.Blackboard {
	return bb
}
