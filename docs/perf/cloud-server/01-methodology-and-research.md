# 01 · 方法论与调研

本项目必须按工业级性能工程处理，不能用“本地能跑”“报告全绿”“平均延迟很好”作为答辩依据。评委会追问负载模型、业务真实性、资源饱和、故障恢复、证据链和架构取舍。本章定义压测方法论和外部参考。

## 核心方法论

### 1. SRE 容量规划

Google SRE 明确把容量规划和常规负载测试绑定：需要定期负载测试，将机器、磁盘、网络等原始容量关联到服务容量；还要找到系统 breaking point，用于回归测试、最坏情况阈值和安全余量决策。

本项目落地原则：

- 不只测“能承受多少”，还要测“从哪里开始坏”。
- 每个容量数字必须绑定环境、脚本、数据集、commit、raw output。
- 必须测试 ramp 和 impulse 两种压力，因为缓存、连接池、JIT/GC、数据库热页会导致渐进压力和突增压力表现不同。
- 必须测试超载后的恢复：降回正常流量后 backlog 是否回落、延迟是否恢复、客户端是否一致。

参考：

- Google SRE, Monitoring Distributed Systems: https://sre.google/sre-book/monitoring-distributed-systems
- Google SRE, Addressing Cascading Failures: https://sre.google/sre-book/addressing-cascading-failures
- Google SRE, Introduction, Demand Forecasting and Capacity Planning: https://sre.google/sre-book/introduction

### 2. RED 方法

对每个服务接口看三类指标：

| 指标 | 本项目含义 |
|---|---|
| Rate | bid/s、snapshot/s、ws-ticket/s、WS publish/s |
| Errors | HTTP 5xx/4xx、业务 reject、admission 429、timeout、dropped iteration |
| Duration | p50/p95/p99/p999、lock wait、tx duration、fanout latency |

关键点：HTTP success 不是业务 success。竞拍系统必须同时区分：

- HTTP success
- bid accepted
- bid rejected
- admission rejected
- idempotency replay
- retry later
- downstream outbox published
- client received and applied

### 3. USE 方法

Brendan Gregg 的 USE 方法要求对每个资源看 Utilization、Saturation、Errors。低平均利用率不能证明没有瞬时饱和，任何非零队列或错误都应调查。

本项目资源清单：

| 资源 | Utilization | Saturation | Errors |
|---|---|---|---|
| CPU | `%usr/%sys/%idle` | run queue, pidstat | steal, throttling |
| Memory | RSS, heap, cache | reclaim, swap, GC pressure | OOM |
| Disk | util, await, IOPS | queue depth | IO error |
| Network | throughput, retransmit | socket backlog, send queue | reset, timeout |
| PostgreSQL | CPU, buffer hit, tx/s | lock wait, pool wait, slow query | deadlock, timeout |
| Redis | ops/s, CPU, memory | blocked clients, latency | evictions, rejected connections |
| Go runtime | goroutines, heap, GC | scheduler latency, queues | panic |
| Outbox | publish/s | backlog, oldest age | DEAD, retry |
| WS hub | fanout/s | per-client queue depth | slow close, dropped |

参考：

- USE Method: https://www.brendangregg.com/usemethod.html
- Linux USE checklist: https://www.brendangregg.com/USEmethod/use-linux.html

## 负载模型

### Closed model

JMeter/PTS 的虚拟用户模式通常是 closed model：用户完成一次请求后再发下一次。系统变慢时，吞吐会自然下降，容易掩盖真实流量到达压力。

适用：

- 模拟固定在线用户行为。
- 验证长连接、用户会话、真实流程。

不适用：

- 声称系统在固定 RPS 下仍稳定。
- 找队列、backlog、过载阈值。

### Open model

k6 `constant-arrival-rate` / `ramping-arrival-rate` 是 open model：按固定到达率发起迭代，不等待系统响应自然降速。它更适合测服务在固定业务到达率下的饱和点。

适用：

- 找 bid/s、publish/s、snapshot/s 的真实瓶颈。
- 验证 backlog 是否在固定到达率下持续增长。

参考：

- k6 constant-arrival-rate: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-arrival-rate
- k6 arrival-rate VU allocation: https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/arrival-rate-vu-allocation
- k6 scenarios and executors: https://grafana.com/docs/k6/latest/using-k6/scenarios/

本项目要求：

