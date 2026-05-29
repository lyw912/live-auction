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

## Artifacts

- JMX: `tests/pts/live-auction-l4b-final-second-1000vu.jmx`
- CSV: `docs/perf/pts/pts_l4b_final_second_1000vu_sessions.csv`
- Reset: `tests/pts/reset-l4b-final-second-pressure.sh`

## PTS UI Configuration

| Setting | Value |
|---|---|
| Pressure mode | Virtual users |
| Traffic model | Manual or uniform ramp-up |
| Maximum virtual users | 1000 |
| Test duration | 6 minutes |
| Ramp-up duration | 1 minute |
| Specify loop count | No |
| Specified IP count | Leave default unless PTS quota requires otherwise |

The PTS page must finish bringing all 1000 VUs online before the burst barrier.
Use a 1-minute ramp-up, or manual speed control that reaches 1000 VUs by minute
1 and holds them until the burst. Do not use a 6-minute ramp-up for this
workload; that can leave late VUs unstarted when the 5:30 barrier opens.
The JMX contains the actual burst barrier: bid threads start quickly, wait for a
shared `burst_wait_ms=330000` target, about 5 minutes 30 seconds after the first
bid thread initializes, and issue one valid bid each against `auc_live`.

The bid thread group uses `LoopController.loops=1`; each VU submits exactly one
bid after the barrier. The final 30 seconds are response/settlement observation
margin, not continuous bid traffic. This run does not align `auc_live.end_at`
with the barrier; a true final-1-to-5-second soft-close sniper test is tracked as
PTS-1B in the L4B Kafka matrix.

## Required Before Run

```bash
SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
bash tests/pts/preflight-l4b-pts-guards.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
bash tests/pts/collect-server-evidence.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
bash tests/pts/verify-l4b-pts-correctness.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
```

Upload the JMX and CSV above to PTS. Do not reuse older `pts_hotspot_sessions.csv`
or `live-auction-hotspot-pressure.jmx` for this L4B final-second run.

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
