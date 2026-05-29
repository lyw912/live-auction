# PTS Workload Matrix

Date: 2026-05-29

Index: `docs/perf/pts/l4b-kafka/README.md`

## Goal

PTS is used for bottleneck discovery and defensible evidence, not for proving a
preselected number. The load plan follows the project brief:

- complex auction correctness under final-second contention;
- millisecond-level realtime recovery and fanout;
- performance, stability, observability, and failure attribution evidence.

The current branch has moved the bid hot path from PG-lane serialization to the
L4B Redis Lua + Kafka ledger path. Therefore a PTS run is valid only if it proves
performance and correctness together. A fast report is a failure if Redis
accepted bids are not durably appended to Kafka, settled into PostgreSQL, exposed
through ordered events, and reconciled without gaps or duplicates.

All downstream-pressure workloads must run with `ADMISSION_ENABLED=false`.
Any HTTP 429, `RATE_LIMITED`, or `BID_AUCTION_TOO_HOT` result invalidates a
downstream bottleneck conclusion for that run.

## L4B Correctness Gates

These gates apply to every downstream-pressure run on the Kafka ledger branch.

| Gate | Required Evidence | Failure Meaning |
|---|---|---|
| Redis ledger classification | `auction_bid_redis_ledger_total` by outcome plus PTS sampler distribution | Cannot explain whether requests were accepted, rejected, retried, or failed. |
| Kafka durability | topic exists, DLQ empty or explained, no append failure delta | Redis accepted work may not be durable. |
| PostgreSQL settlement | `redis_engine_settlements` terminal statuses match accepted ledger attempts | Fast Redis accepts are not enough; money/audit truth did not converge. |
| Auction state continuity | `auctions.seq`, `auction_events.seq`, and accepted bid sequence are continuous | Client recovery and ordering are unsafe. |
| Idempotency | no duplicate `client_bid_id` / idempotency conflict beyond expected replay | Load may have created duplicate logical bids. |
| Winner/order invariant | at most one final winner/order; cap/end/cancel terminal state is unique | Auction correctness broken regardless of latency. |
| Realtime projection | Redis snapshot/history and outbox/fanout lag converge after the run | Browser can show stale or missing authoritative state. |
| Reconciliation | no unreconciled accepted Redis/Kafka ledger rows after bounded settle window | L4B path lost or stranded accepted bids. |
| Engine fencing | `engine_epoch` and `engine_seq` are monotonic; stale epoch settlement is rejected | Split-brain or stale engine writes can override current auction truth. |
| Kafka ordering | `ledger_partition/ledger_offset` preserves `engine_seq` order for one auction | Producer retry, partitioning, or rebalance can reorder settlement. |
| Soft close boundary | no accepted bid after final `end_at`; accepted deltas obey `increment_cents` | Redis and PG disagree about close time or rule enforcement. |
| Redis memory safety | `evicted_keys=0` and `maxmemory_policy=noeviction` | Hot auction state can be evicted and reinitialized incorrectly. |
| DLQ | `auction.dlq` offset sum is zero unless explicitly investigated | Settlement loss or poison was hidden behind successful PTS latency. |

Minimum post-run SQL/metric checks:

- count PTS bid attempts by sampler result;
- count Redis ledger accepted/rejected/error outcomes by metric delta;
- count Kafka append failures and DLQ records;
- count `redis_engine_settlements` by status for `auc_live`;
- count `bids`, `auction_events`, `outbox_events`, and `outbox_delivery` for
  `auc_live`;
- verify `auction_events.seq` has no gaps for `auc_live`;
- verify no duplicate `client_bid_id` for `auc_live`;
- verify no duplicate order for `auc_live`;
- wait for settlement/outbox lag to drain, then re-check.

The executable gate is:

```bash
bash tests/pts/preflight-l4b-pts-guards.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
bash tests/pts/verify-l4b-pts-correctness.sh after-REPORTID-l4b-final-second-1000vu
```

These are two different checks:

- preflight checks whether the implementation and runtime environment have the
  necessary protections before the test starts;
- post-run correctness checks whether the load test actually produced any
  invariant violation.

Both must pass before using a PTS report as performance evidence.

For final consistency after a known settlement backlog, use:

```bash
FINAL_WAIT_SECONDS=300 bash tests/pts/verify-l4b-pts-correctness.sh after-REPORTID-l4b-final-second-1000vu-t5
FINAL_WAIT_SECONDS=1800 bash tests/pts/verify-l4b-pts-correctness.sh after-REPORTID-l4b-final-second-1000vu-t30
```

The script writes `l4b-invariant-gates.tsv` and exits non-zero on any P0 gate
failure. It is intentionally allowed to report P1 follow-up gates separately.
P9 proxy/max-bid integrity is not part of the PTS-1 manual-bid workload; it must
be covered by a dedicated max-bid workload because proxy bidding changes product
semantics and event privacy.

