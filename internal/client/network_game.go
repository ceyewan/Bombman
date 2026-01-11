package client

import (
	"log"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"
	"bomberman/pkg/core"
	"bomberman/pkg/protocol"

	"github.com/hajimehoshi/ebiten/v2"
)

// NetworkGameClient 联机游戏客户端（简化版）
type NetworkGameClient struct {
	game           *Game
	network        *NetworkClient
	playerID       int
	playersMap     map[int]*Player
	inputHistory   []inputFrame
	pendingInputs  []predictedInput
	nextInputFrame int32
	hasAuthState   bool
	authState      authoritativeState

	// 自适应参数
	lastAdaptiveUpdate time.Time

	// 重连状态
	reconnecting         bool
	reconnectDelay       time.Duration
	lastReconnectAttempt time.Time
}

type inputFrame struct {
	frameID     int32
	up, down    bool
	left, right bool
	bomb        bool
}

type predictedInput struct {
	seq         int32 // 输入序号，用于与服务器确认
	frameID     int32
	up, down    bool
	left, right bool
}

type authoritativeState struct {
	frameID          int32
	x, y             float64
	direction        core.DirectionType
	isMoving         bool
	lastProcessedSeq int32 // 服务器确认的最后输入序号
}

// NewNetworkGameClient 创建联机游戏客户端
func NewNetworkGameClient(network *NetworkClient, controlScheme ControlScheme) (*NetworkGameClient, error) {
	game := NewGameWithSeed(network.GetGameSeed())
	game.controlScheme = controlScheme

	// 客户端只渲染状态，不进行权威逻辑
	game.coreGame.IsAuthoritative = false

	client := &NetworkGameClient{
		game:           game,
		network:        network,
		playerID:       int(network.GetPlayerID()),
		playersMap:     make(map[int]*Player),
		reconnectDelay: 2 * time.Second, // 初始重连延迟 2 秒
	}

	return client, nil
}

// Update 更新网络游戏
func (ngc *NetworkGameClient) Update() error {
	now := time.Now()

	// 0. 检查连接状态，触发重连
	if !ngc.network.IsConnected() && !ngc.reconnecting {
		if ngc.network.CanReconnect() {
			// 检查是否可以尝试重连
			if ngc.lastReconnectAttempt.IsZero() || now.Sub(ngc.lastReconnectAttempt) >= ngc.reconnectDelay {
				go ngc.tryReconnect()
			}
		}
		// 连接断开时跳过更新
		return nil
	}

	// 0.1 如果正在重连，跳过更新
	if ngc.reconnecting {
		return nil
	}

	// 0. 更新自适应参数（每秒一次）
	if now.Sub(ngc.lastAdaptiveUpdate) >= time.Second {
		ngc.updateAdaptiveParams()
		ngc.lastAdaptiveUpdate = now
	}

	// 1. 接收服务器状态
	var latestState *gamev1.GameState
	for {
		state := ngc.network.ReceiveState()
		if state == nil {
			break
		}
		latestState = state
	}
	if latestState != nil {
		ngc.applyServerState(latestState)
	}

	// 2. 发送本地输入
	ngc.handleInput()

	if ngc.hasAuthState {
		ngc.reconcileLocalPlayer(ngc.authState)
		ngc.hasAuthState = false
	}

	// 2.1 更新远端玩家插值
	ngc.updateRemoteSmoothing()

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
	if state.MatchEndFrame > 0 {
		ngc.game.matchEndFrame = state.MatchEndFrame
	}

	activePlayers := make(map[int]struct{}, len(state.Players))
	serverTimeMs := ngc.network.EstimatedServerTimeMs()
	if serverTimeMs == 0 {
		serverTimeMs = time.Now().UnixMilli()
	}

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
			if !playerRenderer.isLocal {
				playerRenderer.smoother = NewRemoteSmoother()
			}
			ngc.playersMap[playerID] = playerRenderer
			ngc.game.players = append(ngc.game.players, playerRenderer)
			ngc.game.coreGame.AddPlayer(corePlayer)
			log.Printf("玩家 %d 加入游戏", playerID)
		}

		corePlayer := playerRenderer.corePlayer
		if playerID == ngc.playerID {
			corePlayer.X = protoPlayer.X
			corePlayer.Y = protoPlayer.Y
			corePlayer.Direction = protocol.ProtoDirectionToCore(protoPlayer.Direction)
			corePlayer.IsMoving = protoPlayer.IsMoving

			// 读取服务器确认的输入序号
			lastSeq := int32(0)
			if state.LastProcessedSeq != nil {
				lastSeq = state.LastProcessedSeq[int32(ngc.playerID)]
			}

			ngc.authState = authoritativeState{
				frameID:          state.FrameId,
				x:                protoPlayer.X,
				y:                protoPlayer.Y,
				direction:        corePlayer.Direction,
				isMoving:         protoPlayer.IsMoving,
				lastProcessedSeq: lastSeq,
			}
			ngc.hasAuthState = true
		} else if playerRenderer.smoother != nil {
			playerRenderer.smoother.AddStateSnapshot(
				serverTimeMs,
				protoPlayer.X,
				protoPlayer.Y,
				protocol.ProtoDirectionToCore(protoPlayer.Direction),
				protoPlayer.IsMoving,
			)
		}
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

