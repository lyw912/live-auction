# 07 — 工业界来源与引证（拷打时拿这个挡）

> 全部经 tavily / fetch 检索（未用 websearch）。每条标注**它证明了什么**、**在本方案哪里用**。
> 用途：当评委质疑「你凭什么把决策放 Redis / 凭什么不以 PG 同步决策 / 凭什么异步还敢叫最终决策」时，逐条引。

---

## A. 「PG 解不了单热点行争用」——升级的正当性（中外双证）

1. **crackingwalnuts《System Design: Online Auction (50K bids/sec…)》**
   - 原文："**PostgreSQL is not designed to hold a hot row for hundreds of concurrent transactions.**"
   - 原文："Optimistic concurrency via Valkey CAS scales bid acceptance to 50K/sec with sub-ms latency. **Pessimistic row locks cannot.**"
   - 用在：`02 §4.3`、`04 §5`。这是与本课题**几乎同题**的英文系统设计，结论一致。

2. **Thoughtworks/京东 秒杀架构（中文）**
   - 原文："商品库存数据在 DB 最终会落到单库单表的一行……**无法通过分库分表提高请求的并行度**；DB 加锁非公平竞争易线程饥饿；Redis 单线程不存在共享变量竞争。"
   - 用在：`02 §4.3`、`04 §3`。给中文评委的本土印证。

---

## B. 「决策可异步持久，仍是最终/权威」——又快又对的理论根

3. **撮合引擎系统设计（techinterview / DEV / HFT 论文）**
   - 原文："**the engine is always ahead of persistence — the WAL catches up asynchronously.**"
   - 原文："In-memory databases like Redis… for the live order book — disk persistence is only for the append-only WAL."
   - "Sequence numbers on every order ensure deterministic replay — given the same input sequence, the matching engine always produces identical trades."
   - 用在：`01 §4`、`02 §2.1`、`04 §5`。证明「内存单写者拍板=最终，WAL 异步追上」是交易所标准。

4. **LMAX 架构（Martin Fowler）+ Disruptor 用户指南 + Single Writer Principle**
   - 原文："Event Sourcing provides a way to solve the durability problem for an in-memory system, **running everything in a single thread solves the concurrency issue.**"
   - 输入/输出 journal 保证持久；business logic 经 **gating** 等待 journalling/replication（**并行/批量**，非逐笔同步往返）；可达数千万 msg/s。
   - Single Writer Principle：一处内存只一个写者，消除锁与上下文切换。
   - 用在：`02 §1/§4.2`、`04`。证明「单线程解并发 + 事件溯源解持久」的组合是经过验证的高性能范式。

5. **Sequencer 架构（金融业）**
   - 确定性 FSM、无竞态；"the tick of the clock is in the event stream"（确定性时间）。
   - 用在：`03 §6` 软延时「时间只信服务端、重放用记录时间」。

---

## C. 「日志是真源」——回答「还能说 PG 是唯一真源吗」

6. **Martin Kleppmann《Turning the database inside out》**
   - "A materialized view is just a cached subset of the log, and you could **rebuild it from the log at any time**. There could be many different materialized views onto the same data…"
   - 用在：`02 §4.2`。DB 与缓存都是日志的物化视图——本方案的真源模型基石。

7. **Confluent《Messaging as the Single Source of Truth》**
   - "the source of truth is really the event." Memory Image 模式（数据集载入内存查询）。
   - 用在：`02 §4`。

8. **事件溯源 vs 事件流（event-driven.io）——重要的反面提醒**
   - 警示："Kafka… designed to move things from one place to another, **not to be used as durable databases**."
   - 用在：本方案**不**把 Kafka 当数据库——它是**决策日志的持久/有序总线**，真正的可查询真源仍是 PG 投影。这条让我们的措辞经得起更挑剔的拷打。

---

## D. 「组提交」——持久吞吐怎么扛

9. **CMU 15-445 Logging / GA Tech Logging / PostgreSQL WAL 文档**
   - "use the 'group commit' optimization to **batch multiple log flushes together to amortize overhead.** Flushes happen either when the log buffer is full, or if sufficient time has passed."
   - PostgreSQL `commit_delay`/`commit_siblings`；InnoDB group commit。
   - 用在：`02 §2.2`、`04 §4`。中继批量 produce = 组提交。

10. **Redis 持久化文档**
    - "the **always** policy… **supports group commit**, so if there are multiple parallel writes Redis will try to perform a single fsync operation." `everysec` 最坏丢 1s。
    - 用在：`03 §4.1`、`04 §4`。Redis 本地持久也能组提交，不必逐笔 fsync 串行。

---

## E. 「effectively-once 结算 / 绝不重复扣款」

11. **crackingwalnuts §9 effectively-once**
    - **fencing token（INCR）+ 条件 UPDATE（WHERE fence<token）+ 支付幂等键（绑拍品不绑尝试）** 三层；
      "one SOLD row per auction, one captured charge once the provider acks."
    - 反例提醒："including the fencing token in the [payment idempotency] key breaks the guarantee."（幂等键必须稳定）
    - 用在：`02 §2.3`、`03 §1`、`05 阶段3`。

12. **Outbox / 幂等消费者（多篇）**
    - "at-least-once delivery + idempotent apply"；按 `eventId`（本方案 `engine_seq`）去重。
    - 用在：结算 `ON CONFLICT(auction_id,engine_seq) DO NOTHING`。

13. **单热点调研.md（项目内）§2.2/§3.2/§4/§6**
    - 方案 C 双写地狱风险清单（崩溃以谁为准、重复消费、异步窗口）——本方案用**单写者+投影**结构性消除。
    - 四道保险：幂等键 / fencing token / settlement replay 对账 / dead-letter+gap notice——全部落在 `02/03`。

---

## F. 软延时 / 防狙击

14. **crackingwalnuts §11.2 + eBay 社区讨论**
    - `max_extensions`（如 20）+ `absolute_end_time = original + 30min` 硬天花板，CAS 内 clamp，防无限延长。
    - 用在：`03 §6`、`06`（补当前 Lua 缺的绝对天花板）。

---

## 检索方式声明

- 工具：`tavily_search`（advanced）+ `tavily_extract`（advanced）抓取全文；未使用 websearch（遵用户约束）。
- 关键检索词：group commit WAL durability、LMAX Disruptor single writer、event sourcing log source of truth Kleppmann、Redis AOF appendfsync WAIT、matching engine sequencer deterministic replay、秒杀 Redis Lua 预扣 异步落库 对账、online auction anti-sniping soft close、transactional outbox idempotent exactly once。
- 上线性能数字须按 `docs/current/evidence-policy.md` 用真实 Linux 压测填实，本文仅作设计期论证与引证。
