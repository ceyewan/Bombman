package client

import (
	"log"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
	"bomberman/pkg/protocol"

	"github.com/hajimehoshi/ebiten/v2"
)

// NetworkGameClient 联机游戏客户端（与现有 Game 兼容）
type NetworkGameClient struct {
	game       *Game // 复用现有的 Game 结构
	network    *NetworkClient
	playerID   int
	playersMap map[int]*Player // 玩家 ID -> Player
}

// NewNetworkGameClient 创建联机游戏客户端
func NewNetworkGameClient(network *NetworkClient, controlScheme ControlScheme) (*NetworkGameClient, error) {
	// 创建游戏对象
	game := NewGame()
	game.controlScheme = controlScheme

	// 在网络模式下，客户端只负责渲染和预测移动，不处理游戏规则逻辑（如爆炸破坏地图）
	// 所有游戏状态变化都由服务器权威决定
	game.coreGame.IsAuthoritative = false

	client := &NetworkGameClient{
		game:       game,
		network:    network,
		playerID:   int(network.GetPlayerID()),
		playersMap: make(map[int]*Player),
	}

	// 使用服务器下发的初始地图
	initialMap := network.GetInitialMap()
	if initialMap != nil {
		client.syncMap(initialMap)
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

	// 2. 处理本地输入并发送到服务器
	ngc.handleInput()

	// 3. 更新游戏逻辑（用于插值）
	ngc.game.coreGame.Update(1.0 / FPS)

	// 4. 同步渲染器
	ngc.game.syncRenderers()

	// 5. 更新玩家动画（非输入）
	for _, player := range ngc.game.players {
		player.UpdateAnimation(1.0 / FPS)
	}

	// 6. 处理网络事件
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
func (ngc *NetworkGameClient) applyServerState(state *gamev1.ServerState) {
	activePlayers := make(map[int]struct{}, len(state.Players))

	// 同步玩家
	for _, protoPlayer := range state.Players {
		playerID := int(protoPlayer.Id)
		activePlayers[playerID] = struct{}{}

		// 查找或创建玩家
		playerRenderer, exists := ngc.playersMap[playerID]
		if !exists {
			// 新玩家
			corePlayer := protocol.ProtoPlayerToCore(protoPlayer)
			if corePlayer == nil {
				continue
			}

			if playerID == ngc.playerID {
				// 本地玩家：不插值
				corePlayer.IsSimulated = false
			} else {
				// 其他玩家：使用插值
				corePlayer.IsSimulated = true
			}

			playerRenderer = NewPlayerFromCore(corePlayer)
			ngc.playersMap[playerID] = playerRenderer
			ngc.game.players = append(ngc.game.players, playerRenderer)
			ngc.game.coreGame.AddPlayer(corePlayer)
			log.Printf("玩家 %d 加入游戏", playerID)
		}

		// 更新玩家状态
		corePlayer := playerRenderer.corePlayer
		if playerID == ngc.playerID {
			// 本地玩家：直接使用服务器权威状态
			corePlayer.X = protoPlayer.X
			corePlayer.Y = protoPlayer.Y
			corePlayer.Direction = protocol.ProtoDirectionToCore(protoPlayer.Direction)
			corePlayer.IsMoving = protoPlayer.IsMoving
			corePlayer.Dead = protoPlayer.Dead
			corePlayer.Character = protocol.ProtoCharacterTypeToCore(protoPlayer.Character)
		} else {
			// 其他玩家：使用插值
			corePlayer.SetNetworkPosition(protoPlayer.X, protoPlayer.Y)
			corePlayer.Dead = protoPlayer.Dead
			corePlayer.Direction = protocol.ProtoDirectionToCore(protoPlayer.Direction)
			corePlayer.IsMoving = protoPlayer.IsMoving
		}
	}

	// 移除服务器状态中已不存在的玩家
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

	// 同步炸弹
	ngc.syncBombs(state.Bombs)

	// 同步爆炸
	ngc.syncExplosions(state.Explosions)

	// 同步地图（如果有）
	if state.Map != nil {
		ngc.syncMap(state.Map)
	}
}

// syncBombs 同步炸弹
func (ngc *NetworkGameClient) syncBombs(protoBombs []*gamev1.BombState) {
	// 清空旧炸弹
	ngc.game.coreGame.Bombs = ngc.game.coreGame.Bombs[:0]

	// 添加新炸弹
	for _, protoBomb := range protoBombs {
		bomb := protocol.ProtoBombToCore(protoBomb)
		if bomb != nil {
			ngc.game.coreGame.AddBomb(bomb)
		}
	}
}

// syncExplosions 同步爆炸
func (ngc *NetworkGameClient) syncExplosions(protoExplosions []*gamev1.ExplosionState) {
	// 清空旧爆炸
	ngc.game.coreGame.Explosions = ngc.game.coreGame.Explosions[:0]

	// 添加新爆炸
	for _, protoExplosion := range protoExplosions {
		explosion := protocol.ProtoExplosionToCore(protoExplosion)
		if explosion != nil {
			ngc.game.coreGame.Explosions = append(ngc.game.coreGame.Explosions, explosion)

			// 增量同步地图：根据爆炸范围清理砖块
			// 因为客户端不再进行权威计算（IsAuthoritative=false），且服务器可能仅在初始时刻发送全量地图，
			// 所以我们需要依赖服务器发来的爆炸范围来即时更新本地地图状态。
			for _, cell := range explosion.Cells {
				if ngc.game.coreGame.Map.GetTile(cell.GridX, cell.GridY) == core.TileBrick {
					ngc.game.coreGame.Map.SetTile(cell.GridX, cell.GridY, core.TileEmpty)
				}
			}
		}
	}
}

// syncMap 同步地图
func (ngc *NetworkGameClient) syncMap(protoMap *gamev1.MapState) {
	gameMap, err := protocol.ProtoMapToCore(protoMap)
	if err != nil {
		log.Printf("同步地图失败: %v", err)
		return
	}

	ngc.game.coreGame.Map = gameMap
	ngc.game.mapRenderer = NewMapRenderer(gameMap)
}

// handleInput 处理输入并发送到服务器
func (ngc *NetworkGameClient) handleInput() {
	// 获取本地玩家
	localPlayer := ngc.playersMap[ngc.playerID]
	if localPlayer == nil || localPlayer.corePlayer.Dead {
		return
	}

	// 获取输入状态
	up, down, left, right, bomb := getInputState(ngc.game.controlScheme)

	// 发送输入到服务器
	ngc.network.SendInput(up, down, left, right, bomb)
}

// handleNetworkEvents 处理网络事件
func (ngc *NetworkGameClient) handleNetworkEvents() {
	// 玩家加入
	if playerJoin := ngc.network.ReceivePlayerJoin(); playerJoin != nil {
		corePlayer := protocol.ProtoPlayerToCore(playerJoin.Player)
		if corePlayer != nil {
			corePlayer.IsSimulated = true
			playerRenderer := NewPlayerFromCore(corePlayer)
			ngc.playersMap[int(corePlayer.ID)] = playerRenderer
			ngc.game.players = append(ngc.game.players, playerRenderer)
			ngc.game.coreGame.AddPlayer(corePlayer)
			log.Printf("玩家 %d 加入", corePlayer.ID)
		}
	}

	// 玩家离开
	if playerID := ngc.network.ReceivePlayerLeave(); playerID >= 0 {
		if playerRenderer, exists := ngc.playersMap[int(playerID)]; exists {
			// 从渲染列表移除
			for i, p := range ngc.game.players {
				if p == playerRenderer {
					ngc.game.players = append(ngc.game.players[:i], ngc.game.players[i+1:]...)
					break
				}
			}
			// 从游戏逻辑移除
			for i, p := range ngc.game.coreGame.Players {
				if p.ID == int(playerID) {
					ngc.game.coreGame.Players = append(ngc.game.coreGame.Players[:i], ngc.game.coreGame.Players[i+1:]...)
					break
				}
			}
			delete(ngc.playersMap, int(playerID))
			log.Printf("玩家 %d 离开", playerID)
		}
	}

	// 游戏结束
	if gameOver := ngc.network.ReceiveGameOver(); gameOver != nil {
		ngc.game.gameOver = true
		log.Printf("游戏结束！获胜者: %d", gameOver.WinnerId)
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