func (ngc *NetworkGameClient) updateRemoteSmoothing() {
	serverTimeMs := ngc.network.EstimatedServerTimeMs()
	if serverTimeMs == 0 {
		serverTimeMs = time.Now().UnixMilli()
	}

	for _, player := range ngc.playersMap {
		if player.isLocal || player.smoother == nil {
			continue
		}
		player.smoother.UpdateInterpolation(serverTimeMs, player.corePlayer)
	}
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
	// 游戏结束时不发送输入
	if ngc.game.gameOver {
		return
	}

	localPlayer := ngc.playersMap[ngc.playerID]
	if localPlayer == nil || localPlayer.corePlayer.Dead {
		return
	}

	up, down, left, right, bomb := getInputState(ngc.game.controlScheme)
	serverFrame := ngc.network.EstimatedServerFrame()
	if serverFrame <= 0 {
		serverFrame = ngc.game.coreGame.CurrentFrame
	}
	// 使用自适应输入提前帧数
	inputLeadFrames := ngc.GetInputLeadFrames()
	desiredFrame := serverFrame + inputLeadFrames
	if ngc.nextInputFrame < desiredFrame {
		ngc.nextInputFrame = desiredFrame
	}
	targetFrame := ngc.nextInputFrame
	ngc.nextInputFrame++

	if len(ngc.inputHistory) > 0 && ngc.inputHistory[len(ngc.inputHistory)-1].frameID == targetFrame {
		last := &ngc.inputHistory[len(ngc.inputHistory)-1]
		last.up, last.down, last.left, last.right, last.bomb = up, down, left, right, bomb
	} else {
		ngc.inputHistory = append(ngc.inputHistory, inputFrame{
			frameID: targetFrame,
			up:      up,
			down:    down,
			left:    left,
			right:   right,
			bomb:    bomb,
		})
		if len(ngc.inputHistory) > InputBufferSize {
			ngc.inputHistory = ngc.inputHistory[len(ngc.inputHistory)-InputBufferSize:]
		}
	}

	// 发送输入并获取序号
	start := 0
	if len(ngc.inputHistory) > InputSendWindow {
		start = len(ngc.inputHistory) - InputSendWindow
	}
	inputs := make([]*gamev1.InputData, 0, len(ngc.inputHistory)-start)
	for _, item := range ngc.inputHistory[start:] {
		inputs = append(inputs, &gamev1.InputData{
			FrameId: item.frameID,
			Up:      item.up,
			Down:    item.down,
			Left:    item.left,
			Right:   item.right,
			Bomb:    item.bomb,
		})
	}
	seq := ngc.network.SendInputBatch(inputs)

	// 应用预测输入，记录 seq
	ngc.applyPredictedInput(seq, targetFrame, up, down, left, right)
}

