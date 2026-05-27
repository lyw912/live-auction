# P3-18 Rejected Bid Realtime Policy Review

Date: 2026-05-28

## Findings

No blocking issues found in the reviewed diff.

## Review Scope

- `backend/internal/auction/bid.go`
- `backend/internal/auction/bid_integration_test.go`
- `docs/adr/p3-03-rejected-bid-realtime-policy.md`
- `docs/perf/cloud-server/09-current-code-reconciliation.md`
- `docs/evidence/p3-18-rejected-bid-realtime-policy.md`
- `docs/design-v2-industrial/06-realtime-and-recovery.md`

## Checks

- PostgreSQL remains bid/idempotency truth.
- Ordinary `BID_TOO_LOW`, `AUCTION_ENDED`, and `AUCTION_NOT_ACTIVE` rejects
  still produce accurate HTTP responses and completed idempotency records.
- Ordinary rejects remain visible through `bids`, monitor rejects, and flight
  recorder timelines.
- Ordinary rejects no longer append `bid_rejected` to full-room
  `auction_events` / `outbox_events`.
- Accepted bids, sold events, terminal events, order/payment events, and retained
  policy rejects still use durable realtime.
- Public auction `seq` does not advance for ordinary non-state rejects.

## Tests

Passed:

- `go test ./internal/auction`
- `go test ./internal/gateway`
- `go test ./internal/invariant`
- `go test -p 1 ./...`
- `pnpm exec node tests/load/validate-k6-suite.mjs`
- `git diff --check`

## Residual Risk

Cloud performance impact is unproven until a fresh ECS/PTS run compares
accepted/rejected distributions and outbox produced/published/backlog metrics.
