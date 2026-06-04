# S2 Settlement Diagnosis And Judge Defense

> Status: current diagnostic note, 2026-06-04.
> Scope: S2 steady-auction bid-decision soak, PostgreSQL settlement convergence,
> read-interference follow-up, and judge-facing answers. This document is
> evidence interpretation, not a new workload definition.

## 1. Executive Verdict

S2 currently proves two different things with different confidence levels:

| Claim | Current verdict | Evidence |
|---|---|---|
| 30-minute independent-ECS bid-decision soak | PASS | `s2-ecs-30m-20260604T095720`: 85,499/85,499 final decisions, HTTP p99 3.30ms, custom decision p99 4ms, dropped 0, all verifier gates PASS |
| M1 steady bid decision p99 <= 100ms | PASS | `s2-stair-1000-setbased-logsuppressed-100s-20260602T211311`: HTTP p99 5.44ms, p99.9 32.21ms, 0 dropped iterations |
| Redis/Lua not the source of tail regression | PASS | Redis SLOWLOG LEN = 0; Redis LATENCY LATEST empty during diagnostic runs |
| Correct eventual settlement after verifier wait | PASS | Same run: `l4b-invariant-gates.tsv` all P0/P1 PASS after verifier wait |
| Async settlement convergence <= 100s | FAIL / near miss | Same run timed out at 102s with Kafka lag 1371, settlement_total 69774/70999 |
| Async settlement convergence <= 110s | FAIL | `s2-stair-1000-workers4-110s-20260603T1928`: timed out at 112s with Kafka lag 1275, settlement_total 69925/70999 |
| Async settlement convergence within 120s product buffer | ACCEPTABLE WITH POLL DISCLOSURE | `s2-stair-1000-120s-20260603T1942`: 119s still had lag 286; the 122s sample confirmed lag 0, settlement_total 70999/70999, Redis pending 0, outbox unpublished 0 |
| Direct-SETTLED fast rejected SQL | REJECTED and reverted | `s2-stair-1000-directsettled-100s-20260602T212330`: worse lag 32033 at 101s; verifier failed due to incomplete convergence |
| HTTP read-interference under bid load | CURRENT_FAILING / bottleneck evidence | `s2-read-ecs-15m-20260604T113330`: 100 bid attempts/s plus 2000/5000/10000 HTTP reads/s exposed read-path/DB-pool saturation; bid path stayed p99 5.68ms, but reads reached only ~2142/s actual with 2,057,742 dropped iterations |

The honest judge-facing statement:

> "The foreground decision engine is comfortably inside the steady 100ms p99
> target under a 200/s -> 600/s -> 1000/s open-arrival local stair. The remaining
> bottleneck is asynchronous PostgreSQL settlement drain for rejected decisions,
> not Redis Lua or bid decision latency. Set-based rejected settlement improved
> convergence from 158s to roughly the 100-122s band depending on local noise.
> The 110s terminal rerun still failed; the 120s product-buffer rerun drained at
> the 122s confirmation sample. S2 is therefore foreground-pass and
> correctness-after-drain-pass. Payment/finality can be defended as a 120s
> business buffer with explicit polling tolerance, but not as a strict hard
> real-time 120.000s bound."

Update after the independent k6 ECS run on 2026-06-04:

> "We also ran a 30-minute independent-ECS S2-long-soak at 20/s -> 60/s ->
> 100/s offered bid attempts. It delivered 85,499 final bid decisions with zero
> dropped iterations, zero HTTP failures, zero auth/ACL failures, zero admission
> contamination, HTTP p99 3.30ms, and S2 custom decision p99 4ms. Service-side
> verification found all 85,499 decisions settled, Kafka lag 0, Redis pending 0,
> DLQ empty, outbox drained, complete engine_seq, and all P0/P1 gates PASS. This
> is a bid-decision endurance and convergence PASS. It does not claim
> accepted-heavy WebSocket fanout. The follow-up S2-read-interference run found
> the read-path ceiling before the bid path failed: bid decisions stayed p99
> 5.68ms at ~100/s, while snapshot/leaderboard/my-bids reads accumulated
> second-level tails and k6 dropped iterations under the 5k/10k read stages."

## 1a. Current 30-Minute Independent-ECS S2 Evidence

Run label:

```text
s2-ecs-30m-20260604T095720
```

Workload shape:

```text
tool/source      : independent same-VPC ECS k6
service target   : service ECS private IP on :18080
model            : open-model ramping-arrival-rate
stage 1          : 20 offered bid attempts/s for 10 min
stage 2          : 60 offered bid attempts/s for 10 min
stage 3          : 100 offered bid attempts/s for 10 min
ramp-down        : 30s
runtime profile  : BID_ENGINE_MODE=redis_ledger, ADMISSION_ENABLED=false
auction          : auc_live, reset immediately before run
auth path        : mock user headers for seeded k6_bidder_* users
```

k6 result:

| Signal | Value | Interpretation |
|---|---:|---|
| k6 exit code | 0 | thresholds passed |
| total HTTP bid requests | 85,499 | offered load was delivered |
| final `ENGINE_*` decisions | 85,499 | every request reached a final business decision |
| `ENGINE_ACCEPTED` | 61 | accepted update count; bounded by price ladder and amount model |
| `ENGINE_REJECTED` | 85,438 | correct low-price/stale decisions; still persisted and verified |
| dropped iterations | 0 | k6 did not under-deliver the arrival-rate plan |
| HTTP failures | 0 | no transport/protocol failure |
| auth/ACL failures | 0 | seeded users and room ACL were valid |
| admission contamination | 0 | pressure reached the engine; no `429`/`RATE_LIMITED` pollution |
| non-decision failures | 0 | no vague 409/other user-visible interruption |
| HTTP p99 | 3.30ms | below S2 steady 100ms target |
| S2 custom decision p99 | 4ms | request-to-final-decision custom trend |
| max active VUs used | 1 | responses were fast enough that arrival-rate did not need many concurrent VUs |
| k6 host CPU | mostly 3-7% | pressure host had headroom |
| k6 RSS / FD | about 83MB / 86 FD | no load-generator resource pressure |
| TCP retransmits | essentially 0 | no obvious network pollution |

Service-side post-run state:

| Signal | Value |
|---|---:|
| auction engine mode | `redis_ledger` |
| auction engine_seq | 85,499 |
| settled accepted decisions | 61 |
| settled rejected decisions | 85,438 |
| pending settlements | 0 |
| failed settlements | 0 |
| Redis pending decisions | 0 |
| Kafka consumer lag | 0 |
| Kafka DLQ | empty / PASS |
| outbox ready/publishing/dead/retrying/ack_pending | 0 / 0 / 0 / 0 / 0 |
| outbox recent delivery state | `PUBLISHED` / `ACKED` |
| final auction state | `ACTIVE`, current price 315000, winner `k6_bidder_77`, accepted count 61, engine not paused |

Verifier output:

```text
l4b-correctness.txt        PASS
l4b-invariant-gates.tsv    all P0/P1 PASS
l4b-kafka-gates.tsv        PASS
l4b-redis-gates.tsv        PASS
l4b-redis-pending-gates.tsv PASS
l4b-reject-reason-gates.tsv PASS
```

Key gate meanings:

- `engine_seq_complete`: settled engine_seq is complete from 1 through 85,499.
- `every_bid_has_settled_ledger`: every durable decision has terminal
  Redis/Kafka/PG settlement coverage.
- `bid_too_low_rejects_justified`: rejected bids carry decision-time basis.
- `auction_winner_matches_highest_accepted`: final current winner and price
  match the highest accepted bid.
- `outbox_drained`: no unpublished outbox delivery remains.
- `kafka_consumer_group_lag_zero`: settlement consumer drained fully.

Evidence directory:

```text
docs/perf/pts/evidence/incoming/s2-ecs-30m-20260604T095720/
```

Important files:

```text
k6-summary.json                         # on k6 ECS
k6-samples.jsonl                        # on k6 ECS
k6-host/*                               # k6 CPU/RSS/fd/network evidence
l4b-correctness.txt                     # service-side verifier report
l4b-invariant-gates.tsv                 # P0/P1 invariant gate table
metrics.prom                            # service metrics snapshot
postgres-summary.txt                    # PostgreSQL summary
redis-info.txt                          # Redis health/memory/policy
```

### Why 61 Accepted Bids Is Not A Capacity Failure

In an ascending auction, accepted updates are a business-rule output, not the
capacity denominator. Once the price ladder rises, most later attempts are
correctly rejected. A reject is still a final adjudication that must be:

```text
idempotent -> Redis decision logged -> Kafka ledgered -> PostgreSQL settled
-> reject basis auditable -> verifier checked
```

This run therefore proves bid-decision goodput and settlement coverage for 85,499
decisions. It does not prove accepted-heavy fanout. Only 61 accepted updates
created public price-change fanout, so M2/fanout claims must come from S3.

### Boundary To State To Judges

Say:

> "S2-long-soak proves the engine can sustain normal bid-decision traffic for 30
> minutes with no dropped iterations, no admission pollution, and full async
> convergence. Because this script intentionally models an ascending auction,
> most decisions are low-price rejections; those are still real decisions and
> are persisted and verified. We do not use this run to claim WebSocket fanout
> capacity or high-RPS read interference. Those are separate S2-read and S3
> workloads."

Do not say:

> "This proves the entire live room under 1000/s mixed user traffic."

That would overclaim, because the run did not include 1000/s readers or held
WebSocket viewers.

## 1b. S2-read-interference Design, Result, And Boundary

The next S2 run answers a different judge question:

> "If hundreds or thousands of viewers poll room state, leaderboard, and their
> own bid history, does bid p99 or settlement safety degrade?"

Workload asset:

```text
tests/load/s2-read-interference.js
```

Default 15-minute independent-ECS shape:

| Stage | Duration | Bid attempts/s | HTTP reads/s | Total offered RPS |
|---|---:|---:|---:|---:|
| read-2k | 5 min | 100 | 2,000 | 2,100 |
| read-5k | 5 min | 100 | 5,000 | 5,100 |
| read-10k | 5 min | 100 | 10,000 | 10,100 |
| ramp-down | 30s | 0 | 0 | 0 |

Read mix:

| Endpoint | Share | Peak RPS | Why it matters |
|---|---:|---:|---|
| `GET /api/auctions/auc_live` | 80% | 8,000/s | main room state/snapshot polling |
| `GET /api/auctions/auc_live/leaderboard?limit=5` | 15% | 1,500/s | ranking/price comparison path |
| `GET /api/users/me/bids` | 5% | 500/s | personal history/status path |

Why 2000/5000/10000 reads/s:

- It creates a 10:1 read-to-bid ratio, matching the business intuition that most
  live-room users watch or refresh state rather than bid. At peak the ratio is
  100:1, which intentionally attacks the read path harder than the normal soak.
- It reaches 10,100 offered HTTP RPS at peak while keeping the bid rate identical
  to the validated S2-long-soak peak. This isolates the question "what did reads
  do to bid p99?".
- A single c9i.xlarge k6 host should comfortably generate this HTTP RPS if k6
  CPU, dropped iterations, sockets, and retransmits stay clean. If k6 host
  headroom is lost, the result is ENV_LIMIT, not a service conclusion.

Pass/fail gates:

| Gate | Target |
|---|---:|
| bid decision p99 under read load | < 100ms |
| auction snapshot read p99 | < 200ms |
| leaderboard read p99 | < 200ms |
| bid-history read p99 | < 300ms |
| dropped iterations | < 500, ideally 0 |
| HTTP failure rate | 0 |
| auth/ACL failures | 0 |
| admission contamination | 0 |
| non-decision bid failures | 0 |
| read failures | 0 |
| Kafka consumer lag after drain | 0 |
| Redis pending decisions after drain | 0 |
| PG non-terminal settlements after drain | 0 |
| outbox unpublished after drain | 0 |
| verifier P0/P1 gates | PASS |

Primary risk being tested:

```text
reader traffic -> DB pool wait / CPU / query latency -> bid p99 drift
reader traffic -> Redis/PG contention -> settlement/outbox convergence drift
```

Formal run on 2026-06-04:

```text
label           : s2-read-ecs-15m-20260604T113330
tool/source     : independent same-VPC ECS k6
service target  : service ECS private IP on :18080
duration        : 15 min + 30s ramp-down
bid rate        : 100/s, 100/s, 100/s
read rate       : 2000/s, 5000/s, 10000/s
read mix        : 80% snapshot, 15% leaderboard, 5% my-bids
runtime profile : BID_ENGINE_MODE=redis_ledger, ADMISSION_ENABLED=false
auction         : auc_live, reset immediately before run
```

k6 result:

| Signal | Value | Interpretation |
|---|---:|---|
| k6 exit code | 99 | threshold failure; this is not a clean pass |
| total HTTP requests delivered | 2,083,756 | actual delivered throughput averaged about 2240 req/s |
| dropped iterations | 2,057,742 | open-arrival plan was under-delivered after reads slowed |
| HTTP failure rate | 0 | no transport/protocol failure despite latency |
| bid final decisions | 91,499 | bid lane delivered about 98.4/s |
| read successes | 1,992,257 | read lane delivered about 2142/s |
| accepted / rejected | 28 / 91,471 | ascending-auction price ladder quickly made most bids valid rejects |
| bid p99 | 5.68ms | Redis engine bid path stayed far below 100ms |
| snapshot p99 | 1.60s | main room snapshot read path exceeded target |
| leaderboard p99 | 4.07s | leaderboard read path was the worst tail |
| my-bids p99 | 884.8ms | personal history read path also exceeded target |
| k6 CPU / RSS | about 30-44% CPU / 1.6-1.9GB RSS | load generator CPU was not the first bottleneck |
| k6 FD / VU state | about 4100 FD; `READ_MAX_VUS=4000` filled | VUs were occupied waiting for slow read responses |

Service-side evidence at immediate collection time:

| Signal | Value | Interpretation |
|---|---:|---|
| `GET /api/auctions/auc_live` count | 1,595,209 | snapshot route dominated read load |
| `GET /leaderboard` count | 299,512 | leaderboard route was lower volume but highest p99 |
| `GET /api/users/me/bids` count | 99,687 | my-bids route still added DB pressure |
| `POST /bids` count | 91,713 | bid traffic stayed close to 100/s target |
| `db_pool_max_conns` | 90 | service DB pool configured for 90 conns |
| `db_pool_conns{total}` | 90 | pool reached configured size |
| `db_pool_empty_acquire_total` | 3,257,372 | callers frequently waited for an available DB connection |
| `db_pool_empty_acquire_wait_seconds_total` | 2,281,594s | cumulative DB-pool wait confirms read-path saturation |
| service process RSS / FD | about 134MB / 208 FD | no service memory or FD explosion in post snapshot |
| Go goroutines | 44 | no goroutine leak signal in post snapshot |
| Redis evicted / rejected connections | 0 / 0 | Redis was not the visible failure source |
| Redis pending decisions | 0 | Redis relay pending hash drained |
| outbox ready/publishing/dead/retry/ack_pending | all 0 | public event outbox drained |
| Kafka lag at immediate verifier | 7857 on partition 15 | settlement consumer had not fully caught up in the first gate |
| immediate verifier exit | 1 | P0 `kafka_consumer_group_lag_zero`, `no_non_terminal_settlements`, `v3_relay_stream_complete` failed during collection window |

