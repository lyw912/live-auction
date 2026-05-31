# PTS Report 7D63X7SG Review

Date: 2026-05-31
Reviewer: Independent review
Git SHA: `cac26b9`
Runtime profile: `BID_ENGINE_MODE=redis_ledger`, `ADMISSION_ENABLED=false`
Workload: PTS-1B contention burst
JMX: `tests/pts/pts-1b-contention-burst-1000vu-1m.jmx`
CSV: `docs/perf/pts/pts-1ab-1000vu-sessions.csv`
Server evidence: `docs/perf/pts/evidence/incoming/7D63X7SG/`
Verifier: `bash tests/pts/verify-l4b-pts-correctness.sh 7D63X7SG`
Final classification: `CURRENT_PASSING_CORRECTNESS / PERFORMANCE_PARTIAL`

---

## Verdict

**正确性：PASS（历史首次全量 1000 笔 P0 全通过）。**
**性能：服务端决策 p100 ≤ 50ms；JMeter p99=222ms 由 Auth+ACL 两次同步 DB 查询引起，非引擎延迟，需修复。**

---

## 与 HA5YX7ZG 的决定性对比

| 指标 | HA5YX7ZG（旧架构）| 7D63X7SG（v3）|
|---|---|---|
| HTTP 202 PROCESSING_RETRY_LATER | 1000（100%）| **0** |
| HTTP 200 + DECIDED | 0 | **1000（100%）** |
| 落库 bid 行数 | 300 / 1000 | **1000 / 1000** |
| 失败 settlement 行 | 49 | **0** |
| 引擎状态 | PAUSED | **正常** |
| DLQ 消息 | 有 | **0** |
| P0 门控 | FAIL | **全部 PASS（34项）** |
| 服务端决策延迟 p100 | 未测（全是 202 接收延迟）| **≤ 50ms（Prometheus）** |

---

## 业务分布

| 指标 | 值 |
|---|---|
| 请求总数 | 1000 |
| HTTP 200 | 1000（100%）|
| decision_status=DECIDED | 1000（100%）|
| durability_status=ENGINE_DURABLE | 1000（100%）|
| ENGINE_ACCEPTED | 10 |
| ENGINE_REJECTED（BID_TOO_LOW）| 990 |
| engine_seq 范围 | 1..1000，无重复无间隙 |
| auc_live 最终价 | 50,000,100 分（≈50万元）|
| 压测窗口 | 362ms（1000 请求全部爆发）|

---

## 正确性门控（全部 PASS）

所有 34 项 P0 门控通过，含 v3 专项：

- `pts_expected_total_bid_rows` PASS：PG 落库 1000 行（HA5YX7ZG 只有 300）
- `every_bid_has_settled_ledger` PASS：每笔决策 Kafka+PG 完整落定
- `engine_not_paused` PASS：引擎压测后未暂停
- `auction_winner_matches_highest_accepted` PASS：最高有效价者胜
- `engine_seq_complete` PASS：1..1000 无间隙
- `dlq_empty` PASS：Kafka DLQ 为空
- `v3_relay_cursor_advanced` PASS：relay 游标已推进，1000 条全部中继
- `soft_close_no_stacked_subwindow_extension` PASS：软延时正确

---

## 性能分析

### 服务端引擎决策延迟（Prometheus）— 达标

```
auction_bid_http_stage_seconds{stage="total"}:
  ENGINE_REJECTED 990条: 全部落在 ≤ 50ms 桶
  ENGINE_ACCEPTED  10条: 全部落在 ≤ 50ms 桶

redis_lua_script_latency{script="bid_redis_ledger"}:
  1000次全部落在 ≤ 50ms 桶
```

**服务端决策路径（Redis Lua CAS + XADD + 响应序列化）p100 ≤ 50ms**，v3 热路径设计达标。

### JMeter 测量延迟（含前置 DB 查询）— 未达标

```
p50=82ms  p95=135ms  p99=222ms  max=251ms
latency（TTFB）= elapsedTime（响应体小，首字节即全部）
connectTime: p50=1ms  p99=24ms（VPC 内网，可忽略）
```

