package ai

import (
	"bomberman/pkg/core"
)

type AIController struct {
	PlayerID int
	bb       Blackboard
	tree     Node
	danger   DangerField
}

func NewAIController(playerID int) *AIController {
	c := &AIController{
		PlayerID: playerID,
	}

	// 初始化黑板
	c.bb.Danger = &c.danger

	// 构建行为树
	// Root Sequence: 先确保安全，再执行攻击

	// 1. 生存逻辑 (Sequence)
	// 如果 InDanger -> 执行 Escape
	//   -> Escape Running: 返回 Running (中断后续)
	//   -> Escape Success: 返回 Success (继续后续，说明已安全)
	//   -> Escape Failure: 返回 Failure (无路可逃?) -> 应该让它继续尝试或者Fail
	survivalSeq := &Sequence{
		Children: []Node{
			&Condition{Check: condIsInDanger},
			&Action{Do: actEscape},
		},
	}

	// 安全选择器 (Selector)
	// 尝试执行生存逻辑，如果不需要生存（InDanger=false），则默认为安全（Success）
	safetySelector := &Selector{
		Children: []Node{
			survivalSeq,
			// 如果不处于危险中（survivalSeq 失败），则返回 Success 继续执行攻击
			&Action{Do: func(bb *Blackboard) Status { return StatusSuccess }},
		},
	}

	// 2. 攻击逻辑 (Sequence)
	attackSeq := &Sequence{
		Children: []Node{
			&Condition{Check: condCanPlaceBomb},
			&Action{Do: actFindBrick},
			&Action{Do: actMoveToTarget},
			&Action{Do: actPlaceBomb},
		},
	}

	// 根节点：顺序执行 安全检查 -> 攻击
	c.tree = &Sequence{
		Children: []Node{
			safetySelector,
			attackSeq,
		},
	}

	return c
}

func (c *AIController) Decide(game *core.Game) core.Input {
	player := getPlayerByID(game, c.PlayerID)
	if player == nil || player.Dead {
		return core.Input{}
	}

	// 1. 重置黑板状态
	c.bb.ResetFrame(game, player)

	// 2. 更新感知 (DangerField)
	c.danger.Update(game)

	// 3. 执行行为树
	c.tree.Tick(&c.bb)

	// 4. 返回决策结果
	return c.bb.NextInput
}

func getPlayerByID(game *core.Game, id int) *core.Player {
	for _, p := range game.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// SetConfig 兼容旧接口，虽然我们暂时没用配置
func (c *AIController) SetConfig(cfg interface{}) {}
