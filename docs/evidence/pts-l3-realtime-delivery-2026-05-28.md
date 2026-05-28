# PTS L3 Realtime Delivery Evidence

Date: 2026-05-28

Scope: implements L3 realtime delivery from commit `1d31bf9 docs: add PTS1-Refactoring docs`.

## Implemented

- Wired WebSocket heartbeat config through both `cmd/server` and router-created realtime servers:
  - `WS_HEARTBEAT_INTERVAL=20s`
  - `WS_HEARTBEAT_TIMEOUT=5s`
- Exposed heartbeat settings in `auction_realtime_config_limit` metrics.
- Heartbeat timeout now closes the socket and cancels the connection context so the per-connection write loop does not remain alive after liveness failure.
- Stale Redis snapshot recovery is classified as `redis_stale` and records `payload_json.stale=true` in `user_activity_events`, so monitor recovery rows can separate stale fallback from clean history/DB/Redis recovery.
- H5 reconnect now uses bounded exponential backoff with jitter and honors server `Retry-After` from WS ticket admission rejects.
- H5 snapshot recovery has an in-flight guard to avoid duplicate snapshot fetches on concurrent gap/countdown/reconnect signals.
- H5 dangerous actions are disabled while WebSocket is `connecting`, `recovering`, or `disconnected`.
- H5 no longer polls `/api/auctions/{id}` every 2.5s while WebSocket is connected, recovery is idle, and countdown has not expired. Snapshot fetches remain for gap, stale, disconnected, recovering, and local-zero sync paths.
- Added route-mocked H5 coverage proving healthy WebSocket state does not create periodic snapshot traffic.

## Explicit Non-Goals

- This slice does not implement Redis guard or Redis ledger.
- PostgreSQL remains auction price/winner/order truth.
- No new online-user, fanout, P99, or QPS capacity number is claimed from these local tests.
- The H5 healthy-connection test is UI contract coverage; final `1000+` online claims still require native WS load evidence.

## Validation

Commands run:

```text
go test ./internal/realtime ./internal/observability ./internal/gateway -run "TestServeWS|TestHub|TestSnapshot|TestNormalizeOptions|TestSnapshotSource|TestSetAdmissionConfig|TestBidAdmission|TestPostgresBidLane" -count=1
go test -p 1 ./...
pnpm --filter mobile-h5 build
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --grep "WebSocket|snapshot quiet|connecting|seq gap|stale snapshot|recovering and disconnected" --workers=1 --reporter=line
pnpm exec playwright test tests/e2e/atmosphere-engine.spec.ts --project=mobile-h5 --reporter=line
```

Result: all passed.

## Remaining Evidence Gate

Before claiming `1000+` single-room online support, run a native WS fanout/reconnect profile that records connection success, fanout lag, close reasons, RSS, goroutines, file descriptors, and recovery source distribution.
