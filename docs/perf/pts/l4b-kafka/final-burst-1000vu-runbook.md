# L4B Final-Burst Hotspot 1000VU PTS Runbook

Date: 2026-05-29

Index: `docs/perf/pts/l4b-kafka/README.md`

## Purpose

This run measures single-auction final-burst contention for the L4B engine:

- Redis Lua hot state;
- Apache Kafka durable bid ledger;
- PostgreSQL settlement truth;
- `ADMISSION_ENABLED=false`;
- `BID_ENGINE_MODE=redis_ledger`;
- no bid-lane overload protection as the target behavior.

Any HTTP 429, `RATE_LIMITED`, or `BID_AUCTION_TOO_HOT` result is pressure
configuration contamination for this run.

Any `ENGINE_PAUSED` or HTTP `409 Conflict` caused by
`REDIS_ENGINE_PENDING_KAFKA_APPEND_UNKNOWN` is not accepted performance evidence.
It means the Redis/Kafka recovery path fail-closed. After `C33WX7MG`, the
correct behavior is: reconcile drains Redis pending decisions, treats a live
pending-append lock as in-flight work, and only pauses when pending decisions
remain unrecoverable after that bounded recovery attempt.

## Artifacts

- PTS-1A accepted ladder JMX: `tests/pts/pts-1a-accepted-ladder-1000vu-1m.jmx`
- PTS-1B contention burst JMX: `tests/pts/pts-1b-contention-burst-1000vu-1m.jmx`
- Full-check 1-minute JMX: `tests/pts/live-auction-l4b-final-burst-1000vu-1m.jmx`
- Longer 2-minute JMX: `tests/pts/live-auction-l4b-final-burst-1000vu-2m.jmx`
- Legacy 6-minute JMX: `tests/pts/live-auction-l4b-final-second-1000vu.jmx`
- CSV: `docs/perf/pts/pts-1ab-1000vu-sessions.csv`
- Reset: `tests/pts/reset-l4b-final-second-pressure.sh`

## PTS UI Configuration

| Setting | Value |
|---|---|
| Pressure mode | Virtual users |
| Traffic model | Manual or uniform ramp-up |
| Maximum virtual users | 1000 |
| Test duration | 1 minute |
| Ramp-up duration | 1 minute |
| Specify loop count | Yes: 1 |
| Specified IP count | Leave default unless PTS quota requires otherwise |
| CSV data file | Enable Split File / 切分文件 |

The PTS page must bring all 1000 VUs online from the start. Use manual speed
with start percentage 100%. The pressure event itself is the one-shot release
near the final 15 seconds of the 1-minute scene, not a continuous one-minute
loop. Use PTS-1A when the claim is accepted hot path, Kafka ledger, PG
settlement, and outbox/realtime event creation. Use PTS-1B when the claim is
simultaneous contention, deterministic `BID_TOO_LOW`, engine order, and final
highest-price winner.

The CSV must be uploaded with PTS/JMeter data-file splitting enabled. Without
splitting, each pressure agent starts reading the same CSV from line 1; a
2-agent run then reports about 1000 HTTP samples but only about 500 unique
business bids because the second agent reuses the same users and idempotency
keys.

PTS-1B is a contention/reject workload. Do not use it to claim accepted-bid
throughput or settlement capacity.

The JMX contains the actual burst barrier: bid threads start quickly, wait for a
shared absolute-time barrier near `burst_wait_ms=40000`, and then issue one
valid bid each against `auc_live`. The barrier aligns to
`accepted_barrier_quantum_ms=10000` so multiple PTS agents share the same
wall-clock release boundary. The 10-second quantum is safe here because the wait
is 40 seconds, not 54 seconds. The 1-minute accepted-dominant JMX releases
accepted bids as a 10ms ordered ladder by default
(`accepted_ladder_step_ms=10`). `KY3UX7QG` proved that 1ms was below the real
PTS/JMeter cross-agent scheduling precision: it produced 1000 unique bids but
251 were correctly rejected after arrival reordering. `TR3VX7RG` proved the
opposite harness failure: `54000ms` plus a 10-second barrier quantum pushed the
first bid to the scene end and produced zero business traffic. `WT3VX7WG` proved
that a 1-second barrier and lexical CSV ordering still leave too much harness
drift for accepted-hot-path evidence. `913WX7HG` proved that the corrected
numeric CSV and 10-second barrier reached all 1000 business bids, but 5ms still
created enough PTS cross-agent arrival inversions to reject 316 lower prices.
Do not set the interval to 0ms for the
accepted-dominant run: simultaneous different-price arrivals naturally turn into
a record-high contention workload where most lower prices are correctly
rejected after a higher price wins the engine order.

