# Evidence Record

Feature/Gate: P0-13 concurrency gates and narration focus

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose PostgreSQL/Redis

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Concurrency tests use PostgreSQL integration tests and the real repository methods. Auction truth remains protected by row locks plus PostgreSQL partial unique indexes.

## Expected Invariant

- `active-race`: two scheduled auctions in one room race to start; exactly one becomes ACTIVE.
- `cancel-cap-race`: cancel and cap bid racing on one auction produce exactly one terminal state.
- `concurrent-final-second`: many users bid near end; accepted bid seq is monotonic and one server-side winner remains.
- `narrate-race`: two narrate-start commands in one room produce one narrating auction.
- Narrate start/stop routes exist and write real auction events/outbox.

## Result

PASS

## Observed Data

```text
?    live-auction/backend/cmd/server [no test files]
ok   live-auction/backend/internal/auction 3.374s
?    live-auction/backend/internal/config [no test files]
ok   live-auction/backend/internal/gateway (cached)
ok   live-auction/backend/internal/outbox 17.410s
?    live-auction/backend/internal/platform/errors [no test files]
?    live-auction/backend/internal/platform/logger [no test files]
ok   live-auction/backend/internal/realtime 51.993s
ok   live-auction/backend/internal/scheduler 14.023s
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

This slice covers backend concurrency gates. Diagnostic monitor APIs, reconnect-storm DB rebuild bounding, frontend state matrix and UI overlap gates remain separate P0 work.

## Next Action

Continue P0 with diagnostics monitor APIs and real anomaly/outbox/scheduler panels.
