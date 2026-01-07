package client

import (
	"log"
	"math"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
	"bomberman/pkg/protocol"

	"github.com/hajimehoshi/ebiten/v2"
)

// PendingInput 待确认的输入（用于客户端预测重放）
type PendingInput struct {
	Seq   int32
	Up    bool
	Down  bool
	Left  bool
	Right bool
	Bomb  bool
}

// NetworkGameClient 联机游戏客户端（与现有 Game 兼容）
type NetworkGameClient struct {
	game       *Game // 复用现有的 Game 结构
	network    *NetworkClient
	playerID   int
	playersMap map[int]*Player // 玩家 ID -> Player

	// ========== 客户端预测相关 ==========
	pendingInputs       []PendingInput // 未被服务器确认的输入队列
	lastConfirmedSeq    int32          // 服务器已确认的最后输入序号
	lastServerX         float64        // 服务器确认的最后位置
	lastServerY         float64
	reconciliationDiffX float64 // 平滑纠错的残余差值
	reconciliationDiffY float64

	// ========== 插值缓冲相关 ==========
	serverTimeMs       int64 // 估算的服务器当前时间（毫秒）
	localTimeOffsetMs  int64 // 本地时间与服务器时间的差值
	lastStateReceiveMs int64 // 上次收到状态的本地时间
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
		game:               game,
		network:            network,
		playerID:           int(network.GetPlayerID()),
		playersMap:         make(map[int]*Player),
		pendingInputs:      make([]PendingInput, 0, core.InputBufferSize),
		lastConfirmedSeq:   0,
		serverTimeMs:       0,
		localTimeOffsetMs:  0,
		lastStateReceiveMs: 0,
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

	// 2. 处理本地输入：立即应用（客户端预测）并发送到服务器
	ngc.handleInputWithPrediction()

	// 3. 应用平滑纠错（Reconciliation 的残余差值）
	ngc.applySmoothReconciliation()

	// 4. 更新远端玩家的插值
	ngc.updateRemotePlayersInterpolation()

	// 5. 更新游戏逻辑（炸弹冷却等）
	ngc.game.coreGame.Update(1.0 / FPS)

	// 6. 同步渲染器
	ngc.game.syncRenderers()

	// 7. 更新玩家动画
	for _, player := range ngc.game.players {
		player.UpdateAnimation(1.0 / FPS)
	}

	// 8. 处理网络事件
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
	// 更新服务器时间估算
	localNow := time.Now().UnixMilli()
	if state.ServerTimeMs > 0 {
		ngc.serverTimeMs = state.ServerTimeMs
		ngc.localTimeOffsetMs = localNow - state.ServerTimeMs
	}
	ngc.lastStateReceiveMs = localNow

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
				// 本地玩家：不使用插值，由客户端预测控制
				corePlayer.IsSimulated = false
			} else {
				// 其他玩家：使用插值缓冲
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
			// ========== 本地玩家：客户端预测 + Reconciliation ==========
			ngc.reconcileLocalPlayer(corePlayer, protoPlayer, state)
		} else {
			// ========== 远端玩家：添加到插值缓冲区 ==========
			corePlayer.AddStateSnapshot(
				state.ServerTimeMs,
				protoPlayer.X,
				protoPlayer.Y,
				protocol.ProtoDirectionToCore(protoPlayer.Direction),
				protoPlayer.IsMoving,
			)
			corePlayer.Dead = protoPlayer.Dead
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

// reconcileLocalPlayer 本地玩家的状态校验和回滚
func (ngc *NetworkGameClient) reconcileLocalPlayer(corePlayer *core.Player, protoPlayer *gamev1.PlayerState, state *gamev1.ServerState) {
	// 获取服务器已确认的输入序号
	confirmedSeq := int32(0)
	if state.LastProcessedInputSeq != nil {
		if seq, ok := state.LastProcessedInputSeq[int32(ngc.playerID)]; ok {
			confirmedSeq = seq
		}
	}

	// 服务器权威位置
	serverX := protoPlayer.X
	serverY := protoPlayer.Y

	// 更新死亡、方向等非位置状态（这些不需要预测）
	corePlayer.Dead = protoPlayer.Dead
	corePlayer.Character = protocol.ProtoCharacterTypeToCore(protoPlayer.Character)

	// 如果死亡，直接同步位置
	if corePlayer.Dead {
		corePlayer.X = serverX
		corePlayer.Y = serverY
		ngc.pendingInputs = ngc.pendingInputs[:0]
		return
	}

	// 移除已确认的输入
	newPendingInputs := make([]PendingInput, 0, len(ngc.pendingInputs))
	for _, input := range ngc.pendingInputs {
		if input.Seq > confirmedSeq {
			newPendingInputs = append(newPendingInputs, input)
		}
	}
	ngc.pendingInputs = newPendingInputs
	ngc.lastConfirmedSeq = confirmedSeq

	// 计算预测误差
	predictedX := corePlayer.X
	predictedY := corePlayer.Y

	// 先回滚到服务器位置
	corePlayer.X = serverX
	corePlayer.Y = serverY

	// 重放未确认的输入
	for _, input := range ngc.pendingInputs {
		dx, dy := ngc.calculateMoveDelta(input)
		corePlayer.Move(dx, dy, ngc.game.coreGame)
	}

	// 重放后的位置
	replayedX := corePlayer.X
	replayedY := corePlayer.Y

	// 计算误差
	errorX := replayedX - predictedX
	errorY := replayedY - predictedY
	errorDist := math.Sqrt(errorX*errorX + errorY*errorY)

	if errorDist > core.ReconciliationSmoothThreshold {
		// 误差过大，直接拉回到重放后的位置
		// 位置已经是 replayedX, replayedY，无需修改
		ngc.reconciliationDiffX = 0
		ngc.reconciliationDiffY = 0
	} else if errorDist > 0.1 {
		// 小误差，使用平滑纠错
		// 保持当前预测位置，设置差值让后续帧平滑修正
		corePlayer.X = predictedX
		corePlayer.Y = predictedY
		ngc.reconciliationDiffX = replayedX - predictedX
		ngc.reconciliationDiffY = replayedY - predictedY
	}
	// 误差极小，无需任何纠正
}

// calculateMoveDelta 根据输入计算移动增量
func (ngc *NetworkGameClient) calculateMoveDelta(input PendingInput) (float64, float64) {
	deltaTime := 1.0 / FPS
	speed := core.PlayerDefaultSpeed * deltaTime

	dx, dy := 0.0, 0.0
	if input.Up {
		dy = -speed
	}
	if input.Down {
		dy = speed
	}
	if input.Left {
		dx = -speed
	}
	if input.Right {
		dx = speed
	}

	return dx, dy
}

// applySmoothReconciliation 应用平滑纠错
func (ngc *NetworkGameClient) applySmoothReconciliation() {
	if ngc.reconciliationDiffX == 0 && ngc.reconciliationDiffY == 0 {
		return
	}

	localPlayer := ngc.playersMap[ngc.playerID]
	if localPlayer == nil {
		return
	}

	corePlayer := localPlayer.corePlayer

	// 每帧修正一部分差值
	correctionX := ngc.reconciliationDiffX * core.ReconciliationSmoothFactor
	correctionY := ngc.reconciliationDiffY * core.ReconciliationSmoothFactor

	corePlayer.X += correctionX
	corePlayer.Y += correctionY

	ngc.reconciliationDiffX -= correctionX
	ngc.reconciliationDiffY -= correctionY

	// 残余差值很小时归零
	if math.Abs(ngc.reconciliationDiffX) < 0.1 && math.Abs(ngc.reconciliationDiffY) < 0.1 {
		ngc.reconciliationDiffX = 0
		ngc.reconciliationDiffY = 0
	}
}

// updateRemotePlayersInterpolation 更新远端玩家的插值
func (ngc *NetworkGameClient) updateRemotePlayersInterpolation() {
	// 估算当前服务器时间
	localNow := time.Now().UnixMilli()
	estimatedServerTime := localNow - ngc.localTimeOffsetMs

	for playerID, playerRenderer := range ngc.playersMap {
		if playerID == ngc.playerID {
			continue // 跳过本地玩家
		}

		corePlayer := playerRenderer.corePlayer
		if corePlayer.IsSimulated {
			corePlayer.UpdateInterpolation(estimatedServerTime)
		}
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

// handleInputWithPrediction 处理输入：立即应用（客户端预测）并发送到服务器
func (ngc *NetworkGameClient) handleInputWithPrediction() {
	// 获取本地玩家
	localPlayer := ngc.playersMap[ngc.playerID]
	if localPlayer == nil || localPlayer.corePlayer.Dead {
		return
	}

	// 获取输入状态
	up, down, left, right, bomb := getInputState(ngc.game.controlScheme)

	// 发送输入到服务器（会自动递增 seq）
	seq := ngc.network.SendInputWithSeq(up, down, left, right, bomb)

	// 只有有移动输入时才记录
	if up || down || left || right {
		// 保存到待确认队列
		pendingInput := PendingInput{
			Seq:   seq,
			Up:    up,
			Down:  down,
			Left:  left,
			Right: right,
			Bomb:  bomb,
		}
		ngc.pendingInputs = append(ngc.pendingInputs, pendingInput)

		// 限制队列大小
		if len(ngc.pendingInputs) > core.InputBufferSize {
			ngc.pendingInputs = ngc.pendingInputs[1:]
		}

		// 立即应用移动（客户端预测）
		dx, dy := ngc.calculateMoveDelta(pendingInput)
		corePlayer := localPlayer.corePlayer
		corePlayer.Move(dx, dy, ngc.game.coreGame)

		// 更新方向和移动状态
		if dx > 0 {
			corePlayer.Direction = core.DirRight
		} else if dx < 0 {
			corePlayer.Direction = core.DirLeft
		} else if dy > 0 {
			corePlayer.Direction = core.DirDown
		} else if dy < 0 {
			corePlayer.Direction = core.DirUp
		}
		corePlayer.IsMoving = true
	} else {
		localPlayer.corePlayer.IsMoving = false
	}
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