Use three separate final-second evidence workloads instead of one overloaded
JMX:

- `PTS-1B` final-window contention: simultaneous VU pressure proves engine order,
  deterministic rejects, and final highest-price winner.
- `PTS-1A` accepted ladder: small ordered intervals prove accepted hot-path
  throughput without disabling the real low-price rule.
- `PTS-1C` hammer-boundary sniper: aligns `end_at` with the pressure window to
  prove soft close, cap, and end-time races.

`PTS-2` RPS capacity is a later capacity-discovery workload. It should use fixed
request-rate steps after PTS-1/1A/1B prove the business semantics.

The bid thread group uses `LoopController.loops=1`; each VU submits exactly one
bid after the barrier. A post-bid JSR223 hold sampler keeps the thread alive
until about 60 seconds so Alibaba PTS does not auto-end the scene after only a
partial VU cohort completes. The hold is not bid traffic. These PTS-1A/1B files
do not align `auc_live.end_at` with the barrier; a true final-1-to-5-second
soft-close sniper test should be a separate PTS-1C artifact.

After the run, `POST PTS-1 hotspot bid` must be close to 1000 samples. If PTS
reports tens of thousands of bid samples, the run used platform-level looping or
the wrong uploaded plan and must be discarded as `HARNESS_GAP`.

## Required Before Run

```bash
L4B_PROFILE=pts-1a SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
bash tests/pts/preflight-l4b-pts-guards.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
bash tests/pts/collect-server-evidence.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
bash tests/pts/verify-l4b-pts-correctness.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
```

Upload the selected PTS-1A or PTS-1B JMX and CSV above to PTS. Do not reuse older
`pts_hotspot_sessions.csv`, `live-auction-hotspot-pressure.jmx`, or the older
multi-thread-group/2-minute JMX files for this 1-minute PTS-1 run.

## Must Report After Run

- PTS report id;
- bid P50/P90/P99 and HTTP error classification;
- `auction_bid_redis_ledger_total`;
- `auction_bid_redis_ledger_seconds`;
- Kafka topic lag/backlog if available;
- `redis_engine_settlements` status counts;
- `bids`, `auction_events`, `outbox_delivery` counts;
- Redis info and command stats;
- DB wait/lock snapshot;
- whether any 429/rate/admission result appeared.

## Required After Run

```bash
bash tests/pts/collect-server-evidence.sh after-REPORTID-l4b-final-second-1000vu
bash tests/pts/verify-l4b-pts-correctness.sh after-REPORTID-l4b-final-second-1000vu
```

If settlement or outbox backlog is still draining, rerun the correctness gate at
T+5 minutes and T+30 minutes:

```bash
FINAL_WAIT_SECONDS=300 bash tests/pts/verify-l4b-pts-correctness.sh after-REPORTID-l4b-final-second-1000vu-t5
FINAL_WAIT_SECONDS=1800 bash tests/pts/verify-l4b-pts-correctness.sh after-REPORTID-l4b-final-second-1000vu-t30
```

Any P0 failure in `l4b-invariant-gates.tsv` blocks a performance claim even if
PTS latency looks good.

## Two-Layer Correctness Check

Layer 1 checks whether the implementation and runtime environment contain the
required protections before pressure starts:

```bash
bash tests/pts/preflight-l4b-pts-guards.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
```

Layer 2 checks whether the pressure run actually exposed any correctness breach:

```bash
bash tests/pts/verify-l4b-pts-correctness.sh after-REPORTID-l4b-final-second-1000vu
```

Both scripts exit non-zero on P0 failures. A run is not usable as performance
evidence unless both layers pass.
