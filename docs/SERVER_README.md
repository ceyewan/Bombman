# Bomberman 服务器实现说明

## 文件结构

```
internal/server/
├── game_server.go    # 服务器核心逻辑
├── connection.go     # 客户端连接管理
```

## 架构设计

### 1. GameServer（服务器核心）

**位置**：`internal/server/game_server.go`

**主要功能**：
- 维护游戏状态（使用 `core.Game`）
- 管理所有客户端连接
- 60 TPS 游戏循环更新
- 广播游戏状态到所有客户端

**关键组件**：

```go
type GameServer struct {
    game        *core.Game              // 核心游戏状态
    frameId     int32                   // 当前帧号
    connections map[int32]*Connection   // 玩家连接
    nextPlayerID int32                  // 下一个分配的玩家ID

    // 网络和控制
    listener    net.Listener
    ctx         context.Context
    cancel      context.CancelFunc
}
```

**主要方法**：

- `Start()` - 启动服务器
- `Shutdown()` - 优雅关闭
- `gameLoop()` - 游戏主循环（60 TPS）
- `broadcastState()` - 广播游戏状态
- `handleJoinRequest()` - 处理加入请求
- `handleClientInput()` - 处理客户端输入

### 2. Connection（连接管理）

**位置**：`internal/server/connection.go`

**主要功能**：
- 管理单个客户端连接
- 处理消息接收和发送
- 实现长度前缀协议（4字节长度 + 消息体）

**关键组件**：

```go
type Connection struct {
    conn     net.Conn
    server   *GameServer
    playerID int32
    sendChan chan []byte  // 异步发送队列
}
```

**主要方法**：

- `Handle()` - 启动连接处理
- `Close()` - 关闭连接
- `Send()` - 发送数据（异步）
- `receiveLoop()` - 接收循环
- `sendLoop()` - 发送循环
- `handleMessage()` - 处理接收到的消息

## 网络协议

### 消息格式

采用**长度前缀协议**：

```
┌──────────────┬──────────────────┐
│ Length (4B)  │ Payload (Var)    │
└──────────────┴──────────────────┘
```

1. **Length**：4 字节，大端序（Big Endian）
2. **Payload**：Protobuf 序列化的 `GamePacket`

### 支持的消息

#### 客户端 → 服务器

1. **JoinRequest** - 加入游戏
   ```go
   message JoinRequest {
       CharacterType character_type = 1;
   }
   ```

2. **ClientInput** - 玩家输入
   ```go
   message ClientInput {
       bool up = 1;
       bool down = 2;
       bool left = 3;
       bool right = 4;
       bool bomb = 5;
       int32 seq = 6;
   }
   ```

#### 服务器 → 客户端

1. **GameStart** - 游戏开始
   ```go
   message GameStart {
       int32 your_player_id = 1;
       MapState initial_map = 2;
   }
   ```

2. **ServerState** - 游戏状态同步（60 TPS）
   ```go
   message ServerState {
       int32 frame_id = 1;
       repeated PlayerState players = 2;
       repeated BombState bombs = 3;
       repeated ExplosionState explosions = 4;
       MapState map = 5;  // 仅在变化时发送
   }
   ```

3. **GameOver** - 游戏结束
   ```go
   message GameOver {
       int32 winner_id = 1;
   }
   ```

4. **PlayerLeave** - 玩家离开
   ```go
   message PlayerLeave {
       int32 player_id = 1;
   }
   ```

## 数据转换

**位置**：`pkg/protocol/converter.go`

### 枚举转换

```go
// Direction
core.DirUp (1) ↔ proto.DIRECTION_UP (1)
core.DirDown (0) ↔ proto.DIRECTION_DOWN (2)  // 注意索引差异！
core.DirLeft (2) ↔ proto.DIRECTION_LEFT (3)
core.DirRight (3) ↔ proto.DIRECTION_RIGHT (4)

// CharacterType
core.CharacterWhite (0) ↔ proto.CHARACTER_TYPE_WHITE (1)  // -1 转换！
core.CharacterBlack (1) ↔ proto.CHARACTER_TYPE_BLACK (2)
core.CharacterRed (2) ↔ proto.CHARACTER_TYPE_RED (3)
core.CharacterBlue (3) ↔ proto.CHARACTER_TYPE_BLUE (4)
```

### 结构转换

- `CorePlayerToProto()` - `core.Player` → `gamev1.PlayerState`
- `CoreBombToProto()` - `core.Bomb` → `gamev1.BombState`
- `CoreExplosionToProto()` - `core.Explosion` → `gamev1.ExplosionState`
- `CoreMapToProto()` - `core.GameMap` → `gamev1.MapState`

