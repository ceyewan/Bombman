# AI 系统（行为树版）

本目录使用行为树重构 AI 逻辑，避免单体文件，并将“生存优先 + 攻击 + 游荡”拆分为清晰的节点。

## 行为树结构

- Root Selector
  - Survival Sequence（最高优先级）
    - IsInDanger
    - FindSafe
    - MoveToSafe
  - Attack Sequence（敌人优先）
    - HasBombCapacity
    - FindTarget（敌人优先，其次砖块）
    - PreCheckEscape（模拟放置能否逃生）
    - MoveToTarget
    - PlaceBomb
  - Wander Action（低优先级兜底）

## 关键策略

- 危险热力图（DangerField）基于服务器帧，计算每个格子最早爆炸时间。
- 连锁爆炸：若 A 爆炸覆盖 B，则 B 的实际爆炸帧取 min。
- 放炸弹前必须进行逃生模拟（BFS + BombFuseFrames 时间窗口）。
- 思考频率保留（性能考虑），进入危险或炸弹数量变化时强制刷新。

## 文件说明

- `controller.go`：AI 控制器，驱动行为树，缓存输入与思考节流。
- `bt/`：行为树基础设施（Selector / Sequence / Action / Condition）。
- `blackboard.go`：黑板数据，跨节点共享状态。
- `danger_field.go`：危险热力图与连锁爆炸传播。
- `pathfinder.go`：BFS 路径与逃生模拟。
- `behaviors_survival.go`：生存分支节点。
- `behaviors_attack.go`：攻击分支节点（敌人优先）。
- `behaviors_wander.go`：游荡兜底节点。
