# S2 — 正常竞价 / Steady Auction & Soak

> Maps to: brief 挑战二 "毫秒级实时同步" + 规则(加价/自动延时); rubric 性能 + 稳定性.
> Headline: **steady decision p99 ≤ 100 ms** at a sustained offered rate + **M4 no leak**.
> Tool: **independent-ECS k6 soak** for stability + **optional PTS RPS S2 chart** when budget permits.
> Source assets: `tests/load/s2-steady-soak.js` and `tests/pts/L2-protocol/pts-2p4-steady-interactive-auction.jmx`.
> Expanded split: `S2-long-soak`, `S2-convergence-drain`,
> `S2-capacity-stair`, and `S2-read-interference` are governed by
> [s2-s3-expanded-test-design.md](s2-s3-expanded-test-design.md).

## 1. The business moment

Not the final-second crush — the *normal* minutes of a live auction. A few
hundred to a few thousand people watch; a **minority actively bid**; the price
climbs the increment ladder step by step; auto-extension fires when someone bids
near the end. This must stay smooth for 10+ minutes with no latency drift and no
resource leak. It is what proves the system is *stable*, not just *fast once*.

## 2. The realistic bid model (and what to reject)

> **Reject "80% of VUs bid every 2 s on the one auction."** On a single ascending
> auction that is ~hundreds–thousands of writes/sec onto one key, ~all rejected —
> a self-inflicted hot-key write storm, not realistic bidding. (Research: this is
> an adversarial hotspot; it also self-throttles under stress = coordinated
> omission.) That belongs in S1's burst, clearly labeled, not here.

Defensible steady model:

```
viewers connected        : 2000–5000 (held; mostly read-only, drive fanout context)
active bidder sessions   : 5–15% of viewers
offered bid arrivals     : OPEN model, ramp 20/s → 60/s → 100/s   (intended raises, not re-bids)
bid amounts              : escalate over time so a HEALTHY fraction accept and climb the ladder
noise                    : include rejects (stale client_seen_seq), self-leading attempts
accepted update rate     : measure it (≈ how fast the price legitimately climbs, e.g. 1–10/s)
duration                 : 10 min (PTS chart) / 30–60 min (local soak)
```

The fanout pressure is driven by **accepted updates × subscribers**, not by
rejected attempts. Report offered bids *and* accepted updates separately.

## 3. Two runs, two tools (on purpose)

### 3a. Local k6 soak (30–60 min, the leak gate — 0 VUM)
k6 `ramping-arrival-rate` is the cleanest current open-model asset and costs
nothing. Run it against the server; scrape server metrics via Prometheus →
Grafana. This is the required S2 run.

```javascript
import http from 'k6/http'; import { check } from 'k6';
export const options = {
  scenarios: {
    steady_bids: {
      executor: 'ramping-arrival-rate',          // OPEN model
      startRate: 20, timeUnit: '1s',
      preAllocatedVUs: 50, maxVUs: 200,
      stages: [
        { target: 20,  duration: '10m' },
        { target: 60,  duration: '10m' },
        { target: 100, duration: '10m' },
      ],
    },
  },
  thresholds: { 'http_req_duration{sampler:bid-decision}': ['p(99)<100'], dropped_iterations: ['count<200'] },
};
export default function () {
  const amt = ladderBase + Math.floor((Date.now()-t0)/climbMs)*increment;
  const res = http.post(`${BASE}/api/auctions/auc_live/bids`,
    JSON.stringify({ client_bid_id:`s2-${__VU}-${__ITER}`, idempotency_key:`s2-${__VU}-${__ITER}`,
                     amount_cents: amt }), { headers: authHdr(__VU), tags:{ sampler:'bid-decision' } });
  check(res, { 'final decision': r => { const j=r.json(); return j.durability_status==='ENGINE_DURABLE'
                 && /ENGINE_(ACCEPTED|REJECTED|SOLD)/.test(j.result); } });
}
```
> **Report `dropped_iterations`** alongside p99 — it is the explicit overload
> signal that closed-loop tests hide. Zero (or near-zero) dropped iterations means
> the offered rate was actually delivered.

