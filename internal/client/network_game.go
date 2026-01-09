package client

import (
	"log"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
	"bomberman/pkg/protocol"

	"github.com/hajimehoshi/ebiten/v2"
)

// NetworkGameClient 联机游戏客户端（简化版）
type NetworkGameClient struct {
	game       *Game
	network    *NetworkClient
	playerID   int
	playersMap map[int]*Player
}

// NewNetworkGameClient 创建联机游戏客户端
func NewNetworkGameClient(network *NetworkClient, controlScheme ControlScheme) (*NetworkGameClient, error) {
	game := NewGameWithSeed(network.GetGameSeed())
	game.controlScheme = controlScheme

	// 客户端只渲染状态，不进行权威逻辑
	game.coreGame.IsAuthoritative = false

	client := &NetworkGameClient{
		game:       game,
		network:    network,
		playerID:   int(network.GetPlayerID()),
		playersMap: make(map[int]*Player),
	}

	return client, nil
}

// Update 更新网络游戏
func (ngc *NetworkGameClient) Update() error {
	// 1. 接收服务器状态
	state := ngc.network.ReceiveState()
	if state != nil {
		ngc.applyServerState(state)
	}

	// 2. 发送本地输入
	ngc.handleInput()

	// 3. 同步渲染器
	ngc.game.syncRenderers()

	// 4. 更新玩家动画
	for _, player := range ngc.game.players {
		player.UpdateAnimation(core.FrameSeconds)
	}

	// 5. 处理事件
	ngc.handleNetworkEvents()

	return nil
}

// Draw 绘制游戏
func (ngc *NetworkGameClient) Draw(screen *ebiten.Image) {
	ngc.game.Draw(screen)
}

// Layout 设置布局
func (ngc *NetworkGameClient) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ngc.game.Layout(outsideWidth, outsideHeight)
}

// applyServerState 应用服务器状态
func (ngc *NetworkGameClient) applyServerState(state *gamev1.GameState) {
	ngc.game.coreGame.CurrentFrame = state.FrameId

	activePlayers := make(map[int]struct{}, len(state.Players))

	for _, protoPlayer := range state.Players {
		playerID := int(protoPlayer.Id)
		activePlayers[playerID] = struct{}{}

		playerRenderer, exists := ngc.playersMap[playerID]
		if !exists {
			corePlayer := protocol.ProtoPlayerToCore(protoPlayer)
			if corePlayer == nil {
				continue
			}
			playerRenderer = NewPlayerFromCore(corePlayer)
			playerRenderer.isLocal = playerID == ngc.playerID
			ngc.playersMap[playerID] = playerRenderer
			ngc.game.players = append(ngc.game.players, playerRenderer)
			ngc.game.coreGame.AddPlayer(corePlayer)
			log.Printf("玩家 %d 加入游戏", playerID)
		}

		corePlayer := playerRenderer.corePlayer
		corePlayer.X = protoPlayer.X
		corePlayer.Y = protoPlayer.Y
		corePlayer.Direction = protocol.ProtoDirectionToCore(protoPlayer.Direction)
		corePlayer.IsMoving = protoPlayer.IsMoving
		corePlayer.Dead = protoPlayer.Dead
		corePlayer.Character = protocol.ProtoCharacterTypeToCore(protoPlayer.Character)
		corePlayer.NextPlacementFrame = int32(protoPlayer.NextPlacementFrame)
		corePlayer.MaxBombs = int(protoPlayer.MaxBombs)
	}

	// 移除已不存在的玩家
	for playerID, playerRenderer := range ngc.playersMap {
		if _, ok := activePlayers[playerID]; ok {
			continue
		}

		for i, p := range ngc.game.players {
			if p == playerRenderer {
				ngc.game.players = append(ngc.game.players[:i], ngc.game.players[i+1:]...)
				break
			}
		}
		for i, p := range ngc.game.coreGame.Players {
			if p.ID == playerID {
				ngc.game.coreGame.Players = append(ngc.game.coreGame.Players[:i], ngc.game.coreGame.Players[i+1:]...)
				break
			}
		}
		delete(ngc.playersMap, playerID)
		log.Printf("玩家 %d 离开（状态同步）", playerID)
	}

	ngc.syncBombs(state.Bombs)
	ngc.syncExplosions(state.Explosions)
	ngc.applyTileChanges(state.TileChanges)
}

