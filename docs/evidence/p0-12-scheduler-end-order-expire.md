# Evidence Record

Feature/Gate: P0-12 scheduler end auction and order expiration

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose PostgreSQL/Redis

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Scheduler tests use PostgreSQL-backed `scheduler_jobs`. Auction state remains PostgreSQL truth. Jobs only trigger timed commands; every auction terminal mutation locks the auction row and writes auction event plus outbox in the same transaction.

## Expected Invariant

- `Start` creates a durable `END_AUCTION` job.
- End job with no winner transitions ACTIVE to ENDED and creates no order.
- End job with winner transitions ACTIVE to SOLD and creates one pending order.
- End job re-reads locked `end_at`; if a bid extended the auction, the job reschedules and does not hammer early.
- Expired RUNNING job lease can be reclaimed by another worker and completes only once.
- Pending order expiration transitions to `ORDER_EXPIRED` and `FORFEITED`; later mock payment rejects with `ORDER_ALREADY_EXPIRED`.
- Scheduler failures retry with staggered `next_attempt_at` and write anomaly on DEAD.

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
ok   live-auction/backend/internal/realtime (cached)
ok   live-auction/backend/internal/scheduler 2.497s
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

This slice covers scheduler correctness for auction end and order expiration. `active-race`, `narrate-race`, diagnostic monitor APIs and frontend timer behavior remain separate P0 slices.

## Next Action

Continue P0 with remaining concurrency gates and diagnostics.
