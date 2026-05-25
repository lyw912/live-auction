# P4-02 Auction Flight Recorder

> Date: 2026-05-26 Asia/Shanghai  
> Status: AUTHORITATIVE for P4-R2 forensic auction timeline.  
> Scope: host-only monitor API for one auction's rules, bids, events, outbox delivery, orders, payment events, snapshots, and anomalies.

## What Changed

Added a host-only flight recorder endpoint:

```text
GET /api/monitor/auctions/{auction_id}/flight-recorder?limit=50&timeline_limit=100
```

It returns:

- `summary`: current auction row, item, room, price/winner, seq, status, rule version.
- `rules`: all rule versions for the auction.
- `orders`: order truth rows for the auction.
- `payment_events`: provider/payment boundary events joined through order.
- `anomalies`: auction-scoped anomaly rows.
- `timeline`: unified chronological rows from `auction_events`, `bids`, `outbox_events/outbox_delivery`, `orders`, `payment_events`, `system_anomaly_events`, and `snapshot_rebuild_events`.

The timeline is intentionally DB-truth first. Redis/WebSocket projections remain delivery/recovery surfaces, not money truth.

## Why This Matters

P4-R1 answers whether invariants still hold. P4-R2 answers why a contested auction ended as it did.

During future stress, abuse, or Linux baseline runs, the verifier can say `FAIL` or `PASS`, while the flight recorder can show:

- which bid row and auction event advanced a seq;
- which reject reason was recorded;
- whether outbox delivery was READY, ACK_PENDING, ACKED, NAK_RETRY_WAIT, or TERM;
- whether payment/order rows match terminal auction state;
- whether snapshot rebuilds or anomalies explain recovery behavior.

## Evidence

Commands run:

```powershell
cd backend
go test -count=1 ./internal/gateway
```

Result:

- `go test -count=1 ./internal/gateway`: PASS.

The integration test proves:

- host-only access for `/api/monitor/auctions/{id}/flight-recorder`;
- summary contains the requested auction id;
- rules are returned from `auction_rules`;
- anomalies are returned from `system_anomaly_events`;
- timeline contains real `auction_event`, `bid`, `outbox`, `anomaly`, and `snapshot_rebuild` rows.

## Boundaries

This does not introduce proxy bidding or any new auction rule. It is an observability and evidence tool over the existing fixed-increment model.

`P4-R4 Optional Proxy-Bid ADR` is deferred because proxy bidding changes product semantics from manual fixed-increment bidding to automatic max-bid bidding. The official project brief and current rule docs require fixed increment bidding, so proxy bidding is not a hidden optimization task.
