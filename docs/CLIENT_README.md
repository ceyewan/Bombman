# Bomberman 客户端实现说明

## 文件结构

```
internal/client/
├── network.go         # 网络管理器
├── network_game.go    # 联机游戏层
├── game.go           # 单机游戏层（已存在）
├── player.go         # 玩家渲染（已存在）
├── bomb.go           # 炸弹渲染（已存在）
├── map.go            # 地图渲染（已存在）
└── character.go      # 角色定义（已存在）
```

## 架构设计

### 双模式支持

客户端支持**单机**和**联机**两种模式，完全兼容现有代码：

1. **单机模式**：使用现有的 `Game` 结构
2. **联机模式**：使用 `NetworkGameClient` 包装 `Game`

### 核心组件

#### 1. NetworkClient（网络管理器）

**位置**：`internal/client/network.go`

**主要功能**：
- TCP 连接管理
- 消息收发（长度前缀协议）
- 加入游戏流程
- 状态接收（channel 缓冲）

**关键结构**：

```go
type NetworkClient struct {
    conn     net.Conn
    playerID int32
    connected bool

    // 消息队列（channel）
    stateChan       chan *gamev1.ServerState
    gameStartChan   chan *gamev1.GameStart
    gameOverChan    chan *gamev1.GameOver
    playerJoinChan  chan *gamev1.PlayerJoin
    playerLeaveChan chan int32

    // 发送队列
    sendChan  chan []byte
    inputSeq  int32
}
```

**主要方法**：

- `Connect()` - 连接服务器并发送加入请求
- `Close()` - 优雅关闭连接
- `SendInput()` - 发送玩家输入
- `ReceiveState()` - 接收游戏状态（非阻塞）
- `GetPlayerID()` - 获取玩家 ID

#### 2. NetworkGameClient（联机游戏层）

**位置**：`internal/client/network_game.go`

**主要功能**：
- 复用现有的 `Game` 结构
- 应用服务器状态
- 本地玩家输入发送
- 其他玩家插值渲染
- 网络事件处理

**关键设计**：

```go
type NetworkGameClient struct {
    game       *Game          // 复用现有 Game
    network    *NetworkClient
    playerID   int
    playersMap map[int]*Player // 玩家 ID -> Player
}
```

**工作流程**：

```
┌─────────────────────────────────────┐
│  Update() 每帧调用                   │
├─────────────────────────────────────┤
│  1. ReceiveState() - 接收服务器状态  │
│  2. applyServerState() - 应用状态    │
│     - 本地玩家：直接更新              │
│     - 其他玩家：SetNetworkPosition() │
│  3. SendInput() - 发送输入到服务器   │
│  4. Update() - 更新插值              │
│  5. syncRenderers() - 同步渲染器     │
│  6. handleNetworkEvents() - 处理事件 │
└─────────────────────────────────────┘
```

## 运行客户端

### 单机模式

```bash
# 默认角色（红色）
go run cmd/client/main.go

# 指定角色 (0=白, 1=黑, 2=红, 3=蓝)
go run cmd/client/main.go -character=0

# 指定控制方案 (wasd 或 arrow)
go run cmd/client/main.go -control=arrow
```

### 联机模式

```bash
# 连接到本地服务器
go run cmd/client/main.go -server=localhost:8080

# 连接到远程服务器
go run cmd/client/main.go -server=192.168.1.100:8080

# 指定角色和控制方案
go run cmd/client/main.go -server=localhost:8080 -character=1 -control=arrow
```

## 玩家插值

### 本地玩家

- **不使用插值**：`IsSimulated = false`
- 服务器直接更新状态（Dead, Character）
- 位置由服务器控制，客户端不预测

### 其他玩家

- **使用插值**：`IsSimulated = true`
- 调用 `SetNetworkPosition()` 设置目标位置
- `Update()` 中的 `updateLerp()` 自动计算插值
- 渲染使用插值后的 X/Y 坐标

**示例代码**：

```go
// 应用服务器状态
if playerID == localPlayerID {
    // 本地玩家
    player.Dead = protoPlayer.Dead
    player.Character = ProtoCharacterTypeToCore(protoPlayer.Character)
} else {
    // 其他玩家：使用插值
    player.SetNetworkPosition(protoPlayer.X, protoPlayer.Y)
    player.Dead = protoPlayer.Dead
    player.Direction = ProtoDirectionToCore(protoPlayer.Direction)
}
```

## 网络协议

### 消息格式

与服务器一致，使用**长度前缀协议**：

```
┌──────────────┬──────────────────┐
│ Length (4B)  │ Payload (Var)    │
└──────────────┴──────────────────┘
```

### 消息接收

**非阻塞接收**（channel 缓冲）：

