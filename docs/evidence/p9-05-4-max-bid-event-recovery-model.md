# P9-S5-4 Max Bid Event And Recovery Model Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S5-4 public/private Max Bid recovery boundary<br>
> ADR: `docs/adr/p9-04-max-bid-pre-bid-decision.md`

## Changed

Defined and implemented the Max Bid recovery boundary without adding a private WebSocket transport:

- public automatic bids remain normal `bid_accepted` or `auction_sold` events with optional `bid_source=AUTO_MAX_BID`;
- public outbox, WebSocket, Redis history, and DB-generated realtime snapshots do not include `max_amount_cents`, `max_bid_intent`, or intent IDs;
- authenticated `GET /api/auctions/{id}` now includes `max_bid_intent` only for the current user when that user has an intent;
- auction lists and another user's auction snapshot omit the private intent;
- `GET /api/auctions/{id}/max-bid-intent` remains the explicit current-user private read path.

## Validation

```text
go test ./internal/gateway ./internal/realtime
```

Result: PASS.

Covered:

- owner `GET /api/auctions/{id}` includes only that user's `max_bid_intent`;
- another active room member's snapshot does not include `max_bid_intent` or `max_amount_cents`;
- public auction list does not include private intent fields;
- DB realtime snapshot rebuild omits private Max Bid fields even when an active intent exists.

## Review

- No public room broadcast carries private Max Bid amount or intent ID.
- No new private WebSocket channel was added; that remains a separate transport decision.
- H5 recovery can explain the current user's Max Bid state through authenticated REST snapshot fields without trusting client-side simulation.

## Known Limits

- H5 controls and disclosure are pending P9-S5-5.
- PC aggregate/audit surfaces are pending P9-S5-6.
- Fat-finger/churn abuse behavior remains pending P9-S5-7.
