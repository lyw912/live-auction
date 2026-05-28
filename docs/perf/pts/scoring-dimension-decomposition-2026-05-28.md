# 评分维度逐词拆解与工程映射

Date: 2026-05-28

Status: PTS-1 架构重构评分对齐材料

## 高并发架构硬核优化逐项拆解

官方原文：

```text
Redis 分层缓存策略，读写分离
分布式锁解决出价幂等性，绝对不允许一笔出价扣两次钱
WebSocket 房间级路由隔离，支持单直播间 1000+ 用户同时在线
```

### Redis 分层缓存策略

| 词 | 工程含义 | 本项目落点 | 证据 |
|---|---|---|---|
| Redis | 不是只做普通缓存，而是承担热点读、限流、实时历史、可选热竞价状态机。 | admission GCRA、WS ticket、realtime history/snapshot；拟新增 Redis ledger bidding engine。 | Redis latency、eviction、blocked clients、script latency、recovery tests。 |
| 分层 | 不同数据按一致性要求分层，不能一个 Redis 打天下。 | L1 admission/rate limit；L2 current auction projection；L3 WS history/snapshot；L4 可选 hot-engine state；L5 reconciliation ledger。 | 架构图、key 设计、降级表、故障注入。 |
| 缓存 | 缓存只能服务可重建数据；价格/winner 若进入 Redis 必须升格为 hot-engine state 并配 ledger。 | 默认 PG truth + Redis projection；`redis_ledger` 模式下 Redis 是热路径状态机，PG 是 settlement/audit truth。 | Redis/DB 对账、stream replay、invariant verifier。 |
| 策略 | 有 TTL、失效、重建、防击穿、容量上限、降级。 | snapshot TTL、history TTL、rebuild semaphore、Redis-down fail-open/paused policy。 | reconnect storm、Redis down、snapshot rebuild bounded gates。 |

### 读写分离

| 词 | 工程含义 | 本项目落点 | 风险边界 |
|---|---|---|---|
| 读 | 大量围观者读当前价、倒计时、排名、历史。 | Redis snapshot/history、H5 local derived countdown、leaderboard query/cache。 | 读不能裁判成交；必须带 seq/source。 |
| 写 | 出价、取消、截拍、支付是强一致写。 | PG baseline 写；Redis ledger 模式写入 Lua state + stream，再 settlement。 | 写路径必须单 auction 串行。 |
| 分离 | watcher 读压不能挤占 bid 写压；snapshot 不能拖慢 bid path。 | 独立 WS/recovery 限流、snapshot rebuild semaphore、DB pool/route metrics。 | PTS 需分别测 bid、snapshot、ticket、WS。 |

### 分布式锁解决出价幂等性

官方说“分布式锁”，但工业实现不应机械套 RedLock。这里要拆成两个问题：互斥与幂等。

| 词 | 工程含义 | 推荐实现 |
|---|---|---|
| 分布式 | 多实例部署下同一 auction 仍只有一个有效序列化决策者。 | PG row lock baseline；Redis engine 使用 `{auction_id}` hash tag + Lua 原子脚本 + `engine_epoch` fencing。 |
| 锁 | 锁不是目的，目的是同一拍卖状态转换原子化。 | 避免无限等待锁；用 per-auction lane、Lua CAS、bounded retry。 |
| 出价 | 出价包含规则校验、当前价/winner、soft close、cap sold、事件 seq。 | 必须在同一原子单元里处理，不能校验和更新分离。 |
| 幂等性 | 同一 `client_bid_id` 重试返回同一结果，不重复扣保证金/生成订单/写 bid。 | Redis idem key + DB idempotency_records + unique constraints + request_hash。 |
| 绝对不允许 | 这是 P0 不变量，不是性能优化项。 | invariant checker：duplicate bid/order/payment/idempotency consistency。 |
| 一笔出价扣两次钱 | 当前项目是 mock 支付/保证金语义，等价风险是 duplicate order/payment transition。 | orders unique, payment idempotency, provider_event_id unique, settlement idempotent upsert。 |

### WebSocket 房间级路由隔离

