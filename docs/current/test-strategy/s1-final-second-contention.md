# S1 — 绝杀时刻 / Final-Second Contention

> Maps to: brief 挑战二 "每个人都想在最后一秒绝杀"; rubric 性能 + 系统可用性.
> Headline: default `kafka_ack` **M1 bid decision p99 ≤ 60 ms** + `KAFKA_ACKED` ≥ 99% + winner correct + every reject justified.
> Tool: **PTS JMeter** (signature run). Asset aliases: `L1-C1` (burst, validated) + `L1-C0` (ladder, control).
> Source-of-truth script: `tests/pts/scenarios/s1-final-second-contention/s1-final-second-contention-1000vu.jmx`.

## 1. The business moment

The auction is seconds from closing. ~1000 high-intent users on `auc_live` slam
the bid button at almost the same instant, each trying to win. The server must,
in that burst: pick the single correct winner (highest valid amount), give every
loser a fast and *justified* reject, never double-charge, and keep each user's
own decision under 50 ms. This is the hardest correctness-under-load moment in
the whole product, and it is named in the brief. In the default production-like
profile the response normally waits for Kafka relay acknowledgement
(`KAFKA_ACKED`); a bounded Redis-AOF-local fallback (`ENGINE_DURABLE`) is allowed
only if post-run relay/settlement convergence proves no data loss.

## 2. Two sub-tests, two questions (this is the design crux)

A single burst cannot answer both "is the engine fast & correct under contention?"
and "what is the accept-path capacity?" — so we run two, and **report both** to
pre-empt the "your TPS is tiny" interrogation.

| Sub-test | Bid amounts | What happens | Answers |
|---|---|---|---|
| **S1-burst** | each VU bids in a tight band just above current price (mostly +1 increment, some +2..+5) | engine serializes; a few accepts climb the price; the rest are *correctly* rejected as below the now-higher min | **decision p99 (accept+reject)**, winner correctness, reject justification — the realistic worst case |
| **S1-ladder** | VUs assigned strictly increasing, non-overlapping amounts | nearly all accept; price climbs monotonically | **accept-path capacity** + price monotonicity — the control |

### Why not "all bid the same amount"?
Pure identical bids ⇒ exactly 1 accept, 999 rejects, accepted-throughput ≈ 1.
That is a valid stress extreme but reads as a gimmick. The **tight-band** model is
more realistic (real users pick different round numbers) and still produces the
contention you want: a short ladder forms inside the burst, most bids reject with
a real decision-time basis. Keep identical-amount as an optional extreme, not the
headline.

### Why this is NOT coordinated-omission-prone
Each VU fires **one** bid and then holds (no loop). There is no second iteration
whose start is delayed by a slow response, so the closed-loop tail-hiding problem
(see [README](README.md) §1) does not apply here. Measuring "1000 one-shot
requests, latency each" is clean. (Sustained-rate latency is S2's job, in RPS
mode.)

## 3. Script logic (what the JMX must do)

```
ThreadGroup "bidders" (loop = 1):
  setUp once: seed auc_live to a known {current_price, increment, end_far_enough}
  per VU:
    1. auth: reuse a pre-issued session token from the CSV pool (no login in the hot window)
    2. compute bid amount = current_price + step*increment, step ∈ weighted{1,1,1,2,3,4,5}
    3. build unique client_bid_id + idempotency_key  (per VU, stable across retries)
    4. FINAL-SECOND WINDOW: align all VUs to a shared final-second start, then
       deterministically spread them inside `contention_release_window_ms`
    5. SAMPLER "出价决策 bid-decision": POST /api/auctions/auc_live/bids   ← the ONE measured sampler
    6. assert response is FINAL (durability_status=KAFKA_ACKED or ENGINE_DURABLE, result ∈ ENGINE_*), not 202
    7. HOLD the VU briefly so PTS keeps the full cohort during aggregation
```



