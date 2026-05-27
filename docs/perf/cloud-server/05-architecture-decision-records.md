# 05 · 架构决策记录

本章记录当前性能重构相关 ADR。每个 ADR 都必须能被证据推翻。如果后续压测证明前提不成立，应更新 ADR。

## ADR-001: PostgreSQL 继续作为竞拍事实真源

Status: Accepted for P0

Decision:

竞拍金钱相关状态继续由 PostgreSQL 事务和 row lock 保证。Redis、Centrifugo、Kafka、NATS、WebSocket 都不能成为 P0 truth source。

理由：

- bid、auction state、winner、order、idempotency、outbox 必须原子提交。
- PostgreSQL 提供事务、约束、审计、恢复。
- 当前证据没有证明 row lock 是主瓶颈。

反对意见：

- Redis Lua 更快。
- Kafka event sourcing 更可扩展。

回应：

- 性能不是唯一目标。竞拍系统最先被拷问的是错价、错 winner、重复订单、崩溃恢复。
- Redis Lua 方案必须补偿审计、持久化、重放、幂等和 reconciliation，P0 风险过高。
- Kafka event sourcing 可作为 P2/P3，但同样要处理单 auction key 顺序热点。

推翻条件：

- accepted-only 压测证明 PostgreSQL row lock 是明确主瓶颈。
- 已有完整 reconciliation proof 和故障恢复测试。

## ADR-002: Outbox 保留，但 relay 必须重构

Status: Accepted with blocker

Decision:

保留 transactional outbox，但当前 app-owned relay 不合格，必须批量化和指标化。

当前证据：

```text
PENDING > 304000
PUBLISHED ≈ 11695
oldest pending > 1h
Redis publish avg ≈ 0.29ms
CPU/IO/内存未显示主瓶颈
```

Required fix:

- batch claim。
- ordered window。
- batch Redis publish。
- batch mark published。
- watermark 降频。
- backlog_by_shard 和 claim_latency 指标。

不接受的修复：

- 只增加机器规格。
- 只开更多 relay worker。
- 只引入 Centrifugo。
- 只把 admission 调低掩盖问题。

## ADR-003: 当前不把 Centrifugo 作为 P0 解法

Status: Accepted

Decision:

P0 不引入 Centrifugo 解决本轮 outbox 瓶颈。Centrifugo 作为 P1/P2 realtime gateway 候选。

理由：

- 当前瓶颈发生在事件从 DB outbox 到 Redis/WS 之前。
- Redis publish pipeline 很快，广播层没有被证明是主瓶颈。
- Centrifugo 官方也要求顺序发布才能保证同 channel 顺序；并行发布同 channel 不保证顺序。
- Centrifugo history 是短期恢复缓存，不是竞拍审计 truth。

未来引入定位：

```text
PostgreSQL = truth
auction_events/outbox = durable event log
relay = ordered durable publisher
Centrifugo = realtime delivery gateway
Redis/NATS = broker/cache
```

引入门槛：

- outbox relay 已能稳定 drain。
- self hub 在 WS fanout/reconnect/slow consumer 上成为明确瓶颈。
- PoC 证明顺序、恢复、权限、运维收益大于复杂度。

## ADR-004: Rejected Bid 不应默认进入全房间实时 outbox

Status: Proposed

Decision:

将 `bid_rejected` 从全房间 durable realtime outbox 中拆出。本人拒绝结果由 HTTP response 保证，审计另存，房间热度使用聚合事件或指标。

理由：

- 本轮 298201 个 rejected event 放大了 outbox 压力。
- rejected bid 不改变价格、winner、终态。
- 对全体观众逐条广播失败出价的业务价值低。

风险：

- 产品若需要展示抢拍氛围，可能失去细粒度失败动画。

缓解：

- 聚合 `bid_reject_count`、`bid_attempt_heat`。
- 保留 audit 查询。
- 前端文案不承诺所有失败 bid 实时可见。

验收：

- accepted path outbox backlog 明显下降。
- audit 完整。
- 用户本人拒绝体验不退化。

## ADR-005: PTS VU 模式和 k6 arrival-rate 分工

Status: Accepted

Decision:

PTS/JMeter 用于云端正式流程和展示性报告；k6 arrival-rate 用于精确找后端固定到达率瓶颈。两者互补，不混用结论。

理由：

- VU 模式会随系统响应变慢自然降吞吐，适合用户流程。
- arrival-rate 更适合测服务在固定 RPS 下的饱和和 backlog。

验收：

- 每个报告写明 closed/open model。
- 容量声明必须写清 load model。

## ADR-006: Admission 分为生产保护和下游压测两种语义

Status: Accepted

Decision:

所有压测必须声明 admission profile：

- `production-guarded`: admission on。
- `downstream-pressure`: admission off。

理由：

- admission-on 证明保护策略，不证明 DB/outbox 极限。
- admission-off 找真实下游瓶颈，不代表生产默认容量。

验收：

- 报告必须包含 `auction_admission_enabled`。
- 如果 429 或 admission reason 占主导，不得声称下游瓶颈。

## ADR-007: 不因单轮 100 VU 结果直接推翻整体架构

Status: Accepted

Decision:

当前证据足以要求 outbox relay 结构性重构，但不足以直接推翻 PostgreSQL truth、modular monolith 或整体架构。

理由：

- 主瓶颈已定位在 outbox relay。
- PostgreSQL lock wait 不是当前最大耗时。
- 机器资源未显示主瓶颈。
- Redis publish 很快。

推翻整体架构的门槛：

- relay 修复后，accepted-only 和 multi-room 压测仍无法达成目标。
- PG hot row 被证明不可接受，且业务目标必须支持更高单 auction accepted bid/s。
- self hub 被证明无法在目标连接数下稳定运行。
- 有替代架构 PoC 和故障恢复证据。
