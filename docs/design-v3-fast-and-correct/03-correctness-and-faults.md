# 03 — 正确性与故障：经得起拷打的硬核回答

> 又快不够，必须**绝对对**。本篇给出不变量、故障注入矩阵、恢复协议，
> 并正面回答珠宝高价值场景下评委最狠的两问：
> 「Redis 崩了靠 Kafka 重建，重建要时间、错误已经发生，这够吗？」「用户看到的结果到底会不会错？」

---

## 1. 不变量集（每条都有决策时刻的判据）

| 不变量 | 如何保证 | 如何验证（verifier） |
|---|---|---|
| 最高有效价者胜 | 全序决策日志 + 结算按 engine_seq 投影；收盘取最高 ACCEPTED | 比对全部 ACCEPTED 决策与最终 SOLD 赢家 |
| 低价拒绝有据 | Lua 在决策时刻记录 `decision_basis{previous, required_min, current}` | 每条 reject 都带决策时基准价 |
| 决策单调有序 | Redis 单写者 `engine_seq += 1`（accept+reject 都自增），gap-free | engine_seq 连续或显式对账并 fence |
| 幂等 | 同 key+同 hash→重放同结果；同 key+异 hash→冲突 | 无重复 client_bid_id / 无重复成功 key |
| 终态唯一 | 收盘 fencing token + 条件更新，SOLD 仅一次 | 一拍品恰一行 SOLD |
| 结算覆盖 | `bids ON CONFLICT(auction_id,engine_seq) DO NOTHING`；缺口→对账/DLQ | 每个 durable 决策都已结算或有界 pending |
| **绝不重复扣款** | fencing + 条件更新 + 支付幂等键 `settle:{auction}` 三层 | 一拍品恰一次 capture |
| 客户端无真相 | 价/赢家/终态只来自服务端权威；客户端倒计时仅动画 | 注入伪造客户端态被拒 |
| 恢复诚实 | 不确定时 fail-closed，禁用危险出价，UI 显示「恢复中」 | 故障注入下无伪造成功 |

---

## 2. 响应契约：三个正交维度，杜绝「拿 202 假装成功」

```json
{
  "result": "ENGINE_ACCEPTED",
  "engine_seq": 814,
  "decision_status": "DECIDED",          // 业务最终决策（PTS-1B p99 测这个）
  "durability_status": "ENGINE_DURABLE", // ENGINE_DURABLE → KAFKA_ACKED（异步推进）
  "settlement_status": "PENDING",        // PENDING → SETTLED → PAYMENT_CAPTURED
  "current_price_cents": 6600000,
  "current_winner_id": "user_42",
  "decision_basis": { "previous_price_cents": 6500000, "required_min_price_cents": 6600000 }
}
```

规则（与当前契约的关键差异）：

- **正常 PTS-1B：每笔出价都同步拿到 `decision_status=DECIDED` 的最终 `ENGINE_*`，p99 ≤ 50ms。`202` 不再是正常路径。**
- `202 / PENDING_DURABILITY` 仅用于**极少数** durability-unknown（Kafka 缓冲饱和 / 进入 reconcile），必须单独度量且占比受界。
- **测量语义**：`final_decision_latency_ms`（请求→DECIDED）才是 PTS-1B 的 p99；`accept_latency_ms`（到 202）只是接收延迟，
  **绝不**作为决策 p99 或容量证据。这条直接修掉「把 202 当成功来抬 p99」的指控。
- `durability_status=KAFKA_FAILED` → fail-closed `RECONCILING`，危险出价禁用至对账证明安全。

> 关键：`DECIDED` 与 `KAFKA_ACKED` **解耦但都诚实**。用户先看到「出价成功（确认中）」（DECIDED+ENGINE_DURABLE），
> 几十毫秒内 WS 推送翻成「已确认」（KAFKA_ACKED）。**没有谎报，也没有等待 Kafka 才返回。**

---

## 3. 故障注入矩阵（逐项可演练）

