# PTS Pressure Test Plan

> 2026-05-31 supersession notice: this plan contains historical PG-lane /
> Redis-guard pressure strategy. Use it for background only. Current PTS-1B
> execution and pass/fail rules are in `tests/pts/MANIFEST.md` and
> `docs/current/performance-correctness-contract.md`.

This document defines what each pressure workload is allowed to prove. It is the
planning layer above the concrete runbooks and JMX files.

Related execution docs:

- `docs/perf/pts/full-pressure-runbook.md`
- `docs/perf/pts/core-pressure-config.md`
- `docs/perf/cloud-server/03-workload-matrix-and-pts-config.md`

## Prime Rule

Do not mix different load goals into one capacity claim.

A single hot auction proves correctness and latency under contention. Multi-room
accepted pressure proves platform write throughput. Reject-heavy pressure proves
business rejection stability. These are different claims and must be reported
separately.

Before every paid PTS run:

1. reset dedicated pressure data;
2. collect a before evidence snapshot;
3. run one clearly named workload;
4. collect after evidence immediately;
5. pull PTS report details and sampling logs;
6. classify the run as `PASS`, `BOTTLENECK_FOUND`, `HARNESS_GAP`, or
   `ENV_LIMIT`.

PDF reports are useful for trend charts, but PDF alone is not bottleneck
evidence. Architecture decisions require PTS API data plus server, DB, Redis,
outbox, and runtime evidence.

## Current Constraints

| Item | Current value |
|---|---|
| PTS source | Alibaba Cloud VPC internal network |
| Region | cn-heyuan |
| Target | `http://172.16.179.112:18080` |
| PTS mode | virtual users |
| Current quota | start with 1000 concurrency; raise only after bottleneck attribution is clear |
| Downstream pressure | `ADMISSION_ENABLED=false`; `redis_guard` plus bounded per-auction lane for hotspot optimization validation |
| Auth | real session CSV, no mock auth |
| Evidence root | `docs/perf/pts/evidence/` |
| Current focus | PTS-1/L4a post-`1L29X7UG` hotspot optimization validation, not final judge evidence |

## Current Hotspot Optimization Policy

The active phase is PTS-1/L4a post-`1L29X7UG` optimization validation. It is
not a production-configuration review and not final evidence for judges. Final
judge-facing performance claims will be produced later after fresh, clean
pressure runs.

The current objective is:

```text
Reduce invalid/stale traffic entering PostgreSQL, shorten Redis guard stale
windows after accepted bids, and keep single-auction PostgreSQL concurrency
bounded by the application lane.
```

Default validation profile:

- `ADMISSION_ENABLED=false` is required.
- `BID_ENGINE_MODE=redis_guard` is the default L4a path.
- `BID_LANE_WORKERS=1`, `BID_LANE_QUEUE_SIZE=2048`, and
  `BID_LANE_QUEUE_TIMEOUT=3s`.
- `BID_AUCTION_TOO_HOT` and `BID_RETRY_LATER` are valid overload-protection
  signals in this validation profile if they are measured together with queue
  wait and downstream DB lock reduction.

`1L29X7UG` is the failure baseline for this repair: Redis guard reported mostly
`STALE`, `auction_bid_lock_wait_seconds_sum` reached about `40973s`, and DB pool
wait reached about `236216s`. The next run must compare guard reject rate,
projection update outcomes, queue wait/rejects, DB lock wait, tx duration, and
outbox lag against that evidence.

Use a high-lane diagnostic profile only when explicitly trying to expose a
lower downstream limiter, and label that run `HARNESS_EXPLORATION`:

```bash
BID_LANE_WORKERS=256 BID_LANE_QUEUE_SIZE=100000 BID_LANE_QUEUE_TIMEOUT=10m \
  bash tests/pts/prepare-cloud-pressure.sh
```

Do not use high-lane exploration numbers as user-facing latency or capacity
claims.

## Workload Matrix

