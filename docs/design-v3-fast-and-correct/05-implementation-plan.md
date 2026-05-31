# 05 — 可落地实施计划：删什么、改什么、怎么验

> 原则：**一个连贯架构，不设 mode、不新旧并行、不缝补丁。** 分阶段是为了可验证地推进，不是为了并存两套。
> 下面每条都指到当前代码的具体符号（`backend/internal/redisengine/engine.go` 等）。

---

## 阶段 0 — 冻结与基线（0.5 天）

- 在 Linux 压测环境固定 PTS-1B 基线，确认 HA5YX7ZG 现象可复现（全 `202` / 300 落库 / 引擎 PAUSED）。
- 部署/确认 **invariant verifier** 能对「期望 1000 笔」而非 300 子集判定（调研 §5 纪律：先校验器，再动架构）。

## 阶段 1 — 把决策日志写进 Lua，热路径就此结束（核心）

**改 `ledgerRunner`（`engine.go:55-299`）**：在 `store_decision` 内，除现有
`HSET pending_key`（保留作热态/重放）外，**原子追加决策到 Redis Stream**：

```lua
-- 在 store_decision(result) 内，cjson.encode 后：
redis.call('XADD', KEYS[5], '*',                  -- KEYS[5] = bid:log:{auction}
  'engine_seq', tostring(result['engine_seq']),
  'payload', encoded)
-- pending_key 仍写，用于 idempotency 重放与「未中继」可见性
```

**改 `PlaceBid`（`engine.go:411-577`）**：Lua 返回 `OK` 后，**直接**：

```go
// 删除对 appendDecisionBeforeReturn 的调用，替换为：
return result.response(auction.DurabilityStatusEngineDurable, auction.DecisionStatusDecided), nil
```

**删除**：`appendDecisionBeforeReturn`（`:579-731`）、`waitForPendingAppendTurn`（`:733-763`）、
`pendingAppendLock*` 一族（SetNX 单锁、`waitForPendingAppendTurn` 的所有分支）。
热路径不再有任何同步 Kafka / append 锁 / 按序等待。

**新增**响应枚举：`DurabilityStatusEngineDurable`（在 `auction` 包），契约见 `03` 第 2 节。

> 验收：单元/集成测试断言 PlaceBid 在 Lua 成功后**不**触达 Kafka；1000 并发同拍品 100% 返回 DECIDED。

## 阶段 2 — 组提交中继（把 append 移出热路径）

**重写** worker 的 append 路径（现 `ProcessPendingAppends`/`appendPendingDecisions`/`refreshPendingAppendLock` 一族）为
**单写者批量中继**：

```go
// 每拍品分区一个中继 goroutine（分片间并行，分片内单写者保序）
for {
  batch := XREAD(stream=bid:log:{auction}, count<=512, block<=2ms)   // 组提交窗口
  if len(batch)==0 { continue }
  msgs := encodeBatch(batch)                                         // 按 engine_seq 有序
  err := kafka.ProduceBatch(topic=bids.decided, key=auction_id, acks=all, idempotent=true, msgs)
  if err != nil { handleRelayFailure(); continue }                  // 有界缓冲；超界→pause
  XACK/记录 high-water(batch)
  markIdemKafkaAcked(batch)                                          // idem_key.kafka_append_status=ACKED
  wsPushConfirmed(batch.engine_seqs)                                 // 可选：翻牌"已确认"
}
```

**幂等 producer**：`enable.idempotence=true`，按 `auction_id` 分区，保单拍品分区内顺序。
**有界缓冲**：Stream 未中继长度设阈值；超阈值或临近收盘 → `pause` 该拍品（fail-closed），不无限静默积压。

> 验收：1000 笔决策在 ~10–30ms 内全部 `KAFKA_ACKED`；中继无逐笔锁；杀掉中继再起，从 Stream high-water 续传不重不漏。

## 阶段 3 — 结算改为按 engine_seq 幂等投影，删除回写 Redis

**删除** `Worker.refreshRedisSettledState`（`engine.go:2408-2470`）及一切结算→Redis 活状态写回。
Redis 活态**只前进**，只在显式 reconcile/resume 时由日志重建。

**改** `settlePayload`/`settleAccepted`/`settleRejected`/`insertBid`：以 `(auction_id, engine_seq)` 唯一约束做幂等：

```sql
-- migration: 唯一约束
ALTER TABLE bids ADD CONSTRAINT uq_bids_auction_engineseq UNIQUE (auction_id, engine_seq);
-- 插入
INSERT INTO bids(... , auction_id, engine_seq, ...) VALUES (...)
ON CONFLICT (auction_id, engine_seq) DO NOTHING;
```