Current independent-ECS result:

- `s2-ecs-30m-20260604T095720`: 85,499 bid attempts and 85,499 final
  `ENGINE_*` decisions over the 20/s -> 60/s -> 100/s 30-minute shape;
  k6 exit 0, dropped iterations 0, HTTP failures 0, auth/ACL failures 0,
  admission contamination 0, non-decision failures 0; HTTP p99 3.30ms and
  custom S2 decision p99 4ms.
- Server convergence and correctness passed after service-side evidence
  collection: 61 `ENGINE_ACCEPTED`, 85,438 `ENGINE_REJECTED`, all 85,499
  settlements terminal, Kafka consumer lag 0, Redis pending decisions 0, DLQ
  empty, outbox drained, engine seq complete, and all verifier P0/P1 gates PASS.
- Evidence path:
  `docs/perf/pts/evidence/incoming/s2-ecs-30m-20260604T095720/`.

Interpretation boundary: this is a bid-decision endurance and convergence pass.
It is not accepted-heavy fanout evidence and it is not read-interference
evidence. The rejected decisions still exercise Redis decision logging, Kafka
ledger relay, PostgreSQL settlement, and verifier gates, but only 61 accepted
updates drove outbox/WebSocket fanout. Use `S2-read-interference` for HTTP read
pressure and S3 for WebSocket fanout.

### 3a-bis. S2 read interference (HTTP polling under bid load)

The live-room polling question is separate from long bid-decision soak. Use
`tests/load/s2-read-interference.js` from an independent k6 ECS:

```
bid attempts: 100/s
HTTP reads  : 2000/s -> 5000/s -> 10000/s
mix         : 80% GET auction snapshot, 15% leaderboard, 5% my bid history
duration    : 15 min default (5 min per stage)
```

Evidence required: bid p99 under read load, read p99 by route, dropped
iterations, k6 host health, DB pool wait/connection counts, Redis/Kafka/PG/outbox
convergence, and the same correctness verifier gates.

Current measured status on 2026-06-04:

```text
label       : s2-read-ecs-15m-20260604T113330
verdict     : CURRENT_FAILING / bottleneck evidence, not a clean 10k-read pass
bid result  : ~98.4 decisions/s, p99 5.68ms, HTTP failures 0
read result : ~2142 successes/s actual, snapshot p99 1.60s,
              leaderboard p99 4.07s, my-bids p99 884.8ms
k6 signal   : 2,057,742 dropped iterations; READ_MAX_VUS=4000 filled
service     : DB pool max/total 90; empty-pool acquires 3,257,372;
              empty-pool wait total 2,281,594s
correctness : immediate convergence gate failed; late verifier passed after
              Kafka/settlement drain
```

Reduced clean-ceiling attempt:

```text
label       : s2-read-clean-ecs-15m-20260604T120823
shape       : 100 bid/s + 2000/s -> 3000/s -> 4000/s reads
verdict     : CURRENT_FAILING / lower-ceiling bottleneck evidence
bid result  : ~98.4 decisions/s, p99 5.70ms, HTTP failures 0
read result : ~2081 successes/s actual, snapshot p99 1.02s,
              leaderboard p99 2.72s, my-bids p99 596ms
k6 signal   : 524,423 dropped iterations; READ_MAX_VUS=2500 filled around
              a ~2.3k/s target point
service     : DB pool max/total 90; empty-pool acquires 2,996,066;
              empty-pool wait total 1,315,846s
correctness : immediate convergence gate failed; late verifier passed after
              Kafka/settlement drain
```

Use these as read-path ceiling evidence. Do not claim 3000/s, 4000/s, 5000/s, or
10000/s read capacity until a later clean-ceiling run or read-path optimization
proves it. The next display candidate should be 1500/s -> 1800/s -> 2000/s
reads, or 2000/s flat for 15 minutes.

