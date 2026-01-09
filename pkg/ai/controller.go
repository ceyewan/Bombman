package ai

import (
	"math/rand"
	"time"

	"bomberman/pkg/ai/bt"
	"bomberman/pkg/core"
)

type AIController struct {
	PlayerID int
	rnd      *rand.Rand

	thinkIntervalFrames int
	thinkCounter        int
	cachedInput         core.Input

	blackboard Blackboard
	tree       bt.Node
	danger     DangerField
}

func NewAIController(playerID int) *AIController {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(playerID)))
	controller := &AIController{
		PlayerID:            playerID,
		rnd:                 rnd,
		thinkIntervalFrames: core.AIThinkIntervalFrames,
	}

	controller.blackboard = Blackboard{
		RNG:    rnd,
		Danger: &controller.danger,
	}

	controller.tree = &bt.Selector{Children: []bt.Node{
		&bt.Sequence{Children: []bt.Node{
			&bt.Condition{Check: condInDanger},
			&bt.Action{Do: actFindSafe},
			&bt.Action{Do: actMoveToSafe},
		}},
		&bt.Sequence{Children: []bt.Node{
			&bt.Condition{Check: condHasBombCapacity},
			&bt.Action{Do: actFindTarget},
			&bt.Action{Do: actPreCheckEscape},
			&bt.Action{Do: actMoveToTarget},
			&bt.Action{Do: actPlaceBomb},
		}},
		&bt.Action{Do: actWander},
	}}

	return controller
}

func (c *AIController) Decide(game *core.Game) core.Input {
	player := getPlayerByID(game, c.PlayerID)
	if player == nil || player.Dead {
		return core.Input{}
	}

	c.blackboard.ResetFrame(game, player)

	bombCount := len(game.Bombs)
	inDanger := false
	pos := getPlayerGrid(player)
	if pos.GridX >= 0 && pos.GridX < core.MapWidth && pos.GridY >= 0 && pos.GridY < core.MapHeight {
		inDanger = c.danger.InDanger(pos.GridX, pos.GridY)
	}

	force := false
	if inDanger && !c.blackboard.LastInDanger {
		force = true
	}
	if bombCount != c.blackboard.LastBombs {
		force = true
	}
	c.blackboard.LastInDanger = inDanger
	c.blackboard.LastBombs = bombCount

	c.thinkCounter++
	if !force && c.thinkCounter < c.thinkIntervalFrames {
		return c.cachedInput
	}

	c.danger.Update(game)
	c.blackboard.ForceThink = force
	c.thinkCounter = 0
	c.blackboard.NextInput = core.Input{}

	_ = c.tree.Tick(c.blackboard.AsBT())
	c.cachedInput = c.blackboard.NextInput
	return c.cachedInput
}

func getPlayerByID(game *core.Game, playerID int) *core.Player {
	for _, player := range game.Players {
		if player.ID == playerID {
			return player
		}
	}
	return nil
}
