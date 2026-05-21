# Evidence Record

Feature/Gate: P0-10 WebSocket ticket auth foundation

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose Redis 7

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Realtime ticket tests use Redis. Server routes include `/api/auth/ws-ticket` and `/ws`.

## Expected Invariant

- WS tickets are stored in Redis with TTL and consumed one time.
- Browser-compatible subprotocol format can carry `ticket.<token>`.
- WS server validates ticket scope and can replay Redis history or snapshot fallback.

## Result

PASS

## Observed Data

```text
?    live-auction/backend/cmd/server [no test files]
ok   live-auction/backend/internal/auction (cached)
?    live-auction/backend/internal/config [no test files]
ok   live-auction/backend/internal/gateway 7.290s
ok   live-auction/backend/internal/outbox (cached)
?    live-auction/backend/internal/platform/errors [no test files]
?    live-auction/backend/internal/platform/logger [no test files]
ok   live-auction/backend/internal/realtime 7.252s
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

This is a foundation sub-slice. It does not yet prove browser WS connection with an actual client, bounded send queues, hub broadcast, slow-consumer close, forged-room test, or reconnect storm controls.

## Next Action

Add actual WebSocket client tests, bounded hub queues, slow-consumer handling and forged-room coverage.
