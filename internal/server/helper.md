### 3.3 心跳机制实现 ❌ **未实现**

**文件：`internal/server/connection.go`（新增部分）**

**说明**：连接管理已实现，但缺少心跳机制（Ping/Pong）。

```go
package server

import (
    "context"
    "sync/atomic"
    "time"

    gamev1 "bomberman/api/gen/bomberman/v1"
)

const (
    HeartbeatInterval = 5 * time.Second
    HeartbeatTimeout  = 15 * time.Second
)

// Connection 连接（新增心跳相关字段）
type Connection struct {
    // ... 原有字段 ...

    lastRecvTime  atomic.Value  // time.Time
    lastPingTime  atomic.Value  // time.Time
    rtt           atomic.Int64  // 往返时间（毫秒）
}

// startHeartbeat 启动心跳协程
func (c *Connection) startHeartbeat(ctx context.Context) {
    ticker := time.NewTicker(HeartbeatInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // 检查超时
            lastRecv := c.lastRecvTime.Load().(time.Time)
            if time.Since(lastRecv) > HeartbeatTimeout {
                log.Printf("连接 %d 心跳超时，断开", c.PlayerID)
                c.Close()
                return
            }

            // 发送 Ping
            c.sendPing()
        }
    }
}

// sendPing 发送心跳包
func (c *Connection) sendPing() {
    ping := &gamev1.Ping{
        ClientTime: time.Now().UnixMilli(),
    }
    c.lastPingTime.Store(time.Now())
    c.Send(gamev1.MessageType_MSG_PING, ping)
}

// handlePong 处理心跳响应
func (c *Connection) handlePong(pong *gamev1.Pong) {
    c.lastRecvTime.Store(time.Now())

    // 计算 RTT
    if pong.ClientTime > 0 {
        rtt := time.Now().UnixMilli() - pong.ClientTime
        c.rtt.Store(rtt)
    }
}

// GetRTT 获取往返时间
func (c *Connection) GetRTT() time.Duration {
    return time.Duration(c.rtt.Load()) * time.Millisecond
}

// onMessageReceived 收到任何消息时调用
func (c *Connection) onMessageReceived() {
    c.lastRecvTime.Store(time.Now())
}
```

---

---

## 🔧 五、Room 服务器重构

### 5.1 Room 结构重构

**文件：`internal/server/room.go`**

