# Evidence Record

Feature/Gate: P0-11 WebSocket completion

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose PostgreSQL/Redis

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Realtime integration tests use `httptest` plus a real `nhooyr.io/websocket` client. PostgreSQL stores auction truth; Redis stores WS ticket, history and snapshot projection.

## Expected Invariant

- Browser WS auth succeeds with `auction.v1` plus `ticket.<token>` subprotocol.
- Invalid or reused ticket is rejected.
- Forged room/auction scope is rejected before WS accept.
- Reconnect can replay Redis history by `last_seq`.
- History gap falls back to server snapshot.
- Outbox relay publishes to Redis first, then broadcasts to connected in-process WS clients.
- Auction event queues are bounded and slow consumers are closed with `SLOW_CONSUMER` behavior.

## Result

PASS

## Observed Data

```text
?    live-auction/backend/cmd/server [no test files]
ok   live-auction/backend/internal/auction (cached)
?    live-auction/backend/internal/config [no test files]
ok   live-auction/backend/internal/gateway (cached)
ok   live-auction/backend/internal/outbox (cached)
?    live-auction/backend/internal/platform/errors [no test files]
?    live-auction/backend/internal/platform/logger [no test files]
ok   live-auction/backend/internal/realtime 27.532s
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

This slice implements a single-process hub. It is enough for local P0 correctness, but it is not a multi-instance shared fanout design. `reconnect-storm` DB rebuild bounding and frontend `out-of-order-detection` recovery remain future P0 slices.

## Next Action

Continue P0 with scheduler/end-job gates or diagnostics depending on milestone order.
