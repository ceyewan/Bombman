# AI 系统（行为树版）

本目录使用行为树重构 AI 逻辑，避免单体文件，并将"生存优先 + 攻击 + 游荡"拆分为清晰的节点。

## 行为树结构

```
Root Selector
├── Survival Sequence（最高优先级）
│   ├── IsInDanger       - 检查是否在危险区域
│   ├── FindSafe         - 查找安全点（时间敏感）
│   └── MoveToSafe       - 移动到安全点
│
├── Attack Sequence（攻击）
│   ├── HasBombCapacity  - 检查是否还能放炸弹
│   ├── FindTarget       - 寻找目标（根据配置优先炸砖块或敌人）
│   ├── PreCheckEscape   - 预检查放炸弹后能否逃生
│   ├── MoveToTarget     - 移动到目标位置
│   └── PlaceBomb        - 放置炸弹
│
└── Wander Action（游荡兜底）
    └── 选择安全方向移动
```

## 智力控制系统

AI 支持两个难度等级，通过 `AIConfig` 配置：

```go
// 预设配置：普通难度
var AIConfigNormal = AIConfig{
    ThinkIntervalFrames: 6,    // 100ms 思考间隔
    MistakeRate:         0.05, // 5% 失误率
    FullChainRecursion:  false, // 简化连锁爆炸计算
    PreferBricks:        true,  // 优先炸砖块开路
}

// 预设配置：困难难度
var AIConfigHard = AIConfig{
    ThinkIntervalFrames: 3,    // 50ms 思考间隔
    MistakeRate:         0.0,  // 无失误
    FullChainRecursion:  true, // 完整连锁爆炸计算
    PreferBricks:        false, // 敌人优先
}
```

### 配置参数说明

| 参数 | 说明 |
|------|------|
| `ThinkIntervalFrames` | 思考间隔（帧），值越小 AI 反应越快 |
| `MistakeRate` | 随机失误率 (0.0-1.0)，值越高 AI 越容易犯错 |
| `FullChainRecursion` | 是否启用完整连锁爆炸计算 |
| `PreferBricks` | 是否优先炸砖块而非追击敌人 |

### 使用方法

```go
// 使用默认配置（普通难度）
ai := NewAIController(playerID)

// 使用困难配置
ai := NewAIControllerWithConfig(playerID, &AIConfigHard)

// 自定义配置
customConfig := &AIConfig{
    ThinkIntervalFrames: 4,
    MistakeRate:         0.02,
    FullChainRecursion:  true,
    PreferBricks:        true,
}
ai := NewAIControllerWithConfig(playerID, customConfig)

// 动态修改配置
ai.SetConfig(&AIConfigHard)
```

## 关键策略

### 危险热力图

`DangerField` 基于服务器帧计算每个格子的最早爆炸时间：

- `Earliest[y][x]`: 该格子最早的爆炸帧
- `Level[y][x]`: 危险等级 (0.0-1.0)
- `InDanger(x, y)`: 当前是否在危险区
- `SafeAtFrame(x, y, frame)`: 指定帧时是否安全

### 连锁爆炸

若炸弹 A 的爆炸覆盖炸弹 B，则 B 的实际爆炸帧取 min(A.ExplodeAt, B.ExplodeAt)。

- **普通难度**: 只计算一层连锁
- **困难难度**: 完整递归计算所有连锁

### 逃生时间计算

修复了关键 bug：移动一格需要 16 帧（而非之前错误的 1 帧）

```go
// framesPerTile = TileSize / PlayerSpeedPerFrame = 32 / 2 = 16
nextFrame := n.Frame + framesPerTile
```

### 安全点查找

时间敏感的 BFS 搜索：

1. 首先检查"原地不动"是否安全（等炸弹爆炸后该位置仍安全）
2. 使用 BFS 搜索，考虑到达每个格子的时间
3. 用 `SafeAtFrame(x, y, arriveFrame)` 检查到达时是否安全

### 方向惯性

减少移动抖动：

- 记录上一帧方向和持续帧数
- 如果上一方向仍可行且未超过惯性帧数，继续保持
- 最多保持 8 帧同一方向

### 游荡行为

改进的游荡逻辑：

- 优先选择安全且可行走的方向
- 保持同一方向约 30 帧（0.5 秒）
- 遇到障碍或危险区才换方向

## 文件说明

| 文件 | 说明 |
|------|------|
| `config.go` | AI 配置结构和预设 |
| `controller.go` | AI 控制器，驱动行为树，应用失误率 |
| `bt/bt.go` | 行为树基础设施（Selector / Sequence / Action / Condition） |
| `blackboard.go` | 黑板数据，跨节点共享状态 |
| `danger_field.go` | 危险热力图与连锁爆炸传播 |
| `pathfinder.go` | BFS 路径与逃生模拟 |
| `behaviors_survival.go` | 生存分支节点（时间敏感的安全点查找） |
| `behaviors_attack.go` | 攻击分支节点（支持优先级配置） |
| `behaviors_wander.go` | 游荡兜底节点（避开危险区） |

## 调试建议

1. **AI 不躲炸弹**: 检查 `InDanger` 返回值，确认危险热力图是否正确更新
2. **AI 炸死自己**: 检查 `canEscapeAfterPlacement` 的帧计算，确认使用 `framesPerTile`
3. **AI 抖动**: 检查方向惯性是否生效，`DirectionFrames` 是否正确累加
4. **AI 不炸砖块**: 检查 `PreferBricks` 配置，以及 `findBrickTarget` 返回值
5. **AI 失误太多/太少**: 调整 `MistakeRate` 参数