```go
package server

import (
    "context"
    "log"
    "sync"
    "time"

    gamev1 "bomberman/api/gen/bomberman/v1"
    "bomberman/pkg/ai"
    "bomberman/pkg/core"
    "bomberman/pkg/protocol"
)

// GamePhase 游戏阶段
type GamePhase int

const (
    PhaseWaiting GamePhase = iota
    PhaseCountdown
    PhasePlaying
    PhaseGameOver
)

// Room 游戏房间
type Room struct {
    ID     string
    ctx    context.Context
    cancel context.CancelFunc

    // 游戏状态
    game         *core.Game
    frameID      int32
    phase        GamePhase
    phaseFrames  int // 当前阶段剩余帧数

    // AI 控制器
    enableAI      bool
    aiControllers map[int32]*ai.AIController

    // 连接管理
    connections  map[int32]*Connection
    nextPlayerID int32
    mu           sync.RWMutex

    // 输入管理
    inputBuffer           map[int32]*InputBuffer
    lastProcessedInputSeq map[int32]int32

    // 地图变化追踪（用于增量同步）
    tileChanges     []core.TileChange
    tileChangeFrame int32

    // 通道
    joinCh   chan joinRequest
    leaveCh  chan int32
    inputCh  chan inputMessage
    closeCh  chan struct{}
}

// InputBuffer 输入缓冲区（支持客户端预测）
type InputBuffer struct {
    inputs    []InputRecord
    maxSize   int
    lastSeq   int32
}

// InputRecord 输入记录
type InputRecord struct {
    Seq         int32
    TargetFrame int32
    Input       core.Input
    ReceivedAt  time.Time
}

type joinRequest struct {
    conn   *Connection
    respCh chan joinResponse
}

type joinResponse struct {
    playerID int32
    success  bool
    err      error
}

type inputMessage struct {
    playerID int32
    input    *gamev1.ClientInput
}

// NewRoom 创建房间
func NewRoom(id string, seed int64, enableAI bool) *Room {
    ctx, cancel := context.WithCancel(context.Background())

    return &Room{
        ID:                    id,
        ctx:                   ctx,
        cancel:                cancel,
        game:                  core.NewGame(seed, true),
        frameID:               0,
        phase:                 PhaseWaiting,
        enableAI:              enableAI,
        aiControllers:         make(map[int32]*ai.AIController),
        connections:           make(map[int32]*Connection),
        nextPlayerID:          1,
        inputBuffer:           make(map[int32]*InputBuffer),
        lastProcessedInputSeq: make(map[int32]int32),
        tileChanges:           make([]core.TileChange, 0),
        joinCh:                make(chan joinRequest, 10),
        leaveCh:               make(chan int32, 10),
        inputCh:               make(chan inputMessage, 256),
        closeCh:               make(chan struct{}),
    }
}

// Run 房间主循环
func (r *Room) Run() {
    ticker := time.NewTicker(core.FrameDuration)
    defer ticker.Stop()

    log.Printf("房间 %s 启动", r.ID)

    for {
        select {
        case <-r.ctx.Done():
            log.Printf("房间 %s 关闭", r.ID)
            return

        case req := <-r.joinCh:
            r.handleJoin(req)

        case playerID := <-r.leaveCh:
            r.handleLeave(playerID)

        case msg := <-r.inputCh:
            r.handleInput(msg)

        case <-ticker.C:
            r.tick()
        }
    }
}

// tick 每帧更新
func (r *Room) tick() {
    r.frameID++

    switch r.phase {
    case PhaseWaiting:
        r.tickWaiting()
    case PhaseCountdown:
        r.tickCountdown()
    case PhasePlaying:
        r.tickPlaying()
    case PhaseGameOver:
        r.tickGameOver()
    }
}

// tickWaiting 等待阶段
func (r *Room) tickWaiting() {
    // 检查是否有足够玩家开始游戏
    r.mu.RLock()
    playerCount := len(r.connections)
    r.mu.RUnlock()

    if playerCount >= 2 || (playerCount >= 1 && r.enableAI) {
        r.startCountdown()
    }
}

// startCountdown 开始倒计时
func (r *Room) startCountdown() {
    r.phase = PhaseCountdown
    r.phaseFrames = core.GameStartCountdownFrames

    // 添加 AI 玩家
    if r.enableAI {
        r.addAIPlayers()
    }

    // 广播游戏开始事件
    r.broadcastEvent(&gamev1.GameEvent{
        FrameId: r.frameID,
        Event: &gamev1.GameEvent_GameStart{
            GameStart: &gamev1.GameStartEvent{
                CountdownFrames: int32(r.phaseFrames),
            },
        },
    })
}

// tickCountdown 倒计时阶段
func (r *Room) tickCountdown() {
    r.phaseFrames--

    if r.phaseFrames <= 0 {
        r.phase = PhasePlaying
        log.Printf("房间 %s 游戏开始", r.ID)
    }

    // 每帧广播状态
    r.broadcastState()
}

// tickPlaying 游戏进行阶段
func (r *Room) tickPlaying() {
    // 1. 应用玩家输入
    r.applyInputs()

    // 2. AI 决策
    r.updateAI()

    // 3. 更新游戏逻辑
    r.game.Update(r.frameID)

    // 4. 收集地图变化
    r.collectTileChanges()

    // 5. 检查游戏结束
    if r.game.IsGameOver() {
        r.endGame()
    }

    // 6. 广播状态
    r.broadcastState()
}

// tickGameOver 游戏结束阶段
func (r *Room) tickGameOver() {
    r.phaseFrames--

    if r.phaseFrames <= 0 {
        r.resetGame()
    }

    r.broadcastState()
}

// applyInputs 应用所有玩家输入
func (r *Room) applyInputs() {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for playerID, buffer := range r.inputBuffer {
        input := buffer.GetLatestInput()
        if input != nil {
            r.game.ApplyInput(int(playerID), input.Input)
            r.lastProcessedInputSeq[playerID] = input.Seq
        }
    }
}

// updateAI AI 决策
func (r *Room) updateAI() {
    for playerID, controller := range r.aiControllers {
        input := controller.Decide(r.game, r.frameID)
        r.game.ApplyInput(int(playerID), input)
    }
}

// handleJoin 处理玩家加入
func (r *Room) handleJoin(req joinRequest) {
    r.mu.Lock()
    defer r.mu.Unlock()

    // 检查房间是否已满
    if len(r.connections) >= MaxPlayers {
        req.respCh <- joinResponse{success: false, err: ErrRoomFull}
        return
    }

    // 分配玩家 ID
    playerID := r.nextPlayerID
    r.nextPlayerID++

    // 创建玩家
    startPos := r.getStartPosition(int(playerID))
    player := core.NewPlayer(int(playerID), startPos.X, startPos.Y)
    r.game.AddPlayer(player)

    // 注册连接
    r.connections[playerID] = req.conn
    r.inputBuffer[playerID] = NewInputBuffer(64)
    r.lastProcessedInputSeq[playerID] = 0

    req.conn.PlayerID = playerID

    log.Printf("玩家 %d 加入房间 %s", playerID, r.ID)

    req.respCh <- joinResponse{
        playerID: playerID,
        success:  true,
    }
}

// handleLeave 处理玩家离开
func (r *Room) handleLeave(playerID int32) {
    r.mu.Lock()
    defer r.mu.Unlock()

    delete(r.connections, playerID)
    delete(r.inputBuffer, playerID)
    delete(r.lastProcessedInputSeq, playerID)

    // 标记玩家死亡
    if player := r.game.GetPlayer(int(playerID)); player != nil {
        player.Dead = true
    }

    log.Printf("玩家 %d 离开房间 %s", playerID, r.ID)

    // 广播离开事件
    r.broadcastEvent(&gamev1.GameEvent{
        FrameId: r.frameID,
        Event: &gamev1.GameEvent_PlayerLeft{
            PlayerLeft: &gamev1.PlayerLeftEvent{
                PlayerId: playerID,
            },
        },
    })
}

// handleInput 处理玩家输入
func (r *Room) handleInput(msg inputMessage) {
    r.mu.Lock()
    defer r.mu.Unlock()

    buffer, ok := r.inputBuffer[msg.playerID]
    if !ok {
        return
    }

    buffer.AddInput(InputRecord{
        Seq:         msg.input.Seq,
        TargetFrame: msg.input.TargetFrame,
        Input:       protocol.InputFromProto(msg.input),
        ReceivedAt:  time.Now(),
    })
}

// broadcastState 广播游戏状态
func (r *Room) broadcastState() {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // 构建状态
    state := protocol.GameStateToProto(
        r.game,
        r.frameID,
        gamev1.GamePhase(r.phase),
        r.lastProcessedInputSeq,
        r.tileChanges,
    )

    // 清空地图变化
    r.tileChanges = r.tileChanges[:0]

    // 发送给所有连接
    for _, conn := range r.connections {
        conn.Send(gamev1.MessageType_MSG_GAME_STATE, state)
    }
}

// broadcastEvent 广播游戏事件
func (r *Room) broadcastEvent(event *gamev1.GameEvent) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for _, conn := range r.connections {
        conn.Send(gamev1.MessageType_MSG_GAME_EVENT, event)
    }
}

// addAIPlayers 添加 AI 玩家
func (r *Room) addAIPlayers() {
    currentPlayers := len(r.connections)
    aiCount := MaxPlayers - currentPlayers

    for i := 0; i < aiCount; i++ {
        playerID := r.nextPlayerID
        r.nextPlayerID++

        startPos := r.getStartPosition(int(playerID))
        player := core.NewPlayer(int(playerID), startPos.X, startPos.Y)
        r.game.AddPlayer(player)

        // 创建 AI 控制器
        level := ai.AILevelNormal
        if i == aiCount-1 {
            level = ai.AILevelHard // 最后一个 AI 更难
        }
        r.aiControllers[playerID] = ai.NewAIController(int(playerID), level, r.game.Seed)

        log.Printf("AI 玩家 %d (难度 %d) 加入房间 %s", playerID, level, r.ID)
    }
}

// getStartPosition 获取玩家出生点
func (r *Room) getStartPosition(playerID int) struct{ X, Y float64 } {
    positions := []struct{ X, Y float64 }{
        {float64(core.TileSize), float64(core.TileSize)},                                                     // 左上
        {float64(core.ScreenWidth - 2*core.TileSize), float64(core.TileSize)},                                // 右上
        {float64(core.TileSize), float64(core.ScreenHeight - 2*core.TileSize)},                               // 左下
        {float64(core.ScreenWidth - 2*core.TileSize), float64(core.ScreenHeight - 2*core.TileSize)},          // 右下
    }

    idx := (playerID - 1) % len(positions)
    return positions[idx]
}

// endGame 结束游戏
func (r *Room) endGame() {
    r.phase = PhaseGameOver
    r.phaseFrames = core.GameOverDelayFrames

    // 找出胜利者
    winnerID := int32(-1)
    for _, p := range r.game.GetAlivePlayers() {
        winnerID = int32(p.ID)
        break
    }

    r.broadcastEvent(&gamev1.GameEvent{
        FrameId: r.frameID,
        Event: &gamev1.GameEvent_GameOver{
            GameOver: &gamev1.GameOverEvent{
                WinnerId: winnerID,
            },
        },
    })

    log.Printf("房间 %s 游戏结束，胜利者: %d", r.ID, winnerID)
}

// resetGame 重置游戏
func (r *Room) resetGame() {
    r.game = core.NewGame(time.Now().UnixNano(), true)
    r.frameID = 0
    r.phase = PhaseWaiting
    r.aiControllers = make(map[int32]*ai.AIController)

    // 重新添加现有玩家
    r.mu.Lock()
    for playerID := range r.connections {
        startPos := r.getStartPosition(int(playerID))
        player := core.NewPlayer(int(playerID), startPos.X, startPos.Y)
        r.game.AddPlayer(player)
    }
    r.mu.Unlock()

    log.Printf("房间 %s 已重置", r.ID)
}

// collectTileChanges 收集地图变化
func (r *Room) collectTileChanges() {
    changes := r.game.Map.GetAndClearChanges()
    if len(changes) > 0 {
        r.tileChanges = append(r.tileChanges, changes...)
        r.tileChangeFrame = r.frameID
    }
}

// Close 关闭房间
func (r *Room) Close() {
    r.cancel()
    close(r.closeCh)
}

// ===== InputBuffer 实现 =====

// NewInputBuffer 创建输入缓冲区
func NewInputBuffer(maxSize int) *InputBuffer {
    return &InputBuffer{
        inputs:  make([]InputRecord, 0, maxSize),
        maxSize: maxSize,
        lastSeq: 0,
    }
}

// AddInput 添加输入
func (b *InputBuffer) AddInput(record InputRecord) {
    // 忽略旧输入
    if record.Seq <= b.lastSeq {
        return
    }

    b.inputs = append(b.inputs, record)
    b.lastSeq = record.Seq

    // 限制大小
    if len(b.inputs) > b.maxSize {
        b.inputs = b.inputs[len(b.inputs)-b.maxSize:]
    }
}

// GetLatestInput 获取最新输入
func (b *InputBuffer) GetLatestInput() *InputRecord {
    if len(b.inputs) == 0 {
        return nil
    }

    // 取出并移除
    input := b.inputs[0]
    b.inputs = b.inputs[1:]
    return &input
}
```
