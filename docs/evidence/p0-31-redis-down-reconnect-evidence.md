# Redis Down Reconnect Evidence

Feature/Gate: Redis down reconnect and snapshot degradation

Date: 2026-05-22

Commit: pending

Environment: Windows 11, Go 1.26.3, local PostgreSQL/Redis from Docker Compose

Command: `go test ./internal/realtime -count=1 -v`

Raw Output Path: terminal output in development session

## Setup

Realtime tests use PostgreSQL for auction truth and Redis for tickets/history/snapshots. The Redis-down ticket test points `TicketStore` at `127.0.0.1:1` with short timeouts to simulate unavailable Redis without stopping shared local infra.

## Expected Invariant

- WS tickets remain Redis-backed and one-time; if Redis is unavailable, ticket issue/consume fails closed.
- Reconnect recovery first tries Redis history/snapshot, then bounded DB snapshot rebuild.
- If DB snapshot rebuild capacity is saturated and no Redis snapshot exists, the server returns `snapshot_unavailable` with `retry_after_ms`.
- Snapshot rebuild saturation writes a real `SNAPSHOT_REBUILD_SATURATED` row in `system_anomaly_events`.
- Existing stale/fresh Redis snapshot behavior is not relabeled unless the rebuild path is saturated.

## Result

PASS for the implemented Redis-down/reconnect degradation evidence.

## Observed Data

- `TestTicketStoreRedisDownFailsClosed` passed: ticket issue failed while Redis was unavailable, and consume returned a Redis availability error rather than treating the token as a valid/invalid consumed ticket.
- `TestSnapshotRebuildSaturationFallsBackToStaleOrUnavailable` passed: saturated rebuild returned `snapshot_unavailable` when no Redis snapshot existed.
- The same test verified a `SNAPSHOT_REBUILD_SATURATED` anomaly was inserted for the auction.
- Realtime suite still passed browser ticket auth/reuse, forged-room rejection, history replay, snapshot fallback, outbox fanout, slow-consumer closure, and reconnect-storm singleflight bounding.

## Failure Interpretation

If ticket issue succeeds without Redis, the one-time cross-process ticket guarantee has drifted. If saturated snapshot rebuild does not write an anomaly, the diagnostic page cannot explain reconnect degradation.

## Known Limits

- This does not make new WebSocket connections work while Redis is unavailable; the current P0 behavior is fail-closed for ticket issue/consume.
- This does not implement a bid rate limiter, so `Redis down bid limit` remains not applicable.
- This is backend evidence, not a browser reconnect storm or multi-client network jitter test.

## Next Action

Keep Redis-down ticket behavior documented as fail-closed. Remaining optional P0 tightening is focused PC rule cross-field UI cases or documenting the bid-rate-limit scope adjustment.
