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
	config   *AIConfig

	thinkIntervalFrames int
	thinkCounter        int
	cachedInput         core.Input

	blackboard Blackboard
	tree       bt.Node
	danger     DangerField
}

// NewAIController 创建 AI 控制器，使用默认配置（普通难度）
func NewAIController(playerID int) *AIController {
	return NewAIControllerWithConfig(playerID, &AIConfigNormal)
}

// NewAIControllerWithConfig 创建 AI 控制器，使用指定配置
func NewAIControllerWithConfig(playerID int, config *AIConfig) *AIController {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(playerID)))

	if config == nil {
		config = &AIConfigNormal
	}

	controller := &AIController{
		PlayerID:            playerID,
		rnd:                 rnd,
		config:              config,
		thinkIntervalFrames: config.ThinkIntervalFrames,
	}

	controller.blackboard = Blackboard{
		RNG:    rnd,
		Danger: &controller.danger,
		Config: config,
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

	// 更新危险场
	c.danger.Update(game)

	// 如果刚放了炸弹，立即清空逃生目标，强制重新计算
	if c.cachedInput.Bomb {
		c.blackboard.EscapeTo = nil
	}

	c.blackboard.ForceThink = force
	c.thinkCounter = 0
	c.blackboard.NextInput = core.Input{}

	_ = c.tree.Tick(c.blackboard.AsBT())

	// 应用随机失误
	if c.config.MistakeRate > 0 && c.rnd.Float64() < c.config.MistakeRate {
		// 失误：随机改变输入或什么都不做
		switch c.rnd.Intn(3) {
		case 0:
			// 什么都不做
			c.blackboard.NextInput = core.Input{}
		case 1:
			// 随机方向
			dirs := []core.Input{
				{Up: true},
				{Down: true},
				{Left: true},
				{Right: true},
			}
			c.blackboard.NextInput = dirs[c.rnd.Intn(4)]
		case 2:
			// 保持原输入（不失误）
		}
	}

	c.cachedInput = c.blackboard.NextInput
	return c.cachedInput
}

// GetConfig 获取当前配置
func (c *AIController) GetConfig() *AIConfig {
	return c.config
}

// SetConfig 设置新配置
func (c *AIController) SetConfig(config *AIConfig) {
	if config == nil {
		return
	}
	c.config = config
	c.blackboard.Config = config
	c.thinkIntervalFrames = config.ThinkIntervalFrames
}

func getPlayerByID(game *core.Game, playerID int) *core.Player {
	for _, player := range game.Players {
		if player.ID == playerID {
			return player
		}
	}
	return nil
}