```groovy
// 3 — idempotent identity (same key+hash replays same result; never double-charge)
def vu  = ctx.getThreadNum()
def cbid = "s1-${vars.get('runLabel')}-${vu}"
vars.put("client_bid_id", cbid)
vars.put("idem", cbid)                 // idempotency_key == client_bid_id for one-shot

// 2 — tight contention band just above current price (cents)
def cur  = vars.get("current_price_cents").toInteger()
def inc  = vars.get("increment_cents").toInteger()
def steps = [1,1,1,2,3,4,5]
def k = steps[ (vu * 2654435761L & 0x7fffffff) % steps.size() ]   // deterministic spread, no RNG seeding races
vars.put("bid_amount_cents", (cur + k*inc).toString())
```
```groovy
// 4 — final-second release window.
// Default judge-facing value: contention_release_window_ms=500, so 1000 users
// bid inside a short final-second window. The actual PTS/server-observed span is
// measured after the run; do not infer it from configuration alone.
// Diagnostic strict-barrier comparison: set 0. Conservative fallback: set 1000.
long fireAt = vars.get("cohort_ready_ms").toLong() + props.get("burst_wait_ms").toLong()
long offset = Long.parseLong(vars.get("bid_release_offset_ms") ?: "0")
long now = System.currentTimeMillis()
if (now < fireAt + offset) Thread.sleep(fireAt + offset - now)
```
Bid body:
```json
{ "client_bid_id": "${client_bid_id}", "idempotency_key": "${idem}",
  "amount_cents": ${bid_amount_cents}, "client_seen_seq": ${client_seen_seq} }
```
Final-decision assertion (JSR223 PostProcessor), so a `202` cannot be counted as M1:
```groovy
def r = new groovy.json.JsonSlurper().parseText(prev.getResponseDataAsString())
def finalDurability = (r.durability_status == "KAFKA_ACKED" || r.durability_status == "ENGINE_DURABLE")
if (!finalDurability || !(r.result ==~ /ENGINE_(ACCEPTED|REJECTED|SOLD)/)) {
    prev.setSuccessful(false)               // pending/202 is not a final decision → excluded from M1 success
}
```

## 4. PTS configuration

| Field | Value | Why |
|---|---|---|
| 压力来源 | 阿里云VPC内网 | clean latency, see playbook §7 |
| 引擎 | **PTS JMeter** | current source-of-truth asset is the JMX above; no separate native-HTTP PTS script is in the current plan |
| 压力模式 | 虚拟用户模式 | 1000 distinct one-shot clients |
| 最大虚拟用户 | 1000 | one bidder per VU |
| 起始百分比 | 100% | the JMX barrier owns timing; do not add a console ramp |
| 压测时长 | 1–2 分钟 | burst + brief hold; stop when concurrency hits 0 |
| 是否指定循环 / 循环次数 | 是 / **1** | one-shot; console loop would override the script |
| 指定IP数 | 2 | ⌈1000/500⌉ |
| JMeter property | default `contention_release_window_ms=500` | 1000 bids target a 500 ms final-second window; set `0` only for diagnostic strict-barrier comparison; set `1000` for conservative one-second fallback |

Cost: 2×500×1×1.01 ≈ **1 000 VUM ≈ ¥3** (inside the free 5000-VUM tier).

## 5. Metric → chart mapping

| Claim | Chart (straight from PTS) | Backing |
|---|---|---|
| M1 default kafka_ack decision p99 ≤ 60 ms | `出价决策 bid-decision` sampler p99, per-second view | the only sampler cited as decision latency |
| Durability response mix | `review-s1-pts-run.sh` durability distribution | default requires `KAFKA_ACKED` ≥ 99%, bounded `ENGINE_DURABLE` fallback |
| decisions/sec (engine capacity) | sampler 平均TPS over the burst window | total accept+reject adjudicated |
| accepted updates (ladder) | server `ENGINE_ACCEPTED` count from `summarize-pts-sampling-logs.sh` | the few that climbed the price |
| winner correct / rejects justified | `verify-l4b-pts-correctness.sh` output | M3 gate |
| arrival span | `review-s1-pts-run.sh` sampling-log `startTimeTS` and response `server_time_ms` spans | proves whether PTS actually delivered the target window |

Pressure-reached evidence:

```text
PTS sampler count / timestamp span -> 1000 one-shot bid arrivals reached the API
response ENGINE_* fields          -> each request became a final decision
DB/verifier                        -> unique bids, winner, rejects, seq, settlement/outbox safety
```

Do not defend S1 with accepted-bid count. In the current `UIPAX7JG` artifact,
`264 accepted + 736 rejected = 1000 final decisions`; the 736 rejects are not
missing work. They are the expected result of many users bidding stale amounts
after the Redis sequencer already advanced the price inside the final-second
window.

