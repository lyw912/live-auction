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
| Auction state continuity | public `auction_events.seq` is continuous and accepted/sold settlements have matching public event rows | Client recovery and ordering are unsafe. |
| Idempotency | no duplicate `client_bid_id` / idempotency conflict beyond expected replay | Load may have created duplicate logical bids. |
| Winner/order invariant | at most one final winner/order; cap/end/cancel terminal state is unique | Auction correctness broken regardless of latency. |
| Realtime projection | Redis snapshot/history and outbox/fanout lag converge after the run | Browser can show stale or missing authoritative state. |
| Reconciliation | no unreconciled accepted Redis/Kafka ledger rows after bounded settle window | L4B path lost or stranded accepted bids. |
| Engine pause | `engine_paused=false` after the run | The system fail-closed during pressure; the report is not performance evidence. |
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
| Seq continuity and audit recovery | Preflight: DB unique engine seq indexes. Post-run: `redis_engine_seq_matches_settlement`, `accepted_settlement_has_public_event`, `no_public_auction_event_seq_gap` | P0 automated |
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
| 1A | PTS-1A accepted ladder | Accepted hot path; Kafka/PG/outbox throughput under real rules | VU | 1000VU release as a small ordered ladder proves accepted throughput without disabling the low-price rule. |
| 1B | PTS-1B contention burst | Core challenge: final-window bid correctness; backend service; performance | VU | 1000VU simultaneous bid pressure proves engine order, deterministic low-price rejects, and final highest-price winner. |
| 1C | PTS-1C true last-second soft-close sniper | Core challenge: soft close, cap/end race | VU | Same one-shot shape, but `end_at` is aligned with the barrier so bids land in the final 1-5 seconds. This is the explicit hammer-boundary test. |
| 2 | PTS-2 sustained accepted-bid/outbox saturation | Performance; stability; observability; data governance | RPS or parameterized JMeter thread pacing | Finds the real throughput knee: Redis command latency, Kafka append latency, settlement lag, DB writes, outbox/backlog, and runtime CPU/GC. |
| 3 | PTS-3 watcher fanout overlay | Millisecond realtime sync; frontend interaction; system availability | VU / connection oriented | Runs with real bid events when possible. Tests whether accepted bid events reach many watchers without fanout lag, slow closes, goroutine/RSS growth, or queue buildup. |
| 4 | PTS-4 reconnect storm with stale seq | Recoverable realtime; availability; stability | VU | Tests Redis history/snapshot fallback, snapshot rebuild semaphore, DB pressure, and whether clients recover to authoritative state. |
| 5 | PTS-5 hot/cold multi-room isolation | Stability; core challenge optimization; observability | Mixed VU/RPS | Proves one hot auction does not corrupt or collapse cold-room latency/fanout, and that room-scoped diagnostics are useful. |
| 6 | PTS-6 admission-on overload profile | Gateway; system availability | VU or RPS | This is not capacity discovery. It proves product protection behavior only after downstream bottlenecks are known. |

## First Runs: PTS-1 Final-Second Evidence

Use this run first because it maps directly to the official scene: a live room
where many bidders wait and then bid near the hammer.

The final-second evidence is split into separate workloads because a real
simultaneous auction and an all-accepted hot path answer different questions.
When different prices arrive at the same time, the first high price can
correctly make later lower prices `BID_TOO_LOW`; that proves contention
semantics, not accepted throughput. The accepted ladder keeps the real
low-price rule enabled but spaces bids narrowly enough that each bid should be
valid in engine order.

Prepared JMX files are one-shot bursts, not loops. The bid thread group has
`LoopController.loops=1`. A post-bid hold sampler keeps each VU alive until
about 60 seconds so Alibaba PTS does not auto-end after a partial one-shot
cohort.

Artifacts:

- PTS-1A accepted ladder JMX: `tests/pts/pts-1a-accepted-ladder-1000vu-1m.jmx`
- PTS-1B contention/reject JMX: `tests/pts/pts-1b-contention-burst-1000vu-1m.jmx`
- CSV: `docs/perf/pts/pts-1ab-1000vu-sessions.csv`
- Reset: `tests/pts/reset-l4b-final-second-pressure.sh`
- Runbook: `docs/perf/pts/l4b-kafka/final-burst-1000vu-runbook.md`

PTS UI:

| Setting | Value |
|---|---|
| Pressure mode | Virtual users |
| Traffic model | Manual speed or uniform ramp-up |
| Maximum virtual users | 1000 |
| Test duration | 1 minute |
| Ramp-up duration | 1 minute |
| Specify loop count | Yes: 1 |
| Specified IP count | Default unless PTS quota requires otherwise |
| CSV data file | Enable Split File / 切分文件 |

For PTS-1A, use manual speed with 100% start so all 1000 virtual users are alive
before the one-shot barrier release. The current accepted ladder opens near the
final 15 seconds of a 1-minute scene and uses `burst_wait_ms=40000`,
`accepted_barrier_quantum_ms=10000`, and `accepted_ladder_step_ms=10`. The
10-second wall-clock barrier aligns multiple PTS agents; the 40-second wait
keeps the ladder away from scene teardown. The hold after the POSTs is only to
keep the PTS scene alive; it is not additional bid traffic.

The bid amount and release offset are derived from each CSV `user_id`; they do
not rely on a JMeter/JVM property counter as a global sequence. In PTS
multi-agent execution, JMeter properties are process-local, so a property
counter can generate duplicate or overlapping prices and convert an accepted-bid
workload into a reject-dominant workload. `KY3UX7QG` also proved that 1ms was
below real PTS/JMeter cross-agent scheduling precision: it produced 1000 unique
bids but 251 were correctly rejected after arrival reordering. `TR3VX7RG`
proved that a 10-second barrier quantum with a 54-second wait can also invalidate
the run by pushing all business POSTs to the 60-second scene boundary.
`WT3VX7WG` proved that lexical CSV ordering and a 1-second barrier still produce
too much harness drift for accepted-hot-path evidence. `913WX7HG` proved that
5ms still leaves too much cross-agent arrival inversion on PTS for an
accepted-dominant workload, although the backend consistency gates pass. The CSV
data file must be split across pressure agents; otherwise each agent starts at
CSV line 1 and a 1000-sample report can still produce only about 500 unique
business bids.

Expected bid sampler count is approximately 1000. A report with a much larger
`POST PTS-1 hotspot bid` count is not an extreme successful PTS-1 run; it is a
harness mismatch and belongs either to PTS-2 sustained saturation or to a failed
PTS-1 setup review.

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
- accepted/sold ratio high enough to prove this was the accepted hot-path run;
- `auction_bid_redis_ledger_seconds`;
- Kafka bid topic and DLQ status;
- settlement rows and status counts;
- Redis latency/memory/evictions/blocked clients;
- DB pool/lock/slow query snapshot;
- invariant check: public event seq continuity, accepted settlement event
  coverage, no duplicate idempotency response, one final
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
