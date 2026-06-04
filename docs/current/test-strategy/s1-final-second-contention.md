# S1 — 绝杀时刻 / Final-Second Contention

> Maps to: brief 挑战二 "每个人都想在最后一秒绝杀"; rubric 性能 + 系统可用性.
> Headline: **M1 bid decision p99 ≤ 50 ms** + winner correct + every reject justified.
> Tool: **PTS JMeter** (signature run). Asset aliases: `L1-C1` (burst, validated) + `L1-C0` (ladder, control).
> Source-of-truth script: `tests/pts/L1-component/pts-1b-contention-burst-1000vu-1m.jmx`.

## 1. The business moment

The auction is seconds from closing. ~1000 high-intent users on `auc_live` slam
the bid button at almost the same instant, each trying to win. The server must,
in that burst: pick the single correct winner (highest valid amount), give every
loser a fast and *justified* reject, never double-charge, and keep each user's
own decision under 50 ms. This is the hardest correctness-under-load moment in
the whole product, and it is named in the brief.

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
    6. assert response is FINAL (durability_status=ENGINE_DURABLE, result ∈ ENGINE_*), not 202
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
// still bid in a short final-second window, but do not depend on a zero-ms
// load-generator scheduling wall. Diagnostic microburst: set
// contention_release_window_ms=0. Conservative one-second fallback: set 1000.
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
if (r.durability_status != "ENGINE_DURABLE" || !(r.result ==~ /ENGINE_(ACCEPTED|REJECTED|SOLD)/)) {
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
| JMeter property | default `contention_release_window_ms=500` | 1000 bids target a 500 ms final-second window; set `0` only for diagnostic strict microburst; set `1000` for conservative one-second fallback |

Cost: 2×500×1×1.01 ≈ **1 000 VUM ≈ ¥3** (inside the free 5000-VUM tier).

## 5. Metric → chart mapping

| Claim | Chart (straight from PTS) | Backing |
|---|---|---|
| M1 decision p99 ≤ 50 ms | `出价决策 bid-decision` sampler p99, per-second view | the only sampler cited as decision latency |
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

Do not defend S1 with accepted-bid count. In the current `5D92X7QG` artifact,
`7 accepted + 993 rejected = 1000 final decisions`; the 993 rejects are not
missing work. They are the expected result of many users bidding stale amounts
after the Redis sequencer already advanced the price inside the final-second
window.

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
