# Bomberman AI 玩家开发设计文档

## 1. 需求背景

- **目标**：为游戏添加 AI 玩家功能。
- **场景**：
    1.  **服务器模式**：通过配置决定是否在房间中添加 AI。当玩家进入时，可以与 AI 对战。
    2.  **单机模式**：支持 AI 玩家（启用并完善被注释掉的方法）。
- **要求**：AI 的行为和表现应尽可能与普通用户一致，且复用现有的游戏逻辑。

## 2. 架构分析

为了最大化复用代码并保持架构清晰，我们将逻辑、渲染和网络分离。

### 核心分层

*   **pkg/core** (核心逻辑层): 包含游戏规则、状态、地图等。AI 的核心决策逻辑也应位于此层（或新建 `pkg/ai`），因为它是纯逻辑且两端共用的。
*   **Packet/Network** (协议层): 定义数据传输格式。AI 的行为最终转换为 Input 数据流，与真实玩家一致。
*   **Client** (单机/表现层): 负责渲染。在单机模式下，Client 需要负责实例化和驱动 AI。
*   **Server** (联机/控制层): 负责管理房间。在联机模式下，Server 负责实例化 AI 并在游戏循环中驱动它们。

## 3. 详细设计

### 3.1 核心逻辑 (pkg/ai)

新建 `pkg/ai` 包，实现 `AIController`。

*   **输入**:
    *   `Game State`: 地图数据、玩家位置、炸弹位置、火焰位置。
    *   `Self`: AI 自身的数据（ID、位置、速度）。
*   **输出**:
    *   `core.Input`: 模拟一帧的按键操作 (`Up`, `Down`, `Left`, `Right`, `Bomb`)。

**AI 决策流程 (每帧或每隔几帧)**:
1.  **安全评估 (Safety Map)**: 分析当前地图，标记出即将爆炸的炸弹范围（危险区）。
2.  **状态机/行为树**:
    *   **生存优先**: 如果当前位置在危险区，寻找最近的安全格子并移动。
    *   **进攻**: 如果安全，寻找可破坏的砖块或敌人。
    *   **放置炸弹**: 如果附近有目标且自身有退路，放置炸弹。
3.  **路径规划**: 使用 BFS 或 A* 算法计算移动路径。
4.  **输入生成**: 将下一步的移动方向转换为 `core.Input`。

### 3.2 客户端实现 (单机模式)

*   **位置**: `cmd/client/main.go` & `internal/client/player.go`
*   **改动**:
    1.  取消 `main.go` 中 `addAIPlayers` 的注释。
    2.  在 `client.Player` 结构中集成 `AIController`。
    3.  在 `Player.Update` 方法中：
        ```go
        if p.corePlayer.IsSimulated {
            // 如果是单机模式 (NetworkClient == nil)
            input := p.aiController.Decide(game)
            core.ApplyInput(game, p.ID, input, deltaTime)
        }
        ```

### 3.3 服务端实现 (联机模式)

*   **位置**: `internal/server/room.go`
*   **改动**:
    1.  **配置**: 添加启动参数/配置项（如 `AIMode` 或 `MinPlayers`）来决定是否开启 AI。
    2.  **生命周期**:
        *   当第一个真实玩家加入房间时，检查配置。如果需要 AI，则调用 `core.NewPlayer` 创建 AI 角色并加入 `game.Players`。
        *   Server 为每个 AI 玩家维护一个 `AIController` 实例。
    3.  **游戏循环 (Tick)**:
        *   在处理完网络包输入后，遍历所有 AI 玩家。
        *   调用 `controller.Decide()` 获取输入。
        *   调用 `core.ApplyInput()` 应用输入。
    4.  **广播**:
        *   AI 玩家状态会随着 `ServerState` 广播给所有客户端。
        *   客户端无需知道这是 AI，只需像渲染普通玩家一样渲染即可。

## 4. 实施步骤

1.  **基础框架搭建**:
    *   创建 `pkg/ai/controller.go`。
    *   定义基本的接口和结构体。
    *   实现一个最简单的"随机移动" AI 进行测试。

2.  **单机模式集成**:
    *   修改 `client/main.go` 启用 AI。
    *   修改 `client/player.go` 接入 `AIController`。
    *   验证 AI 能在单机地图中出现并移动。

3.  **服务端模式集成**:
    *   修改 `server/room.go`，在房间初始化时添加 AI。
    *   在 Server Tick 中驱动 AI。
    *   验证客户端连入服务器后能看到 AI 并在动。

4.  **AI 智能优化**:
    *   实现危险区域检测算法。
    *   实现寻路算法。
    *   实现躲避炸弹逻辑。

## 5. 难点与对策

*   **寻路性能**: Server 可能同时运行多个 AI，寻路算法不能太耗时。
    *   *对策*: 限制 AI 思考频率（例如每秒 10 次而不是 60 次），或者简单的启发式搜索。
*   **卡墙/对齐**: `core` 的移动逻辑需要对齐像素。
    *   *对策*: AI 计算的目标点应该是格子中心，利用现有的 `CornerCorrection` 或手动计算精确的轴向速度。