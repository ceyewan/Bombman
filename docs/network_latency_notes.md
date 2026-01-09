# Bomberman 网络优化实现记录

## 已完成 ✅

### 1. 基于 RTT 的自适应参数调整

**实现时间**: 2026-01-10

**功能**:
- **RTT 统计**: 20 个采样窗口的滑动平均
- **RTT 抖动**: 标准差计算
- **网络质量评估**: 自动评级（优秀/良好/一般/较差）
- **自适应插值延迟**: `RTT + 2×Jitter + 50ms`，范围 50-500ms
- **自适应输入提前帧**: `RTT / 16.6 + 1`，范围 1-6 帧

**日志输出**:
```
[网络] RTT: 45ms, 平均: 48ms, 抖动: 5ms, 质量: 优秀
[自适应] 插值延迟: 148ms, 输入提前: 3帧 (RTT: 48ms, 抖动: 5ms)
```

**修改文件**:
- [internal/client/network.go](../internal/client/network.go) - RTT 统计和日志
- [internal/client/network_constants.go](../internal/client/network_constants.go) - 常量定义
- [internal/client/network_smoothing.go](../internal/client/network_smoothing.go) - 动态插值延迟
- [internal/client/network_game.go](../internal/client/network_game.go) - 自适应参数计算

---

### 2. 服务器全局限流

**实现时间**: 2026-01-10

**功能**:
- **限流器**: 使用 `golang.org/x/time/rate` 标准库令牌桶算法
- **全局消息限流**: 每个连接所有消息类型（Ping/Join/Input）共享限流器
- **限流阈值**: 60 消息/秒，突发容量 100（正常游戏 60 TPS + 偶尔 Ping 足够）
- **防御范围**: 防止任意消息洪水攻击（Ping 风暴、Input 洪水、Join 洪水等）

**修改文件**:
- [internal/server/connection.go](../internal/server/connection.go) - 全局限流逻辑

**实现细节**:
```go
import "golang.org/x/time/rate"

const (
    // 全局消息限流：每秒最多 60 个消息
    globalMessageRateLimit = rate.Limit(60)
    // 限流器突发容量（允许短时突发）
    globalMessageBurst = 100
)

type Connection struct {
    // ...
    rateLimiter *rate.Limiter // 所有消息类型共享
}

// 在 handleMessage 中检查（处理任何消息类型前）
if !c.rateLimiter.Allow() {
    return fmt.Errorf("rate limit exceeded")
}
```

**移除**: `room.go` 中原有的自定义输入限流器（已被全局限流替代）

---

## 待实现 📋

### 一、弱网环境优化

#### 1.1 当前问题
- ~~**无网络质量监测**：RTT 只用于时间同步，未用于自适应调整~~ ✅ 已完成
- **心跳机制过于简单**：只有 15 秒超时检测，无法识别网络质量变化
- **协议切换不支持**：TCP/KCP 在启动时确定，运行时无法切换

#### 1.2 建议方案

**A. 网络质量评估系统** ✅ 已完成
- ~~RTT 平均值、抖动统计~~
- ~~网络质量等级评估~~

**B. 自适应参数调整** ✅ 已完成
- ~~`InterpolationDelayMs`：根据 RTT + 2×Jitter 动态调整~~
- ~~`InputLeadFrames`：根据 RTT/16.6 + 1 计算~~
- 发送频率：弱网时降低状态广播频率（60→30 TPS）⏳ 待实现

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

**B. 输入限流** ✅ 已完成
- ~~使用 `x/time/rate` 标准库实现全局限流~~
- ~~每个连接所有消息类型共享限流器~~
- ~~60 消息/秒，突发容量 100~~

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

## 优先级建议

| 优先级 | 功能 | 工作量 | 收益 | 状态 |
|-------|------|--------|------|------|
| P0 | 增量状态编码 | 中 | 高（带宽减少 50%+） | ⏳ 待实现 |
| P0 | 掉线重连 | 中 | 高（用户体验大幅提升） | ⏳ 待实现 |
| P1 | ~~网络质量监测~~ | 低 | 中（为自适应打基础） | ✅ 已完成 |
| P1 | ~~全局限流~~ | 低 | 中（防止滥用） | ✅ 已完成 |
| P2 | TCP/KCP 动态切换 | 高 | 中（弱网场景提升） | ⏳ 待实现 |
| P2 | 对象池优化 | 低 | 低（减少 GC） | ⏳ 待实现 |
| P3 | Prometheus 监控 | 低 | 低（运维可观测性） | ⏳ 待实现 |

---

## 下一步建议

### 推荐 1：掉线重连（P0）
**理由**：
- 用户体验提升最明显
- 网络波动时不用重新排队
- 实现难度适中

### 推荐 2：增量状态编码（P0）
**理由**：
- 带宽节省 50%+
- 降低服务器 CPU 和网络压力
- 为未来扩展更多玩家做准备