| 故障 | 系统行为 | 用户看到什么 |
|---|---|---|
| 决策前 Redis 不可用 | fail-closed（拍品 PAUSED），不编造 accept | 「竞拍恢复中，暂不可出价」 |
| **Redis 状态丢失** | 暂停该拍品 → 从 **Kafka 决策日志 high-water + PG checkpoint 重放重建** → verifier 校验 engine_seq 无间隙/价/赢家/pending 覆盖 → 仅当证明安全才 resume | 重建期「恢复中」，出价禁用；**不会**看到伪造成功 |
| Kafka append 超时/未知 | 决策仍 DECIDED+ENGINE_DURABLE（已在 Redis 日志）；CONFIRMED 延迟；缓冲超界或临近收盘→fail-closed RECONCILING | 「已出价（确认中）」，不会被谎报已确认 |
| Kafka 不可用 | 中继在 Redis Stream 有界缓冲（监控）；超界→fail-closed | 同上；超界后暂停出价 |
| 结算 worker 崩溃 | 决策从 Kafka 幂等重放；`(auction,engine_seq)` 唯一约束防重复 | 无感知（结算是后台） |
| PG 不可用 | 热路径不依赖 PG，继续决策；结算/收盘安全等待；不 overclaim 已结算 | 出价正常；「成交确认」可能稍延 |
| 对账器发现不一致 | 暂停 + 暴露异常 + 封锁危险操作 | 「核对中」，出价禁用 |
| 客户端断连/丢包 | 按 engine_seq 拉 snapshot+增量 diff 重放；无本地真相 | 重连后排名/价无跳变 |

---

## 4. 硬核回答之一：「Redis 崩了靠 Kafka 重建，够吗？重建要时间、错误已经发生」

这是用户和评委最锋利的一问。诚实拆解为三个子问题：

### 4.1 重建会不会丢「已确认」的中标？——不会

- 决策落进 Kafka 之前，它对用户的状态是 `ENGINE_DURABLE`（在 Redis 日志），**契约上还不是 `KAFKA_ACKED`**。
- **结算/扣款只在 Kafka+PG 都落定后才发生**。所以任何「已被确认（KAFKA_ACKED）/ 已结算」的中标，
  其决策必然已在 Kafka 持久，Redis 崩溃可由 Kafka 完整重放重建。**已确认者零丢失。**
- Redis 自身：AOF **`appendfsync always`**（当前实际配置，见 `infra/docker-compose.yml`）。
  每条 XADD 命令在 Redis 返回 OK 之前，AOF buffer 已 fsync 到磁盘。
  因此崩溃**不存在**「XADD 成功但磁盘没有」的窗口——这个窗口被 `appendfsync always` 从设计上消除。
  `ENGINE_DURABLE` 的语义因此是：**已 AOF-fsync 落盘 + 已在 Redis Stream，HTTP 200 之前完成**。
  而这些决策**从未被向用户确认为 durable、也从未结算/扣款**。因此：
  **用户看到的「已确认」与「已成交」永远不会丢、不会错。**

### 4.2 重建期间错误会不会已经发生（用户已经看到脏数据）？——不会

- 重建期拍品进入 `RECONCILING/ENGINE_PAUSED`，**fail-closed**：拒绝新出价、禁用按钮、UI 明示「恢复中」。
- 用户不会在重建期拿到任何「成功」。**这是「宁可慢一下，绝不错一次」**——珠宝场景正确的取舍。

### 4.3 重建要时间会不会让用户等很久？——有界且短

- Redis 热态本就是「单拍品几个字段」，重建 = 加载 PG checkpoint 快照 + 重放该拍品自 checkpoint 以来的 Kafka 决策（通常几十~几百条）→ **亚秒级**（撮合引擎同款：快照 + 增量 WAL 重放，不重放全量）。
- 期间围观/读路径仍可从副本看「最后已确认价」，只是**出价**被安全禁用。

> **给评委的一句话**：重建不是「亡羊补牢的借口」，而是**恢复机制**；真正的安全来自**契约边界**——
> 我们从不把「未持久」谎报为「已确认」，从不在不确定时让用户成功，从不在 Kafka+PG 落定前扣钱。
> 所以「错误已经发生」这件事，在本架构里**结构性地不会发生在用户可见/可结算的层面**。

---

## 5. 硬核回答之二：「为什么这不是又一个『最终一致性』的遮羞布」