// syncBombs 同步炸弹
func (ngc *NetworkGameClient) syncBombs(protoBombs []*gamev1.BombState) {
	ngc.game.coreGame.Bombs = ngc.game.coreGame.Bombs[:0]
	for _, protoBomb := range protoBombs {
		bomb := protocol.ProtoBombToCore(protoBomb)
		if bomb != nil {
			ngc.game.coreGame.AddBomb(bomb)
		}
	}
}

// syncExplosions 同步爆炸
func (ngc *NetworkGameClient) syncExplosions(protoExplosions []*gamev1.ExplosionState) {
	ngc.game.coreGame.Explosions = ngc.game.coreGame.Explosions[:0]
	for _, protoExplosion := range protoExplosions {
		explosion := protocol.ProtoExplosionToCore(protoExplosion)
		if explosion != nil {
			ngc.game.coreGame.Explosions = append(ngc.game.coreGame.Explosions, explosion)
		}
	}
}

// applyTileChanges 同步地图变化
func (ngc *NetworkGameClient) applyTileChanges(changes []*gamev1.TileChange) {
	if len(changes) == 0 {
		return
	}
	for _, tc := range changes {
		ngc.game.coreGame.Map.SetTile(int(tc.X), int(tc.Y), core.TileType(tc.NewType))
	}
}

// handleInput 发送输入到服务器
func (ngc *NetworkGameClient) handleInput() {
	localPlayer := ngc.playersMap[ngc.playerID]
	if localPlayer == nil || localPlayer.corePlayer.Dead {
		return
	}

	up, down, left, right, bomb := getInputState(ngc.game.controlScheme)
	ngc.network.SendInput(ngc.game.coreGame.CurrentFrame, up, down, left, right, bomb)
}

// handleNetworkEvents 处理网络事件
func (ngc *NetworkGameClient) handleNetworkEvents() {
	for {
		event := ngc.network.ReceiveEvent()
		if event == nil {
			return
		}

		switch e := event.Event.(type) {
		case *gamev1.GameEvent_GameOver:
			ngc.game.gameOver = true
			log.Printf("游戏结束！获胜者: %d", e.GameOver.WinnerId)
		case *gamev1.GameEvent_PlayerLeft:
			playerID := int(e.PlayerLeft.PlayerId)
			if playerRenderer, exists := ngc.playersMap[playerID]; exists {
				for i, p := range ngc.game.players {
					if p == playerRenderer {
						ngc.game.players = append(ngc.game.players[:i], ngc.game.players[i+1:]...)
						break
					}
				}
				for i, p := range ngc.game.coreGame.Players {
					if p.ID == playerID {
						ngc.game.coreGame.Players = append(ngc.game.coreGame.Players[:i], ngc.game.coreGame.Players[i+1:]...)
						break
					}
				}
				delete(ngc.playersMap, playerID)
				log.Printf("玩家 %d 离开", playerID)
			}
		}
	}
}

// Close 关闭网络客户端
func (ngc *NetworkGameClient) Close() {
	if ngc.network != nil {
		ngc.network.Close()
	}
}

// getInputState 获取当前输入状态
func getInputState(scheme ControlScheme) (up, down, left, right, bomb bool) {
	if scheme == ControlWASD {
		up = ebiten.IsKeyPressed(ebiten.KeyW)
		down = ebiten.IsKeyPressed(ebiten.KeyS)
		left = ebiten.IsKeyPressed(ebiten.KeyA)
		right = ebiten.IsKeyPressed(ebiten.KeyD)
		bomb = ebiten.IsKeyPressed(ebiten.KeySpace)
	} else {
		up = ebiten.IsKeyPressed(ebiten.KeyArrowUp)
		down = ebiten.IsKeyPressed(ebiten.KeyArrowDown)
		left = ebiten.IsKeyPressed(ebiten.KeyArrowLeft)
		right = ebiten.IsKeyPressed(ebiten.KeyArrowRight)
		bomb = ebiten.IsKeyPressed(ebiten.KeyEnter)
	}
	return
}