```go
// 接收游戏状态
if state := network.ReceiveState(); state != nil {
    // 处理状态
}

// 接收游戏开始
if gameStart := network.ReceiveGameStart(); gameStart != nil {
    // 处理游戏开始
}
```

### 消息发送

**异步发送**（channel 队列）：

```go
// 发送输入
network.SendInput(up, down, left, right, bomb)
```

## 输入处理

### 单机模式

- 直接调用 `player.Move()`
- 立即更新游戏状态

### 联机模式

- 捕获输入状态
- 发送到服务器
- 服务器验证并广播新状态
- 客户端应用服务器状态

**代码对比**：

```go
// 单机模式（player.go）
if !p.corePlayer.IsSimulated && !p.corePlayer.Dead {
    p.handleInput(deltaTime, controlScheme, coreGame)
}

// 联机模式（network_game.go）
up, down, left, right, bomb := getInputState(controlScheme)
network.SendInput(up, down, left, right, bomb)
```

## 游戏流程

### 单机模式流程

```
启动游戏
    ↓
创建 Game 对象
    ↓
添加本地玩家
    ↓
游戏循环（Update/Draw）
    ↓
处理输入 → 更新逻辑 → 渲染
```

### 联机模式流程

```
启动游戏
    ↓
连接服务器
    ↓
发送 JoinRequest
    ↓
等待 GameStart
    ↓
同步地图
    ↓
游戏循环（Update/Draw）
    ├─ 接收 ServerState
    ├─ 发送 ClientInput
    ├─ 应用服务器状态
    ├─ 更新插值
    └─ 渲染
    ↓
处理网络事件
    ├─ PlayerJoin → 添加玩家
    ├─ PlayerLeave → 移除玩家
    └─ GameOver → 显示结果
```

## 与现有代码兼容

### 复用的组件

- ✅ `Game` - 游戏循环
- ✅ `MapRenderer` - 地图渲染
- ✅ `Player` - 玩家渲染
- ✅ `BombRenderer` - 炸弹渲染
- ✅ `ExplosionRenderer` - 爆炸渲染
- ✅ `CharacterInfo` - 角色配色

### 新增的组件

- `NetworkClient` - 网络管理
- `NetworkGameClient` - 联机游戏包装器
- `NewPlayerFromCore()` - 从 core.Player 创建 Player
- `UpdateAnimation()` - 仅更新动画（不处理输入）

## 错误处理

### 连接错误

- 连接超时 → 退出程序
- 读取失败 → 关闭连接
- 发送失败 → 记录日志，继续运行

### 协议错误

- 反序列化失败 → 记录日志，丢弃消息
- 未知消息类型 → 记录日志，丢弃消息
- 消息过大 → 关闭连接

## 性能优化

### 带宽

- 服务器 60 TPS 发送状态
- 每帧约 1-2 KB
- 客户端缓冲 256 帧状态

### CPU

- 非阻塞接收（channel）
- 异步发送（channel）
- 最小化锁竞争

### 内存

- channel 有界缓冲
- 状态对象复用
- 及时清理断线连接

## 调试技巧

### 查看网络状态

```go
// 在 network.go 中添加日志
log.Printf("收到状态: frame=%d, players=%d", state.FrameId, len(state.Players))
```

### 查看插值效果

```go
// 在 player.go 中添加日志
log.Printf("插值: %d, (%.2f, %.2f) -> (%.2f, %.2f), progress=%.2f",
    player.ID, player.LastNetworkX, player.LastNetworkY,
    player.NetworkX, player.NetworkY, player.LerpProgress)
```

### 测试单机模式

```bash
go run cmd/client/main.go
```

### 测试联机模式

```bash
# 终端 1：启动服务器
go run cmd/server/main.go

# 终端 2、3、4：启动客户端
go run cmd/client/main.go -server=localhost:8080 -character=0
go run cmd/client/main.go -server=localhost:8080 -character=1
go run cmd/client/main.go -server=localhost:8080 -character=2
```

## 下一步

### Phase 4: 渲染增强

- [ ] 添加玩家名称标签
- [ ] 显示 ping 值
- [ ] 显示玩家列表
- [ ] 游戏结束画面

### Phase 5: 用户体验

- [ ] 连接中提示
- [ ] 等待玩家提示
- [ ] 倒计时
- [ ] 音效

### Phase 6: 优化

- [ ] 客户端预测
- [ ] 延迟补偿
- [ ] 断线重连
- [ ] 画面平滑

## 相关文档

- [服务器实现说明](SERVER_README.md)
- [联机版实现指南](MULTIPLAYER_IMPLEMENTATION_GUIDE.md)
- [Proto 协议定义](../api/proto/bomberman/v1/game.proto)

---

**维护者**: Claude Code
**最后更新**: 2026-01-06
