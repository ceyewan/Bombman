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
func ApplyInput(game *Game, playerID int, input Input, deltaTime float64) bool {
	if game == nil {
		return false
	}

	player := getPlayerByID(game, playerID)
	if player == nil || player.Dead {
		return false
	}

	// 计算移动距离
	speed := player.Speed * deltaTime

	// 处理移动
	if input.Up {
		player.Move(0, -speed, game)
	}
	if input.Down {
		player.Move(0, speed, game)
	}
	if input.Left {
		player.Move(-speed, 0, game)
	}
	if input.Right {
		player.Move(speed, 0, game)
	}

	// 处理炸弹
	if input.Bomb {
		bomb := player.PlaceBomb(game)
		if bomb != nil {
			game.AddBomb(bomb)
			return true
		}
	}

	return false
}

func getPlayerByID(game *Game, playerID int) *Player {
	for _, player := range game.Players {
		if player.ID == playerID {
			return player
		}
	}
	return nil
}
