# Evidence Record

Feature/Gate: P0-06 bid command and P0-07 cap/extension/cancel/order

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose PostgreSQL 16

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Core migrations are applied locally. Integration tests create isolated rooms/items/auctions and execute bid/payment commands against PostgreSQL.

## Expected Invariant

- Accepted bid writes bid, auction mutation, auction event, outbox delivery and completed idempotency in one transaction.
- Executable rejected bid is persisted, idempotent and emits a reject event/outbox record.
- Same bid idempotency key plus same request returns the first response without duplicate money state.
- Cap bid sells the auction and creates a unique pending order with held deposit.
- Mock payment with the same idempotency key returns the same paid result without duplicate paid transition.
- Cancelled auction rejects later bids.

## Result

PASS

## Observed Data

```text
?    live-auction/backend/cmd/server [no test files]
ok   live-auction/backend/internal/auction 2.703s
?    live-auction/backend/internal/config [no test files]
ok   live-auction/backend/internal/gateway (cached)
?    live-auction/backend/internal/platform/errors [no test files]
?    live-auction/backend/internal/platform/logger [no test files]
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

This evidence does not yet cover high-concurrency final-second bidding, cancel-cap race under parallel execution, DB lock timeout, scheduler end jobs, outbox relay publication or Redis/WebSocket recovery.

## Next Action

Add explicit concurrency gates and then implement outbox relay plus Redis projection.
