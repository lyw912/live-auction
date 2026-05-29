# L4B Final-Second Hotspot 1000VU PTS Runbook

Date: 2026-05-29

## Purpose

This run measures single-auction final-second contention for the L4B engine:

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
| Traffic model | Uniform ramp-up |
| Maximum virtual users | 1000 |
| Test duration | 6 minutes |
| Ramp-up duration | 6 minutes |
| Specify loop count | No |
| Specified IP count | Leave default unless PTS quota requires otherwise |

The PTS page still uses a 6-minute uniform ramp for comparable report metadata.
The JMX contains the actual burst barrier: bid threads start quickly, wait for a
shared `burst_wait_ms=330000` target, about 5 minutes 30 seconds after the first
bid thread initializes, and issue one valid bid each against `auc_live`.

## Required Before Run

```bash
SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
bash tests/pts/collect-server-evidence.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
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