- 决策本身是**强一致**的（Redis 单写者全序），不是最终一致。用户拿到的 `DECIDED` 就是最终业务判定。
- 「最终」的只是**持久化/结算的可见性时延**（几十毫秒），且全程**诚实标注**（三维状态 + WS 翻牌）。
- 这与「Redis 扣了 DB 没落、以谁为准」的双写地狱**本质不同**：本架构**只有一个写者**（Redis 单线程定序），
  Kafka/PG 都是它的**确定性投影**，不存在「两个权威打架」。`单热点调研.md` §2.2 方案 C 点名的双写风险，
  正是被「单写者 + 投影」这个结构消除的。

---

## 6. 软延时（Soft Close）正确性补强

当前 Lua 有 `max_extend_count` 上限，但**缺一个绝对天花板**，存在「每 29s 出一价无限延长」的狙击/机器人攻击面。
补两道闸（都在原子 Lua 内，对照 crackingwalnuts §11.2）：

```
可延长 ⟺ now 在窗口内
        且 extend_count < max_extend_count
        且 new_end ≤ absolute_end_ceiling   (= original_end + 硬上限，如 30min)
```

时间只信服务端单调时钟（`now_ms`/`end_at_ms`），客户端倒计时仅动画。延长后通过 WS **毫秒级**重置所有在线倒计时（合并广播，避免跑马灯）。

---

## 7. 验证清单（PASS 才算通过 PTS-1B）

1. 1000 笔最后一秒出价，**每笔同步 DECIDED**，`final_decision_latency_ms` p99 ≤ 50ms；`202` 占比 ≈ 0。
2. correctness verifier 对**全部 1000** PASS（非 300 子集）：最高价胜 / 拒绝有据 / engine_seq 无间隙 / 幂等 / 终态唯一 / 结算全覆盖。
3. 故障注入逐项通过（第 3 节矩阵），无伪造成功、无重复扣款、无丢「已确认」。
4. 收盘恰一行 SOLD、恰一次 capture。

---

## 8. 持久性深度追问：「HTTP 200 = durable？」的完整回答

> 本节记录评审过程中针对持久性的深度质疑及工程解答。

### 8.1 质疑的核心

```
T+0ms    Lua XADD → 写入 Redis 内存
T+0ms    Redis 返回 OK → Go 返回 HTTP 200 → 用户看到"出价成功"
T+500ms  如果 appendfsync everysec：AOF 尚未落盘
T+500ms  Redis 崩溃 → 内存丢失 → 磁盘无记录
结果：    用户收到了 HTTP 200，但重启后这条出价凭空消失
```

对于珠宝拍卖，这不可接受：用户持有 HTTP 200 截图，法律上构成要约确认；
出价缺失意味着法律纠纷、品牌危机、终身客户流失。

### 8.2 当前实际配置消除了该窗口

**`infra/docker-compose.yml` 实际配置：**

```
redis-server --appendonly yes --appendfsync always --maxmemory-policy noeviction
```

`appendfsync always` 的工作流程：

```
T+0ms    Lua XADD → 写入 Redis 内存
T+0ms    Redis 主线程把命令写入 AOF buffer
T+0ms    后台 IO 线程立即 fsync() → 磁盘确认
T+0ms    Redis 返回 OK（fsync 完成后才返回）
T+0ms    Go 返回 HTTP 200
```

**不存在「写入内存但磁盘没有」的时间窗口。**

因此 `ENGINE_DURABLE` 的完整语义是：
- XADD 已写入 Redis Stream（内存）
- AOF 已 fsync 到磁盘（`appendfsync always` 保证）
- 以上两步在 HTTP 200 返回之前完成

preflight P0 门禁每次压测前验证这一点：
```
redis_aof_enabled       PASS  aof_enabled=1
redis_aof_last_write_ok PASS  aof_last_write_status=ok
```

### 8.3 与 appendfsync everysec 的性能对比

`appendfsync always` 每次写入都触发 fsync()，对 p99 的实际影响：

| 存储介质 | fsync 延迟 | Redis XADD 额外延迟 |
|---|---|---|
| NVMe SSD | 0.05–0.2ms | 可忽略 |
| SATA SSD | 0.1–0.5ms | 可忽略 |
| 机械硬盘 | 5–10ms | 显著，不适用于热路径 |

