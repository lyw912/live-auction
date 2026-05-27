# P3-18 Rejected Bid Realtime Policy

Date: 2026-05-28

## Change

Implemented the conservative ADR in
`docs/adr/p3-03-rejected-bid-realtime-policy.md`.

Ordinary non-state bid rejections now:

- return an accurate HTTP business response;
- complete bid idempotency with the same response for replay;
- write a rejected row to `bids` for audit, monitor rejects, and flight
  recorder;
- do not create a full-room `bid_rejected` `auction_events` / `outbox_events`
  row.

Accepted bids, sold events, terminal events, order/payment events, and retained
policy rejects still use durable realtime.

## Why

Cloud PTS report `3IVNW7TF` found the app-owned outbox relay / realtime delivery
chain as the first bottleneck, with rejected bids dominating produced events.
Ordinary failed bids do not change public auction truth and do not need
full-room durable replay.

## Gates

Required local gates:

- `go test ./internal/auction`
- `go test ./internal/gateway`
- `go test ./internal/invariant`
- `go test -p 1 ./...`
- `pnpm exec node tests/load/validate-k6-suite.mjs`
- `git diff --check`

Required follow-up cloud gate:

- rerun PTS downstream-pressure and accepted/rejected profile separation on ECS;
- report DB-full accepted/rejected counts, outbox produced/published/pending,
  bid lock wait, and during host metrics.

## Known Limits

This evidence record proves implementation behavior, not capacity improvement.
Performance impact remains unproven until fresh cloud evidence exists.
