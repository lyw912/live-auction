# 测试与 SRE 拷问

父文档：[答辩索引](00-defense-index.md)

## Q1：怎么证明系统是对的？

30 秒：不看 HTTP 200，而看 PG 真相和 Redis/Kafka/outbox 交叉门禁：赢家等于最高 accepted、engine_seq 1..N 无空洞、每个 reject 有 decision_basis、Kafka lag=0、outbox drained、DLQ empty。

3 分钟：S1-S5 每个性能场景都要配 correctness verifier。P4 risk simulator 用真实 API 复现幂等重放、ACL、支付双击。

## Q2：Redis/Kafka/PG 故障怎么测？

回答：S4 fault 通过 SIGKILL/FLUSHALL/Toxiproxy 注入，预期不是所有请求成功，而是“故障期间不产生假成功，恢复后 settlement/outbox 收敛”。Redis 丢失是 `RECONCILING`，Kafka 故障是 `ENGINE_DURABLE` 降级，PG 故障下热决策可以继续但 settlement 延后。

## Q3：RTO/RPO 是多少？

诚实回答：当前更强的是机制和 pass/fail 证据，标准化 RTO/RPO 数字还应由故障脚本统一记录。不能把历史描述包装成生产 SLA。生产化要把故障注入脚本输出恢复时间、丢失窗口、待结算收敛时间。

## Q4：凌晨 3 点怎么定位？

顺序：

1. `/readyz` 看依赖。
2. Grafana 看 bid latency、Kafka lag、outbox lag、WS reconnect、slow consumer。
3. `/api/monitor/redis-engine` 看 engine paused/reconciling。
4. flight recorder 查单个拍品从 bid decision 到 order/outbox。
5. 必要时 retry dead outbox 或保持 pause 直到 checkpoint/rebuild 通过。
