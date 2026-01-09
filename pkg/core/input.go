package core

// Input 表示一帧内玩家的输入
type Input struct {
	Up    bool
	Down  bool
	Left  bool
	Right bool
	Bomb  bool
}

// ApplyInput 将输入应用到指定玩家
// 速度现在是像素/帧，不再需要 deltaTime
func ApplyInput(game *Game, playerID int, input Input, currentFrame int32) bool {
	if game == nil {
		return false
	}

	player := getPlayerByID(game, playerID)
	if player == nil || player.Dead {
		return false
	}

	// 速度已经是像素/帧，直接使用
	speed := player.Speed
	moveX := 0.0
	moveY := 0.0

	if input.Up {
		moveY -= speed
	}
	if input.Down {
		moveY += speed
	}
	if input.Left {
		moveX -= speed
	}
	if input.Right {
		moveX += speed
	}

	// 斜向移动时进行归一化，避免速度变快
	if moveX != 0 && moveY != 0 {
		moveX *= 0.70710678
		moveY *= 0.70710678
	}

	// 保持单轴移动以兼容拐角修正逻辑
	if moveY != 0 {
		player.Move(0, moveY, game)
	}
	if moveX != 0 {
		player.Move(moveX, 0, game)
	}

	// 处理炸弹
	if input.Bomb {
		bomb := player.PlaceBomb(game, currentFrame)
		if bomb != nil {
			game.AddBomb(bomb)
			return true
		}
	}

	return false
}

// 根据玩家ID获取玩家对象
func getPlayerByID(game *Game, playerID int) *Player {
	for _, player := range game.Players {
		if player.ID == playerID {
			return player
		}
	}
	return nil
}
