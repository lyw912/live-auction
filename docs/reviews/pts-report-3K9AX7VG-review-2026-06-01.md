# PTS Report 3K9AX7VG Review

> Date: 2026-06-01
> Classification: HARNESS_GAP
> Workload: intended L2-P1 bid + WebSocket fanout

## Verdict

`3K9AX7VG` is not valid L2-P1 evidence. It does not prove that the 8c32G host,
Redis/Kafka/PostgreSQL design, or L1-F optimization damaged bid performance.

The run reached the WebSocket open path, but it did not execute the intended
1000-bid burst. The 27-30s response time shown by PTS was the JMeter WebSocket
sampler dwell/heartbeat window, not user-visible bid latency.

## Evidence

PTS report details:

- Report: `L2-P1-20260601232332`
- Runtime: 2026-06-01 23:23:32 to 23:24:39
- AgentCount: 18
- VUM: 8630
- Only reported sampler: `open and hold WebSocket viewer`
- Sampler count: 7992
- AvgRt: 27206 ms
- No `POST L2-P1 hot bid` sampler in the report details

PTS sampling logs:

- File: `docs/perf/pts/evidence/incoming/3K9AX7VG/pts-sampling-logs/sampling-logs.jsonl`
- Sampled rows: 79
- All rows are `open and hold WebSocket viewer`
- p99 elapsed: 27657 ms
- Response message: `WS_MESSAGES_1`

Server evidence:

- File: `docs/perf/pts/evidence/incoming/3K9AX7VG/postgres-summary.txt`
- `bids=1`, `accepted=1`, `rejected=0`
- `auction_bid_redis_ledger_total{result="ENGINE_ACCEPTED"} 1`
- `auction_bid_gateway_stage_seconds_sum{stage="total"} ~= 4.5ms` for the single bid
- `auction_ws_heartbeat_timeout_total 7993`
- `auction_ws_recover_total{result="history"} 7992`

Server log count for the run window:

- `POST /api/auth/ws-ticket`: 7992
- `GET /ws`: 7992
- `POST /api/auctions/auc_live/bids`: 0 in the 23:23-23:25 L2-P1 window

Host state after the run was idle: CPU, memory, Redis, and PostgreSQL were not
saturated. There was no evidence of DB lock wait, Redis overload, Kafka backlog,
or hot bid path saturation.

## Root Cause

Two harness issues caused the misleading result:

1. The L2-P1 WebSocket sampler held the socket inside one JSR223 sampler. JMeter
   reports sampler elapsed time, so PTS displayed the long-connection hold /
   heartbeat close duration as "response time".

2. The bid window was delayed twice: `bid_delay_sec=35` delayed the bidder
   ThreadGroup, and `burst_wait_ms=35000` delayed the actual bid sampler again.
   The intended bid burst therefore landed around 70s after start. The run was
   manually stopped at 67s because the PTS panel already showed ~30s response
   time, so the formal bid burst did not run.

## Fix Applied

`tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx`:

- Set default `bid_delay_sec=0`.
- Keep `burst_wait_ms=35000` as the only bid barrier after WebSocket ramp.
- Added `SampleResult.samplePause()` / `sampleResume()` around WebSocket hold so
  PTS RT represents open/auth/connect cost, not viewer dwell time.
- Added explicit ping/pong frame request handling in the Java WebSocket listener.

`docs/perf/pts/l2-l4-upload-and-pressure-config.md`:

- Updated L2-P1 properties to `bid_delay_sec=0`, `burst_wait_ms=35000`.
- Documented that WebSocket dwell time must not be interpreted as request RT.
- Documented that a run with no `POST L2-P1 hot bid` sampler is harness-invalid.

## Next Valid Run Gate

The next L2-P1 run must satisfy all of these before performance is interpreted:

- PTS report contains both `POST L2-P1 hot bid` and `open and hold WebSocket viewer`.
- Server DB shows exactly 1000 bid attempts after correctness verification.
- Sampling logs for `POST L2-P1 hot bid` have p99 <= 100ms.
- WebSocket sampler RT is no longer around the hold duration.
- Server metrics show no admission contamination: `auction_admission_enabled 0`
  and no dominant `RATE_LIMITED`.