## Risk Coverage Cross-Check

| Risk | PTS-1 Automatic Gate | Status |
|---|---|---|
| Redis/Kafka/PG/Outbox convergence | Preflight: Kafka topic/index guards. Post-run: `redis_kafka_pg_accepted_match`, `auction_accepted_count_matches_pg`, `outbox_drained` | P0/P1 automated |
| Redis accepted but Kafka append lost | Preflight: pending write/delete/recovery guards. Post-run: `redis_pending_decisions_empty` plus settlement/Kafka-position gates | P0 automated; crash-before-append recovery still needs fault injection |
| Money/order uniqueness and cap race | Preflight: engine seq/order indexes. Post-run: `at_most_one_order`, `increment_grid_valid`, `no_accepted_after_final_end` | P0 automated |
| Seq continuity and audit recovery | Preflight: DB unique engine seq indexes. Post-run: `no_accepted_bid_seq_gap`, `no_auction_event_seq_gap` | P0 automated |
| Clock/order inversion | Post-run: `no_created_at_seq_inversion` | P0 automated |
| Consumer offset and settlement correspondence | Preflight: Kafka all-acks/sync/key and offset index. Post-run: `kafka_position_present`, `kafka_offset_matches_engine_order`, `kafka_consumer_group_lag_zero`, consumer-group offset snapshot | P0 automated plus P1 lag gate |
| Split-brain stale writer | Preflight: stale epoch rejection and CAS update guards. Post-run: `engine_epoch_seq_monotonic` | P0 automated for data; network partition requires fault injection |
| Redis eviction | Preflight and post-run: `redis_no_eviction`, `redis_noeviction_policy` | P0 automated |
| Kafka reordering | Preflight: `auction_id` message key and all-acks sync writer. Post-run: `kafka_offset_matches_engine_order` | P0 automated |
| Cross-auction contamination | Post-run: `no_cross_auction_event_payload_leak` | P0 automated |
| DLQ | Preflight: DLQ topic exists. Post-run: `dlq_empty` | P0 automated |

Fault-injection workloads still required before a release-level resilience claim:

- Redis kill/restart while `bid:{auction}:engine:pending` is non-empty;
- Kafka consumer restart/rebalance under active settlement lag;
- broker or app crash after Redis Lua decision and before Kafka append;
- network partition/stale Redis engine epoch test;
- dedicated P9 max-bid/proxy-bid workload when proxy bidding is in scope.

This table is not a claim that the system has already survived those faults.
It states whether PTS-1 can detect the bad state if it appears during a normal
final-second run. Crash/restart, rebalance, network partition, and proxy-bid
privacy require separate workloads because a normal PTS run does not create
those failure modes.

## Why Both VU And RPS Modes Are Needed

Alibaba Cloud PTS exposes two useful pressure models:

- VU mode: client-side concurrency/session behavior. Use it when the question is
  "how many users are simultaneously online or act at the same time?"
- RPS mode: server-side request arrival rate. Use it when the question is "at
  what request rate does this endpoint or subsystem saturate?"

For this project, VU mode is the right first choice for final-second auctions,
watchers, reconnect storms, and slow consumers. RPS mode is the right follow-up
for sustained bid/outbox throughput and snapshot endpoint saturation because it
keeps offered load explicit even when response time increases.

## Workload Priority

| Priority | Workload | Main Challenge / Score Dimension | PTS Mode | Why It Matters First |
|---:|---|---|---|---|
| 1 | PTS-1 final-burst single-hot-auction | Core challenge: final-second bid correctness; backend service; performance | VU | One-shot 1000VU bid burst against one auction. This validates the new L4B/Kafka hot path without turning the run into sustained looping pressure. |
| 1B | PTS-1B true last-second soft-close sniper | Core challenge: soft close, cap/end race | VU | Same one-shot shape, but `end_at` is aligned with the barrier so bids land in the final 1-5 seconds. This is the explicit hammer-boundary test. |
| 2 | PTS-2 sustained accepted-bid/outbox saturation | Performance; stability; observability; data governance | RPS or parameterized JMeter thread pacing | Finds the real throughput knee: Redis command latency, Kafka append latency, settlement lag, DB writes, outbox/backlog, and runtime CPU/GC. |
| 3 | PTS-3 watcher fanout overlay | Millisecond realtime sync; frontend interaction; system availability | VU / connection oriented | Runs with real bid events when possible. Tests whether accepted bid events reach many watchers without fanout lag, slow closes, goroutine/RSS growth, or queue buildup. |
| 4 | PTS-4 reconnect storm with stale seq | Recoverable realtime; availability; stability | VU | Tests Redis history/snapshot fallback, snapshot rebuild semaphore, DB pressure, and whether clients recover to authoritative state. |
| 5 | PTS-5 hot/cold multi-room isolation | Stability; core challenge optimization; observability | Mixed VU/RPS | Proves one hot auction does not corrupt or collapse cold-room latency/fanout, and that room-scoped diagnostics are useful. |
| 6 | PTS-6 admission-on overload profile | Gateway; system availability | VU or RPS | This is not capacity discovery. It proves product protection behavior only after downstream bottlenecks are known. |

