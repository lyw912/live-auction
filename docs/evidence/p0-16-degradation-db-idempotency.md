# Evidence Record

Feature/Gate: P0-16 degradation gates for DB lock timeout and idempotency timeout

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose PostgreSQL/Redis

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Auction integration tests hold PostgreSQL row locks and insert expired idempotency records to exercise failure paths through the real repository.

## Expected Invariant

- Holding the auction row lock causes bid execution to return `BID_RETRY_LATER`.
- Lock timeout does not create duplicate bid rows.
- Expired `PROCESSING` idempotency returns `IDEMPOTENCY_TIMEOUT`.
- Expired idempotency is marked `FAILED` with result code `IDEMPOTENCY_TIMEOUT`, avoiding indefinite retry-later.

## Result

PASS

## Observed Data

```text
?    live-auction/backend/cmd/server [no test files]
ok   live-auction/backend/internal/auction (cached)
?    live-auction/backend/internal/config [no test files]
ok   live-auction/backend/internal/gateway 8.058s
ok   live-auction/backend/internal/outbox 33.987s
?    live-auction/backend/internal/platform/errors [no test files]
?    live-auction/backend/internal/platform/logger [no test files]
ok   live-auction/backend/internal/realtime 34.026s
ok   live-auction/backend/internal/scheduler 2.027s
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

`Redis down bid limit` is not applicable yet because no bid rate-limit implementation exists. `clock-step-backward` scheduler pause is not implemented in this slice.

## Next Action

Continue P0 with remaining frontend H5/PC state gates or document explicit known limits for unimplemented failure gates before final P0 closure.