| 词 | 工程含义 | 本项目落点 | 证据 |
|---|---|---|---|
| WebSocket | bid HTTP 只返回个人结果；房间状态靠 WS 广播。 | `/ws` ticket/subprotocol, room+auction params, hub publish。 | ws-auth-browser, fanout, reconnect, slow-consumer gates。 |
| 房间级 | 用户只能订阅自己有权限的 room；不同 room 事件不串。 | room ACL, membership check, room context routing。 | forged-room, multi-room isolation tests。 |
| 路由 | 后端必须按 room/auction 定位连接集合，不能全局广播。 | hub room map, auction event publish keyed by auction_id/room_id。 | fanout metrics, no cross-room leak invariant。 |
| 隔离 | 热房间不能拖垮冷房间；慢客户端不能拖垮健康客户端。 | bounded client queue, slow-consumer close, shard/worker ownership。 | hot/cold stress, slow consumer, RSS/goroutine bounds。 |

### 支持单直播间 1000+ 用户同时在线

这句话不能靠口号。需要拆成“连接数、在线时长、事件频率、用户行为、资源上限、体验正确性”。

| 词 | 工程含义 | 验收方式 |
|---|---|---|
| 支持 | 不是能连上一次，而是稳定在线、收事件、能恢复、资源可控。 | 3-5 分钟 WS 连接保持；bid traffic 同时运行。 |
| 单直播间 | 一个 room/auction 的热点 fanout，不是多房间平均摊薄。 | one hot room workload。 |
| 1000+ 用户 | 1000 条长连接，含健康客户端、慢客户端、断线重连客户端。 | k6/PTS WS 场景，记录成功率、close reason、fanout lag。 |
| 同时在线 | 并发连接同时存在，不是累计登录。 | active connection gauge and FD/RSS/goroutine metrics。 |
| 超越基础要求 10 倍 | 对评委的差异化声明，必须有原始证据和环境说明。 | Linux/ECS evidence, raw output, no fake local-only final claim。 |

## 评分表逐词拆解

### 技术实现与工程完整度，50%

| 原词 | 评委真实在看什么 | 本项目必须展示什么 |
|---|---|---|
| 技术实现 | 不是 PPT，是真代码、真接口、真状态。 | PC/H5/Go API/DB/Redis/WS/outbox/scheduler 全链路可跑。 |
| 工程完整度 | 是否有异常路径、测试、监控、可维护结构。 | auth/ACL/idempotency/recovery/diagnostics/runbook/evidence。 |
| 完整工程链路 | 用户从创建商品到支付历史是否闭环。 | 商品上架、规则配置、开拍、出价、成交、订单、mock 支付、历史。 |
| 竞拍数据采集 | 不只 accepted bid，reject/用户行为/WS/订单也要采。 | bids、auction_events、outbox、system_anomaly_events、flight recorder。 |
| 出价 | 核心写路径。 | increment/cap/soft close/cancel race/idempotency/seq。 |
| 用户行为 | 围观、加入、点击、重连、支付等行为能驱动状态和诊断。 | chat/join/WS ticket/reconnect/payment/history。 |
| 数据治理 | schema、枚举、trace、幂等、审计、清理。 | integer cents、request_hash、trace_id、seq、unique constraints、reconciliation。 |
| 开源模型调用（可选） | 可选，不应抢核心时间。 | 可以不做；若做只做 host prompt/risk assist，不参与裁判。 |
| 后端服务 | 规则、状态机、事务、实时、调度。 | Repository/gateway/realtime/outbox/scheduler/invariant。 |
| 出价校验 | 服务端权威规则。 | invalid amount, increment mismatch, above cap, self-leading, ended/inactive。 |
| 状态机管控 | 所有终态不可逆，竞态确定。 | DRAFT/SCHEDULED/ACTIVE/SOLD/ENDED/CANCELLED transitions。 |
| 接口网关 | API 不是裸路由。 | auth, ACL, schema, rate limit, idempotency, error code, trace。 |
| 前端交互 | 用户能感知 pending/reject/recover/sold，而不是静态页面。 | H5 bid dock、PC console、leaderboard、animation、payment sheet。 |
| 氛围动画 | 必须事件驱动，不能假成功。 | bid_accepted/outbid/extended/sold/cancelled animations。 |
| 实时反馈 | HTTP 个人结果 + WS 房间状态。 | pending, rejected reason, leading/outbid, gap recovery。 |
| 顺畅闭环度 | 每一步失败后还能解释和恢复。 | no dead-end state; runbook and monitor pages。 |

### 可用性、性能、稳定性、一致性、可观测性

