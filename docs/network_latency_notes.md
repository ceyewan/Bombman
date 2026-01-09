# 全面审计报告：Bomberman 网络优化建议

### 一、弱网环境优化

#### 1.1 当前问题
- **心跳机制过于简单**：只有 15 秒超时检测，无法识别网络质量变化
- **协议切换不支持**：TCP/KCP 在启动时确定，运行时无法切换
- **无网络质量监测**：RTT 只用于时间同步，未用于自适应调整

#### 1.2 建议方案

**A. 网络质量评估系统**
```go
type NetworkQuality struct {
    RTTAvg      float64   // 平均 RTT
    RTTJitter   float64   // RTT 抖动（标准差）
    PacketLoss  float64   // 丢包率（通过 seq 间隙估算）
    LastUpdate  time.Time
}

// 质量等级
const (
    QualityExcellent = iota // RTT < 50ms, Jitter < 10ms
    QualityGood             // RTT < 100ms, Jitter < 30ms
    QualityFair             // RTT < 200ms, Jitter < 50ms
    QualityPoor             // RTT >= 200ms or Jitter >= 50ms
)
```

**B. 自适应参数调整**
- `InterpolationDelayMs`：根据 RTT + 2×Jitter 动态调整
- `InputLeadFrames`：根据 RTT/16.6 + 1 计算
- 发送频率：弱网时降低状态广播频率（60→30 TPS）

**C. KCP 自动切换方案**
```
客户端逻辑：
1. 连续 3 次 Ping 超时（>500ms）→ 标记为"弱网"
2. 弱网状态下，尝试建立 KCP 备用连接
3. KCP 连接成功后，发送 ReconnectRequest（带 sessionToken）
4. 服务器验证 token，迁移会话到 KCP 连接
5. 旧 TCP 连接优雅关闭
```

---

### 二、掉线重连机制

#### 2.1 当前问题
- **无会话持久化**：连接断开 = 玩家退出
- **无重连协议**：没有 ReconnectRequest/Response 消息
- **状态无法恢复**：玩家位置、炸弹等状态丢失

#### 2.2 建议方案

**A. 会话 Token 机制**
```protobuf
// 新增消息
message JoinResponse {
    // ... 现有字段
    string session_token = 10;  // 用于重连的唯一令牌
    int64 session_expire = 11;  // 过期时间戳
}

message ReconnectRequest {
    string session_token = 1;
}

message ReconnectResponse {
    bool success = 1;
    string error_message = 2;
    GameState current_state = 3;  // 完整状态快照
}
```

**B. 服务器端会话管理**
```go
type SessionStore struct {
    sessions map[string]*PendingSession  // token -> session
    mu       sync.RWMutex
}

type PendingSession struct {
    PlayerID    int32
    RoomID      string
    Character   CharacterType
    ExpireAt    time.Time
    PlayerState *core.Player  // 断线时的状态快照
}

// 玩家断线时
func (r *Room) onPlayerDisconnect(playerID int32) {
    // 不立即删除玩家，而是标记为"掉线"
    player := r.game.GetPlayer(playerID)
    player.IsDisconnected = true
    
    // 保存到 SessionStore，30秒过期
    token := generateSessionToken()
    store.Save(token, &PendingSession{
        PlayerID:    playerID,
        RoomID:      r.id,
        PlayerState: player.Clone(),
        ExpireAt:    time.Now().Add(30 * time.Second),
    })
}

// 玩家重连时
func (s *GameServer) handleReconnect(conn *Connection, req ReconnectRequest) {
    session := store.Get(req.Token)
    if session == nil || session.ExpireAt.Before(time.Now()) {
        // Token 无效或过期，需要重新加入
        return sendError(conn, "Session expired")
    }
    
    // 恢复会话
    room := s.roomManager.GetRoom(session.RoomID)
    room.ResumePlayer(session.PlayerID, conn)
    store.Delete(req.Token)
}
```

**C. 客户端重连流程**
```
1. 检测到连接断开
2. 显示"正在重连..."提示
3. 尝试重新建立连接（最多 3 次，间隔 1/2/4 秒）
4. 发送 ReconnectRequest（带缓存的 sessionToken）
5. 成功：接收完整状态快照，恢复游戏
6. 失败：提示用户返回主菜单
```

---

### 三、性能优化

#### 3.1 当前问题
- **状态广播冗余**：每帧发送完整状态（4 玩家 ≈ 350 字节/帧）
- **无增量编码**：即使玩家静止也发送位置
- **GC 压力**：频繁创建 `[]byte` 和 proto 对象

#### 3.2 建议方案