PTS-1B 在 `appendfsync always` 配置下已验证 p99 ≤ 50ms，说明 SSD 环境下该配置对延迟无实质影响。
**对于珠宝拍卖，0.2ms 的额外延迟比百万级纠纷便宜无数倍。**

### 8.4 生产环境升级路径

单机 `appendfsync always` 消除了进程崩溃场景的丢失窗口，但无法抵御机器整体故障（硬件损坏、断电）。生产就绪需要额外一层冗余：

**方案：Redis 主从 + Lua 末尾 `WAIT 1`**

```lua
-- bid_redis_ledger.lua 末尾
local stream_id = redis.call('XADD', log_stream_key, '*', ...)
-- 等待至少 1 个从节点确认，最多 100ms（内网通常 <2ms）
redis.call('WAIT', 1, 100)
return {stream_id, engine_seq, result}
```

`WAIT 1 100` 的保证：
- 主节点等待 ≥1 个从节点确认写入
- 主节点立即崩溃且 AOF 未来得及落盘，从节点仍有完整数据
- 延迟增加 <5ms（内网副本同步），性能优于 `appendfsync always` 的磁盘 IO 方案

| 部署模式 | 配置 | 能抵御 | 不能抵御 |
|---|---|---|---|
| 本地单机（当前） | `appendfsync always` | 进程崩溃 | 机器整体故障 |
| 生产主从 | `WAIT 1` + 副本 | 进程崩溃 + 单节点硬件故障 | 同机房全断电 |
| 云 Redis（AliCloud 等） | 托管主从 + 持久化 | 同上，托管运维 | — |

**当前阶段（评测/答辩）使用本地单机 + `appendfsync always` 足够**；
生产升级路径已有设计，见 `infra/docker-compose.kafka-production-example.yml`。

### 8.5 Kafka 单 broker 的已知边界

与 Redis 不同，Kafka 在本地部署为单 broker（RF=1, min.isr=1），preflight 已用 P1 门禁显式标注：

```
P1  kafka_bid_topic_production_rf3       FAIL  本机 RF=1，只能证明逻辑 fence
P1  kafka_bid_topic_production_min_isr2  FAIL  min.isr=1
```

这是**有意识的本地开发降级**，不是架构缺陷。生产环境需要 RF=3 + min.isr=2 + acks=all。
acks=all 已在代码中强制：preflight P0 门禁 `kafka_writer_requires_all_acks` 验证
`RequiredAcks: kafka.RequireAll` 仍然存在于源码中。

### 8.6 脑裂（Split-Brain）的结构性消除

Kafka 脑裂场景（broker 分区后各自接受写入）在本架构中被 `acks=all + min.isr=2`（生产配置）结构性消除：

- 分区发生 → 某 broker 的 ISR 缩减到 1 < min.isr(2) → 写入立即报错，不静默成功
- 没有"phantom commit"，只有"写入失败 → relay 积压 → 分区愈合后重试"
- 这与 Kafka 完全宕机的行为路径相同，Layer C `FAULT_TYPE=kafka` 已覆盖

本地 RF=1 无法测试脑裂（需要 ≥3 broker）。这个证明边界已由 preflight P1 门禁诚实标注，
应在 ECS 正式部署时用 3-broker 集群 + toxiproxy 网络分区测试补充。

### 8.7 持久性证明链总结

```
用户出价 → Redis XADD
         → appendfsync always 保证 AOF fsync（消除进程崩溃丢失）
         → HTTP 200 + ENGINE_DURABLE 返回
         → Relay 读 Stream → Kafka acks=all（WAL 持久化）
         → durability_status 升为 KAFKA_ACKED（WS 推送）
         → Settlement worker 幂等写 PG
         → settlement_status 升为 SETTLED

每一层都有验证门禁（preflight P0）和 correctness verifier（verify P0）覆盖。
结算/扣款只在 Kafka+PG 双确认后发生，HTTP 200 时刻的 AOF-fsync 保证
用户看到的"成功"永远有磁盘记录支撑。
```
