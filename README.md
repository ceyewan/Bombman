# Bomberman

基于 Go 语言和 Ebiten 引擎的多人在线炸弹人游戏，采用**权威服务器架构**，支持大厅匹配、断线重连、AI 对战等功能。

## 特性

- **单机/联机双模式**：可独立运行核心逻辑，也支持多人联机对战
- **权威服务器架构**：服务器维护唯一真相，60 TPS 游戏循环
- **平滑插值渲染**：其他玩家使用 LERP 插值，避免位置跳跃
- **TCP/KCP 双协议**：支持可靠 TCP 和低延迟 KCP 传输
- **大厅匹配系统**：支持创建房间、加入房间、房间列表、准备开始
- **断线重连**：断线后 60 秒内可重连，使用 KCP 协议恢复连接
- **AI 对战**：服务器可启用 AI 填充空位

## 环境要求

- Go 1.23+
- Buf CLI（Protobuf 代码生成）

## 快速开始

### 安装工具

```bash
make install-tools
```

### 生成 Protobuf 代码

```bash
make gen
```

### 运行游戏

```bash
# 启动服务器（TCP + AI）
make server

# 启动客户端（带大厅）
make client

# 或启动两个客户端测试
make clients
```

## 命令行参数

### 服务器 (cmd/server/main.go)

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8080` | 服务器监听地址 |
| `-proto` | `tcp` | 网络协议：`tcp` 或 `kcp` |
| `-enable-ai` | `false` | 是否启用 AI 玩家填充空位 |

**示例：**

```bash
# TCP 服务器（默认端口）
go run cmd/server/main.go

# KCP 服务器
go run cmd/server/main.go -proto=kcp

# 启用 AI 的服务器
go run cmd/server/main.go -enable-ai

# 自定义地址
go run cmd/server/main.go -addr=:9000 -proto=tcp -enable-ai
```

### 客户端 (cmd/client/main.go)

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-server` | `""` | 服务器地址（留空=单机模式） |
| `-proto` | `tcp` | 网络协议：`tcp` 或 `kcp` |
| `-character` | `0` | 角色类型：0=白, 1=黑, 2=红, 3=蓝 |
| `-control` | `wasd` | 控制方案：`wasd` 或 `arrow` |
| `-quick` | `false` | 跳过大厅，直接加入默认房间 |

**示例：**

```bash
# 单机模式
go run cmd/client/main.go

# 联机模式（大厅）
go run cmd/client/main.go -server=localhost:8080

# 快速加入默认房间
go run cmd/client/main.go -server=localhost:8080 -quick

# 使用黑色角色 + 方向键
go run cmd/client/main.go -server=localhost:8080 -character=1 -control=arrow
```

## Makefile 命令

| 命令 | 说明 |
|------|------|
| `make help` | 显示帮助信息 |
| `make build` | 编译服务器和客户端 |
| `make local` | 启动单机版游戏 |
| `make server` | 启动联机服务器（TCP + AI） |
| `make client` | 启动联机客户端（带大厅） |
| `make clients` | 启动两个客户端（测试用） |
| `make gen` | 生成 Protobuf 代码 |
| `make clean` | 清理生成的文件 |
| `make help-dev` | 显示开发命令详细说明 |

## 项目结构

```
bomberman/
├── api/                    # Protobuf 定义和生成代码
│   ├── gen/               # 生成的 Go 代码
│   └── proto/             # .proto 源文件
├── pkg/                   # 共享包（客户端+服务器）
│   ├── core/              # 游戏核心逻辑
│   └── protocol/          # 协议辅助方法
├── cmd/                   # 可执行程序入口
│   ├── client/            # 客户端主程序
│   └── server/            # 服务器主程序
└── internal/              # 内部实现
    ├── client/            # 客户端内部逻辑
    │   ├── game.go        # 单机游戏
    │   ├── network.go     # 网络管理器
    │   ├── lobby_client.go # 大厅客户端
    │   └── network_game.go # 联机游戏
    └── server/            # 服务器内部逻辑
        ├── game_server.go # 服务器主入口
        ├── listener.go    # 传输层抽象（TCP/KCP）
        ├── connection.go  # 连接管理
        ├── room_manager.go # 房间管理
        └── room.go        # 房间和游戏循环
```

## 核心架构

### 权威服务器模式

- 服务器运行完整的游戏逻辑，60 TPS 更新
- 客户端发送输入，接收服务器状态进行渲染
- 本地玩家使用预测减少延迟感
- 其他玩家使用插值平滑显示

### 大厅系统

- 客户端连接后进入大厅界面
- 可查看房间列表、创建房间、加入房间
- 房间内所有玩家准备好后房主可开始游戏
- 游戏结束后返回大厅

### 断线重连

- 客户端 5 秒无收包视为断线
- 断线后玩家状态保留 60 秒
- 重连时使用 KCP 协议建立新连接
- 服务器恢复玩家连接，同步当前游戏状态

## 游戏参数

- **屏幕尺寸**：640x480
- **格子大小**：32x32
- **地图尺寸**：20x15
- **服务器 TPS**：60
- **客户端 FPS**：60
- **最大玩家数**：4

## 网络协议

见 [api/proto/bomberman/v1/game.proto](api/proto/bomberman/v1/game.proto)

**消息类型：**

| 方向 | 消息 | 说明 |
|------|------|------|
| C→S | ClientInput | 客户端输入 |
| C→S | JoinRequest | 加入大厅 |
| C→S | CreateRoomRequest | 创建房间 |
| C→S | JoinRoomRequest | 加入房间 |
| C→S | RoomActionRequest | 房间操作（准备/开始/离开） |
| C→S | ReconnectRequest | 重连请求 |
| S→C | ServerState | 游戏状态同步 |
| S→C | GameStart | 游戏开始 |
| S→C | GameEvent | 游戏事件 |
| S→C | RoomListResponse | 房间列表 |
| S→C | RoomStateUpdate | 房间状态更新 |
| S→C | ReconnectResponse | 重连响应 |
