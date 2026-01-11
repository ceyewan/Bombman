package ai

import (
	"bomberman/pkg/core"
)

// Blackboard 共享数据
type Blackboard struct {
	// 基础状态
	Game   *core.Game
	Player *core.Player
	Frame  int32

	// 智能感知
	Danger *DangerField

	// 行为状态
	Path          []core.GridPos // 当前规划的路径
	CurrentTarget *core.GridPos  // 当前最终目标（如某块砖或安全点）
	NextInput     core.Input     // 本帧的输入
}

// ResetFrame 重置每帧状态
func (bb *Blackboard) ResetFrame(game *core.Game, player *core.Player) {
	bb.Game = game
	bb.Player = player
	bb.Frame = game.CurrentFrame
	bb.NextInput = core.Input{}

	// BombJustPlaced 需要在逻辑处理完后重置，或者由 Action 显式设置
	// 这里不重置 BombJustPlaced，因为它可能跨帧（比如放置那一帧之后的思考）
	// 但实际上我们每一帧都会重新思考，所以应该由 PlaceBomb Action 设置为 true，
	// 然后下一帧 Survival Sequence 检测到它，处理完后设为 false。
}
