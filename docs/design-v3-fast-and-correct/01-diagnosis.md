# 01 — 独立评审组诊断：根因在代码里，不在文档里

> 评审方式：不信文档、不信既有「证据」，直接读 `backend/internal/redisengine/engine.go`（3481 行）的真实热路径。
> 评审身份：资深后端 / 运维 / 测试 / 产品四视角联合拷打。结论：**这是架构错误，不是调参问题。**

---

## 1. 决策逻辑本身是对的、也是快的（先表扬，避免误伤）

`ledgerRunner` 的 Lua 脚本（`engine.go:55-299`）在**单次原子调用**内完成了：

- 幂等去重：`idem_key` 命中且 `request_hash` 一致 → `REPLAY`；不一致 → `IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST`（冲突，非重放）。✔ 符合宣讲版「分布式锁解决幂等性」。
- Redis 态缺失时用 PG 冷快照 seed（`state_json`），缺快照则 `RECONCILING` fail-closed。✔
- 全部竞拍规则：`AUCTION_NOT_ACTIVE` / `AUCTION_ENDED` / `REJECTED_SELF_LEADING` / `BID_ABOVE_CAP` / 0 起拍（`accepted_bid_count<=0` 用 `start_price`）/ 加价网格（`(amount-base)%increment==0`）/ `BID_TOO_LOW` / 封顶 `amount==cap → ENGINE_SOLD`。✔ 符合挑战一「复杂规则零漏洞」。
- 软延时：窗口内且 `extend_count<max_extend_count` 时 `end_at += extend_by`。✔（**唯一小缺口**：缺一个 `absolute_end` 绝对天花板，详见 `03`。）
- **分配无间隙 `engine_seq`**（accept 和 reject 都自增），写 `pending_key` 哈希 + `pending_auctions` 集合。✔ 这是定序真源。

**这是教科书级的「单热点串行化」实现**（对照 `单热点调研.md` §6.2.1 工业级行）。Redis 单线程 + Lua 原子性
天然消灭了「两个买家同时看到自己最高价」的脏读。**性能与正确性的根，决策这一环没问题。**

---

## 2. 根因：持久化被「同步 + 严格按序 + 单锁」钉死在热路径

读 `PlaceBid`（`engine.go:411-577`）→ `appendDecisionBeforeReturn`（`:579-731`）→
`waitForPendingAppendTurn`（`:733-763`）。决策完成后，要返回 `ENGINE_*` **必须先**：

1. **`waitForPendingAppendTurn`**：本决策只有在自己是**当前最小未 append 的 engine_seq** 时才轮到它 append。
   若 `min_pending_seq < 本 seq`（前面还有没 append 的）→ 返回 `pendingWorker` → **`202`**。
2. **单拍品锁**：`SetNX(pendingAppendLockKey(auctionID))`，**一个拍品同时只允许一个写者** append。
   抢不到锁 → 返回 `202 PROCESSING_RETRY_LATER`（`:630`/`:661`）。
3. **同步 Kafka `e.ledger.Append(...)`**，`acks=all` 等 broker ack（`:683`），再写 Redis ack 标记（`:709`）。

把这三点叠起来，在 **1000 个最后一秒、同一拍品**的请求下会发生什么：

- Lua 给它们瞬间分配了 seq 1..1000（快）。
- 但持久化被**重新串行化**：只有「最小 pending seq」的持有者、且抢到那**唯一一把锁**的人，
  才能**一次一个**、**同步等 Kafka ack** 地 append。其余全部撞上「锁忙 / 不是最小 seq」→ 返回 `202`。
- 单次 Kafka `acks=all` append ≈ 1–5ms；1000 个串行 ≈ **数秒**。窗口内根本排不过来。

> **这就是 HA5YX7ZG 的真相**：1000 个请求全 `202`、只有 300 个落库、引擎 `KAFKA_LEDGER_SETTLEMENT_NOT_TERMINAL` 暂停。
> 不是 PTS 把 `202` 当失败，是**服务端真的没给出最终决策**。

### 一句话定性

