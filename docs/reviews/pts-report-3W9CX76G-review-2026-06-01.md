# PTS Report 3W9CX76G Review

> Date: 2026-06-01
> Classification: HARNESS_GAP
> Workload: intended L2-P1 bid + WebSocket fanout

## Verdict

`3W9CX76G` is not valid L2-P1 evidence because the WebSocket cohort was gone
before the bid burst. It is useful diagnostic evidence:

- The bid hot path stayed fast under the partial L2-P1 run.
- The Java HttpClient WebSocket load generator did not keep 8000 sockets alive
  past the server heartbeat window.
- PTS ended because the configured one-loop JMeter thread groups finished after
  their samplers completed; this is expected PTS/JMeter behavior, not a backend
  crash.

## What Happened

PTS report:

- Start: 2026-06-01 23:51:18
- End: 2026-06-01 23:52:12
- AgentCount: 18
- VUM: 11498
- Report detail count:
  - `POST L2-P1 hot bid`: 55
  - `open and hold WebSocket viewer`: 7992

Sampling logs are more complete than the report summary:

- File: `docs/perf/pts/evidence/incoming/3W9CX76G/pts-sampling-logs/sampling-logs.jsonl`
- `POST L2-P1 hot bid`: 983 samples, p99 25ms, max 30ms
- `open and hold WebSocket viewer`: 6407 sampled rows, p99 3632ms

Server evidence:

- File: `docs/perf/pts/evidence/incoming/3W9CX76G/postgres-summary.txt`
- `bids=983`, `accepted=5`, `rejected=978`
- `auction_bid_redis_ledger_total{ENGINE_ACCEPTED}=5`
- `auction_bid_redis_ledger_total{ENGINE_REJECTED}=978`
- `auction_ws_heartbeat_timeout_total=7992`
- `auction_ws_connections{room="room_main"}=0` at collection

## Root Cause

L2-P1 still had the bid barrier too late for the current Java load-generator WS
behavior:

- WS opened successfully.
- Server heartbeat is configured as 20s interval + 5s timeout.
- Java load-generator sockets all closed on heartbeat timeout.
- The bid burst started around 35s after run start.
- Therefore fanout was not measured during active WS occupancy.

The scene auto-ended because PTS/JMeter had one-loop thread groups. Once the WS
samplers ended and the bid samplers completed, there was no remaining active work
to hold the scenario open.

## Fix Applied

`tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx`:

- `burst_wait_ms` default changed from `35000` to `15000`.
- `bid_delay_sec` remains `0`.

This makes the one-shot bid burst happen while the WS cohort is connected and
before the current load-generator heartbeat issue closes sockets.

`docs/perf/pts/l2-l4-upload-and-pressure-config.md`:

- Updated L2-P1 properties to `burst_wait_ms=15000`.
- Documented that `3W9CX76G` is a WS-client hold issue, not a bid-path failure.

## Next Valid Run Gate

The next L2-P1 run must satisfy:

- `POST L2-P1 hot bid` sampled count close to 1000.
- Server DB bid count close to 1000.
- `POST L2-P1 hot bid` p99 <= 100ms.
- At least one WS message is observed during the bid burst, or server-side WS
  metrics/logs show connected viewers at the bid window.
- If WebSocket heartbeat timeouts still occur after the bid burst, classify that
  under WS long-hold/reconnect tests, not as L2-P1 bid latency failure.

