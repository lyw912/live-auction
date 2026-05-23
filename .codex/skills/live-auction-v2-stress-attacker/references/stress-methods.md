# Stress Methods Reference

## Workload Model

- Use closed/VU models for connection count, WebSocket session behavior, slow consumers, and browser-like loops.
- Use open/arrival-rate models for HTTP bid/admission/outbox pressure when the offered request rate must stay stable even as latency rises.
- If an open model cannot sustain arrival rate, distinguish SUT saturation from load-generator saturation.

## Escalation Knobs

- duration: short smoke -> 2-5 minute local stress -> longer soak only when useful.
- VUs/connections: increase until fanout, memory, socket, or generator limit appears.
- arrival rate: increase until latency, errors, retry-later, or limiter distribution changes.
- room shape: one hot auction, hot/cold room, many rooms, many cold watchers.
- event shape: accepted bids, rejected bids, cap/terminal bids, outbox poison, reconnect gap.
- failure shape: Redis down, slow consumer, relay paused, DB lock held, stale last_seq.

## Diagnostics

PostgreSQL:

- `pg_stat_activity` for wait events, active queries, blocking patterns.
- `pg_locks` for lock contention.
- `EXPLAIN (ANALYZE, BUFFERS)` for hot queries when safe on local data.
- connection pool metrics for wait and saturation.

Outbox:

- backlog by status.
- oldest READY age and max delivery lag.
- retry and DEAD counts.
- head-of-line blocked auction.
- dead/live tuple ratio and relation/index size.

WebSocket:

- connection success/failure.
- fanout lag from event commit to client receive if available.
- slow close count and reason.
- per-client or per-room queue depth.
- goroutines, heap/RSS, GC, CPU.

Redis:

- command latency.
- memory and eviction.
- blocked clients.
- fail-open/fail-closed events for rate-limit/recovery paths.

Environment:

- load generator CPU/RAM.
- backend CPU/RAM.
- Docker resource limit.
- local port or socket exhaustion.
- Windows/WSL/Linux boundary.

## Verdict Meanings

- `BOTTLENECK_FOUND`: SUT or architecture limit has evidence and next action.
- `HARNESS_GAP`: scripts, seed, metrics, or invariant checks are too weak to answer the question.
- `ENV_LIMIT`: laptop, OS, Docker, or load generator hit a limit before the SUT bottleneck is isolated.
- `NO_REGRESSION_WITH_CEILING`: no regression up to a clearly stated local ceiling, without capacity claim.
- `BLOCKED`: required service/tool/permission is unavailable.
