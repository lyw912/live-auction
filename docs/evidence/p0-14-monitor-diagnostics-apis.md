# Evidence Record

Feature/Gate: P0-14 monitor diagnostics APIs

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Go 1.26.3, Docker Compose PostgreSQL/Redis

Command:

```powershell
go test ./...
```

Raw Output Path: this record

## Setup

Monitor API tests use the full gateway router with mock auth and real PostgreSQL rows produced by repository operations or direct diagnostic setup. No fake dashboard data is returned.

## Expected Invariant

- `GET /api/monitor/auctions` returns real auction state including item, price, seq, accepted count and last event time.
- `GET /api/monitor/anomalies` returns real `system_anomaly_events` rows.
- `GET /api/monitor/outbox` returns real outbox delivery rows and lag.
- `GET /api/monitor/scheduler` returns real scheduler jobs.
- Monitor endpoints are host-only.

## Result

PASS

## Observed Data

```text
?    live-auction/backend/cmd/server [no test files]
ok   live-auction/backend/internal/auction (cached)
?    live-auction/backend/internal/config [no test files]
ok   live-auction/backend/internal/gateway 31.846s
ok   live-auction/backend/internal/outbox (cached)
?    live-auction/backend/internal/platform/errors [no test files]
?    live-auction/backend/internal/platform/logger [no test files]
ok   live-auction/backend/internal/realtime (cached)
ok   live-auction/backend/internal/scheduler (cached)
?    live-auction/backend/internal/storage [no test files]
```

## Failure Interpretation

None.

## Known Limits

This slice implements backend diagnostic APIs. PC diagnostic UI pages and frontend screenshot/overlap gates remain separate P0 frontend work.

## Next Action

Continue P0 with reconnect-storm DB rebuild bounding or frontend H5/PC state gates.