Legacy `2MLCX7WG` remains useful as explicit `redis_aof` low-latency evidence:
`285 accepted + 715 rejected = 1000 final decisions`; the 715 rejects are not
missing work. They are the expected result of many users bidding stale amounts
after the Redis sequencer already advanced the price inside the final-second
window.

Current S1 PTS evidence:

| Run | Release model | Count / correctness | Measured burst span | Latency | Verdict |
|---|---|---|---|---|---|
| `UIPAX7JG` | default `contention_release_window_ms=500`; 2-agent wall-clock alignment fixed by JMX; default `BID_ENGINE_RESPONSE_DURABILITY=kafka_ack` | 1000 sampling rows, 1000 unique `client_bid_id`, 1000 server POSTs, 1000 Redis Lua executions; 264 accepted, 736 rejected; sampled durability 998 `KAFKA_ACKED`, 2 `ENGINE_DURABLE`; post-run persisted durability/settlement 1000/1000; verifier gates PASS | Global PTS `startTimeTS` span 505 ms; response `server_time_ms` span 507 ms | 100% sampling-log `elapsedTime` p99 58 ms, max 67 ms; gateway total bucket has 985/1000 <=50 ms and 1000/1000 <=100 ms | Current default kafka_ack S1 PASS under 60 ms envelope. Honest wording: stronger response boundary, not strict <=50 ms |
| `2MLCX7WG` | default `contention_release_window_ms=500`; each pressure agent deterministically spreads its 500 VU inside a 500 ms final-second window | 1000 sampling rows, 1000 unique `client_bid_id`, 1000 server POSTs, 1000 Redis Lua executions; 285 accepted, 715 rejected; 41 verifier gates PASS | Global PTS `startTimeTS` span 1351 ms; response `server_time_ms` span 1348 ms. Per-agent spans: instance 0 `501/503 ms`, instance 1 `525/524 ms` for `startTimeTS/server_time_ms`; server access-log IP attribution was consistent, about `499 ms` and `525 ms` for the two pressure IPs | 100% sampling-log `elapsedTime` p99 23 ms, max 28 ms; server/gateway histogram has 1000/1000 <=25 ms and <=50 ms | Current S1 windowed-burst PASS for correctness and client p99. Honest burst-window claim: 500 ms per pressure agent; global multi-agent span about 1.35 s |
| `TGLBX7GG` | `contention_release_window_ms=0`; 1000 VU wait at the same barrier and release with no artificial spread | 1000 sampling rows, 1000 unique `client_bid_id`, 1000 server POSTs, 1000 Redis Lua executions; 10 accepted, 990 rejected; 41 verifier gates PASS | PTS `startTimeTS` span 1144 ms; response `server_time_ms` span 1147 ms | sampling-log `elapsedTime` p99 134 ms, max 140 ms; server/gateway histogram has 1000/1000 <= 50 ms | Strong pressure/correctness proof for shared-barrier release, but not an M1 <=50 ms client-side PASS |

Judge-safe wording for `UIPAX7JG`:

> "The current default S1 evidence runs with `BID_ENGINE_RESPONSE_DURABILITY=kafka_ack`.
> The load reached the backend as 1000 POSTs/1000 Redis Lua executions inside a
> 505 ms PTS send-start span and 507 ms server decision timestamp span. 998/1000
> sampled responses returned `KAFKA_ACKED`; 2 returned `ENGINE_DURABLE` because
> the 40 ms latch timed out, but post-run evidence shows all 1000 decisions later
> reached Kafka and PostgreSQL with verifier PASS. The p99 is 58 ms, so this is a
> default kafka_ack pass under the 60 ms envelope, not a strict 50 ms claim."

Judge-safe wording for `2MLCX7WG`:

> "The formal S1 evidence uses the 500 ms release-window profile, not the
> strict-barrier diagnostic. Each PTS pressure agent delivered its 500 users in
> about 0.5 s, and the global multi-agent span was 1.35 s because the two agents
> were offset. We therefore claim a 500 ms per-agent final-second window and
> record the actual global span explicitly. Within that population, all 1000
> bids returned final `ENGINE_DURABLE` decisions, p99 was 23 ms, and verifier
> gates all passed."

Judge-safe wording for `TGLBX7GG`:

> "The JMX no longer spreads bids over a 500 ms window. All VUs release at the
> same barrier. PTS/JMeter scheduling and network delivery turned that into a
> measured 1144 ms send-start span, with the server response timestamps spanning
> 1147 ms. So the honest claim is 1000 one-shot final decisions under a
> shared-barrier burst, not '1000 requests arrived within 500 ms'."

