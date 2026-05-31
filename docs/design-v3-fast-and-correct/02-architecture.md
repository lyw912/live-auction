# 02 — 架构：单写者决策日志（Single-Writer Decision Log）

> 这是金融撮合引擎 / LMAX / 事件溯源在「单热点英式竞价」场景下的正确落地。
> 一句话：**单写者在内存里拍板并立即返回；追加式决策日志用组提交异步落定；PG 是日志的幂等投影。**

---

## 1. 组件与职责（每个只做一件事）

| 组件 | 职责 | 绝不做 |
|---|---|---|
| **网关 Gateway** | 鉴权、房间/ACL、防抖+令牌桶限流、幂等快路径、响应契约 | 热路径上读/写 PG 热行 |
| **Redis 单写者引擎（Lua CAS）** | 唯一**决策权威**：原子定价/定赢家/定结束、分配无间隙 `engine_seq`、原子 `XADD` 写决策日志 | 依赖 PG 快照做实时决策；被任何外部写回 |
| **决策日志流 `bid:log:{auction}`（Redis Stream）** | 拍品内**有序、追加式**的决策 WAL（内存热日志） | 被裁剪到「未中继」之前 |
| **中继 Relay（单写者/分区）** | **组提交**批量消费 Stream → 批量 produce 到 Kafka（幂等 producer，按 auction 保序） | 逐笔加锁/逐笔同步等 ack |
| **Kafka `bids.decided.{...}`** | 跨进程**持久、有序**的决策总线（长留存，可重放重建） | 被当成可查询数据库 |
| **结算消费者 Settlement** | 按 `engine_seq` **幂等投影**进 PG（bids/审计/订单），收盘做 effectively-once 成交 | **回写 Redis 活状态**（已删除） |
| **PostgreSQL** | **持久系统真源**：结算、订单、审计、历史查询 | 成为热路径同步决策点 |
| **对账器 Reconciler** | 比对 Redis/Kafka/PG/outbox，一致则放行，不一致则暂停+暴露+封锁危险操作 | 向用户/运维隐藏不确定性 |
| **实时投影 WS/Outbox** | 房间级路由下发服务端权威状态、`已确认` 翻牌、断连重放 | 用客户端状态编造价/赢家 |

> 借鉴 crackingwalnuts 拍卖系统设计的精炼判据：**「正确性只活在两处：决策 CAS（谁赢一次出价）+ 收盘 fencing token（谁的结算落定）。其余都是投递与扇出。」**

---

## 2. 端到端数据流

### 2.1 热路径（同步，≤50ms，用户在这里拿到最终决策）

```
POST /auctions/{id}/bids  (Idempotency-Key == client_bid_id)
  1. 网关：auth → ACL → 防抖/令牌桶（单用户 2s≤1 次，防机器人/狙击）→ 幂等快路径
  2. Redis EVAL ledger CAS（单次原子）：
       - 幂等：命中且 hash 一致→REPLAY；不一致→冲突
       - 规则全集：active/ended/self-leading/cap/0起拍/加价网格/too-low
       - engine_seq += 1（accept 与 reject 都自增，gap-free）
       - 更新 price/winner/end_at/extend_count
       - XADD bid:log:{auction} *  <决策 JSON, 以 engine_seq 为序>   ← 决策+WAL 原子同步
  3. 立即返回最终决策：
       { result: ENGINE_ACCEPTED|REJECTED|SOLD,
         engine_seq, decision_status: "DECIDED",
         durability_status: "ENGINE_DURABLE",   // 已在 Redis 日志(+副本)
         settlement_status: "PENDING",
         current_price_cents, current_winner_id, end_at, decision_basis{...} }
```

**为什么此刻就能算最终决策？** 因为它满足撮合引擎/LMAX 对「权威」的全部要求：
确定性、单写者定序（`engine_seq` 无间隙）、幂等、且**已写入追加式日志**。
持久化/结算是**后续维度**，不是「能否拍板」的前置。

### 2.2 持久化（异步，关键路径之外，组提交）

```
Relay loop（每拍品分区一个单写者；单写者原则）：
  entries = XREAD/XAUTOCLAIM bid:log:{auction}  (一次取一批, 最多 N 条或 T 毫秒)
  produce 一次批量到 Kafka(bids.decided, key=auction_id, acks=all, enable.idempotence=true)
  收到批量 ack → XACK/记录 high-water → 可选: 通过 WS 推送这批 engine_seq 的 "CONFIRMED"
```

这是**组提交**（DB group commit / Kafka linger 批处理 / LMAX journaling 同一思想）：
1000 个决策 → 几次批量 produce → 全部在 ~10–30ms 内 Kafka 落定。**无逐笔锁、无逐笔往返。**

### 2.3 结算（PG 幂等投影 + 收盘 effectively-once）

```
Settlement consumer（按 engine_seq 顺序消费 Kafka）：
  INSERT INTO bids(... , auction_id, engine_seq, ...)
    ON CONFLICT (auction_id, engine_seq) DO NOTHING      ← 至少一次投递 + 幂等 = 有效一次
  （审计、accepted 投影、outbox 实时事件，全部按 engine_seq 幂等）

收盘（定时器触发，settlement coordinator）：
  token = INCR fence:auction:{id}                         ← fencing token
  winner = PG 中 amount 最高、engine_seq 最小做 tiebreak 的 ACCEPTED
  UPDATE auctions SET status='SOLD', winner_id=?, final_price=?, settlement_fence=token
    WHERE id=? AND (settlement_fence IS NULL OR settlement_fence < token)   ← 条件更新
  支付：Idempotency-Key = "settle:{auction_id}"（绑拍品不绑尝试，稳定）   ← 绝不重复扣款
  UPDATE ... settlement_status='PAYMENT_CAPTURED'; 发 outbox auctions.sold
```

