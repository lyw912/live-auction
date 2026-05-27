# 04 · 瓶颈深挖任务

每个任务都可以独立展开为 issue、PR、压测轮次和答辩材料。完成标准不是“写了代码”，而是“有证据证明假设成立或被推翻”。

## Task P0-1: 修复压测业务模型

问题：

本轮 HTTP 成功率 100%，但真实 bid 中约 94% 是业务拒绝。`BID_TOO_LOW` 和 `AUCTION_ENDED` 污染了有效成交容量结论。

当前证据：

```text
bids total=316400
accepted=18199
rejected=298201
采样: BID_TOO_LOW=1948, AUCTION_ENDED=1064, ACCEPTED=176
```

要做：

- seed 每轮重置 bids/outbox/auction state。
- auction `end_at` 设置为 now + 压测时长 + 观察窗口。
- JMX amount 生成改为基于全局时间和目标 rate，而不是每线程局部计数。
- 分离 accepted-pressure 和 rejected-pressure。

验收：

- accepted-pressure 中 `ACCEPTED >= 80%`。
- `AUCTION_ENDED=0`。
- bid seq 连续。
- 业务结果写入报告，不只看 HTTP code。

评委会问：

> 你的 643 TPS 是有效竞价 TPS 还是请求 TPS？

可防御答案：

> 当前只能称为请求 TPS。修复后会提供 accepted bid TPS、rejected bid TPS、outbox event TPS 三套数字。

## Task P0-2: Outbox Relay 批量化重构

问题：

outbox 生产速度远高于消费速度，压测后仍有 30 万 PENDING。

当前证据：

```text
PENDING > 304000
PUBLISHED ≈ 11695
all pending shard_id=13
oldest pending > 1h
Redis publish avg ≈ 0.29ms
```

要做：

- `ProcessBatch` 不能只是循环 `ProcessOne`。
- 一次 claim 一个连续 seq window。
- 一次 Redis pipeline 发布多条。
- 一次 DB update mark 多条 published。
- watermark 刷新按 batch 或周期进行。
- 指标拆分 claim/publish/mark/watermark。

验收：

- 在同等 100 VU 下，压测结束后 backlog 不持续增长。
- steady-state `published/s >= produced/s`。
- 单 auction seq 单调。
- poison event 不阻断后续超过设计阈值。

评委会问：

> 为什么不直接多开 relay worker？

可防御答案：

> 同 auction 必须保序，单 hot auction 落在一个 ordering key。多 worker 能扩多 auction，但不能绕过单 key 顺序瓶颈。必须先提升单 key ordered drain 效率或调整事件策略。

## Task P0-3: Rejected Event 策略

问题：

本轮 298201 个 `bid_rejected` 全部进入 outbox，放大了 realtime 链路压力。实际产品是否需要把每个拒绝 bid 广播给所有用户，需要重新审视。

候选策略：

| 策略 | 优点 | 风险 |
|---|---|---|
| 全量 reject 入 outbox | 审计一致，诊断完整 | 高压下广播无价值事件，backlog 爆炸 |
| 只给本人 HTTP response，不广播 | 极大减压 | 观众侧缺少失败氛围 |
| 聚合 reject 指标事件 | 保留热度信号 | 不能逐条恢复 |
| 只持久化 audit，不进 realtime outbox | 审计保留，实时减压 | 需要区分 outbox 类型 |

建议：

- `bid_accepted`、terminal event 必须 durable realtime。
- `bid_rejected` 默认不进入全房间 realtime outbox。
- 对恶意/风控/热度可写 audit 表或聚合指标。
- 对本人拒绝由 HTTP response 返回，不依赖 WS。

验收：

- accepted path 不受 reject flood 影响。
- reject audit 可查。
- 产品文案不承诺全房间展示每次失败 bid。

评委会问：

> 你是不是为了性能牺牲业务完整性？

可防御答案：

> 不是。竞拍核心状态由 accepted bid、price、winner、terminal event 决定。失败出价对本人必须准确，对全房间不是状态真相。我们把 audit 和 realtime 分层，减少无业务价值的广播。

## Task P0-4: PostgreSQL 热行锁归因

问题：

bid path 使用 auction row lock 保证同 auction 金钱正确性。当前 lock wait 平均约 8.2ms，不是主瓶颈，但在更高 accepted bid 压力下可能成为下一瓶颈。

要做：

- 采集 lock wait histogram。
- 采集 tx duration、pool wait、deadlock、slow query。
- 对 accepted-only pressure 单独测试。
- 比较单 auction 和多 auction。

验收：

- 能回答单 hot auction 最大 accepted bid/s。
- 能回答多 auction 是否水平扩展。
- 如果要改架构，必须证明 row lock 已成为主瓶颈。

评委会问：

> 为什么不用 Redis Lua 做竞价真源？

可防御答案：

> P0 金钱和订单正确性优先，PostgreSQL 提供事务、约束、审计和恢复。Redis Lua 可作为 P2 设计，但必须有 reconciliation proof，不能为性能牺牲 money correctness。

## Task P0-5: During Evidence 采集

问题：

当前 CPU/IO/内存证据多为压测后采集，只能证明压测后资源未打满，不能证明峰值期间无瞬时饱和。

要做：

- 每轮 before/during/after 采集。
- during 每 30-60 秒一次。
- 增加 pidstat、dmesg、netstat/sar、pprof。

验收：

- 每轮报告包含时间线。
- 能对齐 PTS p99 峰值和机器/DB/Redis 指标。
- 没有“凭感觉排除资源瓶颈”。

## Task P0-6: PTS 明细和业务结果分析器

问题：

PTS sampling logs 不是全量，但可以作为业务结果抽样。服务侧 DB 才是全量事实。

要做：

- 写分析脚本读取 sampling JSONL。
- 输出 sampler、HTTP code、success、elapsed p50/p95/p99、bid result、reject reason。
- 与 DB 全量结果交叉验证。

验收：

- 每次 PTS 后自动生成 `analysis.md`。
- 报告明确区分 sampled 和 full DB。

## Task P1-1: Centrifugo 评估

问题：

Centrifugo 能解决 WS fanout 和连接扩展，但不能解决 outbox 未发布。

要做：

- 在 outbox relay 修复后，构建 Centrifugo PoC。
- 对比 self hub vs Centrifugo：连接数、fanout latency、RSS、reconnect storm。
- 验证 channel 顺序发布约束。

验收：

- 只有当 self hub 成为瓶颈时才引入。
- 引入后仍以 PostgreSQL/outbox 为 truth。
- Centrifugo history 只作为短期恢复缓存。

## Task P1-2: 多房间扩展

问题：

单 hot auction 不代表平台容量。工业场景通常同时存在多个房间和热点偏斜。

要做：

- 1 hot + N cold rooms。
- N medium rooms。
- Zipf 分布房间热度。

验收：

- hot room 不污染 cold room correctness。
- outbox shard 分布可解释。
- 指标按 room/auction/shard 聚合。
