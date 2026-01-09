package ai

import (
	"bomberman/pkg/ai/bt"
	"bomberman/pkg/core"
)

func actWander(bb bt.Blackboard) bt.Status {
	board := bb.(*Blackboard)
	if board.RNG == nil {
		return bt.StatusFailure
	}

	// Change direction occasionally.
	if board.NextInput == (core.Input{}) {
		choice := board.RNG.Intn(4)
		switch choice {
		case 0:
			board.NextInput.Up = true
		case 1:
			board.NextInput.Down = true
		case 2:
			board.NextInput.Left = true
		case 3:
			board.NextInput.Right = true
		}
	}

	return bt.StatusRunning
}
