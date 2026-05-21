# Evidence Record

Feature/Gate: P0-15 reconnect storm snapshot bounding

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose PostgreSQL/Redis

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Realtime tests use PostgreSQL and Redis. Redis history/snapshot is cleared to force snapshot fallback. Tests call the same server recovery path used by WebSocket reconnect.

## Expected Invariant

- Many stale reconnects for the same auction share one DB snapshot rebuild through per-auction singleflight.
- Global DB snapshot rebuild concurrency is bounded by a semaphore.
- If Redis has a snapshot, the server returns it without rebuilding from DB.
- If Redis misses and rebuild capacity is saturated, the server returns `snapshot_unavailable` with `retry_after_ms` instead of stampeding DB.

## Result

PASS

## Observed Data

```text
?    live-auction/backend/cmd/server [no test files]
ok   live-auction/backend/internal/auction (cached)
?    live-auction/backend/internal/config [no test files]
ok   live-auction/backend/internal/gateway 31.823s
ok   live-auction/backend/internal/outbox (cached)
?    live-auction/backend/internal/platform/errors [no test files]
?    live-auction/backend/internal/platform/logger [no test files]
ok   live-auction/backend/internal/realtime 28.062s
ok   live-auction/backend/internal/scheduler (cached)
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

The fallback response is a backend WS message, not a frontend recovery UI. Redis snapshot hits are returned as stored; they are not relabeled stale unless a later frontend/client protocol slice requires that distinction.

## Next Action

Continue P0 with remaining failure/degradation gates or frontend H5/PC state gates.
