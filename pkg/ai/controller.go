package ai

import (
	"math/rand"
	"time"

	"bomberman/pkg/core"
)

// AIController AI 控制器
type AIController struct {
	PlayerID        int
	changeDirTicker float64
	currentInput    core.Input
	rnd             *rand.Rand
}

// NewAIController 创建新的 AI 控制器
func NewAIController(playerID int) *AIController {
	return &AIController{
		PlayerID:        playerID,
		changeDirTicker: 0,
		// 使用基于 ID 的种子，保证同一局游戏中相同 ID 的 AI 行为有一定确定性（可选）
		// 或直接用 time.Now().UnixNano()
		rnd: rand.New(rand.NewSource(time.Now().UnixNano() + int64(playerID))),
	}
}

// Decide 根据当前游戏状态决定下一帧的输入
func (c *AIController) Decide(game *core.Game, deltaTime float64) core.Input {
	player := getPlayerByID(game, c.PlayerID)
	if player == nil || player.Dead {
		return core.Input{}
	}

	// 简单的随机游走逻辑：每隔一段时间改变一次方向
	c.changeDirTicker -= deltaTime
	if c.changeDirTicker <= 0 {
		c.changeDirTicker = 0.5 + c.rnd.Float64()*1.0 // 0.5 ~ 1.5 秒改变一次方向
		c.pickNewRandomDirection()
	}

	return c.currentInput
}

func (c *AIController) pickNewRandomDirection() {
	// 重置输入
	c.currentInput = core.Input{}

	// 随机选择一个方向 (0: 停止, 1: 上, 2: 下, 3: 左, 4: 右)
	// 稍微加大停止的概率
	choice := c.rnd.Intn(6)

	switch choice {
	case 1:
		c.currentInput.Up = true
	case 2:
		c.currentInput.Down = true
	case 3:
		c.currentInput.Left = true
	case 4:
		c.currentInput.Right = true
	case 5:
		// 偶尔尝试放炸弹 (1/6 概率)
		c.currentInput.Bomb = true
	}
}

func getPlayerByID(game *core.Game, playerID int) *core.Player {
	for _, player := range game.Players {
		if player.ID == playerID {
			return player
		}
	}
	return nil
}