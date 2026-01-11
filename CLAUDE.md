# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**重要：必须使用中文回答用户的所有问题和解释。**

## 项目概述

Bomberman 是一个使用 Go 语言和 Ebiten 游戏引擎编写的多人联机游戏。项目采用**权威服务器架构**，核心逻辑与网络层完全解耦。

### 核心特性

- **权威服务器**：服务器维护唯一真相，60 TPS 游戏循环
- **大厅匹配系统**：房间列表、创建/加入房间、准备开始
- **断线重连**：断线后 60 秒内可重连，使用 KCP 协议恢复
- **TCP/KCP 双协议**：支持可靠 TCP 和低延迟 KCP 传输
- **平滑插值渲染**：其他玩家使用 LERP 插值避免位置跳跃

## 快速开始

```bash
# 生成 Protobuf 代码
make gen

# 启动服务器（TCP + AI）
make server

# 启动客户端（带大厅）
make client

# 启动两个客户端测试
make clients
```

### 命令行参数

**服务器** ([cmd/server/main.go](cmd/server/main.go)):
| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8080` | 监听地址 |
| `-proto` | `tcp` | 协议：tcp/kcp |
| `-enable-ai` | `false` | 启用 AI 填充空位 |

**客户端** ([cmd/client/main.go](cmd/client/main.go)):
| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-server` | `""` | 服务器地址（留空=单机） |
| `-proto` | `tcp` | 协议：tcp/kcp |
| `-character` | `0` | 角色：0=白, 1=黑, 2=红, 3=蓝 |
| `-control` | `wasd` | 控制：wasd/arrow |
| `-quick` | `false` | 跳过大厅直接加入默认房间 |

## 架构设计

### 核心设计原则

1. **完全解耦**：`pkg/core` 不依赖网络代码，可独立运行
2. **权威服务器**：服务器运行完整游戏逻辑，客户端只负责渲染和输入
3. **单线程房间**：每个房间使用单 goroutine + channel 处理消息，避免锁竞争

### 目录结构

```
bomberman/
├── pkg/
│   ├── core/              # 游戏核心逻辑（无网络依赖）
│   └── protocol/          # Protobuf 转换辅助
├── internal/
│   ├── server/
│   │   ├── game_server.go # 服务器入口、连接路由
│   │   ├── room_manager.go # 房间管理、匹配逻辑
│   │   ├── room.go        # 房间：60 TPS 游戏循环、消息处理
│   │   ├── listener.go    # TCP/KCP 传输层抽象
│   │   └── connection.go  # 连接包装、Session 接口
│   └── client/
│       ├── network.go     # 网络管理器、断线重连
│       ├── lobby_client.go # 大厅 UI 和状态管理
│       ├── network_game.go # 联机游戏状态同步、插值
│       └── game.go        # 单机游戏
└── api/proto/bomberman/v1/game.proto  # 协议定义
```

## 关键实现

### 1. 房间生命周期

**房间状态流转**（[internal/server/room.go](internal/server/room.go)）:
```
WAITING -> COUNTDOWN -> PLAYING -> GAME_OVER -> WAITING
```

**玩家加入流程**:
1. 客户端连接后发送 `JoinRequest`（room_id="" 表示快速匹配）
2. `RoomManager.JoinLobby` 查找或创建房间
3. 房间运行在独立 goroutine，通过 channel 接收消息：
   - `joinCh` - 加入请求
   - `reconnectCh` - 重连请求
   - `inputCh` - 玩家输入
   - `leaveCh` - 玩家断线
   - `actionCh` - 房间操作（准备/开始/离开）

### 2. 断线重连机制

**客户端检测**（[internal/client/network.go](internal/client/network.go)）:
- `checkHealthLoop`：5 秒无收包视为断线
- 超时后触发 `Reconnect()` 方法

**重连流程**:
```go
// 1. 关闭旧连接，等待所有 goroutine 结束
nc.Close()
nc.wg.Wait()

// 2. 重置所有内部状态（关键！）
nc.resetInternalState()  // 清空通道、重置序号、重建 context

// 3. 使用 KCP 建立新连接
conn := nc.dialKCP()

// 4. 发送 ReconnectRequest（携带 session_token）
// 5. 等待 ReconnectResponse（包含当前 GameState）
```