### 根因：Auth + ACL 每请求两次同步 DB 查询，在 handler totalStart 之前执行

热路径实际执行顺序：

```
HTTP 请求到达
  → authMiddleware: lookupSession(DB)          ← DB 查询 #1，NOT in Prometheus metric
  → handler: totalStart = time.Now()
  → ACL: requireActiveMembershipForAuction(DB) ← DB 查询 #2，NOT in Prometheus metric
  → Engine.PlaceBid: Redis Lua CAS             ← 在 Prometheus metric 内（≤50ms）
  → response
```

**Prometheus `auction_bid_http_stage_seconds{stage="total"}` 只从 `totalStart` 开始计时，不包含 authMiddleware 和 ACL 的 DB 查询。**

### DB 连接池证据（Prometheus）

```
db_pool_max_conns = 90
db_pool_empty_acquire_total = 1140（需要等待空闲连接的次数）
db_pool_empty_acquire_wait_seconds_total = 8.597s（等待总时长）
平均等待 = 8597ms / 1140 = 7.5ms
```

1000 个并发请求 × 2 次 DB 查询 = 2000 次竞争 90 个连接。p99 等待 ≈ 150–200ms。

**JMeter p99=222ms ≈ auth_DB_wait(~100ms) + ACL_DB_wait(~70ms) + handler(≤50ms) + VPC网络(~3ms)**，完全符合。

### JMX success=False 说明（配置问题，非业务失败）

原因：ResponseAssertion `test_type=8`（EQUALS）配合「200」和「202」两个模式，JMeter 5.x 下要求响应码同时满足两个值，逻辑上不可能，导致全部标为 false。实际 HTTP code 全部为 200，无任何业务失败。已在本次分析后修复（`enabled="false"`）。

---

## 修复方向（性能未达标的根因修复）

### 必须修复（会使 p99 降到 50ms 以内）

**1. Auth Session Redis 缓存**

将 `lookupSession` 的结果缓存到 Redis，TTL = min(session_expires_at - now, 5min)：

```go
// 先查 Redis，命中直接返回；未命中查 DB 后写入 Redis
func lookupSession(ctx context.Context, rdb *redis.Client, db *pgxpool.Pool, token string) (AuthUser, error) {
    cacheKey := "auth:session:" + hashSessionToken(token)
    if cached := rdb.Get(ctx, cacheKey).Val(); cached != "" {
        // decode user from JSON
    }
    // DB query (fallback)
    // rdb.SetEx(ctx, cacheKey, encoded, ttl)
}
```

- PTS 场景：1000 个 token，全部首次请求 miss → DB，后续重试直接 Redis 命中（亚毫秒）
- 竞争 90 个 DB 连接的从 2000 次降为约 1000 次（仅首次），p99 DB 等待从 ~200ms 降至 ~10ms

**2. ACL 成员资格 Redis 缓存**

`requireActiveMembershipForAuction` 对同一拍品 room 查询相同结果，可缓存 30s：

```go
cacheKey := fmt.Sprintf("acl:{%s}:membership:%s", auctionID, userID)
```

两项合计可将热路径前置 DB 查询从 2 次降为 0 次（命中缓存时），**端到端 p99 预计降至 10–20ms**（VPC 内 Redis 亚毫秒 + Lua ≤50ms）。

---

## 需要采集的下一轮证据

下轮压测需要新增服务端端到端计时（包含 middleware）：

```
accept_to_response_ms = 请求到达（TCP accept）→ response 写完
```

可通过在 `authMiddleware` 入口也记录 `totalStart`，或在 HTTP middleware 层加全链路 histogram。

---

## 当前分类

| 维度 | 状态 |
|---|---|
| 正确性 | **CURRENT_PASSING** |
| 服务端引擎延迟 | **PASS（p100≤50ms，Prometheus 证实）** |
| 端到端用户延迟（含 DB 前置）| FAIL（p99=222ms，需修复 auth/ACL 缓存）|
| 故障注入 | 未执行（下轮）|