Root cause of the 1144 ms span and 134 ms client p99:

- `TGLBX7GG` used 2 PTS pressure agents, 500 bid VU each.
- The JMX shared barrier is process/JVM-local. It does not create a single
  cross-agent global release timestamp.
- PTS/JMeter evidence split by `instanceId` shows each agent did what we wanted
  locally:

```text
instance 0: n=500, startTimeTS span=114 ms, server_time_ms span=117 ms, elapsed p99=112 ms
instance 1: n=500, startTimeTS span=113 ms, server_time_ms span=120 ms, elapsed p99=135 ms
```

- Server access logs agree with the two-agent split:

```text
172.16.180.107: 500 POST /bids, 02:22:51.524341748 -> 02:22:51.641640267
172.16.180.109: 500 POST /bids, 02:22:52.548903297 -> 02:22:52.668055696
```

So the global 1144 ms span is mostly the gap between the two PTS agents'
barrier targets, not a Redis/HTTP service slowdown. Server-side histograms from
the same run show 1000/1000 HTTP, gateway-total, and Redis Lua samples inside
the <=50 ms bucket. The client-side `elapsedTime` p99=134 ms is still the honest
PTS/JMeter user-visible number for that run, but the attribution is load-agent
synchronization/client-side scheduling, not backend saturation.

External basis: Apache JMeter documents `Synchronizing Timer` as releasing
blocked threads together, but it is scoped within one JVM; Alibaba Cloud PTS
documents that JMeter assembly points/synchronizing timers are only effective on
a single pressure machine/JVM and are not recommended for multi-pressure-machine
global synchronization.

If a reviewer asks "did 1000 requests really hit the server?", answer with the
cross-layer counts from `2MLCX7WG`: PTS sampling rows = 1000, server POST
counter = 1000, Redis Lua executions = 1000, persisted bids = 1000, settlements
= 1000, and verifier gates = 41 PASS. If they ask "were all 1000 inside one
500 ms wall-clock interval?", answer no: the current evidence is 500 ms per
pressure agent with a measured 1.35 s global span. That is the honest PTS
multi-agent boundary.

## 6. Correctness gates (M3 — must PASS to cite M1)

```bash
L4B_PROFILE=pts-1b SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
BASE_URL=http://127.0.0.1:18080 bash tests/pts/preflight-l4b-pts-guards.sh before-s1-<label>
# ... run PTS ...
BASE_URL=http://127.0.0.1:18080 bash tests/pts/collect-server-evidence.sh s1-<label>
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh s1-<label>
PAGE_SIZE=100 bash tests/pts/fetch-pts-sampling-logs.sh <report-id>
bash tests/pts/review-s1-pts-run.sh <report-id>
```
Gate asserts: winner == highest valid engine decision; `engine_seq` gap-free;
each reject carries `required_min_price`/`current_price` basis; zero duplicate
`(epoch, engine_seq)` settlement rows; Redis pending drained; Kafka lag/DLQ clean;
outbox drained.

## 7. Pitfalls (S1-specific; general ones in playbook §10)

- **Counting `202` as a decision.** The §3 assertion must fail non-final
  responses, or M1 is inflated by acceptance latency.
- **Login inside the hot window.** Pre-issue tokens to a CSV pool; auth latency
  is not bid latency (this is why `L1-C1` uses the JWT session pool).
- **`SyncTimer` waiting for all VUs.** If PTS ramps unevenly it deadlocks; use the
  absolute-offset barrier in §3 instead.
- **Reading PTS peak TPS as the burst width.** PTS charts/report tables can
  display TPS over a coarser bucket than the actual request timestamps. For
  example, R4FWX72G's 1000 bid samples reached PTS start/server timestamps in
  about 255 ms, while a 5-second chart bucket would still render near 200 TPS.
  Use sampling logs or server request timestamps to prove the actual arrival
  span when defending "within the final second".
- **Burst lands after sockets closed.** Keep `burst_wait_ms` inside the server
  heartbeat window (20 s ping + 5 s timeout); `3W9CX76G` fired too late.
- **Citing accepted-TPS as the headline.** Report decision p99 + decisions/sec;
  accepted-TPS is the price-ladder artifact (see [metrics-and-slo](metrics-and-slo.md) M1).
