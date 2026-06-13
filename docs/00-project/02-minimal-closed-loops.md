# 最小闭环索引

父文档：[项目总览](00-overview.md)
相关文档：[文档库入口](../README.md)

本页把“评委给一个场景，问你怎么办”映射到最底层闭环文档。

| 场景 | 最小闭环文档 | 你要讲清的核心 |
|---|---|---|
| 买家点一次出价 | [单次出价闭环](../03-backend/01-bid-decision-closed-loop.md) | H5 `client_bid_id` -> Gateway -> Lua -> Kafka ACK/ENGINE_DURABLE |
| 弱网重复点击/同 key 改金额 | [出价幂等键 L4](../03-backend/auction-bid/01-idempotency-key-closed-loop.md) | HTTP key、request hash、Redis idem、PG unique |
| 出价低于最低价/不在网格 | [Lua 价格规则 L4](../03-backend/auction-bid/02-lua-price-rule-closed-loop.md) | min next bid、increment grid、服务端权威拒绝 |
| 高额误触 | [误触确认 L4](../03-backend/auction-bid/03-fat-finger-confirm-closed-loop.md) | pending confirm token、二次确认、重校验 |
| 一口价成交 | [cap sold/order L4](../03-backend/auction-bid/04-cap-sold-order-closed-loop.md) | cap 触发 SOLD、订单唯一、支付幂等 |
| Redis 接受但 Kafka ACK 慢 | [Kafka ACK 持久性 L4](../03-backend/auction-bid/05-kafka-ack-durability-closed-loop.md) | `KAFKA_ACKED` vs `ENGINE_DURABLE` |
| Kafka 重复投递 | [Kafka 结算闭环](../03-backend/02-kafka-settlement-closed-loop.md) | at-least-once + PG unique/CAS = 业务幂等 |
| Kafka redelivery | [结算重投幂等 L4](../03-backend/settlement/01-kafka-redelivery-idempotency.md) | settlement unique + engine seq CAS |
| SOLD 双建单 | [订单 exactly-once L4](../03-backend/settlement/02-order-creation-exactly-once.md) | `orders.auction_id UNIQUE` |
| Outbox 投递失败 | [Outbox retry L4](../03-backend/settlement/03-outbox-publish-retry.md) | claim、retry、failed/dead、operator signal |
| Redis 被清空 | [Redis 丢失恢复](../03-backend/03-redis-loss-recovery.md) | RECONCILING -> checkpoint/Kafka/PG rebuild -> 校验恢复 |
| Redis state missing | [RECONCILING L4](../03-backend/recovery/01-redis-state-missing-reconciling.md) | fail-closed，不猜状态 |
| checkpoint 重建 | [Checkpoint rebuild L4](../03-backend/recovery/02-checkpoint-rebuild.md) | engine_epoch/seq/state_hash |
| 手机断网重连 | [WebSocket 恢复](../04-realtime/01-websocket-recovery-closed-loop.md) | ticket + last_seq + history/db/snapshot |
| WS ticket 被偷/复用 | [Ticket scope L4](../04-realtime/websocket/01-ticket-scope-consume.md) | 一次性 consume、scope、membership |
| WS seq 断档 | [last_seq recovery L4](../04-realtime/websocket/02-last-seq-recovery.md) | history 连续性，否则 snapshot |
| 弱网客户端不读 WS | [慢消费者 L4](../04-realtime/websocket/03-slow-consumer-disconnect.md) | 有界队列、写超时、断开恢复 |
| H5 倒计时/出价弱网 | [H5 竞拍闭环](../05-frontend/01-mobile-h5-closed-loop.md) | 服务器时间锚、pending/uncertain、AbortController、幂等重试 |
| H5 响应丢失 | [H5 timeout/uncertain L4](../05-frontend/mobile-h5/01-bid-timeout-uncertain-retry.md) | 8s timeout、保留 pending、同 key 重试 |
| 手机时间不准 | [服务端时间倒计时 L4](../05-frontend/mobile-h5/02-countdown-server-time-anchor.md) | 只用本地 elapsed，不本地落槌 |
| H5 收到 out-of-order 事件 | [seq gap 快照恢复 L4](../05-frontend/mobile-h5/03-seq-gap-snapshot-recovery.md) | gap 先 snapshot，不套用半截事件 |
| 主播创建拍品/改规则 | [PC 控制台闭环](../05-frontend/02-pc-console-closed-loop.md) | 规则配置、冻结、后端权威错误 |
| AI 生成商品文案 | [AI 运营闭环](../03-backend/04-ai-ops-closed-loop.md) | JSON schema、归一化、人审、fallback |
| 凌晨事故排查 | [可观测性与运维](../06-observability/00-ops-observability.md) | metrics -> dashboard -> monitor API -> flight recorder |
| 评委质疑性能数字 | [证据映射](../07-performance-and-evidence/00-evidence-map.md) | 证据分级、S1-S5、不能只看 HTTP 200 |
| 恶意/错误输入 | [风险矩阵](../08-tests-and-risk/00-risk-and-abuse-matrix.md) | 幂等冲突、低价、错网格、ACL、支付双击、AI 编造 |

## 讲闭环的固定模板

1. 输入是什么：HTTP body、header、WS query、AI request。
2. 第一层守卫：鉴权、ACL、admission、schema、状态。
3. 权威决策在哪里：Redis Lua、PG transaction、WS recovery、AI normalization。
4. 写了哪些状态：Redis key、Kafka topic、PG table、outbox、frontend state。
5. 异常分支：超时、重复、状态缺失、权限失败、慢消费者。
6. 怎么证明：测试名、门禁名、监控面板、flight recorder。
7. 扩展怎么办：分片、HA、重跑证据、补 parity test。
