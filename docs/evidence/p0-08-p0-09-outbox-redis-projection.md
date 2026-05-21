# Evidence Record

Feature/Gate: P0-08 outbox relay and P0-09 Redis snapshot/history

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose PostgreSQL 16 and Redis 7

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Integration tests use PostgreSQL outbox rows produced by auction lifecycle/bid transactions and Redis for projection keys.

## Expected Invariant

- Relay claims pending/failed delivery rows and respects same-auction head-of-line ordering.
- Relay writes Redis `auction:{id}:events` history and `auction:{id}:snapshot` projection.
- Delivery rows become `PUBLISHED` only after Redis write succeeds.
- Redis publish failure increments attempts; max attempts marks delivery `DEAD` and writes `OUTBOX_DEAD_LETTER` anomaly.
- Snapshot can be rebuilt from PostgreSQL truth and cached in Redis.

## Result

PASS

## Observed Data

```text
?    live-auction/backend/cmd/server [no test files]
ok   live-auction/backend/internal/auction (cached)
?    live-auction/backend/internal/config [no test files]
ok   live-auction/backend/internal/gateway (cached)
ok   live-auction/backend/internal/outbox 9.482s
?    live-auction/backend/internal/platform/errors [no test files]
?    live-auction/backend/internal/platform/logger [no test files]
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

This slice does not yet include a long-running relay worker, WebSocket hub publish, gap notice delivery to clients, reconnect storm controls, or frontend recovery UI.

## Next Action

Implement WebSocket ticket auth, room hub, bounded send queues, history replay and snapshot fallback.
