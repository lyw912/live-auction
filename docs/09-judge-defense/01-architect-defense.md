# 架构师拷问

父文档：[答辩索引](00-defense-index.md)

## Q1：为什么热路径不是 PostgreSQL 行锁？

30 秒：同一拍品最后一秒的 1000 个出价都竞争同一 `auctions` 行，`SELECT FOR UPDATE`/UPDATE 会让冲突事务等待锁释放，p99 被排队放大。Redis Lua 在单线程内原子执行完整规则，没有应用层重试风暴和热点行锁。

3 分钟：项目最早保留了 PG legacy 路径用于测试和对照，但默认配置 `redis_ledger` 指向 Redis engine。PG 仍是结算/订单/审计真相，只是不在热决策链路上。Redis Lua 写决策日志后，Kafka/PG 异步收敛。

追问：乐观锁行不行？高争用下失败重试会放大尾延迟，而且每次重试都要重新读当前价；Redis Lua 直接串行化得到一个全序结果。

## Q2：Kafka 是不是过度设计？

30 秒：不是。Kafka 是决策 WAL、结算重放源和故障证据。Redis 是热态，不是最终真相；没有有序日志，Redis 丢失后无法证明哪些决策该恢复。

3 分钟：响应边界分 `ENGINE_DURABLE` 和 `KAFKA_ACKED`。Kafka 故障时不假装 ACK，而是降级；最终必须用 settlement/outbox/verifier 证明收敛。

## Q3：单写者是瓶颈吗？

30 秒：单拍品天然需要一个总序，否则最高价和延时规则会分叉。单写者是正确性要求，不只是性能折中。

扩展：多拍品按 `auctionID` 分片，Redis Cluster hash tag + Kafka partition + WS room route；单拍品极限则优化 Lua、减少 RTT、批量 relay。

## Q4：为什么不用 Redlock/etcd？

30 秒：分布式锁解决多进程互斥，不解决高频价格决策。每次锁协调增加 RTT，且仍要在锁内读写状态。Redis Lua 已经给出原子性和全序。
