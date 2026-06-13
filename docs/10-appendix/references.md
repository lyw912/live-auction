# 参考资料与外部依据

父文档：[文档库入口](../README.md)

## 官方/一手资料

| 主题 | 链接 | 本项目使用方式 |
|---|---|---|
| Redis Lua scripting | https://redis.io/docs/latest/develop/programmability/eval-intro | 支撑“脚本在 Redis 服务端执行，适合高效原子组合应用逻辑”的选型解释 |
| Redis Functions / EVAL 背景 | https://redis.io/docs/latest/develop/programmability/functions-intro | 解释 EVAL/Functions 的工业化演进 |
| Kafka delivery semantics | https://docs.confluent.io/kafka/design/delivery-semantics.html | 支撑不吹 Kafka 层 exactly-once，区分 at-least-once 和 EOS 条件 |
| Kafka exactly-once 条件 | https://docs.confluent.io/platform/current/clients/confluent-kafka-go/index.html | 说明事务 producer 建立在 idempotent producer 上 |
| PostgreSQL locks | https://www.postgresql.org/docs/current/explicit-locking.html | 支撑行锁冲突等待和 PG 行锁热路径瓶颈解释 |
| MDN WebSocket API | https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API | 支撑浏览器 WS 长连接语义 |
| MDN WebSocketStream | https://developer.mozilla.org/en-US/docs/Web/API/WebSocketStream | 支撑传统 WS 需自管背压，WebSocketStream 才有 stream backpressure |
| MDN AbortSignal | https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal | 支撑 H5 fetch 超时/abort 的标准实现方式 |
| OWASP WebSocket Security Cheat Sheet | https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html | 支撑 WS 不能只靠裸连接，需要认证、授权、Origin/scope 等安全边界 |
| Prometheus alerting rules | https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/ | 支撑告警规则设计 |
| OpenTelemetry docs | https://opentelemetry.io/docs/ | 支撑 traces/metrics/logs 观测模型 |
| TikTok Shop LIVE Countdown Bidding | 官方 Seller Center / Help Center 页面 | 支撑直播电商倒计时竞拍是实际业务形态 |

## 内部可信材料

| 路径 | 用途 |
|---|---|
| `submission/第五组-李烨文-训练营结项文档.md` | 最终提交叙事、S1-S5、业务亮点 |
| `submission/8分钟提问追问备战手册.md` | 答辩问题与追问口径 |
| `submission/20分钟答辩演示设计.md` | 演示节奏 |
| `submission/championship-review-2026-06-10/评委视角终审-直播竞拍全栈系统.md` | 第三方终审视角和缺陷清单 |
| `submission/championship-review-2026-06-10/附录-决策路径等价性与校验门禁映射.md` | Lua/PG 等价性和门禁映射 |

## 代码证据入口

| 主题 | 路径 |
|---|---|
| 服务启动 | `backend/cmd/server/main.go` |
| 路由 | `backend/internal/gateway/router.go` |
| 出价网关 | `backend/internal/gateway/auction_handlers.go` |
| Redis 热引擎 | `backend/internal/redisengine/engine.go` |
| Kafka ledger | `backend/internal/redisengine/kafka_ledger.go` |
| 领域规则 | `backend/internal/auction/rules.go` |
| 订单/支付 | `backend/internal/auction/bid.go` |
| 实时服务 | `backend/internal/realtime/server.go`, `hub.go`, `ticket.go` |
| AI | `backend/internal/ai/*`, `backend/internal/gateway/ai_handlers.go` |
| H5 | `frontend/mobile-h5/src/main.tsx`, `domain.ts`, `atmosphere.ts` |
| PC | `frontend/pc-console/src/main.tsx`, `components.tsx` |
| 测试资产 | `tests/pts`, `tests/load`, `tests/risk`, `tests/e2e` |