Late verifier after natural drain:

```text
label    : s2-read-ecs-15m-20260604T113330-late
time     : 2026-06-04T11:57:28+08:00
exit     : 0
result   : all P0/P1 gates PASS
```

Late settled state:

| Signal | Value |
|---|---:|
| auction engine_seq | 91,713 |
| settled accepted decisions | 31 |
| settled rejected decisions | 91,682 |
| non-terminal settlements | 0 |
| Kafka consumer group lag | 0 |
| Redis pending decisions | 0 |
| decision stream length / PG settlements | 91,713 / 91,713 |
| outbox rows | 800 `PUBLISHED` |

Evidence directories:

```text
docs/perf/pts/evidence/incoming/s2-read-ecs-15m-20260604T113330/
docs/perf/pts/evidence/incoming/s2-read-ecs-15m-20260604T113330-late/
```

Clean-ceiling attempt after the 10k attack:

```text
label           : s2-read-clean-ecs-15m-20260604T120823
tool/source     : independent same-VPC ECS k6
duration        : 15 min + 30s ramp-down
bid rate        : 100/s, 100/s, 100/s
read rate       : 2000/s, 3000/s, 4000/s
read mix        : 80% snapshot, 15% leaderboard, 5% my-bids
verdict         : CURRENT_FAILING / lower-ceiling bottleneck evidence
```

k6 result:

| Signal | Value | Interpretation |
|---|---:|---|
| k6 exit code | 99 | threshold failure; still not a clean pass |
| dropped iterations | 524,423 | improved from 10k attack, but still far above gate |
| HTTP failure rate | 0 | no transport/protocol failure |
| bid final decisions | 91,499 | bid lane delivered about 98.4/s |
| read successes | 1,935,576 | read lane delivered about 2081/s |
| accepted / rejected | 31 / 91,468 | business distribution remained valid |
| bid p99 | 5.70ms | bid path still held under read pressure |
| snapshot p99 | 1.02s | better than 10k attack, still above target |
| leaderboard p99 | 2.72s | better than 10k attack, still seconds-level |
| my-bids p99 | 596ms | better than 10k attack, still above target |
| first clear VU ceiling | about 6m35s | reader hit `READ_MAX_VUS=2500` while target was only about 2.3k/s |
| k6 host | RSS about 1.3GB; CPU/memory not globally saturated | still points to service read latency, not load-generator CPU |

Service-side evidence:

| Signal | Value | Interpretation |
|---|---:|---|
| `db_pool_max_conns` / total conns | 90 / 90 | DB pool reached configured maximum again |
| `db_pool_empty_acquire_total` | 2,996,066 | large DB-pool wait count remained |
| `db_pool_empty_acquire_wait_seconds_total` | 1,315,846s | cumulative wait remained severe |
| immediate Kafka lag | 5371, then 3705, then 0 | settlement backlog naturally drained after the run |
| immediate verifier exit | 1 | convergence gates failed in the immediate collection window |
| late verifier label | `s2-read-clean-ecs-15m-20260604T120823-late` | final convergence evidence |
| late verifier exit | 0 | all P0/P1 gates PASS |
| late settled decisions | 31 accepted + 91,468 rejected = 91,499 | every decision reached terminal settlement |
| late stream / PG settlement count | 91,499 / 91,499 | Redis decision log and PG settlement matched |

Evidence directories:

```text
docs/perf/pts/evidence/incoming/s2-read-clean-ecs-15m-20260604T120823/
docs/perf/pts/evidence/incoming/s2-read-clean-ecs-15m-20260604T120823-late/
```

Display-ceiling attempt before fixing the Redis TTL issue:

```text
label           : s2-read-display-ecs-15m-20260604T123644
tool/source     : independent same-VPC ECS k6
duration        : 15 min + 30s ramp-down
bid rate        : 100/s, 100/s, 100/s
read rate       : 1500/s, 1800/s, 2000/s
read mix        : 80% snapshot, 15% leaderboard, 5% my-bids
verdict         : CURRENT_FAILING / P0 hot-ledger TTL defect + P1 read bottleneck
```

k6 result:

| Signal | Value | Interpretation |
|---|---:|---|
| k6 exit code | 99 | threshold failure; not a clean pass |
| dropped iterations | 63,531 | much lower than prior attempts, still above gate |
| HTTP failure rate | 0 | no transport/protocol failure |
| bid final decisions | 91,499 | bid lane delivered about 98.4/s |
| read successes | 1,481,468 | read lane delivered about 1593/s |
| bid p99 | about 5.7ms | bid engine still stayed healthy |
| snapshot p99 | 1.26s | still too slow for room-state polling |
| leaderboard p99 | 3.41s | leaderboard remained the worst read path |
| my-bids p99 | 730ms | still above the read target |
| auth/ACL/admission/non-decision/read failures | 0 | workload reached intended code paths |

Service-side verifier and Redis state:

| Signal | Value | Interpretation |
|---|---:|---|
| PostgreSQL settlements | 31 accepted + 91,468 rejected = 91,499 | PG settlement itself completed |
| Kafka consumer lag | 0 | Kafka was not stuck |
| outbox / Redis pending | drained / 0 | outbox and pending hashes were clean |
| verifier exit | 1 | P0 invariant failure |
| `engine_paused` / reason | `true` / `REDIS_ENGINE_REDIS_BEHIND_DB` | engine entered protective pause |
| Redis state hash | only `paused=1`, `pause_reason=REDIS_ENGINE_REDIS_BEHIND_DB` | state hash existed but was incomplete |
| Redis decision stream length | 0 | `bid:{auc_live}:engine:log` had expired/disappeared |
| anomaly events | repeated `REDIS_ENGINE_REDIS_BEHIND_DB` with `redis_seq=0`, `db_seq=91499` | reconcile repeatedly saw Redis behind PostgreSQL |

Evidence directory:

```text
docs/perf/pts/evidence/incoming/s2-read-display-ecs-15m-20260604T123644/
```

Root cause found:

- The Redis hot-engine state, decision stream, pending hash, and relay cursor
  used `engineStateTTL = 30m`.
- The S2-read workflow can exceed 30 minutes wall-clock when it includes service
  preparation, a 15-minute run, ramp-down, evidence collection, verifier, and
  human handoff.
- Redis key expiry deletes keys at timeout. After the hot state and stream
  expired, reconcile read Redis `engine_seq` as 0 while PostgreSQL had settled
  `engine_seq=91499`, so it correctly failed closed with
  `REDIS_ENGINE_REDIS_BEHIND_DB`.
- A second bug made the state look worse: pause/resume code used bare
  `HSET(state, paused, reason)`. If the full state hash had expired, that HSET
  recreated a partial hash with only pause fields and no `engine_seq`.

P0 fix applied:

- Raised Redis hot-engine TTL from 30 minutes to 24 hours so long soaks and
  evidence collection cannot expire live/recent auction state.
- Changed Redis pause/resume mirroring to update Redis only when the state hash
  still has `engine_seq`. The database remains the source of truth for pause
  state; Redis no longer creates a partial hot state after expiry.
- Added regression tests:
  - `TestRedisLedgerHotStateAndLogTTLExceedsLongSoakWindow`
  - `TestRedisLedgerPauseDoesNotCreatePartialHotState`
- Verified with:

```text
go test ./internal/redisengine
```

P1 read-path mitigation applied:

- Added `bids(user_id, created_at DESC)` for `GET /api/users/me/bids`.
- Added a partial accepted-bid index for leaderboard reads:
  `bids(auction_id, amount_cents DESC, created_at ASC, user_id) WHERE status='ACCEPTED'`.
- Added accepted-bid indexes that match the actual leaderboard query shapes:
  `bids(auction_id, user_id, amount_cents DESC, created_at DESC) WHERE status='ACCEPTED'`
  for per-user best-bid grouping, and
  `bids(auction_id, created_at DESC) WHERE status='ACCEPTED'` for the 30-second
  activity/velocity window.
- Capped `ListBidHistory` at 50 latest rows to avoid full per-user history scans
  under high-frequency polling.
- Added a 250ms Redis-backed HTTP auction snapshot cache plus per-auction
  `singleflight` for `GET /api/auctions/{id}`. This endpoint is 80% of the
  S2-read mix; the short TTL absorbs polling storms without moving bid decisions
  or settlement truth out of Redis ledger / PostgreSQL.
- Added a 5s Redis negative cache for "current user has no max-bid intent" and
  invalidates it on PUT/DELETE. In the S2-read workload most readers have no
  private max-bid intent, so this avoids one empty PostgreSQL lookup per room
  snapshot request while preserving current-user private intent visibility.
- Added `auction_acl_membership_cache_total{result=...}` metrics on the auction
  membership ACL path. The S2-read prepare script already warms Redis ACL keys
  for `k6_bidder_*` and `k6_user_*` with a 12-hour TTL, so ACL should not be the
  bottleneck in this workload. We intentionally did not lengthen the production
  positive-cache TTL because membership revocation freshness matters; the next
  run must prove cache hits dominate before treating ACL as closed.
- Extended `collect-server-evidence.sh` with best-effort
  `pg_stat_statements` and `EXPLAIN (ANALYZE, BUFFERS)` output:
  `postgres-read-attribution.txt` and `postgres-s2-read-explain.txt`.
- Verified with:

```text
go test ./internal/auction
go test ./internal/gateway
```

P1 remaining work:

- The leaderboard endpoint still computes rank from PostgreSQL on each request.
  The durable industrial fix is a Redis/materialized read model for active
  auction leaderboard, with PG as source of record and verifier coverage for
  cache rebuild correctness.
- The next rerun should show whether the short snapshot/negative caches and
  indexes are enough for a judge-safe 1500/1800/2000 display result. If
  leaderboard p99 remains seconds-level, do not tune VUs; implement the
  materialized leaderboard read model.

Interpretation:

- This is a valid bottleneck-finding run, not a successful 10k-read capacity
  result.
- The bid engine path remained healthy under read pressure: about 98.4 final
  bid decisions/s and bid p99 5.68-5.70ms across both read-interference attempts.