| ID | Workload | What It Tests | Business Scene | Main Project Challenge | Scorecard Value | PTS Priority |
|---|---|---|---|---|---|---|
| PTS-0 | Auth/seed smoke | CSV tokens, ACL, `/readyz`, snapshot, ws-ticket, basic bid path | Deployment readiness before paid pressure | Evidence discipline | Evidence quality, backend service | Low cost, run only after script/data changes |
| PTS-1 | Single-auction hotspot | One `auction_id` under final-second contention; row lock, idempotency, seq, winner/order, outbox ordering | One rare item in a hot live room | Complex auction correctness and realtime latency | Core technical challenge, high-concurrency optimization, data governance, system availability | Current top priority |
| PTS-2 | Multi-auction accepted throughput | Many auctions producing accepted bids in parallel | Many live rooms active at the same time | Platform throughput with per-auction ordering | Performance, stability, backend service | Defer until PTS-1 latency improves |
| PTS-3 | Reject/idempotency pressure | `BID_TOO_LOW`, self-leading, duplicate key, retry-later behavior | Users hammering invalid or stale bids | Gateway and business error stability | Interface gateway, observability, stability | Run after accepted workloads are clean |
| PTS-4 | Outbox relay burst | Event production vs relay publish capacity, lag, retry, DEAD, head-of-line | Sustained bid event stream during hot campaigns | Recoverable realtime after commit | System availability, observability, stability | Run if outbox lag appears again |
| PTS-5 | WebSocket fanout | Long-lived watcher connections receiving bid events | Many viewers watching one or many rooms | Millisecond realtime delivery | Frontend interaction, realtime challenge, stability | Prefer dedicated WS/k6 profile if PTS cannot chain ticket + WS cleanly |
| PTS-6 | Reconnect storm | Many clients reconnect with stale `last_seq`; history vs snapshot fallback | Mobile weak network / app background resume | Recoverable realtime | Core challenge optimization, frontend safety, system availability | Use after fanout baseline |
| PTS-7 | Production guarded overload | Admission enabled; stable 429/retry-after and downstream protection | Public traffic protection under overload | Graceful degradation | Production sense, interface gateway | Not a downstream capacity claim |

## Two Accepted-Bid Goals

### PTS-1: Single-Auction Hotspot

This answers:

```text
When many users fight for the same item at the same time, does the auction stay
correct, ordered, recoverable, and diagnosable?
```

It should not be sold as the platform's accepted-bid TPS. A single auction has
one authoritative price, one leading bidder, one sequence, and at most one
terminal winner/order. Accepted mutations are intentionally serialized by the
auction truth path.

Use this workload to prove:

- no wrong current price;
- no duplicate winner/order;
- `seq` is continuous;
- idempotency returns same result or bounded retry-later;
- cap/cancel/end races produce one terminal result;
- outbox preserves same-auction order;
- clients can recover from gaps via history or snapshot.

Expected bottlenecks:

- PostgreSQL auction row lock wait;
- DB pool wait if pool is too small;
- idempotency retry-later windows;
- outbox lag if event production exceeds relay publish.

Current measured bottleneck from report `9VY7W7BF`:

- correctness passed under 1000 VU offered pressure;
- bid P99 was about `2265ms`, which fails the millisecond-level hotspot target;
- outbox and Redis were healthy;
- the bottleneck was PostgreSQL single-auction lock/pool waiting;
- next work is architecture optimization for PTS-1 before more paid capacity
  discovery.

Passing shape:

```text
HTTP errors are zero or explicitly classified.
Accepted/rejected distribution is explained by the workload.
Auction invariants hold.
Outbox has no unbounded backlog.
P99 is reported as hotspot latency, not platform capacity.
```

### PTS-2: Multi-Auction Accepted Throughput

This answers:

```text
When many rooms are live at the same time, how many valid accepted bid writes
can the platform process while preserving per-auction ordering?
```

This is the right workload for paid capacity discovery because it uses the 1000
VU quota without forcing every bid through the same auction row.

Use this workload to prove:

- accepted writes scale across `auction_id`;
- per-auction `seq` remains continuous;
- cold rooms are not corrupted by hot rooms;
- DB pool, PostgreSQL, Redis, outbox relay, and runtime have measured headroom;
- outbox delivery can keep up with accepted event production;
- no room leaks another room's events.

Expected bottlenecks:

- aggregate DB write throughput;
- outbox relay claim/mark/publish throughput;
- Redis publish or history writes;
- Go runtime CPU, heap, goroutines, or file descriptors;
- PTS generator limit.

Passing shape:

```text
ACCEPTED ratio is high by design.
No auction reaches cap or ends early.
Per-auction invariants hold.
PENDING outbox does not grow monotonically.
PTS generator is not the first saturated component.
```

## Recommended Execution Order

For the current post-`1L29X7UG` validation phase, start with the bounded-lane
hotspot profile and raise pressure only after the first run proves guard
freshness, queue wait, DB lock wait, outbox lag, and correctness are
interpretable. Protection responses are acceptable when they are explicit and
measured; they are not a platform capacity claim.

| Step | Workload | Scale | Why this order |
|---|---|---:|---|
| 1 | PTS-0 Auth/seed smoke | 10-50 VU, 1-3 min | Only after JMX, CSV, seed, auth, or deployment changes |
| 2 | PTS-1 Single-auction hotspot validation | 1000 VU | Compare against `1L29X7UG` for guard stale reduction, lock wait reduction, queue behavior, and correctness |
| 3 | PTS-1 Single-auction hotspot validation | raise only if step 2 is stable | Find the bounded-lane user-facing overload point without hiding correctness failures |
| 4 | PTS-1 high-lane diagnostic | explicit override only | Use only if a lower downstream limiter needs proof; label as `HARNESS_EXPLORATION` |
| 5 | PTS-2 Multi-auction accepted throughput exploration | 10000 -> 30000 VU | Separate single-auction serialization limits from platform-wide throughput |
| 6 | PTS-4/5/6 focused follow-ups | Match the bottleneck found above | Drill into outbox, WS fanout, reconnect, Redis, or runtime as indicated |

