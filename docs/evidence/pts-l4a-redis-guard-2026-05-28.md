# PTS L4a Redis Guard

Date: 2026-05-28 Asia/Shanghai

Scope: implements L4a from commit `1d31bf9 docs: add PTS1-Refactoring docs`.

## Implemented

- Added `BID_ENGINE_MODE=redis_guard` as an opt-in mode.
- `redis_guard` keeps PostgreSQL as synchronous bid truth. Redis can only reject conservative cases before the PostgreSQL lane:
  - `BID_TOO_LOW`
  - `BID_INCREMENT_MISMATCH`
  - `BID_ABOVE_CAP`
  - terminal `AUCTION_NOT_ACTIVE` for `SOLD`, `ENDED`, or `CANCELLED` projections
- Missing, stale, unavailable, busy, or malformed Redis guard state falls through to the PostgreSQL lane.
- The PostgreSQL lane still runs in `redis_guard` mode, so accepted bids, winner/order, idempotency, auction seq, events, and outbox remain DB-truth.
- Added dedicated guard projection key `bid:{auction_id}:guard:projection`.
- Outbox publish and snapshot rebuild refresh guard projection from PostgreSQL state. Projection refresh failure is counted but does not block realtime history/snapshot publication.
- Added guard metrics:
  - `auction_bid_redis_guard_total`
  - `auction_bid_redis_guard_seconds`
  - `auction_bid_redis_guard_projection_update_total`
- Added runtime knobs to `.env.example`:
  - `BID_REDIS_GUARD_MAX_STALENESS`
  - `BID_REDIS_GUARD_TIMEOUT`

## Safety Boundaries

- Redis guard does not declare winner, sold, order, settlement, or auction sequence.
- Redis guard rejects do not write bid rows, auction events, outbox rows, orders, or idempotency records.
- ACTIVE auction time/soft-close decisions remain PostgreSQL-only to avoid false rejects while a committed extension is waiting for projection refresh.
- Completed idempotency replay still happens before admission, guard, and lane.
- Guard projection is a short-lived optimization input, not a recovery or money-truth source.
- No P99/QPS improvement is claimed without a same-JMX PTS run.

## Verification

```text
go test ./internal/gateway ./internal/outbox ./internal/redisx -run "TestBidAdmission|TestPostgresBidLane|TestRedisGuard|TestRelayPublishes|TestRelayStreamEpochStableAcrossEventsAndSnapshot|TestRelayRebuildSnapshotWritesRedisSnapshot|TestScript" -count=1
```

Result: PASS.

Covered gates:

- Redis guard conservative reject before PostgreSQL lane.
- Guard reject writes no bid row.
- Missing projection falls through to PostgreSQL truth.
- Stale projection falls through to PostgreSQL truth.
- Redis unavailable falls through to PostgreSQL truth.
- Outbox publish refreshes guard projection without breaking realtime history.
- Snapshot rebuild refreshes guard projection.

## Known Limits

- L4a does not solve the accepted-bid row-lock ceiling; it reduces obvious stale/rejected pressure before PostgreSQL.
- L4b Redis ledger, command log, settlement worker, and reconciler are not implemented in this slice.
- Cloud PTS before/after evidence remains required before claiming latency reduction.