- PTS VU 模式用于云端 JMeter 对比和用户流程压测。
- k6 arrival-rate 用于精确找后端瓶颈。
- 两者结果不能混用为同一个容量数字。

## PostgreSQL 队列表和 SKIP LOCKED

PostgreSQL 文档说明 `SKIP LOCKED` 可用于多个消费者访问 queue-like table，但它会提供不一致视图，不适合一般业务查询。对 outbox 队列表，`SKIP LOCKED` 是合理工具，但必须结合正确索引、批量 claim、明确顺序约束和可观测性。

本项目问题：

- 当前 outbox relay 每次处理一个事件。
- 同 auction 严格顺序依赖 head-of-line 检查。
- 单 auction 高事件量时，即使单次 claim 可以很快，整体消费能力仍远低于生产速度。
- 在 lease/积压/扫描不理想时，claim 查询可退化到秒级。

参考：

- PostgreSQL SELECT locking clause: https://www.postgresql.org/docs/current/sql-select.html
- PostgreSQL explicit locking: https://www.postgresql.org/docs/current/explicit-locking.html

## Transactional Outbox

Debezium 的 outbox event router 体现了成熟实践：业务事务写 outbox，提交后由可靠 relay/CDC 传播到消息系统。它解决的是“数据库提交后进程崩溃导致事件丢失”的问题，不自动解决高吞吐、单 key 顺序、消费者 lag 和 fanout。

本项目保留 outbox 的原因：

- bid、auction_events、outbox、idempotency 必须原子提交。
- WebSocket 不是 truth，Redis 不是 audit。
- 崩溃后必须能从 DB 恢复未发布事件。

必须改进的地方：

- 批量消费。
- key-level ordering。
- backlog 治理。
- publish/mark/watermark 批处理。
- poison/gap 可观测。

参考：

- Debezium Outbox Event Router: https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html

## Kafka/NATS/Centrifugo 的边界

### Kafka

Kafka 的顺序保证以 partition 为边界；同 key 消息进入同 partition 才能保持顺序。提升 partition 数能提升并行度，但单 hot key 仍会落在一个 partition，形成 key-level 热点。

对本项目的启示：

- `auction_id` 是必须保序的 key。
- 多 auction 能水平扩展，单 hot auction 不能靠更多 partition 自动并行。
- 若使用 Kafka，仍要设计单 auction 的吞吐上限、降级和业务聚合。

参考：

- Kafka partition key and ordering: https://www.confluent.io/learn/kafka-partition-key/
- Kafka message key: https://www.confluent.io/learn/kafka-message-key/

### Centrifugo

Centrifugo 是成熟的 realtime gateway，适合连接管理、channel fanout、presence、短期 history/recovery 和横向扩展。它不是数据库 truth，也不替代 durable outbox。

关键边界：

- Centrifugo channel history是热缓存，用于短断线恢复，不应作为审计事实。
- 官方设计说明：同 channel 只有顺序发布或单请求内发布才保证顺序；并行发布到同 channel 不保证顺序。
- Redis/NATS broker 层可能是 at-most-once，Centrifugo 用 offset/history 辅助发现和恢复，但应用仍需能从主数据库恢复状态。

本项目结论：

- Centrifugo 可以作为 P1/P2 的 WS fanout gateway。
- 它不能解决当前 P0 outbox 积压。
- 若引入，定位为 delivery gateway，不是 truth source。

参考：

- Centrifugo design overview: https://centrifugal.dev/docs/getting-started/design
- Centrifugo engines and scalability: https://centrifugal.dev/docs/5/server/engines

## 阿里云 PTS 方法约束

本项目云端 PTS 使用：

- 压力来源：阿里云 VPC 内网。
- 地域：华南2（河源）。
- 目标：ECS 内网 IP `172.16.179.112:18080`。
- CSV 数据源上传至 PTS 数据源。
- JMX 中必须走 `/api/auth/login` 或使用预生成 session CSV，不能依赖 mock auth。

PTS 报告优点：

- 云端同地域压力源。
- JMeter 报告、API 级聚合、采样日志。
- 适合展示正式云端压测证据。

PTS 报告限制：

- HTTP success 不等于业务 success。
- 采样日志可能不是全量请求日志。
- 服务侧证据必须自己采集：DB、Redis、metrics、CPU、IO、网络、Go runtime。

参考：

- Alibaba Cloud PTS JMeter scenario: https://www.alibabacloud.com/help/en/pts/performance-test-pts-3-0/user-guide/create-a-jmeter-scenario1