- The read path became the bottleneck around the 2k/s delivered range. The
  2k/5k/10k run produced 2,057,742 dropped iterations, and the reduced
  2k/3k/4k run still produced 524,423 dropped iterations after reader VUs filled
  around 2.3k/s target.
- The strongest service-side attribution is DB-pool contention: `db_pool_total`
  reached 90 in both runs, with multi-million empty-pool acquire counts and
  million-second cumulative wait totals.
- Correctness was not permanently corrupted in the first two read-interference
  attempts: immediate verifiers failed convergence gates, but late verifiers
  passed after Kafka/settlement drain. The third display attempt exposed a real
  P0 hot-ledger TTL design defect and must be rerun after the TTL fix.
- The honest ceiling statement is: "under this service profile, 100 bid/s stayed
  clean while HTTP reads delivered roughly 2.0-2.1k/s; 3k/4k/5k/10k offered reads
  are not proven and expose read-path optimization work. The 1500/1800/2000
  attempt is invalid as a pass because it triggered Redis engine pause before the
  TTL fix."

What this run still does not prove:

- It does not hold 1000-10000 WebSocket connections.
- It does not measure publish-to-receive fanout p99.
- It does not replace S3. It is HTTP read interference only.

Recommended next run:

```text
After P0/P1 fixes: rerun S2-read display-ceiling with
100 bid/s + 1500/s -> 1800/s -> 2000/s reads.
Required gates: dropped_iterations near zero, read p99 materially below the
pre-fix run, `engine_paused=false`, Redis stream length/settlement counts
consistent, Kafka lag 0, Redis pending 0, outbox drained, verifier P0/P1 PASS.
Required ACL signal: `auction_acl_membership_cache_total{result="hit"}` should
dominate and `db_allowed` / `miss` should remain low after the prepared Redis
ACL warmup.
Required attribution files: `postgres-read-attribution.txt`,
`postgres-s2-read-explain.txt`, `metrics.prom`, `redis-info.txt`,
`k6-summary.json`, and `k6-host/*`.
```

## 2. What This Test Actually Is

The local S2 diagnostic run is not a constant 1000/s run. It is an open-arrival
stair:

```text
Tool       : local k6 via Docker, ramping-arrival-rate
Duration   : 40s @ 200/s, 40s ramp toward 600/s, 40s ramp toward 1000/s,
             plus 30s ramp-down
Total      : 70,999 bid decisions in ~150s
Average    : ~473 decisions/s
Peak       : 1000 offered decisions/s near the top of the stair
VU ceiling : preAllocatedVUs=800, maxVUs=1600; actual active VUs stayed <= 9
Admission  : ADMISSION_ENABLED=false pressure profile
Boundary   : HTTP request -> final ENGINE_* with durability_status=ENGINE_DURABLE
```

This is an open model. `dropped_iterations=0` means the offered load was actually
delivered; the script did not hide overload by slowing down like a closed VU loop.

## 3. Why There Is No M2 Fanout p99 In This S2 Result

No M2 fanout p99 is claimed from these local S2 runs.

Reason:

- The local S2 k6 script only posts bids and measures M1 decision latency plus
  async convergence.
- It does not hold WebSocket observers and does not compute client
  `publish -> receive` latency.
- The server metric `auction_fanout_latency_seconds` may appear because a few
  accepted updates produce outbox/WS work, but this is not a valid M2 proof:
  the run accepted only about 5 updates and did not hold thousands of clients.

M2 belongs primarily to S3:

```text
S2 proves: steady bid decision path + stability + settlement convergence.
S3 proves: accepted updates x subscribers fanout path, e.g. 10k WS observers.
```

If a judge asks why S2 does not include fanout p99:

> "We deliberately separate attribution. S2 is an open-arrival decision and soak
> workload. Fanout pressure is accepted updates times subscribers, so we measure
> it in S3 with many held WebSocket clients. Mixing them in this diagnostic run
> would obscure whether a tail came from Redis decision, PG settlement, or WS
> fanout."

## 4. Evidence Timeline

| Run | Code state / purpose | Decisions | M1 p99 / p99.9 | Dropped | Convergence result | Interpretation |
|---|---:|---:|---:|---:|---:|---|
| `s2-stair-1000-batchdrain-20260602T193000` | batch drain before set-based rejected settlement | 80,999 | 5.79ms / 30.51ms | 0 | PASS in 158s | Correct but too slow for close/payment safety |
| `s2-stair-1000-setbased-logdebug-2min-20260602T202733` | set-based rejected batch, logging not fully suppressed | 70,999 | 5.59ms / 29.29ms | 0 | PASS in 105s | Shows set-based direction works, but logging state was not final |
| `s2-stair-1000-setbased-logsuppressed-2min-20260602T203614` | set-based + settlement success logs suppressed | 70,999 | 5.52ms / 28.80ms | 0 | PASS in 122s | Stable M1; convergence still noisy |
| `s2-stair-1000-setbased-workers4-90s-20260602T204827` | 4 settlement workers, 90s gate | 70,999 | 5.38ms / 27.10ms | 0 | FAIL at 90s; lag 84 | Near miss; extra workers do not fully parallelize one hot auction |
| `s2-stair-1000-setbased-logsuppressed-100s-20260602T211311` | current kept code, 100s gate | 70,999 | 5.44ms / 32.21ms | 0 | FAIL at 102s; lag 1371 | Current best diagnostic evidence; M1 pass, 100s convergence near miss |
| `s2-stair-1000-directsettled-100s-20260602T212330` | rejected fast path direct SETTLED trial | 70,999 | 5.36ms / 31.74ms | 0 | FAIL at 101s; lag 32033; verifier incomplete | Rejected and reverted; SQL became heavier and did not help |
| `s2-stair-1000-110s-20260603T1919` | terminal 110s rerun, 1 effective settlement worker | 70,999 | 5.85ms / 25.06ms | 0 | FAIL at 111s; lag 305; verifier later PASS | M1 still strong; 110s convergence not proven |
| `s2-stair-1000-workers4-110s-20260603T1928` | terminal 110s rerun after fixing worker env propagation, workers=4 | 70,999 | 5.56ms / 34.25ms | 0 | FAIL at 112s; lag 1275; verifier later PASS | Extra consumers did not help one hot auction because all messages were on one Kafka partition |
| `s2-stair-1000-120s-20260603T1942` | product-buffer rerun, 1 settlement worker | 70,999 | 5.34ms / 27.68ms | 0 | PASS at 122s confirmation sample; lag 286 at 119s | 120s payment buffer is defensible with poll-boundary disclosure; still not a long M4 no-leak proof |

The failed direct-SETTLED run is useful evidence. It proves we did not keep a
performance tweak just because it sounded plausible.

## 5. What "PG Settlement Write Amplification" Means Here

The simplified explanation "one rejected decision writes many PG tables" is
directionally correct, but it needs precision.

Current fast rejected settlement still writes multiple logical records:

```text
Kafka rejected decision
  -> redis_engine_settlements  audit / replay status
  -> auctions                  advance engine_seq checkpoint
  -> bids                      persisted rejected bid row and decision basis
  -> idempotency_records       replay same HTTP result for retry
  -> redis_engine_checkpoints  Kafka position / replay checkpoint
```

The physical cost is larger than the row count:

- each insert/update maintains indexes;
- each update creates PostgreSQL MVCC old row versions that later need vacuum;
- each transaction writes WAL;
- the same hot auction must advance `engine_seq` in order.

PostgreSQL documents that normal `VACUUM` removes dead row versions from tables
and indexes and marks space reusable; our observed `n_dead_tup` on
`redis_engine_settlements` after S2 runs is consistent with update-heavy
settlement churn. See PostgreSQL routine vacuuming:
https://www.postgresql.org/docs/current/routine-vacuuming.html

So the root cause is not merely "5 tables". It is:

```text
high rejected-decision volume
  x per-decision audit/idempotency/bid materialization
  x index/WAL/MVCC cost
  x one hot auction's ordered settlement checkpoint
  x one Kafka partition carrying that auction's ordered stream
```

The 2026-06-03 4-worker rerun confirmed the partition boundary. Kafka consumer
group evidence showed `auc_live` messages concentrated on one partition while
other partitions had zero log-end offset. A single auction key preserves order by
partition, so extra consumers cannot parallelize that one ordered settlement
chain. More workers may help multi-auction traffic, but it is not a valid fix for
this single-hot-auction S2 gate.

## 6. Why We Do Not Simply Drop Rejected Bids Or Idempotency

Dropping rejected rows or idempotency would be the easiest way to lower PG write
volume, but it is not a safe default for this project.

### Idempotency

Stripe's API guidance is the relevant industrial analogy: for mutating POST
requests, idempotency stores the first result so a retry can safely get the same
status/body rather than perform a duplicate operation. Source:
https://docs.stripe.com/api/idempotent_requests

In this auction system, a rejected bid is still a user-visible final decision.
If the network drops after the server replies, the client may retry the same
`client_bid_id`. The defensible behavior is same key + same hash -> same
`ENGINE_REJECTED` response, not "maybe recompute against a later price".

### Audit And Arbitration

Jewellery auction has high trust requirements. A low-price reject is not just
noise; it may be challenged:

- "Why did my bid lose?"
- "Was I below the required increment at that exact engine_seq?"
- "Did the system reject me because I was stale or because the auction ended?"

The current verifier depends on persisted decision basis and settlement coverage.
Removing rejected bid materialization would require a replacement audit model,
not a deletion.

### What A Safe P1 Would Look Like

Do not remove correctness evidence. A safer P1 would be:

1. Keep idempotency response for every `ENGINE_*` final decision.
2. Keep an immutable audit record with `engine_seq`, amount, user, reason,
   request hash, and decision basis.
3. Consider moving rejected detail from `bids` to a narrower append-only
   `bid_decision_audit` table, or partitioning rejected decisions by auction/time.
4. Keep accepted/sold bids in `bids` because they affect winner, order, payment,
   and public events.
5. Prove with the same verifier that every rejected decision is replayable and
   justified.

That is a schema redesign task, not a small SQL tweak. It is deferred.

## 7. Kafka Worker Scaling Boundary

Adding settlement workers did not linearly improve this S2 run because this test
uses one hot auction. Kafka preserves order per partition, and each partition is
assigned to one consumer in a group. Kafka's own design docs describe topics as
ordered partitions consumed by one consumer within a subscribing group:
https://kafka.apache.org/20/design/design

For one auction keyed to one partition:

```text
more workers != more parallelism for that auction's ordered settlement stream
```

Scaling path:

- many auctions -> partition by `auction_id`, multiple partitions and consumers;
- one auction -> keep one sequencer and optimize per-message settlement cost;
- if one auction must exceed this drain rate, use a dedicated audit schema or a
  business close/payment gate, not unordered parallel settlement.

## 8. Business Interpretation For Jewellery Auction

`ENGINE_DURABLE` means the bidder's decision is safely recorded in Redis hot
state / Redis decision log / idempotency replay record. It does not mean
PostgreSQL accounting is complete.

Business rule:

```text
During live bidding:
  settlement lag is acceptable if M1/M2 stay healthy and replay coverage exists.

At close/payment:
  do not create payment/order truth from incomplete PostgreSQL settlement.
  Show "confirming final result" until Kafka lag = 0, Redis pending = 0,
  PG settlement complete, outbox drained.
```

What a visitor sees:

- During live bidding, the user's bid receives `ENGINE_ACCEPTED` or
  `ENGINE_REJECTED` in milliseconds. In the 120s rerun, HTTP p99 was 5.34ms and
  every response was a final durable decision.
- The live room may continue to show the current Redis/engine result while the
  accounting settlement catches up in the background.
- After the auction closes, the UI should not immediately expose a payment link
  if PG settlement is incomplete. It should show "final result confirming" or
  "settlement in progress" until the convergence gates are zero.
- In the 120s rerun, the exact samples were: 119s still had 286 Kafka/settlement
  records outstanding; the 122s sample confirmed all-zero backlog. That is a
  product/payment-buffer result, not a bid-latency result.

This is stronger than "stop accepting bids 30s before close", because pre-close
blocking conflicts with soft-close and final-second bidding. The safer product
fallback is post-close payment gating.

## 9. Judge Grill: Likely Questions And Defensible Answers

**Q: Why are there only 5 accepted bids? Is the system only handling 0.03 accepted/s?**

A: Accepted updates are governed by the price ladder and the S2 script's amount
model. The engine handled 70,999 final decisions with p99 5.44ms; rejected
decisions are successful adjudications. Accepted-update fanout is measured in S3,
not inferred from S2.

**Q: Why no M2 fanout p99 here?**

A: This S2 local script does not hold WS observers. M2 is publish-to-receive
latency and belongs to S3. S2 only claims M1 and settlement convergence.

**Q: Can you claim 1000/s?**

A: Only as peak offered rate in a 2-minute open-arrival stair, not as constant
1000/s for the entire run. The average was about 473 decisions/s over 70,999
decisions. The important harness signal is `dropped_iterations=0`.

**Q: Why did the 4-worker change not solve convergence?**

A: The auction's Kafka records are ordered in one partition. Kafka consumer-group
parallelism is partition-limited. Four workers help across many auctions or
partitions, not one hot auction stream.

**Q: Why not skip storing rejected bids?**

A: Because the reject is a final user-visible decision. We need idempotent retry
semantics and auditability of reject reason/decision basis. A future P1 can move
rejected detail to a narrower audit table, but cannot remove replay/audit
coverage.

**Q: Why is a 100s or 110s timeout acceptable?**

A: It is not marked PASS. 100s and 110s are stricter diagnostic gates and both
failed in kept terminal runs. Product safety is protected by post-close payment
gating, not by pretending those gates passed.

**Q: Can you relax the gate to 120s without cheating?**

A: Yes, if it is framed as a business finality/payment buffer, not as user-visible
bid latency and not as a strict hard real-time bound. The evidence must include
the actual convergence samples: 119s had lag 286; 122s had lag 0, PG settlement
complete, Redis pending 0, and outbox unpublished 0. The bidder still received
millisecond `ENGINE_*` decisions during the live auction; only order/payment
finality waited for settlement convergence.

**Q: What would you do next if asked to improve it?**

A: Not direct-SETTLED; that was measured and regressed. The next serious P1 is a
rejected-decision audit schema: append-only/narrow table, partitioned by
auction/time, retaining idempotency and verifier coverage. Then rerun S2 and S1
to prove no M1 regression and no correctness loss.

## 10. Current Action Plan

Keep:

- set-based rejected settlement batch;
- Kafka fetch/commit batching;
- settlement success log suppression for `kafka-settlement`;
- aggregated rejected settlement metric increment;
- 120s product-buffer S2 convergence target with explicit poll-boundary
  disclosure, pending a separate long M4 no-leak soak.

Do not keep:

- direct-SETTLED fast rejected SQL trial;
- any optimization that drops idempotency or reject audit without a replacement
  verifier and schema.

Next scenarios after S2:

1. S3 fanout: measure M2 publish-to-receive p99 with real WS observers.
2. S4 remaining faults: Kafka, Redis flush, Redis+Kafka correlated fault.
3. Optional S2 long soak: 30-60 minutes for real M4 no-leak evidence.