**A. 增量状态编码**
```go
// 服务器维护上一帧状态
type DeltaEncoder struct {
    lastPlayers map[int32]PlayerSnapshot
    lastBombs   map[int32]BombSnapshot
}

// 只发送变化的字段
func (e *DeltaEncoder) EncodeDelta(state *GameState) *DeltaState {
    delta := &DeltaState{FrameId: state.FrameId}
    
    for _, p := range state.Players {
        last := e.lastPlayers[p.Id]
        if p.X != last.X || p.Y != last.Y || p.Direction != last.Direction {
            delta.PlayerDeltas = append(delta.PlayerDeltas, &PlayerDelta{
                Id: p.Id, X: &p.X, Y: &p.Y, Direction: &p.Direction,
            })
        }
    }
    // ... bombs, explosions 同理
    return delta
}
```
**节省**：静止玩家 0 字节 vs 原来 ~50 字节

**B. 对象池减少 GC**
```go
var statePool = sync.Pool{
    New: func() interface{} {
        return &gamev1.GameState{
            Players: make([]*gamev1.PlayerState, 0, 4),
        }
    },
}

func (r *Room) broadcastState() {
    state := statePool.Get().(*gamev1.GameState)
    defer statePool.Put(state)
    
    // 重用 slice
    state.Players = state.Players[:0]
    // ... 填充数据
}
```

**C. 批量序列化**
```go
// 当前：每个连接单独序列化
for _, conn := range connections {
    data, _ := proto.Marshal(state)  // 重复序列化 N 次
    conn.Send(data)
}

// 优化：序列化一次，发送多次
data, _ := proto.Marshal(state)
for _, conn := range connections {
    conn.Send(data)  // 共享同一份数据
}
```

**D. 消息合并**
```go
// 低优先级消息（如 TileChange）可以延迟合并
type MessageBatcher struct {
    pending map[int32][]byte
    ticker  *time.Ticker  // 每 50ms 批量发送
}
```

---

### 四、会话管理增强

#### 4.1 当前问题
- **Session 接口过于简单**：缺少元数据
- **无连接统计**：无法监控连接健康状态
- **无限流保护**：恶意客户端可发送大量输入

#### 4.2 建议方案

**A. 增强 Session 接口**
```go
type Session interface {
    ID() int32
    GetRoomID() string
    SetRoomID(roomID string)
    Send(data []byte) error
    Close()
    CloseWithoutNotify()
    SetPlayerID(id int32)
    
    // 新增
    GetConnectionInfo() ConnectionInfo
    GetStats() SessionStats
    SetRateLimit(limit RateLimitConfig)
}

type ConnectionInfo struct {
    RemoteAddr   string
    Protocol     string  // "tcp" or "kcp"
    ConnectedAt  time.Time
    SessionToken string
}

type SessionStats struct {
    BytesSent     int64
    BytesReceived int64
    PacketsSent   int64
    PacketsReceived int64
    RTTMs         int64
    LastActivity  time.Time
}
```

**B. 输入限流**
```go
type RateLimiter struct {
    maxInputsPerSecond int
    inputCount         int
    lastReset          time.Time
}

func (r *Room) handleInput(ev inputEvent) {
    limiter := r.rateLimiters[ev.playerID]
    if !limiter.Allow() {
        log.Printf("玩家 %d 输入过快，已限流", ev.playerID)
        return
    }
    // ... 正常处理
}
```

**C. 连接健康监控**
```go
type HealthMonitor struct {
    connections map[int32]*ConnectionHealth
}

type ConnectionHealth struct {
    RTTSamples    []int64  // 最近 10 个 RTT
    PacketLossRate float64
    Grade         QualityGrade
}

// 定期汇报到日志/监控系统
func (m *HealthMonitor) Report() {
    for id, h := range m.connections {
        log.Printf("Player %d: RTT=%dms, Loss=%.1f%%, Grade=%s",
            id, h.AvgRTT(), h.PacketLossRate*100, h.Grade)
    }
}
```

---

### 五、其他建议

#### 5.1 协议优化
- **消息压缩**：对于大于 256 字节的消息使用 Snappy/LZ4 压缩
- **消息分片**：大消息（如完整地图）分片发送，避免阻塞

#### 5.2 安全性
- **输入校验**：服务器验证输入合法性（如移动速度限制）
- **反作弊**：检测异常移动轨迹
- **Token 签名**：使用 HMAC 签名 sessionToken

#### 5.3 可观测性
```go
// 添加 Prometheus 指标
var (
    activeConnections = prometheus.NewGauge(...)
    messagesPerSecond = prometheus.NewCounter(...)
    rttHistogram      = prometheus.NewHistogram(...)
)
```

---

### 六、优先级建议

| 优先级 | 功能 | 工作量 | 收益 |
|-------|------|--------|------|
| P0 | 增量状态编码 | 中 | 高（带宽减少 50%+） |
| P0 | 掉线重连 | 中 | 高（用户体验大幅提升） |
| P1 | 网络质量监测 | 低 | 中（为自适应打基础） |
| P1 | 输入限流 | 低 | 中（防止滥用） |
| P2 | TCP/KCP 动态切换 | 高 | 中（弱网场景提升） |
| P2 | 对象池优化 | 低 | 低（减少 GC） |
| P3 | Prometheus 监控 | 低 | 低（运维可观测性） |

---

需要我展开其中某个方案的详细实现吗？