## First Run: PTS-1 Final-Burst 1000VU

Use this run first because it maps directly to the official scene: a live room
where many bidders wait and then bid near the hammer.

This prepared PTS-1 is a one-shot burst, not a loop. The bid thread group has
`LoopController.loops=1`. The barrier opens around 5:30 in a 6-minute run and
the remaining time is observation/response margin. It does not intentionally
place bids in the final 1-5 seconds of `end_at`; PTS-1B owns that soft-close
sniper case.

Artifacts:

- JMX: `tests/pts/live-auction-l4b-final-second-1000vu.jmx`
- CSV: `docs/perf/pts/pts_l4b_final_second_1000vu_sessions.csv`
- Reset: `tests/pts/reset-l4b-final-second-pressure.sh`
- Runbook: `docs/perf/pts/l4b-final-second-1000vu-runbook.md`

PTS UI:

| Setting | Value |
|---|---|
| Pressure mode | Virtual users |
| Traffic model | Manual speed or uniform ramp-up |
| Maximum virtual users | 1000 |
| Test duration | 6 minutes |
| Ramp-up duration | 1 minute |
| Specify loop count | No |
| Specified IP count | Default unless PTS quota requires otherwise |

The JMX barrier opens at about 5 minutes 30 seconds. The ramp-up must complete
well before then; otherwise the report is not a true 1000-user simultaneous bid
burst.

Primary verdict:

- `PASS`: no admission contamination, no server errors, 1000 bid attempts are
  classified, invariants hold, and latency is within the measured target for
  this environment.
- `BOTTLENECK_FOUND`: Redis/Kafka/DB/runtime metrics identify the first limiting
  subsystem.
- `HARNESS_GAP`: PTS did not actually start/hold 1000 users before the barrier,
  CSV/auth failed, or response classification is ambiguous.
- `ENV_LIMIT`: PTS client, network, ECS, Docker, file descriptors, CPU, or disk
  saturates before backend bottleneck metrics move.

Required after-run checks:

- PTS sampler distribution and p50/p90/p99 for `POST PTS-1 hotspot bid`;
- zero HTTP 429 / `RATE_LIMITED` / `BID_AUCTION_TOO_HOT` delta;
- accepted/rejected distribution from `auction_bid_redis_ledger_total`;
- `auction_bid_redis_ledger_seconds`;
- Kafka bid topic and DLQ status;
- settlement rows and status counts;
- Redis latency/memory/evictions/blocked clients;
- DB pool/lock/slow query snapshot;
- invariant check: seq continuity, no duplicate idempotency response, one final
  winner/order if cap is reached, no unreconciled accepted Redis ledger rows.

## Second Run: PTS-2 Sustained Accepted-Bid Saturation

Run this after PTS-1 because a one-shot burst may show final-second latency but
does not reveal the sustained throughput knee or backlog growth.

Target:

- Redis Lua ledger command latency;
- Kafka append latency and topic backlog;
- settlement worker lag;
- PostgreSQL write pressure;
- outbox/fanout lag;
- Go CPU/GC/goroutines.

Recommended shape:

- start with 100 RPS equivalent accepted bid attempts for 2 minutes;
- step to 200, 400, 600, 800, and 1000 if error and lag stay controlled;
- stop at the first sustained growth in latency, Kafka backlog, settlement lag,
  Redis latency, DB pool wait, or runtime saturation.

Use RPS mode if PTS can parameterize the JMeter plan cleanly. If not, use the
existing parameterized JMeter core-pressure plan with controlled thread count
and bid pacing, but label it as closed-model pressure.

## Third Run: Realtime And Recovery

Only after the bid path is characterized should PTS focus on realtime:

- watcher fanout with many sockets and a low accepted bid stream;
- reconnect storm with stale `last_seq`;
- slow consumer mix where some clients stop reading.

These runs are scored heavily because the official challenge is not only "accept
bids fast"; the browser must converge back to server-authoritative state under
disconnects, gaps, and backpressure.

## Evidence Discipline

Every PTS run must record:

- commit SHA;
- JMX and CSV path;
- PTS report id;
- PTS mode and traffic model;
- backend env proving `ADMISSION_ENABLED=false` for downstream pressure;
- before/after `collect-server-evidence.sh` directories;
- raw report details from `aliyun pts get-jmeter-report-details`;
- verdict: `PASS`, `BOTTLENECK_FOUND`, `HARNESS_GAP`, or `ENV_LIMIT`;
- exact next workload or diagnostic.
