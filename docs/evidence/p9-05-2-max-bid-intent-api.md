# P9-S5-2 Max Bid Intent API Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S5-2 API for create/update/cancel intent<br>
> ADR: `docs/adr/p9-04-max-bid-pre-bid-decision.md`

## Changed

Added authenticated H5 APIs for the current user's private Max Bid/Pre-Bid intent:

- `GET /api/auctions/{id}/max-bid-intent`
- `PUT /api/auctions/{id}/max-bid-intent`
- `DELETE /api/auctions/{id}/max-bid-intent`

The API:

- requires active room membership for the auction;
- returns only the authenticated user's own private intent;
- requires `Idempotency-Key` for create/update and cancel;
- persists idempotent results in `idempotency_records` with `scope_type=max_bid_intent`;
- rejects same idempotency key with a different request hash;
- does not expose host/global reads of private max amounts.

## Validation

```text
go test ./internal/auction ./internal/gateway
```

Result: PASS.

Covered:

- repository idempotent PUT replay;
- repository idempotent DELETE replay;
- idempotency hash mismatch;
- gateway PUT/GET/DELETE current-user flow;
- current-user isolation;
- foreign room ACL rejection;
- missing `Idempotency-Key` rejection.

## Review

- No automatic bids are executed in this slice.
- No public auction events, outbox events, Redis records, or WebSocket payloads are added.
- API writes remain private PostgreSQL state plus idempotency records.
- ACL uses the existing room membership boundary; `room_id` is not trusted from the client.

## Known Limits

- Fat-finger confirm for max amount is not implemented yet.
- Auction transaction integration is still pending P9-S5-3.
- Private user event/snapshot disclosure is still pending P9-S5-4/P9-S5-5.
