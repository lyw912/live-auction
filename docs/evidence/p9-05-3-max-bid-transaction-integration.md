# P9-S5-3 Max Bid Transaction Integration Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S5-3 row-lock automatic bid settlement<br>
> ADR: `docs/adr/p9-04-max-bid-pre-bid-decision.md`

## Changed

Implemented server-side Max Bid/Pre-Bid settlement inside the existing PostgreSQL auction row-lock transaction:

- accepted manual bids can trigger automatic competing bids before the triggering idempotency record is completed;
- `Start` activates scheduled pre-bid intents in the same transaction as `auction_started`;
- every automatic public price change writes a real `bids` row with `source=AUTO_MAX_BID`;
- every automatic price change writes the normal authoritative `auction_events` and `outbox_events` rows;
- automatic cap bids reuse the existing SOLD/order path and create one order;
- exhausted and terminal intents are marked in PostgreSQL inside the settlement transaction;
- triggering manual bid idempotency replays the final authoritative price/winner after automatic settlement.

The public event payload may include `bid_source=AUTO_MAX_BID`, but does not include `max_amount_cents` or private intent IDs.

## Validation

```text
go test ./internal/auction
go test ./internal/realtime -run TestServeWSReceivesOutboxFanoutWhileConnected -count=1 -v
go test -p 1 ./...
```

Result: PASS.

Additional note:

```text
go test ./...
```

The normal parallel full run still exposes an existing shared-resource realtime timeout in `TestServeWSReceivesOutboxFanoutWhileConnected`; the test passes when run directly and the serial full backend suite passes.

Covered:

- manual accepted bid triggers automatic Max Bid response under the same transaction;
- pre-bid activates on auction start;
- equal max amounts preserve deterministic earlier-intent winner;
- automatic cap bid creates `SOLD` and exactly one order;
- automatic public payloads do not leak private max amount or intent ID;
- manual idempotent replay returns the final automatic-settlement state without duplicating automatic bid rows.

## Review

- PostgreSQL remains auction truth; no Redis or frontend settlement was added.
- Automatic bids are committed only with bid row, event, outbox, and auction mutation in the same transaction.
- Automatic bid rows do not create user-facing idempotency records; their `request_hash` is an internal deterministic audit hash derived from auction, intent, client bid ID, amount, and seq.
- Public WebSocket/outbox payloads expose only public price movement and `AUTO_MAX_BID` source classification.

## Known Limits

- Private/user-scoped event model is still pending P9-S5-4.
- H5 Max Bid disclosure and controls are still pending P9-S5-5.
- PC aggregate/audit surfaces are still pending P9-S5-6.
- Max Bid fat-finger and churn abuse coverage remain pending P9-S5-7.