### 3b. Optional PTS chart (10 min, judge export)
Use this only after the k6 soak is clean and when you need a polished PTS PDF.
The current executable asset is `pts-2p4-steady-interactive-auction.jmx`; there
is no separate native-HTTP PTS script in the current plan. Treat the JMX as S2's PTS chart, with the script's
pacing and sampler exclusions preserved.

```
JMX: tests/pts/L2-protocol/pts-2p4-steady-interactive-auction.jmx
Scale: 2400 WS + 360 active bidder + 240 reader VU
Duration: 10 min
Sampler: S2 steady bid decision p99, S2 fanout observe final seq
Verifier: tests/pts/verify-l2p4-pts-evidence.sh
```

PTS config: JMeter pressure test, VU mode, max VU 3000, specified IPs 6,
duration 10 min, loop count not specified (`是否指定循环=否`), sampling 1%.

## 4. Auto-extension correctness (a rule the steady run must exercise)

The brief requires 自动延时 (bid near close → extend 10–30 s). The steady run
should drive bids into the extension window and assert: the auction end time
advances, late accepts remain valid, and the *final* winner reflects the extended
window. Add to the M3 verifier a check that `ends_at` moved and no bid accepted
after the true (extended) end. This is correctness, reported as PASS/FAIL.

## 5. Settlement Convergence And Payment Safety

`ENGINE_DURABLE` is the user-visible decision boundary, not the accounting
boundary. During the auction, settlement lag does not invalidate bid decisions:
Redis hot state, `engine_seq`, and fanout carry the live experience. At落槌 and
payment time, however, PostgreSQL settlement must be complete before order/payment
is treated as final truth.

Report S2 convergence as:

```
kafka_settlement_lag_peak
converged_seconds = k6_end -> Kafka lag 0 + Redis pending 0 + PG settlements complete + outbox drained
```

Business interpretation for jewellery auctions:

- During live bidding: bounded lag is acceptable if M1/M2 are healthy and
  verifier later proves no loss.
- At close/payment: do not issue payment links from incomplete PG truth. The
  auction enters a short **SETTLING / confirming final result** state until
  Kafka lag, Redis pending decisions, PG settlements, and outbox backlog reach
  zero.
- If convergence exceeds the business window (for example 60s local / 3-5min
  operational payment buffer), alert and keep payment disabled; recovery may
  replay Redis/Kafka state into PG, but must not guess the winner from stale PG.

Do **not** make "stop accepting bids 30s before close" the default mitigation:
it conflicts with soft-close and final-second bidding. The defensible fallback is
post-close payment gating, not pre-close product degradation.

Current S2 bottleneck evidence (local diagnostic, not a final PTS chart):

- baseline `s2-stair-1000-20260602T184500`: 80,999 `ENGINE_DURABLE` decisions,
  M1 p99 5.24ms, p99.9 20.03ms, dropped 0; 180s convergence failed with about
  30k settlement lag.
- batch-drain `s2-stair-1000-batchdrain-20260602T193000`: same 80,999 decisions,
  M1 p99 5.79ms, p99.9 30.51ms, dropped 0; convergence PASS in 158s.
- set-based rejected settlement + settlement log suppression
  `s2-stair-1000-setbased-logsuppressed-100s-20260602T211311`: 70,999
  decisions, HTTP p99 5.44ms, p99.9 32.21ms, dropped 0; 100s convergence gate
  failed at 102s with Kafka lag 1371 and settlement_total 69774/70999; verifier
  later passed after full drain.
- direct-SETTLED rejected fast-path trial
  `s2-stair-1000-directsettled-100s-20260602T212330`: rejected and reverted; it
  worsened convergence (lag 32033 at 101s) and failed the 100s verifier because
  settlement had not completed.
