# P9-S5-6 PC Max Bid Readiness Audit Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S5-6 PC/Seller Max Bid readiness and audit visibility<br>
> ADR: `docs/adr/p9-04-max-bid-pre-bid-decision.md`

## Changed

Added a host-only Max Bid readiness surface for PC:

- `GET /api/host/auctions/{id}/max-bid-summary` returns aggregate counts from PostgreSQL `max_bid_intents`;
- the response includes active, Pre-Bid, Max Bid, applied, exhausted, and cancelled counts plus a source label;
- the response does not include `max_amount_cents`, per-user private ceilings, or bidder ranking by ceiling;
- PC Live Assist shows the aggregate readiness card and links to the auction flight recorder;
- flight recorder bid timeline payload now includes bid row `source`, so `AUTO_MAX_BID` rows are auditable as real bid rows.

## Validation

```text
go test ./internal/gateway
pnpm --filter pc-console exec tsc --noEmit
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console
```

Result: PASS.

Covered:

- host-only ACL for the Max Bid readiness endpoint;
- aggregate counts are computed from real `max_bid_intents` rows;
- endpoint JSON does not leak `max_amount_cents` or seeded private ceiling values;
- flight recorder bid rows expose `source` while still avoiding private max amount fields;
- PC Live Assist renders aggregate readiness and opens flight recorder audit;
- route-mocked PC UI contract asserts no private ceiling text appears in the readiness card or drawer.

## Review

- PostgreSQL remains the only source for Max Bid readiness.
- No host control was added for another user's private intent.
- No client-side proxy bidding, winner calculation, or hammer logic was added.
- Public realtime/private recovery boundaries from P9-S5-4 remain unchanged.

## Known Limits

- PC route-mocked coverage is UI contract coverage, not no-mock demo evidence.
- Fat-finger/churn/abuse handling for Max Bid remains pending P9-S5-7.