## 游戏循环

### 服务器端（60 TPS）

```
┌─────────────────────────────────┐
│  每 16.67ms (1/60 秒)           │
├─────────────────────────────────┤
│  1. 更新游戏逻辑                 │
│     - 处理玩家移动               │
│     - 更新炸弹                   │
│     - 处理爆炸                   │
│     - 碰撞检测                   │
│                                 │
│  2. 广播状态到所有客户端         │
│     - 转换 core → proto         │
│     - 序列化消息                 │
│     - 发送给每个连接             │
└─────────────────────────────────┘
```

### 客户端输入处理

```
客户端按键
    ↓
立即发送 ClientInput
    ↓
服务器接收
    ↓
服务器更新玩家位置
    ↓
服务器广播新状态（60 TPS）
    ↓
客户端接收并应用
```

## 出生点分配

4 个玩家出生在地图的四角：

| 玩家 ID | 位置     | 坐标   |
|---------|----------|--------|
| 1       | 左上角   | (0, 0) |
| 2       | 右上角   | (608, 0) |
| 3       | 左下角   | (0, 448) |
| 4       | 右下角   | (608, 448) |

每个出生点有 3x3 的安全区域（无砖块）。

## 并发安全

### 使用的同步机制

1. **sync.RWMutex** - 保护 `connections` map
   - 读操作使用 `RLock()`
   - 写操作使用 `Lock()`

2. **context.Context** - 控制 goroutine 生命周期
   - 取消时所有 goroutine 自动退出

3. **sync.WaitGroup** - 等待所有 goroutine 结束

4. **channel** - 连接的发送队列
   - 缓冲大小 256
   - 异步发送，不阻塞游戏循环

### 线程安全保证

- `connections` map 读写都受 mutex 保护
- 每个连接有独立的发送队列
- 游戏状态更新在单个 goroutine 中（gameLoop）

## 运行服务器

### 启动服务器

```bash
# 默认端口 :8080
go run cmd/server/main.go

# 指定端口
go run cmd/server/main.go -addr=:9000
```

### 日志输出

```
========================================
  Bomberman 联机服务器
========================================
监听地址: :8080
最大玩家数: 4
服务器 TPS: 60
========================================
服务器正在运行...
按 Ctrl+C 停止服务器
```

### 优雅关闭

按 `Ctrl+C` 触发优雅关闭：

1. 取消上下文（停止所有 goroutine）
2. 关闭监听器（停止接受新连接）
3. 关闭所有客户端连接
4. 等待所有 goroutine 结束
5. 退出程序

## 测试

### 使用 telnet 测试连接

```bash
telnet localhost 8080
```

### 使用 Proto 测试工具

创建测试客户端发送 `JoinRequest`：

```go
packet := protocol.NewJoinRequest(gamev1.CharacterType_CHARACTER_TYPE_WHITE)
data, _ := protocol.Marshal(packet)
// 发送长度前缀 + 数据
```

## 错误处理

### 连接错误

- 读取失败 → 关闭连接
- 发送失败 → 记录日志，继续运行
- 无效消息 → 记录日志，关闭连接

### 游戏逻辑错误

- 玩家不存在 → 忽略输入
- 玩家已死亡 → 忽略输入
- 服务器已满 → 拒绝加入请求

## 性能考虑

### 带宽优化

- 地图仅在变化时发送
- 使用 Protobuf 二进制编码
- 每帧约 1-2 KB（4 个玩家 + 10 个炸弹）

### CPU 优化

- 游戏循环单线程
- 网络收发异步
- 最小化锁竞争

### 内存优化

- 发送队列有界（256）
- 连接断开立即清理
- proto 对象池化（TODO）

## 下一步

### Phase 3: 客户端网络层

- [ ] 实现 TCP 连接
- [ ] 实现 `receiveLoop` 和 `sendLoop`
- [ ] 发送 `JoinRequest`
- [ ] 接收并应用 `ServerState`

### Phase 4: 客户端渲染

- [ ] 渲染其他玩家（使用插值）
- [ ] 渲染炸弹和爆炸
- [ ] 渲染地图
- [ ] 处理玩家输入

## 相关文档

- [联机版实现指南](MULTIPLAYER_IMPLEMENTATION_GUIDE.md)
- [Proto 协议定义](../api/proto/bomberman/v1/game.proto)
- [核心包文档](../pkg/core/)

---

**维护者**: Claude Code
**最后更新**: 2026-01-06