- 110s terminal rerun after adding runtime sampling and fixing
  `REDIS_ENGINE_SETTLEMENT_WORKERS` env propagation:
  `s2-stair-1000-workers4-110s-20260603T1928`: 70,999 decisions, HTTP p99
  5.56ms, p99.9 34.25ms, dropped 0, HTTP failures 0, verifier later PASS after
  full drain; **110s convergence FAIL** at 112s with Kafka lag 1275 and
  settlement_total 69925/70999.
- 120s product-buffer rerun:
  `s2-stair-1000-120s-20260603T1942`: 70,999 decisions, HTTP p99 5.34ms,
  p99.9 27.68ms, dropped 0, HTTP failures 0, all verifier gates PASS after
  drain. Convergence samples show 286 records still outstanding at 119s and full
  drain at the 122s poll (`kafka_group_lag=0`, `settlement_total=70999`,
  `redis_pending_count=0`, `outbox_unpublished=0`).

Interpretation: the foreground decision path is healthy under the S2 local stair,
but settlement convergence remains the limiting safety signal. The 110s target
for this 2-minute local stair was executed and failed. A 120s product payment
buffer is defensible only with polling tolerance: this run confirmed drain at
122s, not at a strict 120.000s hard boundary. Mark the current state as
foreground M1 PASS, final correctness-after-drain PASS, and
payment/finality convergence **acceptable for a 120s business buffer with
explicit poll-boundary disclosure**, not as full S2 `CURRENT_PASS` because the
short stair still does not prove M4 long-soak no-leak.

The 4-worker rerun is also evidence that simply adding settlement workers is not
a complete fix for one hot auction: the Kafka consumer group showed all
`auc_live` messages on one partition, so only one consumer can process the
auction's ordered settlement chain.

Independent-ECS accepted-heavy capacity stair, 2026-06-04:

- `s2-capacity-accepted-ecs-20260604T150519` used the accepted profile
  `50/s -> 100/s -> 200/s -> 400/s -> 600/s`, `CAPACITY_PROFILE=accepted`,
  `NOISE_PCT=0`, fast amount ladder, independent k6 ECS.
- k6 was clean at the synchronous Redis decision boundary: exit 0,
  `dropped_iterations=0`, `http_req_failed=0`, 131,574 final decisions,
  125,376 accepted, 6,198 rejected, decision p99 3.89ms, p99.9 7.47ms,
  max VUs used 4 with k6 CPU/RSS below saturation.
- Server convergence was **not** clean at collection time: Redis engine log had
  131,574 entries matching k6, but PostgreSQL settlement was only about 61k and
  `settlement-workers` still had about 77,888 Kafka lag on the hot partition.
  Verifier reported `redis_kafka_pg_accepted_match=FAIL` and non-terminal
  settlement before a later PostgreSQL shared-memory error in a heavy verifier
  query.
- Classification: `CURRENT_FAILING` for full S2-capacity correctness,
  `CURRENT_PASS` only for the foreground Redis decision layer under this
  accepted-heavy profile. Do not cite it as "600/s accepted capacity pass".

Root cause and fix direction:

- The single hot auction is keyed to one Kafka partition. Official Kafka
  consumer-group semantics allow only one consumer in a group to read a partition
  at a time, so adding settlement workers cannot parallelize this one auction's
  ordered stream by itself.
- The old accepted settlement path paid one transaction per accepted message:
  settlement attempt, `SELECT auctions FOR UPDATE`, auction update, bid insert,
  auction event, outbox event/delivery, idempotency completion, checkpoint, and
  commit. That is correct, but it made accepted-heavy S2 hit the async
  settlement/outbox knee before the Redis decision knee.
- The retained optimization is accepted contiguous-prefix batching. It batches
  only same-auction, same-epoch, consecutive `engine_seq`, same Kafka
  topic/partition, consecutive Kafka offset, non-terminal `ENGINE_ACCEPTED`
  rows. `ENGINE_SOLD`, reject, gap, replay, stale epoch, or identity conflict
  falls back to the pre-existing per-message path. The batch transaction still
  writes `bids`, `auction_events`, `outbox_events`, `outbox_delivery`,
  `idempotency_records`, `redis_engine_settlements`, auction seq/price/winner,
  and the engine checkpoint.
