# P9-S5-1 Max Bid Intent Storage Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S5-1 DB schema and repository for private max bid intents<br>
> ADR: `docs/adr/p9-04-max-bid-pre-bid-decision.md`

## Changed

Added the first P9-S5 implementation slice for private Max Bid/Pre-Bid storage.

Runtime scope:

- New goose migration `backend/migrations/202605270001_max_bid_intents.sql`.
- New private `max_bid_intents` table with one intent per user/auction.
- `idempotency_records.scope_type` now permits future `max_bid_intent` API idempotency.
- `bids.source` now distinguishes `MANUAL` from future `AUTO_MAX_BID`; existing manual bid inserts explicitly write `MANUAL`.
- New Go domain types and constants for intent status/source.
- New repository methods to create/update, read, cancel, and transaction-lock active private intent candidates.
- Repository validation rejects amounts below executable minimum, off increment grid, above cap, or on terminal/non-open auctions.

## Validation

```text
goose -dir migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
```

Result: PASS. Applied `202605270001_max_bid_intents.sql` to the local PostgreSQL database.

```text
go test ./internal/auction
```

Result: PASS after migration.

```text
go test ./...
```

Result: PASS.

## Review

- PostgreSQL remains the only storage for private max amounts.
- No private max amount is written to public auction events, outbox, Redis, or WebSocket paths in this slice.
- Intent create/update/cancel uses the auction row lock so future automatic settlement and cancellation share one serialization point.
- Manual bid rows now carry explicit `source = MANUAL`; no automatic bid rows are generated yet.
- This slice does not change winner, current price, order, outbox, scheduler, or WebSocket behavior.

## Known Limits

- No H5/PC API is exposed yet.
- No automatic bidding is executed yet.
- No public/private realtime event model is wired yet.
- Fat-finger confirm for max amount is deferred to the API slice.