| 原词 | 评委真实在看什么 | 本项目必须展示什么 |
|---|---|---|
| 系统可用性 | 故障时是否有降级和恢复。 | Redis down, DB lock timeout, reconnect storm, outbox poison gates。 |
| 断连重连 | H5 网络波动后不乱价、不误成交。 | last_seq recovery, history replay, snapshot fallback, CTA disabled while recovering。 |
| 异常兜底 | 出现不确定时不能继续错下去。 | RECONCILING/PAUSED, anomaly, replay/repair plan。 |
| 性能 | 有基线、有对比、有瓶颈归因。 | PTS before/after, p95/p99, DB/Redis/WS/runtime metrics。 |
| 稳定性 | 长时间压力下资源不爆。 | queue bounds, FD/RSS/goroutine, Redis memory, outbox lag。 |
| 缓存防击穿 | snapshot/rebuild 不把 DB 打死。 | rebuild semaphore, TTL, stale fallback, retry-after。 |
| 数据一致性 | 价格、赢家、订单、支付不能错。 | invariant verifier, unique constraints, settlement replay。 |
| 可观测性 | 出问题能定位而不是猜。 | metrics, monitor pages, flight recorder, anomaly events, raw evidence。 |
| 竞拍状态监控 | host/评委能看到真实状态。 | current price/winner/seq/end_at/outbox/recovery/reject distribution。 |
| 异常告警 | 故障要结构化记录。 | system_anomaly_events, alert rules, reconciliation incidents。 |

### 技术深度与创新性，25%

| 原词 | 评委真实在看什么 | 本项目差异化回答 |
|---|---|---|
| 技术深度 | 是否理解取舍和失败模式。 | PG row lock vs actor lane vs Redis Lua vs ledger settlement 对比。 |
| 创新性 | 是否有贴合场景的增强，不是堆技术。 | Redis ledger hot engine + product-honest settlement states。 |
| 技术选型 | 技术是否服务问题。 | Go for concurrency, Redis for hot state/projection, PG for audit/settlement, WS for realtime。 |
| React/TypeScript | 前端状态安全和组件化。 | typed API, H5 state matrix, PC diagnostics。 |
| WebSocket | 长连接、恢复、背压。 | room hub, heartbeat, slow close, gap snapshot。 |
| Node/Go | 后端高并发实现。 | Go chosen for goroutine/network/typed service boundaries。 |
| Redis/MySQL 等 | Redis 不能只写在架构图里。 | GCRA, Lua, stream, snapshot, recovery, metrics。 |
| 课题场景适配 | 直播竞拍不是普通 CRUD。 | final-second bidding, soft close, cap sold, room fanout。 |
| 高并发直播竞拍 | 单热点写 + 大量读 + 实时广播。 | bid engine + watcher fanout + admission + queue + reconciliation。 |
| 核心挑战 | 规则复杂和实时同步同时成立。 | no wrong winner + low latency + recoverable WS。 |
| 实时同步 | 不只是“秒级刷新”。 | seq, server time, outbox, WS history/snapshot。 |
| 高并发 | 需要压力模型和证据。 | PTS/k6 workloads, admission-off downstream pressure。 |
| WebSocket 不稳定 | 弱网是正常场景。 | heartbeat, reconnect, snapshot, slow-consumer。 |
| 针对性优化 | 优化命中瓶颈。 | PTS-1 指向 DB lock/pool, 所以做 lane/Redis ledger，而不是盲目加机器。 |
| 独特或前瞻性思考 | 能讲出工业路径。 | hot engine + durable ledger + settlement + reconciliation。 |
| 房间级 WebSocket 路由隔离 | 热房不污染冷房。 | room ACL, hub partition, multi-room isolation evidence。 |
| 出价幂等性设计 | 重试/双击/支付不能重复。 | idempotency key, request hash, Redis idem, DB idem, unique constraints。 |
| 跨端状态同步优化 | PC/H5/重连后一致。 | server seq/time, same event source, snapshot fallback。 |
| 技术差异化优势 | 为什么选这个项目。 | 不止功能完整；能证明热点性能瓶颈、重构路径、故障修复闭环。 |

## 最终答辩句式

```text
我们没有把 Redis 当普通缓存，而是按一致性等级分层：限流、实时投影、历史恢复、可选热竞价状态机分别治理。
出价幂等不是靠一句分布式锁，而是 request_hash、engine_seq、fencing、DB unique constraint 和 replay verifier 共同保证。
WebSocket 不是单机广播 demo，而是 room-scoped long connection，带 heartbeat、slow-consumer、last_seq recovery 和 snapshot fallback。
1000+ 在线不写空话，必须用单房间长连接压力、fanout lag、RSS/FD/goroutine 和重连成功率证明。
```

