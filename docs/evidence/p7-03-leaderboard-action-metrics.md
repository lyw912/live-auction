# P7-03 Leaderboard Action Metrics

> Date: 2026-05-26 Asia/Shanghai  
> Classification: AUTHORITATIVE for P7-S3.

## Change

- Extended `GET /api/auctions/{id}/leaderboard` response with action-oriented fields while keeping the previous fields backward-compatible:
  - `seq`
  - `server_time_ms`
  - `gap_to_next_rank_cents`
  - `next_valid_bid_cents`
  - `state`
  - `active_bidders_30s`
  - `accepted_bids_30s`
  - `price_velocity_cents_per_min`
- Metrics are derived from PostgreSQL `auctions` and accepted `bids`; Redis and frontend state are not used as truth.
- H5 types and mocked e2e payloads now accept the v2 fields without requiring the P7-S4 RankStrip UI.

## Validation

| Command | Result |
|---|---|
| `go test ./internal/auction -run TestRepositoryLeaderboardActionMetrics -count=1` | PASS |
| `go test ./internal/auction -count=1` | PASS |
| `go test ./internal/gateway -run Leaderboard -count=1` | PASS, no matching gateway test |
| `pnpm --filter mobile-h5 build` | PASS |
| `pnpm exec playwright test --project=mobile-h5 -g "realtime leaderboard"` | PASS |
| `pnpm exec playwright test --project=mobile-h5` | PASS, 30 passed |
| `go test ./...` from `backend/` | PASS |
| `pnpm build` | PASS |
| `pnpm test:e2e` | PASS, 42 passed |

## Review

ENGINEERING GATE: PASS.

- PostgreSQL remains leaderboard truth.
- Old fields remain in the JSON shape: current price, winner, my rank, best amount, gap to leader, leader amount, accepted bidder count, and entries.
- New fields are tested with real repository writes from accepted bids, including leader/outbid state and adjacent-rank gap.
- H5 only consumes the compatible shape in S3; P7-S4 owns the visible RankStrip redesign.

## Known Limits

- `price_velocity_cents_per_min` is a 30-second accepted-bid delta scaled to one minute. It is an action metric, not a public capacity or demand claim.
- P7-S4 still owns displaying `gap_to_next_rank_cents` and `next_valid_bid_cents` near the Bid Dock.