**三层叠加 = effectively-once**：fencing token（谁的结算落定）+ 条件更新（旧 token 写不进）+ 支付幂等键（重试返回原 charge）。
保证：**一个拍品恰好一行 SOLD、恰好一次扣款、无丢单**。直接对应宣讲版「绝对不允许一笔出价扣两次钱」。

---

## 3. Redis 数据布局（分层，对应加分项「Redis 分层缓存 / 读写分离」）

| Key | 类型 | 用途 | 读/写 |
|---|---|---|---|
| `bid:engine:state:{auction}` | Hash | 决策态：price/winner/end_at/engine_seq/status... | Lua 写；快照读 |
| `bid:engine:idem:{auction}:{client_bid_id}` | Hash | 幂等记录：request_hash/result_json/durability | Lua 写；重放读 |
| `bid:log:{auction}` | **Stream** | **决策日志（WAL）**，entry 以 engine_seq 排序 | Lua `XADD`；中继 `XREAD/XACK` |
| `fence:auction:{auction}` | String(INT) | 收盘 fencing token | 结算 `INCR` |
| 只读副本 / 边缘短 TTL 缓存 | — | 围观者读「当前价/人数」走副本或 1–3s TTL，**出价校验只走主**（避免读从库旧价引发无效冲突） | 读写分离 |

> Redis Cluster 用 **hash tag** `{auction}` 把同一拍品的 state/idem/log/fence 钉在同一 slot，保证 Lua 跨 key 原子可执行；
> 拍品间按 `auction_id` 分片并行（分片内串行、分片间并行）。超级热点拍品可独占物理节点（`单热点调研.md` §6.4.2）。

---

## 4. 真源模型：回答「还能说 PG 是唯一真源吗？冲突吗？削弱正确性吗？」

这是评委必问、也是用户最纠结的点。逐条正面回答：

### 4.1 把「持久真源」和「决策权威」分开——它们本就是两个角色

- **PG 仍然是持久系统真源**：结算、订单、审计、长期查询的真相都在 PG，**我们没有放弃它**。
- 但**实时拍板权威**是 Redis 单写者；而**「到底决策了什么」的系统真源**是那条
  **追加式有序决策日志**（Redis Stream → Kafka → PG bids 表三处物化）。

> 「源 of truth（持久记录在哪）」≠「decision authority（谁在实时拍板）」。
> 把拍板权放进 Redis，不等于放弃 PG 的持久真源地位。crackingwalnuts 同款表述：
> **"Postgres is the source of truth; Valkey is the hot-path coordinator; Kafka is the delivery bus. Kafka is not the source of truth—bid acceptance is decided by Valkey; durability lives in Postgres."**

### 4.2 这是事件溯源（不是削弱正确性，而是更强）

- 决策日志是**不可变、全序、可重放**的。Redis 热态与 PG 表都是该日志的**确定性物化视图**，
  任何一个都能从日志重放重建（Kleppmann「Turning the database inside out」：DB 与缓存都只是日志的物化视图）。
- 对比「PG 单行就是真相」：日志模型**更强**——(1) 完整不可变历史可审计；(2) 确定性重放可重建任意视图；
  (3) 不对被争抢的单行做破坏性原地更新（破坏性更新会丢历史、且是性能瓶颈本身）。
- LMAX/Fowler 原话："Event Sourcing provides a way to solve the durability problem for an in-memory system,
  running everything in a single thread solves the concurrency issue."——**单线程解并发，事件溯源解持久**。正是本架构。

### 4.3 为什么必须升级（PG 本质上解不了「几百人抢一行」）

不是「PG 慢」这么含糊，而是有工业界明确论断（中外双证，拷打时直接引）：

- 英文系统设计（crackingwalnuts）："**PostgreSQL is not designed to hold a hot row for hundreds of concurrent transactions.**" 因此 `auctions.current_price` **不**逐笔更新；`bids` 追加表能轻松吞数万 insert/s，但单行 update 不行。
- 中文秒杀实践（Thoughtworks/京东）："**商品库存数据在 DB 最终会落到单库单表的一行……无法通过分库分表提高请求的并行度**；DB 是磁盘操作还要加锁，非公平竞争易线程饥饿；Redis 单线程内存操作不存在共享变量竞争。"

所以升级 Redis 决策引擎不是「炫技」，是**单热点强一致更新这一类问题的标准工业解**（`单热点调研.md` §3.2 模式 C）。

### 4.4 那「不以 PG 为同步决策点」会不会牺牲正确性？——不会，且有不变量证明

正确性由 `03` 的不变量集 + 故障矩阵闭环保证。核心论点：
**决策只要满足「单写者原子定序 + 影响钱之前已落追加日志 + 幂等结算进 PG」三条，
就与「PG 行内决策」等价甚至更强**——因为最高有效价由全序日志唯一确定，赢家由结算按日志投影唯一落定。

---

## 5. 与宣讲版/调研的对应一句话

- 宣讲版「Redis 应对高频读写 + 分布式锁解决幂等」→ 本架构把 Redis 用到极致（决策+幂等+WAL 流）。
- `单热点调研.md` §3.2 模式 C（内存状态机 + 事件总线 + Settlement + 冷真源）→ **本架构就是模式 C 的正确落地**，
  并补齐了它点名的难点：Kafka 保序（按 auction 分区 + 幂等 producer）、Redis failover 的 fencing、事件乱序（按 engine_seq 幂等）。

下一篇证明它在所有故障下都对。