- New proof tests: `TestKafkaSettlementBatchesAcceptedPrefix` verifies gap-free
  public seq, accepted bid rows, auction events, outbox delivery rows,
  settlement rows, idempotency completion, and auction engine/public seq after
  a pure accepted prefix. `TestKafkaSettlementBatchesAcceptedPrefixBeforeReject`
  verifies mixed accepted+rejected Kafka batches still settle correctly.

Post-fix rerun policy:

1. First rerun the same accepted profile `50/100/200/400/600` to compare against
   `s2-capacity-accepted-ecs-20260604T150519`.
2. A pass requires both k6 clean **and** post-run convergence: Kafka lag 0,
   Redis pending 0, PG settlements complete, outbox watermarks 0, verifier PASS.
3. If 600/s still leaves async lag, keep it as bottleneck evidence and rerun
   `50/100/200/300/400` or `100/200/300/400` to find the end-to-end clean knee.
   Do not increase to 800/1000 until the async drain gate passes at 600.

Detailed diagnosis and judge-facing answers:
[s2-settlement-diagnosis-and-judge-defense.md](s2-settlement-diagnosis-and-judge-defense.md).

## 6. Metric → chart / panel mapping

| Claim | Source |
|---|---|
| steady decision p99 ≤ 100 ms | PTS `出价决策 bid-decision` p99 (RPS run) / k6 `p(99)` (soak) |
| offered vs accepted rate | PTS TPS + server accepted count; k6 rate vs server `ENGINE_ACCEPTED/s` |
| settlement convergence safe for payment | `s2-convergence-summary.env`; exact `s2-convergence.tsv` samples; Kafka consumer lag; verifier gates `kafka_consumer_group_lag_zero`, `redis_pending_decisions_empty`, `outbox_drained` |
| **M4 no leak** | Grafana: `process_resident_memory_bytes`, `go_memstats_heap_inuse_bytes` (post-GC floor), `go_goroutines`, `process_open_fds` — flat slope over the soak |
| auto-extension correct | M3 verifier extension check |

For current local S2 artifacts, the project also writes self-contained runtime
samples to `s2-runtime-samples.tsv`: `runtime_rss_bytes`,
`runtime_heap_inuse_bytes`, `runtime_heap_alloc_bytes`, `runtime_goroutines`,
`runtime_open_fds`, and DB pool counts. A 2m30s stair can diagnose pressure, but
it is not a substitute for a 30-60 minute M4 no-leak soak.

## 7. Pitfalls

- **Using closed VU loop for the sustained claim.** Use RPS mode (PTS) /
  arrival-rate (k6); otherwise overload hides and p99 is optimistic.
- **Calling a short stair a soak.** A 2-5 minute 1000/s stair can diagnose
  capacity and convergence, but it cannot prove 30-60 minute M4 no-leak.
- **Running k6 on the service node for formal soak evidence.** Acceptable for
  development, but judge-facing S2 should use an independent same-VPC ECS or PTS
  RPS to avoid load-generator resource contention.
- **Mixing read interference into fanout.** High-frequency read traffic belongs
  to `S2-read-interference`; it must not pollute S3 live-fanout samplers.
- **Re-bid storm.** Active bidders should bid to *raise*, not re-fire below the
  current price every 2 s; otherwise you recreate S1's hotspot and call it steady.
- **Soak too short.** < 30 min rarely exposes a slow leak; watch the post-GC
  floor, not the sawtooth peaks.
- **Reading peaks instead of the floor for M4.** A rising *floor* is the leak; a
  sawtooth that resets to a stable floor is healthy.
- **No `pprof` baseline.** Snapshot heap at warm-up and diff at the end
  (`go tool pprof -base`) to localize any growth.
- **Calling M1 success "payment safe".** M1 proves the user saw a final engine
  decision. Payment safety additionally requires settlement convergence.