> **你把决策从被争抢的 PG 行里搬了出来（对的），却又用「Kafka append 必须按 engine_seq 顺序、单锁、同步」
> 在低一层把同一个「几百人抢一行」的串行瓶颈原样重建了一遍。**
> PG 行锁 → 换成了「Kafka append 顺序锁」。瓶颈没消失，只是换了位置。

这也解释了为什么之前每次「调快」都「变错」：一旦把 append 推给后台 worker、热路径提前返回，
就进入下面的第 3 节污染路径。

---

## 3. 「调快就变错」的另一半：结算回写 Redis 活状态

`Worker.refreshRedisSettledState`（`engine.go:2408-2470`）在每次结算后，从 PG 读快照
（`loadSnapshotForRedisState`）并 `HSET` 回 Redis 活状态，包括 `engine_seq` / `current_price` / `current_winner`。

这是 `单热点调研.md` §2.2「方案 C 双写地狱」的教科书反例——**结算投影反向写决策态**：

- PTS-1B 下 PG 结算合法地落后于 Redis 实时决策；
- 该回写把 Redis 的 `engine_seq` / 价 / 赢家**回退**到上一个已结算快照；
- 后续 Lua 复用了已发过的 `engine_seq` → Kafka 出现**重复 ledger 消息**（HA5YX7ZG 实测 offset 52 与 101 都带 `engine_seq=53`）→ 结算 `request hash conflict` → 引擎暂停。

代码里**已经打了补丁**（Lua 里 `if current_engine_seq > snapshot_engine_seq then return 0`）。
但这恰恰证明了问题的本质：**这是个不该存在的反向写**。投影（PG/结算）**永远不应该写回**决策权威（Redis 活状态）。
即便加了 `>` 守卫，等值/并发/快照对应旧 public_seq 等边界依然脆弱——**这类补丁会一直长出来**。

---

## 4. 两条「自我设限」的规则，正是循环的制造者

它们写在 `docs/current/performance-correctness-contract.md` 里，出发点是好的，但直接制造了死循环：

1. **「`ENGINE_ACCEPTED` 只能在 `durability_status = KAFKA_ACKED` 之后返回」** ——
   这条把「同步 Kafka」焊死在热路径上，等价于「每笔出价都要等一次跨进程 `acks=all` 往返」。这是慢的根。
2. **结算 worker 回写 Redis** —— 这是错的根（第 3 节）。

> 工业界的撮合引擎**从不这样**：决策在内存单写者里拍板即视为最终，**WAL 在后面异步追上**
> （techinterview 撮合引擎："the engine is always ahead of persistence — the WAL catches up asynchronously"）。
> 把「持久化已完成」当成「能否返回决策」的前置条件，是把两个正交维度强行耦合。

---

## 5. 四视角拷打小结

| 视角 | 当前系统的致命问题 |
|---|---|
| **资深后端** | 决策与持久化耦合；用 append 顺序锁重建单行瓶颈；结算反向写决策态（双写地狱）。 |
| **运维** | 正常路径退化成 `202` 海；引擎频繁 `PAUSED`；恢复语义里夹着「调快补丁」难以推理。 |
| **测试** | 「最终决策 p99」无法被测（全是 `202` 的接收延迟）；correctness verifier 只能对 300/1000 子集 PASS。 |
| **产品** | 珠宝高价值场景，用户点了出价拿到的是「稍后重试」，看不到「我赢了没」。**这是体验事故**，不是性能优化。 |

---

## 6. 结论

- **不要再打补丁。** 决策环是对的，留着。
- **删掉**热路径上的「按序 + 单锁 + 同步」append（第 2 节）。
- **删掉** `refreshRedisSettledState` 及一切结算→Redis 回写（第 3 节）。
- **改写**持久化为组提交异步中继，**改写**结算为按 `engine_seq` 幂等投影（见 `02`/`05`）。
- **改写**响应契约：同步返回 `DECIDED` 最终决策，`202` 退回为「极少数 durability-unknown」专用（见 `03`）。

这套改动是**一个连贯架构**，不是 mode、不是新旧并行。下一篇给出完整架构。
