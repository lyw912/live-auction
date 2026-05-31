# 04 — 性能预算：50ms 怎么来的，为什么这次真能到

> 不喊口号。逐项拆 50ms 预算，给组提交吞吐算账，并说明为什么当前架构**结构上**到不了、v3 **结构上**能到。

---

## 1. PTS-1B 目标复述

1000 个用户在最后一秒，对**同一个**热拍品出价。**用户可见的最终业务决策 p99 ≤ 50ms**，
且最高有效价胜、拒绝有据、durable/settlement 闭环、故障注入通过。

---

## 2. v3 热路径延迟预算（每笔出价，关键路径）

| 阶段 | 预算 | 说明 |
|---|---|---|
| 客户端→网关网络（同区） | 1–5ms | TLS 连接池复用、HTTP/2；非热路径请求独立限流不挤占 |
| 网关 auth/ACL/decode/幂等快路径 | 1–3ms | 内存/Redis 命中，不读 PG 热行 |
| 令牌桶/防抖 | <0.5ms | 本地或 Redis 计数 |
| 网关↔Redis 1 次 RTT | 0.2–1ms | 同 AZ |
| **Redis Lua CAS（含 XADD）** | **0.1–0.5ms** | 单线程内存原子；一次脚本含全部规则+定序+写日志 |
| 序列化/回包 | <1ms | |
| **合计（典型）** | **~5–12ms** | 余量充足以吸收 GC/抖动到 p99 ≤ 50ms |

热路径上**没有**：同步 Kafka（省去一次 `acks=all` 跨进程往返 1–5ms 且免去排队锁）、同步 PG、单拍品 append 锁、PG 热行读。

---

## 3. 单热点的真正瓶颈：Redis 单 key 吞吐，够不够？

1000 笔全落在同一 key（`bid:engine:state:{auction}`），Redis 单线程**串行**执行。算账：

- 一次 ledger Lua（HMGET 一组字段 + 若干 HSET + 一次 XADD）≈ **10–40µs** 量级的服务端执行时间（纯内存）。
- 1000 笔 ⇒ Redis 端纯执行 ≈ **10–40ms** 累计，且是**流水**消化（请求陆续到达、并发连接复用）。
- 对单个请求而言，其排队深度 = 它前面同 key 的未完成脚本数。最后一秒爆发下，p99 排队 + 执行仍远 < 50ms。
- 这正是 Redis 单线程的优势：**无锁、无上下文切换、无公平性饥饿**（对照秒杀实践「Redis 单线程不存在共享变量竞争」）。

> 对照：当前架构这 1000 笔要**逐个**抢一把锁 + 同步等 Kafka `acks=all`（1–5ms/次）⇒ 串行 1–5s。
> **这就是「结构上到不了」与「结构上能到」的差别**：瓶颈从「1–5ms 的跨进程同步」变成「10–40µs 的内存原子」，差 2–3 个数量级。

---

## 4. 持久化吞吐：组提交（Group Commit）算账

持久化在关键路径之外，但也要扛住 1000 笔/秒级爆发，否则中继积压会触发 fail-closed。

- 中继**批量** `XREAD`（一次取 N≤512 或等 T≤2ms），把一批决策**一次** produce 到 Kafka。
- Kafka 幂等 producer + `linger.ms`/`batch.size` 再做一层 broker 侧批处理。
- 1000 笔 ⇒ 约 2–8 次批量 produce ⇒ 全部在 **~10–30ms** 内 `KAFKA_ACKED`。
- 这是数据库 group commit 的同一原理（CMU 15-445："group commit to batch multiple log flushes together to amortize overhead"；PostgreSQL `commit_delay`/`commit_siblings`；InnoDB group commit）。
- Redis AOF 侧：`appendfsync always` 本身对并发写**也做 group commit**（Redis 官方文档原文："the always policy supports group commit, so if there are multiple parallel writes Redis will try to perform a single fsync"）——即便要更强的本地持久，也不必逐笔 fsync 串行。

> 结论：**决策用单写者解并发瓶颈，持久用组提交解吞吐瓶颈**。两者都不在用户的 50ms 关键路径上互相拖累。

---

## 5. 为什么「同步 Kafka」永远到不了 50ms（数量级论证）

| 方案 | 单笔关键路径必含 | 1000 笔同拍品的 p99 |
|---|---|---|
| 当前：决策 + **同步按序单锁 Kafka** | 1 次 `acks=all`（1–5ms）+ 抢锁 + 排队 | 退化为串行 ~秒级 → 绝大多数 `202` |
| v3：决策 + **XADD（内存）** | 1 次内存 XADD（µs 级） | ~5–12ms 典型，p99 < 50ms |

把 `acks=all` 的跨进程同步从「每一笔的关键路径」上拿掉，是达成 50ms 的**充要条件**。
撮合引擎结论同源："the engine is always ahead of persistence — the WAL catches up asynchronously."

---

## 6. 观测与证据（对应 evidence-policy / 加分项「可观测性」）

每次 PTS-1B 报告必须分开度量并出图：

- `final_decision_latency_ms`（请求→DECIDED）p50/p95/**p99** ← 主指标
- `accept_latency_ms`（到 202，若有）— 仅作接收延迟，**不**当决策 p99
- `durability_lag_ms`（DECIDED→KAFKA_ACKED）、`settlement_lag_ms`（KAFKA_ACKED→PG SETTLED）
- 业务分布：ENGINE_ACCEPTED/REJECTED/SOLD/PAUSED/RECONCILING 计数（不只看 HTTP 码）
- Redis 单 key 执行耗时、中继批大小与积压深度、Kafka produce 批次/lag、对账缺口数
- `202_ratio`、`timeout_ratio`（都应趋近 0）

> 「无当前 workload+profile+verifier+证据分级，不得声称性能」——本预算给的是设计期论证，
> 上线证据须按 `docs/current/evidence-policy.md` 用真实压测填实（Linux 环境，消除 Windows 网络栈干扰，见调研 §5 Phase 2）。

---

## 7. 还能更深吗？（完美主义自检，留作下一阶段）

- 网关↔Redis 同 AZ 部署 / 连接预热 / pipeline 合并幂等读与 EVAL。
- 决策日志 entry 用紧凑编码（msgpack/SBE 思路）降序列化成本。
- 中继按拍品分区并行（分片间并行），单分区内仍单写者保序。
- 收盘定时器精度 <1s 漂移（Flink keyed timer 或等价），临近收盘的 re-arm 与 fencing 条件更新配合防 stale end。
- 超级热点拍品独占节点 / NUMA 亲和（撮合引擎做法），本课题规模通常用不到，但要能答得出。
