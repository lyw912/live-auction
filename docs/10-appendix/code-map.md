# 代码地图

父文档：[文档库入口](../README.md)
相关文档：[参考资料](references.md)

## 后端入口

| 文件 | 作用 | 答辩时怎么用 |
|---|---|---|
| `backend/cmd/server/main.go` | 启动 HTTP、Realtime、Outbox Relay、AI Worker、Kafka Settlement Worker、Scheduler/Fencer | 证明系统组件真实启动，不是文档摆设 |
| `backend/internal/config/config.go` | 配置默认值和环境变量 | 证明默认 `redis_ledger`、AI provider、WS/Admission 参数 |
| `backend/internal/storage/dependencies.go` | 打开 PG/Redis/MinIO 等依赖 | 说明运行依赖 |

## Gateway/API

| 文件 | 作用 |
|---|---|
| `backend/internal/gateway/router.go` | 全部 REST/WS 路由装配、AIGenerator 装配、Redis engine 装配 |
| `backend/internal/gateway/auction_handlers.go` | 商品、拍品、出价、订单、WS ticket、demo、chat/liveops |
| `backend/internal/gateway/bid_admission.go` | 出价 admission / 限流 / replay |
| `backend/internal/gateway/acl.go` | 房间成员 ACL |
| `backend/internal/gateway/monitor_handlers.go` | 监控/诊断 API |
| `backend/internal/gateway/ai_handlers.go` | AI API |

## 竞拍领域

| 文件 | 作用 |
|---|---|
| `backend/internal/auction/model.go` | 状态和输入模型 |
| `backend/internal/auction/rules.go` | 规则校验、Go 价格分类、PG legacy 参考 |
| `backend/internal/auction/repository.go` | 商品/拍品/规则/生命周期 |
| `backend/internal/auction/bid.go` | PG legacy 出价、订单、支付、幂等 |
| `backend/internal/auction/max_bid_intent.go` | 代理最高价意图 |
| `backend/internal/auction/liveops.go` | 直播运营活动 |
| `backend/internal/auction/chat.go` | 聊天 |

## Redis 热引擎

| 文件/符号 | 作用 |
|---|---|
| `backend/internal/redisengine/engine.go` | Redis Lua、Kafka ACK latch、relay、worker、recovery、reconciler，多职责大文件 |
| `ledgerRunner` | 主出价 Lua 脚本 |
| `confirmLedgerRunner` | 误触二次确认 Lua 脚本 |
| `placeBidWithSource` | Go 入口，校验 idempotency key 并调用 Lua |
| `placeBidWithSnapshot` | Lua 执行、冷启动、Kafka ACK 等待、降级 |
| `RunKafkaSettlement` | Kafka -> PG 结算 worker |
| `recoverPendingDecisions` | 恢复 pending 决策 |
| `rebuildRedisFromCheckpoint` | Redis 丢失恢复 |
| `backend/internal/redisengine/kafka_ledger.go` | Kafka ledger 适配 |
| `backend/internal/redisx/keys.go` | Redis key 规范 |
| `backend/internal/redisx/scripts.go` | Lua script runner |

核心难点讲解见：[工程难点与解决方案](../03-backend/05-engineering-difficulties.md)；选型取舍见：[技术选型与工业对标](../01-architecture/02-technology-selection-and-benchmark.md)。

出价/结算/恢复的第二期 L4 下钻：

| 目录 | 用途 |
|---|---|
| `docs/03-backend/auction-bid/` | 幂等、价格规则、误触、一口价、Kafka ACK |
| `docs/03-backend/settlement/` | Kafka redelivery、订单 exactly-once、outbox retry |
| `docs/03-backend/recovery/` | RECONCILING、checkpoint rebuild |

## 实时系统

| 文件 | 作用 |
|---|---|
| `backend/internal/realtime/server.go` | WS ticket 校验、连接、恢复、快照 |
| `backend/internal/realtime/hub.go` | 房间/拍品 Hub、队列、慢消费者 |
| `backend/internal/realtime/ticket.go` | WS ticket |
| `backend/internal/realtime/leaderboard.go` | 排行榜投影 |
| `backend/internal/realtime/admission.go` | WS 连接 admission |

实时/H5 的第二期 L4 下钻：

| 目录 | 用途 |
|---|---|
| `docs/04-realtime/websocket/` | ticket scope、last_seq recovery、slow consumer |
| `docs/05-frontend/mobile-h5/` | 出价 timeout/uncertain、服务端时间倒计时、seq gap snapshot |

## AI

| 文件 | 作用 |
|---|---|
| `backend/internal/ai/types.go` | AI 请求/结果结构、归一化、安全回退 |
| `backend/internal/ai/chat_provider.go` | OpenAI-compatible chat completions adapter + JSON schema |
| `backend/internal/ai/repository.go` | AI job、系统消息、哨兵、复盘、高光 |
| `backend/internal/ai/provider.go` | Generator 接口/默认实现 |

## 前端

| 文件 | 作用 |
|---|---|
| `frontend/mobile-h5/src/main.tsx` | H5 主应用、出价、WS、恢复、订单、Q&A、音效 |
| `frontend/mobile-h5/src/domain.ts` | 领域类型、倒计时、状态派生、文案、工具 |
| `frontend/mobile-h5/src/atmosphere.ts` | 氛围 cue 归一化和强度 |
| `frontend/mobile-h5/src/components.tsx` | H5 组件 |
| `frontend/mobile-h5/src/result.tsx` | 结果/支付页 |
| `frontend/pc-console/src/main.tsx` | PC 主应用和 API |
| `frontend/pc-console/src/components.tsx` | PC 控制台组件 |
| `frontend/shared-design` | 共享设计令牌 |

## 测试与证据

| 目录 | 用途 |
|---|---|
| `backend/internal/**/*_test.go` | Go 单元/集成测试 |
| `tests/pts` | 阿里云 PTS/JMeter 资产、S1-S5 verifier |
| `tests/load` | k6 压测和本地 stress runner |
| `tests/risk` | P4 产品风险模拟器 |
| `tests/e2e` | Playwright H5/PC/e2e/视觉回归 |
| `tests/chaos` | Toxiproxy/故障注入脚本 |

## 数据库迁移阅读顺序

1. `202605220001_init_core.sql`：核心表、outbox、idempotency、orders。
2. `202605230002_room_memberships.sql`：房间 ACL。
3. `202605240001_payment_provider_boundary.sql`：支付 provider 边界。
4. `202605270001_max_bid_intents.sql`：代理最高价。
5. `202605280001_redis_ledger_engine.sql`：Redis engine 字段和 settlement 表。
6. `202605290001_kafka_bid_ledger.sql`：Kafka 位点。
7. `202605300002_redis_engine_checkpoints.sql`：checkpoint。
8. `202606060001_ai_atmosphere_capabilities.sql`：AI job/message/alert。
9. `202606090002/003`：拍品投影一致性约束/触发器。
