# Bomberman 联机版实现指南

本文档提供了基于当前 core 包实现联机版的完整指导。

## 目录

1. [架构概述](#架构概述)
2. [Proto 协议说明](#proto-协议说明)
3. [服务器实现指南](#服务器实现指南)
4. [客户端实现指南](#客户端实现指南)
5. [核心数据转换](#核心数据转换)
6. [网络同步策略](#网络同步策略)
7. [实现检查清单](#实现检查清单)

---

## 架构概述

### 当前架构

项目已完成核心游戏逻辑（`pkg/core`），包括：
- `Game` - 游戏状态管理
- `Player` - 玩家逻辑（支持本地和插值）
- `Bomb` - 炸弹逻辑
- `Explosion` - 爆炸效果
- `GameMap` - 地图系统

### 联机版架构

采用**权威服务器 + 客户端插值**架构：

```
┌─────────┐                    ┌─────────┐
│ 客户端 A │◄────网络同步────►│ 服务器  │
└─────────┘                    └─────────┘
         ▲                             ▲
         │                             │
         └──────────客户端 B/C/D────────┘
```

**关键特性**：
- 服务器维护唯一真相的游戏状态
- 客户端渲染其他玩家时使用 LERP 插值
- 60 TPS 服务器更新频率
- Protobuf 序列化 + TCP/UDP 传输

---

## Proto 协议说明

### 协议文件

协议定义位于 [api/proto/bomberman/v1/game.proto](../api/proto/bomberman/v1/game.proto)

### 核心消息类型

#### 1. **GamePacket** - 顶层消息包（信封模式）

```protobuf
message GamePacket {
  oneof payload {
    ClientInput input = 1;        // C -> S: 玩家输入
    JoinRequest join_req = 2;     // C -> S: 加入请求
    ServerState state = 3;        // S -> C: 游戏状态
    GameStart game_start = 4;     // S -> C: 游戏开始
    GameOver game_over = 5;       // S -> C: 游戏结束
    PlayerJoin player_join = 6;   // S -> C: 玩家加入
    PlayerLeave player_leave = 7; // S -> C: 玩家离开
  }
}
```

#### 2. **ClientInput** - 客户端输入

```protobuf
message ClientInput {
  bool up = 1;
  bool down = 2;
  bool left = 3;
  bool right = 4;
  bool bomb = 5;
  int32 seq = 6;  // 序列号，用于去重
}
```

#### 3. **ServerState** - 服务器状态同步

```protobuf
message ServerState {
  int32 frame_id = 1;
  repeated PlayerState players = 2;
  repeated BombState bombs = 3;
  repeated ExplosionState explosions = 4;  // 新增
  MapState map = 5;  // 仅在地图变化时发送
}
```

#### 4. **PlayerState** - 玩家状态

```protobuf
message PlayerState {
  int32 id = 1;
  double x = 2;           // 使用 double 保持与 core.Player float64 一致
  double y = 3;
  Direction direction = 4;
  bool is_moving = 5;
  bool dead = 6;
  CharacterType character = 7;
}
```

#### 5. **BombState** - 炸弹状态

```protobuf
message BombState {
  double x = 1;
  double y = 2;
  int32 time_left_ms = 3;    // 剩余时间（毫秒）
  int32 explosion_range = 4;
}
```

#### 6. **ExplosionState** - 爆炸状态（新增）

```protobuf
message ExplosionState {
  int32 center_x = 1;
  int32 center_y = 2;
  int32 range = 3;
  int64 elapsed_ms = 4;
  repeated ExplosionCell cells = 5;
}

message ExplosionCell {
  int32 grid_x = 1;
  int32 grid_y = 2;
}
```

### 枚举映射

#### Direction（方向）

| Proto 值 | Core 值 | 说明 |
|----------|---------|------|
| `DIRECTION_UP = 1` | `DirUp = 1` | 上 |
| `DIRECTION_DOWN = 2` | `DirDown = 0` | 下 |
| `DIRECTION_LEFT = 3` | `DirLeft = 2` | 左 |
| `DIRECTION_RIGHT = 4` | `DirRight = 3` | 右 |

**注意**：需要添加转换函数处理索引差异！

#### CharacterType（角色类型）

| Proto 值 | Core 值 | 说明 |
|----------|---------|------|
| `CHARACTER_TYPE_WHITE = 1` | `CharacterWhite = 0` | 经典白 |
| `CHARACTER_TYPE_BLACK = 2` | `CharacterBlack = 1` | 暗夜黑 |
| `CHARACTER_TYPE_RED = 3` | `CharacterRed = 2` | 烈焰红 |
| `CHARACTER_TYPE_BLUE = 4` | `CharacterBlue = 3` | 冰霜蓝 |

**注意**：需要 `-1` 转换！

---

## 服务器实现指南

### 服务器核心结构

```go
// cmd/server/main.go 已存在基础框架
type GameServer struct {
    game       *core.Game      // core 游戏状态
    players    map[int]*PlayerConn  // 连接管理
    frameId    int32
    nextPlayerId int
}

type PlayerConn struct {
    ID       int
    Conn     net.Conn
    InputChan chan *gamev1.ClientInput
}
```

### 服务器主循环

```go
func (s *GameServer) gameLoop() {
    ticker := time.NewTicker(time.Second / 60)  // 60 TPS
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            // 1. 更新游戏逻辑
            s.game.Update(FixedDeltaTime)
            s.frameId++

            // 2. 广播状态到所有客户端
            s.broadcastState()
        }
    }
}
```

### 消息处理

#### 处理客户端输入

```go
func (s *GameServer) handleInput(conn *PlayerConn, input *gamev1.ClientInput) {
    player := s.getPlayer(conn.ID)
    if player == nil || player.Dead {
        return
    }

    // 根据输入移动玩家
    speed := player.Speed * FixedDeltaTime

    if input.Up {
        player.Move(0, -speed, s.game)
    }
    if input.Down {
        player.Move(0, speed, s.game)
    }
    if input.Left {
        player.Move(-speed, 0, s.game)
    }
    if input.Right {
        player.Move(speed, 0, s.game)
    }
    if input.Bomb {
        bomb := player.PlaceBomb(s.game)
        if bomb != nil {
            s.game.AddBomb(bomb)
        }
    }
}
```

#### 处理加入请求

```go
func (s *GameServer) handleJoin(conn net.Conn, req *gamev1.JoinRequest) {
    // 1. 分配玩家 ID
    playerID := s.nextPlayerId
    s.nextPlayerId++

    // 2. 选择出生点
    startX, startY := getSpawnPosition(playerID)

    // 3. 转换角色类型（Proto -> Core）
    coreCharType := protoCharTypeToCore(req.CharacterType)

    // 4. 创建玩家
    player := core.NewPlayer(playerID, startX, startY, coreCharType)
    player.IsSimulated = true  // 服务器控制
    s.game.AddPlayer(player)

    // 5. 发送 GameStart 消息
    initialMap := buildMapState(s.game.Map)
    packet := protocol.NewGameStart(int32(playerID), initialMap)

    data, _ := protocol.Marshal(packet)
    conn.Write(data)
}
```

#### 广播状态

```go
func (s *GameServer) broadcastState() {
    // 1. 转换 Players: core.Player -> gamev1.PlayerState
    protoPlayers := make([]*gamev1.PlayerState, 0, len(s.game.Players))
    for _, p := range s.game.Players {
        protoPlayers = append(protoPlayers, corePlayerToProto(p))
    }

    // 2. 转换 Bombs: core.Bomb -> gamev1.BombState
    protoBombs := make([]*gamev1.BombState, 0, len(s.game.Bombs))
    for _, b := range s.game.Bombs {
        protoBombs = append(protoBombs, coreBombToProto(b))
    }

    // 3. 转换 Explosions: core.Explosion -> gamev1.ExplosionState
    protoExplosions := make([]*gamev1.ExplosionState, 0, len(s.game.Explosions))
    for _, e := range s.game.Explosions {
        protoExplosions = append(protoExplosions, coreExplosionToProto(e))
    }

    // 4. 构造 ServerState 消息
    packet := protocol.NewServerState(s.frameId, protoPlayers, protoBombs, protoExplosions)

    // 5. 序列化并广播
    data, _ := protocol.Marshal(packet)
    for _, conn := range s.players {
        conn.Conn.Write(data)
    }
}
```

---

## 客户端实现指南

### 客户端核心结构

```go
type NetworkGame struct {
    game          *core.Game
    localPlayerID int
    conn          net.Conn

    // 网络通道
    stateChan     chan *gamev1.ServerState
    inputChan     chan *gamev1.ClientInput

    // 输入序列号
    inputSeq      int32
}
```

### 客户端主循环

```go
func (g *NetworkGame) Update() error {
    // 1. 处理服务器状态
    select {
    case state := <-g.stateChan:
        g.applyServerState(state)
    default:
    }

    // 2. 更新游戏逻辑
    g.game.Update(FixedDeltaTime)

    return nil
}
```

### 应用服务器状态

```go
func (g *NetworkGame) applyServerState(state *gamev1.ServerState) {
    for _, protoPlayer := range state.Players {
        player := g.getPlayerByID(int(protoPlayer.Id))

        if int(protoPlayer.Id) == g.localPlayerID {
            // 本地玩家：不插值，直接更新
            player.X = protoPlayer.X
            player.Y = protoPlayer.Y
            player.Dead = protoPlayer.Dead
        } else {
            // 其他玩家：使用插值
            player.SetNetworkPosition(protoPlayer.X, protoPlayer.Y)
        }
    }

    // 同步炸弹
    g.syncBombs(state.Bombs)

    // 同步爆炸
    g.syncExplosions(state.Explosions)

    // 同步地图（如果有）
    if state.Map != nil {
        g.syncMap(state.Map)
    }
}
```

### 发送输入

```go
func (g *NetworkGame) sendInput(input *gamev1.ClientInput) {
    input.Seq = g.inputSeq
    g.inputSeq++

    select {
    case g.inputChan <- input:
    default:
        // 通道满，丢弃旧输入
    }
}
```

---

## 核心数据转换

### 必需的转换函数

创建新文件 `pkg/protocol/converter.go`：

```go
package protocol

import (
    gamev1 "bomberman/api/gen/bomberman/v1"
    "bomberman/pkg/core"
)

// ========== Direction 转换 ==========

// Proto 方向索引：UP=1, DOWN=2, LEFT=3, RIGHT=4
// Core 方向索引：DirUp=1, DirDown=0, DirLeft=2, DirRight=3

func coreDirectionToProto(dir core.Direction) gamev1.Direction {
    switch dir {
    case core.DirUp:
        return gamev1.Direction_DIRECTION_UP
    case core.DirDown:
        return gamev1.Direction_DIRECTION_DOWN
    case core.DirLeft:
        return gamev1.Direction_DIRECTION_LEFT
    case core.DirRight:
        return gamev1.Direction_DIRECTION_RIGHT
    default:
        return gamev1.Direction_DIRECTION_UNSPECIFIED
    }
}

func protoDirectionToCore(dir gamev1.Direction) core.Direction {
    switch dir {
    case gamev1.Direction_DIRECTION_UP:
        return core.DirUp
    case gamev1.Direction_DIRECTION_DOWN:
        return core.DirDown
    case gamev1.Direction_DIRECTION_LEFT:
        return core.DirLeft
    case gamev1.Direction_DIRECTION_RIGHT:
        return core.DirRight
    default:
        return core.DirDown
    }
}

// ========== CharacterType 转换 ==========

// Proto: WHITE=1, BLACK=2, RED=3, BLUE=4
// Core: White=0, Black=1, Red=2, Blue=3
// 需要 -1 转换

func coreCharacterTypeToProto(char core.CharacterType) gamev1.CharacterType {
    return gamev1.CharacterType(char + 1)
}

func protoCharacterTypeToCore(char gamev1.CharacterType) core.CharacterType {
    return core.CharacterType(char - 1)
}

// ========== Player 转换 ==========

func corePlayerToProto(p *core.Player) *gamev1.PlayerState {
    return &gamev1.PlayerState{
        Id:        int32(p.ID),
        X:         p.X,
        Y:         p.Y,
        Direction: coreDirectionToProto(p.Direction),
        IsMoving:  p.IsMoving,
        Dead:      p.Dead,
        Character: coreCharacterTypeToProto(p.Character),
    }
}

func protoPlayerToCore(p *gamev1.PlayerState) *core.Player {
    return &core.Player{
        ID:        int(p.Id),
        X:         p.X,
        Y:         p.Y,
        Direction: protoDirectionToCore(p.Direction),
        IsMoving:  p.IsMoving,
        Dead:      p.Dead,
        Character: protoCharacterTypeToCore(p.Character),
    }
}

// ========== Bomb 转换 ==========

func coreBombToProto(b *core.Bomb) *gamev1.BombState {
    // 计算剩余时间（毫秒）
    elapsed := time.Since(b.PlacedAt)
    timeLeftMs := int32((b.TimeToBomb - elapsed).Milliseconds())

    return &gamev1.BombState{
        X:              float64(b.X),
        Y:              float64(b.Y),
        TimeLeftMs:     timeLeftMs,
        ExplosionRange: int32(b.ExplosionRange),
    }
}

func protoBombToCore(b *gamev1.BombState) *core.Bomb {
    return &core.Bomb{
        X:              int(b.X),
        Y:              int(b.Y),
        TimeToBomb:     time.Duration(b.TimeLeftMs) * time.Millisecond,
        ExplosionRange: int(b.ExplosionRange),
        // PlacedAt 需要根据当前时间推算
    }
}

// ========== Explosion 转换 ==========

func coreExplosionToProto(e *core.Explosion) *gamev1.ExplosionState {
    cells := make([]*gamev1.ExplosionCell, len(e.Cells))
    for i, cell := range e.Cells {
        cells[i] = &gamev1.ExplosionCell{
            GridX: int32(cell.GridX),
            GridY: int32(cell.GridY),
        }
    }

    elapsed := time.Since(e.StartTime)
    return &gamev1.ExplosionState{
        CenterX:    int32(e.CenterX),
        CenterY:    int32(e.CenterY),
        Range:      int32(e.Range),
        ElapsedMs:  elapsed.Milliseconds(),
        Cells:      cells,
    }
}

func protoExplosionToCore(e *gamev1.ExplosionState) *core.Explosion {
    cells := make([]core.ExplosionCell, len(e.Cells))
    for i, cell := range e.Cells {
        cells[i] = core.ExplosionCell{
            GridX: int(cell.GridX),
            GridY: int(cell.GridY),
        }
    }

    return &core.Explosion{
        CenterX:  int(e.CenterX),
        CenterY:  int(e.CenterY),
        Range:    int(e.Range),
        StartTime: time.Now().Add(-time.Duration(e.ElapsedMs) * time.Millisecond),
        Cells:    cells,
    }
}

// ========== Map 转换 ==========

func coreMapToProto(m *core.GameMap) *gamev1.MapState {
    // 转换 [][]core.TileType -> [][]int32
    grid := make([][]int32, m.Height)
    for y := 0; y < m.Height; y++ {
        grid[y] = make([]int32, m.Width)
        for x := 0; x < m.Width; x++ {
            grid[y][x] = int32(m.Tiles[y][x])
        }
    }

    return protocol.FlattenMap(grid)
}

func protoMapToCore(m *gamev1.MapState) (*core.GameMap, error) {
    grid, err := protocol.InflateMap(m)
    if err != nil {
        return nil, err
    }

    gameMap := &core.GameMap{
        Width:  int(m.Width),
        Height: int(m.Height),
        Tiles:  make([][]core.TileType, m.Height),
    }

    for y := 0; y < int(m.Height); y++ {
        gameMap.Tiles[y] = make([]core.TileType, m.Width)
        for x := 0; x < int(m.Width); x++ {
            gameMap.Tiles[y][x] = core.TileType(grid[y][x])
        }
    }

    return gameMap, nil
}
```

---

## 网络同步策略

### 服务器同步频率

- **60 TPS**（每秒 60 次状态广播）
- 每次发送完整的 `ServerState`
- 地图仅在变化时发送（优化）

### 客户端插值

对于其他玩家，使用 core.Player 的插值功能：

```go
// 在 Player.Update() 中自动处理
player.IsSimulated = true  // 启用插值模式

// 服务器更新时设置目标位置
player.SetNetworkPosition(serverX, serverY)
```

### 输入处理

- 客户端立即发送输入到服务器
- 服务器验证并更新位置
- 服务器广播新位置到所有客户端

**未来优化**：客户端预测 + 服务器和解

---

## 实现检查清单

### Phase 1: 基础服务器 ✅

- [ ] 实现 `GameServer` 结构
- [ ] 实现 TCP 监听和连接管理
- [ ] 实现 `handleJoin` 处理加入请求
- [ ] 实现玩家 ID 分配和出生点逻辑
- [ ] 实现 `gameLoop` 60 TPS 更新

### Phase 2: 状态同步

- [ ] 实现 `handleInput` 处理客户端输入
- [ ] 实现 `broadcastState` 广播游戏状态
- [ ] 创建 `pkg/protocol/converter.go` 转换函数
- [ ] 实现 core -> proto 数据转换
- [ ] 测试服务器独立运行

### Phase 3: 客户端连接

- [ ] 实现 `NetworkGame` 结构
- [ ] 实现连接服务器和发送 `JoinRequest`
- [ ] 实现接收 `GameStart` 消息
- [ ] 实现 `receiveLoop` 接收服务器状态
- [ ] 实现 `sendLoop` 发送客户端输入

### Phase 4: 渲染和插值

- [ ] 实现 `applyServerState` 应用服务器状态
- [ ] 本地玩家：直接更新位置
- [ ] 其他玩家：使用 `SetNetworkPosition` 插值
- [ ] 同步炸弹和爆炸效果
- [ ] 同步地图变化

### Phase 5: 游戏流程

- [ ] 实现游戏开始通知
- [ ] 实现游戏结束检测和通知
- [ ] 实现玩家断线处理
- [ ] 实现玩家加入/离开通知
- [ ] 添加 UI 显示（分数、玩家列表等）

### Phase 6: 优化和测试

- [ ] 添加网络延迟补偿
- [ ] 实现增量地图更新
- [ ] 添加输入序列号去重
- [ ] 多客户端测试（2-4 人）
- [ ] 压力测试和性能优化

---

## 快速开始示例

### 1. 生成 Proto 代码

```bash
cd api && buf generate
```

### 2. 启动服务器

```bash
go run cmd/server/main.go -addr=:8080
```

### 3. 启动客户端

```bash
go run cmd/client/main.go -addr=localhost:8080
```

---

## 调试技巧

### 查看网络消息

```go
// 在 Marshal/Unmarshal 时添加日志
log.Printf("Sending: %+v", packet)
data, _ := protocol.Marshal(packet)
log.Printf("Sent %d bytes", len(data))
```

### 监控帧率

```go
// 服务器和客户端都添加 FPS 计数器
if frameId%60 == 0 {
    log.Printf("TPS: 60")
}
```

### 测试插值

```go
// 人为添加延迟观察插值效果
time.Sleep(100 * time.Millisecond)
```

---

## 常见问题

### Q: 为什么枚举值不匹配？

A: Proto 枚举从 1 开始（0 保留为 UNSPECIFIED），而 Go 枚举从 0 开始。需要添加转换函数。

### Q: 如何处理客户端预测？

A: 当前版本未实现。需要为每个输入保存序列号，服务器返回确认后修正预测偏差。

### Q: 地图如何同步？

A: 仅在地图变化（砖块被炸毁）时发送 `MapState`。客户端比较 `frame_id` 判断是否需要更新。

### Q: 如何防止作弊？

A: 服务器验证所有输入和移动。客户端只发送输入，不发送位置。

---

## 相关文档

- [Proto 文件](../api/proto/bomberman/v1/game.proto)
- [Core 包文档](../pkg/core/README.md)
- [协议辅助函数](../pkg/protocol/helper.go)
- [CLAUDE.md](../CLAUDE.md)

---

**最后更新**: 2026-01-06
**维护者**: Claude Code
