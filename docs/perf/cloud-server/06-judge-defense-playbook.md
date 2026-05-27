# 06 · 评委地狱拷问手册

本项目的答辩策略不是回避问题，而是把每个 claim 降到证据能支撑的范围内。评委可能来自大厂后端、SRE、DBA、实时系统、产品、测试、运维、业务方。每个角度的问题都必须有 raw evidence 或明确承认未证明。

## 总体防线

不能说：

```text
我们支持 600 TPS。
我们实时链路没问题。
引入 Centrifugo 就能解决。
服务器 CPU 很空，所以没有瓶颈。
PTS 100% 成功，所以系统达标。
```

可以说：

```text
本轮 PTS HTTP 层在 643 TPS、P99 153ms 下无 HTTP 失败。
但业务上 94% bid 被拒绝，不能等同于有效成交 TPS。
服务侧 outbox 积压超过 30 万，证明 realtime delivery 是当前 P0 瓶颈。
Redis publish 和机器资源不是当前主瓶颈，有指标支持。
下一步按 P0 重新修脚本、修 relay、跑同等压力回归。
```

## 大厂后端评委

问题：

> 你如何保证同一场竞拍不会出现错 winner、错价格、重复订单？

答案要点：

- PostgreSQL auction row lock 作为序列化点。
- bid、auction state、auction_events、outbox、idempotency 在一个事务内提交。
- Redis/WS 只是投影和通知，不参与 money truth。
- 需要指向 bid integration tests 和 DB schema。

还缺的证据：

- 云端高并发 accepted-only 后的 invariant check。
- `seq continuous`、`one winner/order` 自动检查输出。

问题：

> 单 hot auction 只能串行，为什么还能叫工业级？

答案要点：

- 金钱竞拍天然要求同 auction 顺序一致。
- 工业级不是盲目并行，而是明确业务不变量和扩展边界。
- 多 auction 可以水平扩展，单 hot auction 要通过高效串行、降级、聚合、admission 保护和产品策略处理。

## SRE/运维评委

问题：

> 你的容量规划怎么做？安全余量是多少？

答案要点：

- 先找 breaking point，再设 admission 和 capacity target。
- 每个容量数字需要 3-run baseline。
- 记录环境、commit、脚本、数据集、raw output。
- 使用 RED + USE：rate、error、duration、utilization、saturation、errors。

当前不能声称：

- 最终容量。
- 生产安全余量。

问题：

> 为什么说不是机器瓶颈？

答案要点：

- 压测后 CPU idle 约 84%、iowait 约 0.1%、内存 available 约 28GB、磁盘 util 低。
- Redis pipeline 平均约 0.29ms。
- 但承认当前缺 during 连续采样，下一轮补齐。

## DBA 评委

问题：

> outbox 为什么会积压？索引是否正确？是不是 SKIP LOCKED 用错？

答案要点：

- 当前全部 pending 在同一个 shard，说明是单 auction/shard 顺序 drain 低于事件生产速度。
- `SKIP LOCKED` 可以用于 queue-like table，但必须配合正确索引和批量 claim。
- 当前 `ProcessBatch` 只是循环 `ProcessOne`，DB round trip、mark/watermark 成本过高。

下一步证据：

- claim latency histogram。
- EXPLAIN before/after。
- published/s vs produced/s。
- n_dead_tup、index size、vacuum 状态。

## 实时系统评委

问题：

> 为什么不直接用 Centrifugo？

答案要点：

- Centrifugo 解决连接和 fanout，不解决 DB outbox 未发布。
- 当前 Redis publish 快，WS fanout 未被证明是主瓶颈。
- Centrifugo channel 顺序要求顺序发布，不能绕过同 auction 顺序问题。
- 未来可作为 delivery gateway，但 PostgreSQL/outbox 仍是真源。

问题：

> WebSocket 丢消息怎么办？

答案要点：

- 客户端按 `auction_id + seq` dedupe。
- gap 后进入 recovering，拉 history 或 snapshot。
- outbox poison 发布 gap notice。
- 还需用 reconnect storm 和 slow consumer 压测证明。

## 产品经理评委

问题：

> 用户关心的是能不能抢到，不是你的 outbox。当前体验高压下真实吗？

答案要点：

- 当前 HTTP response 仍返回权威结果，所以本人出价结果可知。
- 但房间实时广播存在积压，观众状态可能滞后，这就是 P0 必修问题。
- 产品上要区分“本人出价确认”和“全房间氛围事件”。

问题：

> 每个失败出价都要广播吗？

答案要点：

- 不建议。失败出价不改变价格和 winner。
- 本人失败由 HTTP response 确认。
- 全房间可以用聚合热度事件，避免无价值事件冲垮实时链路。

## 用户视角评委

问题：

> 网络断了再回来，用户会不会看到错价格然后继续出价？

答案要点：

- recovering 状态 CTA 禁用。
- 根据 seq gap 拉 snapshot。
- 客户端不能用本地倒计时自行 hammer。

还缺证据：

- H5 弱网和 reconnect storm 云端测试。
- 真实浏览器 WebSocket 压测。

## 测试工程师评委

问题：

> 你的压测脚本是不是造假？为什么 94% 都是 reject？

答案要点：

- 这轮脚本确实污染业务模型，所以报告不用于 accepted capacity claim。
- 已拆分任务修 amount/end_at/seed。
- 下一轮必须输出 accepted/rejected/admission/idempotency 业务分布。

## 面试官最后一问

> 为什么选你的项目，而不是其他精英项目？

当前最强可防御点：

- 没有把绿色 HTTP 报告包装成虚假容量。
- 能从 PTS、DB、Redis、Prometheus、机器资源交叉归因。
- 明确识别 outbox realtime delivery 是 P0 工业化短板。
- 保持 PostgreSQL truth，不为性能牺牲金钱正确性。
- 把每个未证明能力列为任务，而不是写成营销话术。

当前最大弱点：

- 还没有修复 outbox relay。
- 还没有 accepted-only 云端回归。
- 还没有真实 WS fanout/reconnect storm 云端报告。
- during evidence 不足。

答辩底线：

> 我不会声称已经工业级达标。我会证明我知道工业级要证明什么、当前哪里不达标、为什么不达标、下一步怎么用证据修到达标。
