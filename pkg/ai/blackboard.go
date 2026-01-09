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

	Frame int32

	Target       *core.GridPos
	EscapeTo     *core.GridPos
	NextInput    core.Input
	ForceThink   bool
	LastInDanger bool
	LastBombs    int
}

func (bb *Blackboard) ResetFrame(game *core.Game, player *core.Player) {
	bb.Game = game
	bb.Player = player
	bb.Frame = game.CurrentFrame
	bb.Target = nil
	bb.EscapeTo = nil
	bb.NextInput = core.Input{}
	bb.ForceThink = false
}

func (bb *Blackboard) AsBT() bt.Blackboard {
	return bb
}