After validation is complete, run a separate final evidence sequence for
judge-facing claims. Do not use repair-validation numbers as final capacity
claims.

## Reporting Map

Use this map when writing README, slides, or defense notes.

| Claim Type | Required Workload | Allowed wording | Forbidden wording |
|---|---|---|---|
| Final-second correctness | PTS-1 | "Under single-auction contention, seq/winner/order/outbox invariants held." | "The platform supports N accepted bids/s." |
| Platform accepted throughput | PTS-2 | "Across N auctions, accepted bid throughput reached X with P99 Y under environment Z." | "One auction accepts X bids/s." |
| Reject stability | PTS-3 | "Business rejects stayed deterministic and observable." | "Reject-heavy TPS proves accepted capacity." |
| Outbox capacity | PTS-4 | "Relay kept up with produced event rate X; backlog did not grow." | "WebSocket users all received messages." |
| Fanout capacity | PTS-5 | "N connected watchers received ordered events with bounded lag." | "HTTP bid pressure proves WS capacity." |
| Recovery safety | PTS-6 | "Reconnect storm recovered through history/snapshot with bounded DB rebuild." | "Normal snapshot GET proves weak-network safety." |
| Overload protection | PTS-7 | "Admission returned stable retryable overload responses and protected downstream." | "Admission-on TPS is backend capacity." |

## Current Evidence Classification

| Report | Classification | Reason |
|---|---|---|
| `MBXPW75F` | partial hotspot/reject evidence | DB pool was still `8`; accepted ratio was mixed; useful before DB-pool comparison but not final capacity |
| `KIXXW7AF` | harness gap for accepted hotspot | DB pool `90` improved surface TPS/P99, but `auc_live` had only about 4.6% accepted bids, so the run was mostly `BID_TOO_LOW` reject pressure |
| `9VY7W7BF` | PTS-1 correctness pass, latency bottleneck found | 1000 VU hotspot kept seq/outbox correctness, but bid P99 was about 2265ms; bottleneck is DB row-lock/pool waiting |
| `JB25X72G` | exploration harness gap | Bid sampler reached about 6000 TPS, but `BID_AUCTION_TOO_HOT`/429 dominated because bid-lane ceilings were too low for the exploration objective |
| `1L29X7UG` | high-lane bottleneck proof | `redis_guard` was mostly `STALE`; `auction_bid_lock_wait_seconds_sum` was about `40973s`; DB pool wait was about `236216s`; useful as the failure baseline for this repair |

The next task is not another paid repeat of the same PTS-1 profile. It is an
architecture optimization round for PTS-1 hotspot latency, followed by the same
JMX to prove before/after improvement.

## PTS-1 Optimization Backlog

The target is millisecond-level realtime behavior under a single hot auction
without weakening truth ownership.

Allowed:

- PostgreSQL remains final money truth.
- Redis may be used for projection, pre-admission, stale-bid filtering, and
  fast snapshot/history reads.
- WebSocket remains delivery.
- Application-level queues may shape load before the database lock.

Not allowed:

- Redis becoming authoritative for price, winner, order, or seq.
- Client-side optimistic bid success.
- Direct WebSocket publish without committed outbox.

Current repair scope after `1L29X7UG`:

1. Keep the existing per-auction bounded in-process bid lane enabled for
   `postgres_lane` and `redis_guard`; use one worker per auction for optimized
   single-auction validation.
2. Use Redis guard to reject stale projections only when monotonic price proves
   the bid cannot win, and fall through to PostgreSQL for uncertain stale cases.
3. Refresh the Redis guard projection immediately after an accepted PostgreSQL
   commit, with seq fencing and bounded best-effort retry, while keeping outbox
   relay as the durable repair path.
4. Keep SOLD order creation, auction event, outbox, bid row, and idempotency
   completion inside one PostgreSQL transaction. Do not split order creation
   into async settlement without a new product/API contract.
5. Do not change `synchronous_commit` in the app by default. Test
   `synchronous_commit=local/off` only as an explicitly labeled DB experiment
   with crash-loss tolerance documented.
6. Re-run `tests/pts/archive/historical/live-auction-hotspot-pressure.jmx` and compare against
   `1L29X7UG` for guard stale ratio, guard reject count, projection update
   outcomes, queue wait/rejects, DB pool wait, row-lock wait, tx duration,
   outbox lag, and correctness invariants.
