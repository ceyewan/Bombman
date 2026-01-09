# 联机延迟与抖动问题记录与优化计划

## 当前问题（仍存在的本地抖动）

- 本地玩家在“预测移动 → 收到权威状态”之间出现轻微回退（橡皮筋感）。
- 远端玩家插值已很顺滑，但本地自己视角仍能感觉到小幅抖动。

## 可能原因（推测优先级）

1. **预测帧与服务器帧不完全一致**  
   客户端使用 `EstimatedServerFrame + InputLeadFrames` 生成输入帧号，估计会抖动；服务器按 `r.frameID` 消费输入。帧对不上时，回放会产生小偏差。

2. **客户端碰撞/地图状态与服务器存在 1 帧差异**  
   本地预测依赖 `core.ApplyInput` 的碰撞和拐角修正逻辑，但地图/炸弹/爆炸的同步存在一帧或数帧差异，导致预测位置与权威位置不一致。

3. **硬回滚 + 无阈值纠偏**  
   当前每次拿到权威状态就直接覆盖位置并重放输入，小误差也会被“立刻纠正”，肉眼可见。

4. **输入确认缺失（按帧过滤不够精确）**  
   目前仅按 `frameID` 丢弃 `pendingInputs`，而非使用服务器返回的 `LastProcessedSeq` 来精准确认输入是否被处理，导致回放长度不稳定。

5. **渲染与模拟未分离**  
   本地渲染直接使用模拟位置，预测与纠偏造成的细微跳变没有经过视觉平滑处理。

## 接下来的优化方案（按优先级）

1. **用 `LastProcessedSeq` 精确确认输入**  
   - 服务器已经在状态包中返回 `LastProcessedSeq`。  
   - 客户端改为以 `seq` 为主清理 `pendingInputs`，降低误删/漏删造成的抖动。

2. **纠偏平滑（阈值 + 逐步修正）**  
   - 误差 < `ReconciliationSmoothThreshold`：使用 LERP 缓慢逼近权威位置。  
   - 误差 >= 阈值：瞬间拉回。

3. **客户端模拟帧对齐**  
   - 维护 `clientSimFrame`，以 `EstimatedServerFrame + lead` 为目标帧。  
   - 每帧执行固定步进，把模拟帧追到目标帧（而非每帧只算一次）。

4. **渲染/模拟分离（Presentation Layer）**  
   - 保存 `simX/simY` 与 `renderX/renderY`。  
   - 渲染位置对模拟位置做平滑跟随，本地也能“视觉稳定”。

5. **自适应参数**  
   - `InputLeadFrames` 基于 RTT 动态调整。  
   - `InterpolationDelayMs` 根据 jitter 自适应。

6. **可选：服务器输入延迟缓冲**  
   - 服务器统一延迟处理 1~2 帧输入可减少抖动，但会牺牲部分实时性。  
   - 仅在抖动仍明显时考虑。

## 验证指标（建议）

- `local_error_px`：本地预测与权威位置的误差（平均/峰值）。
- `reconcile_per_sec`：每秒纠偏次数。
- `rtt_ms` 与 `jitter_ms`（Ping/Pong 统计）。

## 相关代码位置

- 客户端预测与回放：`internal/client/network_game.go`
- Ping/Pong 对表：`internal/client/network.go`
- 服务器输入缓冲：`internal/server/room.go`
- 输入确认字段：`pkg/protocol/helper.go`