func (ngc *NetworkGameClient) applyPredictedInput(seq int32, frameID int32, up, down, left, right bool) {
	// 如果最后一个 pending 的 seq 相同，则更新（同一帧多次调用）
	if len(ngc.pendingInputs) > 0 && ngc.pendingInputs[len(ngc.pendingInputs)-1].seq == seq {
		last := &ngc.pendingInputs[len(ngc.pendingInputs)-1]
		last.up, last.down, last.left, last.right = up, down, left, right
	} else {
		ngc.pendingInputs = append(ngc.pendingInputs, predictedInput{
			seq:     seq,
			frameID: frameID,
			up:      up,
			down:    down,
			left:    left,
			right:   right,
		})
		// 限制缓冲区大小
		if len(ngc.pendingInputs) > InputBufferSize {
			ngc.pendingInputs = ngc.pendingInputs[len(ngc.pendingInputs)-InputBufferSize:]
		}
	}

	local := ngc.playersMap[ngc.playerID]
	if local == nil {
		return
	}

	core.ApplyInput(ngc.game.coreGame, ngc.playerID, core.Input{
		Up:    up,
		Down:  down,
		Left:  left,
		Right: right,
		Bomb:  false,
	}, frameID)
}

func (ngc *NetworkGameClient) reconcileLocalPlayer(state authoritativeState) {
	local := ngc.playersMap[ngc.playerID]
	if local == nil || local.corePlayer == nil || local.corePlayer.Dead {
		return
	}

	// ===== 1. 使用 LastProcessedSeq 精确清理已确认的输入 =====
	if state.lastProcessedSeq > 0 && len(ngc.pendingInputs) > 0 {
		idx := 0
		for idx < len(ngc.pendingInputs) && ngc.pendingInputs[idx].seq <= state.lastProcessedSeq {
			idx++
		}
		if idx > 0 {
			ngc.pendingInputs = ngc.pendingInputs[idx:]
		}
	}

	// ===== 2. 从权威位置开始重放未确认的输入 =====
	// 保存当前预测位置用于平滑纠偏
	predictedX := local.corePlayer.X
	predictedY := local.corePlayer.Y

	// 设置为权威位置
	local.corePlayer.X = state.x
	local.corePlayer.Y = state.y
	local.corePlayer.Direction = state.direction
	local.corePlayer.IsMoving = state.isMoving

	// 重放未确认的输入
	for _, in := range ngc.pendingInputs {
		core.ApplyInput(ngc.game.coreGame, ngc.playerID, core.Input{
			Up:    in.up,
			Down:  in.down,
			Left:  in.left,
			Right: in.right,
			Bomb:  false,
		}, in.frameID)
	}

	// ===== 3. 纠偏平滑：如果误差小于阈值，使用 LERP 过渡 =====
	reconcileX := local.corePlayer.X
	reconcileY := local.corePlayer.Y

	dx := predictedX - reconcileX
	dy := predictedY - reconcileY
	errorDist := dx*dx + dy*dy // 使用平方避免开根号

	threshold := ReconciliationSmoothThreshold * ReconciliationSmoothThreshold
	if errorDist > 0 && errorDist < threshold {
		// 小误差：使用 LERP 从预测位置向纠偏位置平滑过渡
		// 新位置 = 预测位置 + (纠偏位置 - 预测位置) * factor
		local.corePlayer.X = predictedX + (reconcileX-predictedX)*ReconciliationSmoothFactor
		local.corePlayer.Y = predictedY + (reconcileY-predictedY)*ReconciliationSmoothFactor
	}
	// 大误差或零误差：直接使用 reconcileX/Y（已设置）
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
			winnerID := e.GameOver.WinnerId
			message := ngc.formatGameOverMessage(winnerID)
			ngc.game.SetGameOverMessage(message)
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

// ========== 自适应网络参数 ==========

// updateAdaptiveParams 根据网络状态自适应调整参数
func (ngc *NetworkGameClient) updateAdaptiveParams() {
	rtt := ngc.network.GetRTTAvg()
	if rtt <= 0 {
		return
	}

	// 计算自适应插值延迟
	interpolationDelay := ngc.calculateInterpolationDelay(rtt)

	// 计算自适应输入提前帧数
	inputLeadFrames := ngc.calculateInputLeadFrames(rtt)

	// 更新所有远端玩家的插值延迟
	updated := false
	for _, player := range ngc.playersMap {
		if !player.isLocal && player.smoother != nil {
			currentDelay := player.smoother.GetInterpolationDelay()
			if currentDelay != interpolationDelay {
				player.smoother.SetInterpolationDelay(interpolationDelay)
				updated = true
			}
		}
	}

	if updated {
		jitter := ngc.network.GetRTTJitter()
		log.Printf("[自适应] 插值延迟: %dms, 输入提前: %d帧 (RTT: %dms, 抖动: %dms)",
			interpolationDelay, inputLeadFrames, rtt, jitter)
	}
}

// calculateInterpolationDelay 计算自适应插值延迟
// 公式: RTT + 2*Jitter + 50ms 余量
func (ngc *NetworkGameClient) calculateInterpolationDelay(rtt int64) int64 {
	jitter := ngc.network.GetRTTJitter()
	delay := rtt + 2*jitter + 50

	// 限制在合理范围
	if delay < MinInterpolationDelayMs {
		return MinInterpolationDelayMs
	}
	if delay > MaxInterpolationDelayMs {
		return MaxInterpolationDelayMs
	}
	return delay
}

// calculateInputLeadFrames 计算自适应输入提前帧数
// 公式: RTT/16.6 + 1 (每帧约 16.6ms @ 60FPS)
func (ngc *NetworkGameClient) calculateInputLeadFrames(rtt int64) int32 {
	// 每帧约 16.6ms
	const msPerFrame = 1000.0 / 60.0
	leadFrames := int32(float64(rtt)/msPerFrame) + 1

	// 限制在合理范围
	if leadFrames < MinInputLeadFrames {
		return MinInputLeadFrames
	}
	if leadFrames > MaxInputLeadFrames {
		return MaxInputLeadFrames
	}
	return leadFrames
}

// GetInterpolationDelay 获取当前自适应插值延迟
func (ngc *NetworkGameClient) GetInterpolationDelay() int64 {
	rtt := ngc.network.GetRTTAvg()
	if rtt <= 0 {
		return DefaultInterpolationDelayMs
	}
	return ngc.calculateInterpolationDelay(rtt)
}

// GetInputLeadFrames 获取当前自适应输入提前帧数
func (ngc *NetworkGameClient) GetInputLeadFrames() int32 {
	rtt := ngc.network.GetRTTAvg()
	if rtt <= 0 {
		return DefaultInputLeadFrames
	}
	return ngc.calculateInputLeadFrames(rtt)
}

// ========== 重连 ==========

// tryReconnect 尝试重连到服务器
func (ngc *NetworkGameClient) tryReconnect() {
	ngc.reconnecting = true
	ngc.lastReconnectAttempt = time.Now()

	log.Printf("连接断开，正在尝试重连...")

	state, err := ngc.network.Reconnect()
	if err != nil {
		log.Printf("重连失败: %v", err)
		ngc.reconnecting = false
		// 指数退避：增加重连延迟，最大 30 秒
		ngc.reconnectDelay = ngc.reconnectDelay * 2
		if ngc.reconnectDelay > 30*time.Second {
			ngc.reconnectDelay = 30 * time.Second
		}
		return
	}

	// 重连成功，恢复延迟
	ngc.reconnectDelay = 2 * time.Second
	ngc.reconnecting = false

	log.Printf("重连成功！恢复游戏状态...")

	// 恢复游戏状态
	if state != nil {
		ngc.applyServerState(state)
	}
}

// formatGameOverMessage formats the game over message based on winner ID
func (ngc *NetworkGameClient) formatGameOverMessage(winnerID int32) string {
	if winnerID == -1 {
		return "Draw!"
	}

	// Find winner name
	for _, p := range ngc.game.coreGame.Players {
		if int32(p.ID) == winnerID {
			if winnerID == int32(ngc.playerID) {
				return "You Win!"
			}
			return "Player " + string(rune('A'+p.ID)) + " Wins!"
		}
	}

	if winnerID == int32(ngc.playerID) {
		return "You Win!"
	}
	return "Player " + string(rune('A'+winnerID)) + " Wins!"
}