**服务器处理**（[internal/server/room.go](internal/server/room.go)）:
- `handleLeave`：断线时软删除，玩家进入 `offlinePlayers` 列表（保留 60 秒）
- `TryReconnect`：支持在线替换连接或离线恢复
- 超时后调用 `handleForceLeave` 硬删除

### 3. 网络同步

**输入同步**（[internal/server/room.go](internal/server/room.go)）:
```go
// 每帧应用输入后清空队列
func (r *Room) processInputs() {
    for playerID, input := range r.inputQueue {
        player := r.game.GetPlayer(playerID)
        if player != nil && !player.Dead {
            player.ApplyInput(input)
        }
    }
    r.inputQueue = make(map[int32]*gamev1.ClientInput)
}
```

**状态广播**（60 TPS）:
- `ServerState` 包含：frame_id、players、bombs、explosions、tile_changes
- 地图只在 `GameStart` 时全量发送，游戏期间只发爆炸清除的砖块

### 4. 插值系统

**本地玩家**：直接使用服务器位置，不插值
**其他玩家**：使用 LERP 插值平滑过渡

```go
// pkg/core/player.go
func (p *Player) updateLerp(deltaTime float64) {
    if !p.IsSimulated {
        return  // 本地玩家不插值
    }
    p.LerpProgress += p.LerpSpeed * deltaTime
    t := min(1.0, p.LerpProgress)
    p.X = p.LastNetworkX + (p.NetworkX - p.LastNetworkX) * t
    p.Y = p.LastNetworkY + (p.NetworkY - p.LastNetworkY) * t
}
```

### 5. 传输层抽象

**Session 接口**（[internal/server/connection.go](internal/server/connection.go)）:
```go
type Session interface {
    Send(msg proto.Message) error
    SetPlayerID(id int32)
    SetRoomID(id string)
    // ...
}
```

支持 TCP 和 KCP 切换，KCP 用于重连（低延迟）。

## 协议定义

**关键消息类型**（[api/proto/bomberman/v1/game.proto](api/proto/bomberman/v1/game.proto)）:

| 方向 | 消息 | 说明 |
|------|------|------|
| C→S | JoinRequest | 加入大厅，room_id=""=快速匹配 |
| C→S | RoomAction | 房间操作（准备/开始/离开） |
| C→S | ReconnectRequest | 重连请求（携带 session_token） |
| C→S | ClientInput | 玩家输入（含多帧数据） |
| S→C | JoinResponse | 加入成功，含 session_token |
| S→C | RoomListResponse | 房间列表 |
| S→C | RoomStateUpdate | 房间状态更新 |
| S→C | GameState | 完整游戏状态 |
| S→C | ReconnectResponse | 重连成功，含当前状态 |

## 常量配置

**服务器**（[internal/server/room.go](internal/server/room.go)）:
- `ServerTPS = 60` - 游戏更新频率
- `OfflinePlayerTimeout = 60s` - 离线玩家保留时间
- `MaxPlayers = 4` - 最大玩家数

**客户端**（[internal/client/network_game.go](internal/client/network_game.go)）:
- `FPS = 60` - 渲染帧率
- `LerpSpeed = 10.0` - 插值速度（约 100ms 跟上服务器）

**地图**（[pkg/core/constants.go](pkg/core/constants.go)）:
- `MapWidth = 20`, `MapHeight = 15`
- `TileSize = 32`
- `PlayerSpeed = 120` 像素/秒

## 注意事项

### 修改网络代码时
1. 确保 goroutine 能正确退出（使用 `context.Context` 和 `sync.WaitGroup`）
2. 避免阻塞在 `binary.Read` 等调用，设置 `SetReadDeadline`
3. 重连时必须调用 `resetInternalState` 清空通道

### 修改房间逻辑时
1. 所有房间内消息通过 channel 发送到房间 goroutine 处理
2. 不要在多个 goroutine 中直接修改房间状态
3. 离线玩家在 `offlinePlayers` 中保留，超时才删除

### 添加新消息类型
1. 在 `game.proto` 中定义
2. 运行 `make gen` 生成代码
3. 在 `MessageType` 枚举中添加类型
4. 在 `codec.go` 中注册类型
5. 在 `handleMessage` 中处理

## 开发命令

```bash
make gen          # 生成 Protobuf 代码
make build        # 编译
make clean        # 清理生成文件
make help-dev     # 详细帮助
```