**收盘 effectively-once**（新增/整合到 settlement coordinator）：

```sql
-- 1) token = INCR fence:auction:{id}   (Redis)
-- 2) 选赢家：最高 amount，engine_seq 最小做 tiebreak 的 ACCEPTED
-- 3) 条件更新：
UPDATE auctions SET status='SOLD', winner_id=$w, final_price=$p, settlement_fence=$token
 WHERE id=$id AND (settlement_fence IS NULL OR settlement_fence < $token);
-- 4) 支付 Idempotency-Key = 'settle:'||$id   (绑拍品不绑尝试)
-- 5) UPDATE settlement_status='PAYMENT_CAPTURED'; 发 outbox auctions.sold
```

> 验收：杀掉结算 worker 重启不产生重复行/重复扣款；收盘恰一行 SOLD、恰一次 capture；
> 注入 Kafka 3× 重投，结算结果与单次一致（redelivery torture）。

## 阶段 4 — Redis 丢失/暂停的重建与恢复协议

**整合** 现有 `resumeRedisEngine`/`rebuildRedisFromCheckpoint`/`checkpointSnapshot`：

- 重建源 = **PG checkpoint 快照 + 自 checkpoint 以来的 Kafka 决策重放**（不重放全量；撮合引擎快照+增量法）。
- 重建后 verifier 校验：engine_seq 无间隙、price/winner 与日志末态一致、pending 覆盖完整、epoch 自增 fence 旧写者。
- 仅当校验通过才 `resume`；期间拍品 `RECONCILING`，出价 fail-closed。
- Redis 配置：AOF `appendfsync everysec` + 同步副本；hash tag `{auction}` 同 slot。

> 验收：注入 Redis flush/重启，拍品自动进 RECONCILING，亚秒级重建并校验通过后 resume；全程无伪造成功、无丢「已确认」。

## 阶段 5 — 响应契约与实时投影

- 网关 `writeBidAdmissionResult` 输出三维状态（`decision_status`/`durability_status`/`settlement_status`）。
- WS：房间级路由隔离；价/赢家/倒计时为服务端权威；`KAFKA_ACKED` 翻牌「已确认」；延长事件毫秒级广播（合并窗口防跑马灯）。
- 断连重连：按 `engine_seq` 拉 snapshot + 增量 diff，无本地真相、无跳变。
- 软延时补绝对天花板 `absolute_end`（`03` §6）。

## 阶段 6 — PTS-1B 压测与证据

- 按 `04` §6 分开度量 `final_decision_latency_ms` 等；`202_ratio`/`timeout_ratio` 趋近 0。
- 全量 1000 笔过 verifier；故障注入矩阵逐项过；按 evidence-policy 归档分级。

---

## 删除/改动清单（一页速查）

| 动作 | 目标符号/位置 | 原因 |
|---|---|---|
| 删 | `appendDecisionBeforeReturn`, `waitForPendingAppendTurn`, `pendingAppendLock*` | 热路径同步按序单锁 append = 重建的瓶颈（`01` §2） |
| 删 | `Worker.refreshRedisSettledState` | 结算回写决策态 = 双写地狱/engine_seq 重用（`01` §3） |
| 改 | `ledgerRunner` Lua | 原子追加 `XADD bid:log:{auction}`（决策+WAL 同步） |
| 改 | `PlaceBid` 尾部 | Lua 成功即返回 `DECIDED`/`ENGINE_DURABLE` |
| 重写 | worker append 路径 | 单写者**组提交**批量 Kafka 中继 |
| 改 | `settle*`/`insertBid` | `(auction_id,engine_seq)` 唯一约束幂等 + 收盘 fencing/条件更新/支付幂等键 |
| 改 | 响应契约/网关/WS | 三维状态；`202` 退为极少数 durability-unknown |
| 加 | 软延时 `absolute_end` 天花板 | 防无限延长狙击 |
| 加 | migration: bids 唯一约束、auctions.settlement_fence | effectively-once |

---

## 风险与回滚

- 每阶段独立可验证、可回滚（git revert 单提交）；阶段 1+2 必须一起上（否则决策落 Stream 但无人中继）。
- 中继/结算是无状态消费者，回滚只需停消费者、保留 Kafka 与 Stream（日志不丢，可重放）。
- 决策日志是真源 → 任意投影出错都可从日志重建，**回滚不丢正确性**。
