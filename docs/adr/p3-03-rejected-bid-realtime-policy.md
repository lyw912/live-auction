# ADR: Rejected Bid Realtime Policy

Date: 2026-05-28

## Status

Accepted for the current release track, pending fresh cloud PTS validation.

## Context

Cloud PTS report `3IVNW7TF` showed that a downstream-pressure run generated far
more rejected bids than accepted bids. Those `bid_rejected` events were written
to durable realtime outbox and amplified relay backlog even though ordinary
rejections do not change the public auction state.

The v2 architecture still requires PostgreSQL as money truth, idempotent bid
responses, and recoverable public state through auction sequence events. The
public recoverable state is price, winner, terminal state, order/payment state,
and server-authoritative auction metadata. A caller's failed bid must be
accurate to that caller, but it does not always need full-room durable replay.

## Decision

Separate rejected bids by state value:

- Accepted bids and terminal/payment/order state changes remain
  `auction_events` plus `outbox_events`.
- Ordinary non-state rejects (`BID_TOO_LOW`, `AUCTION_ENDED`,
  `AUCTION_NOT_ACTIVE`) are returned over HTTP, completed in idempotency, and
  stored in `bids` for audit and diagnostics, but are not appended to full-room
  durable realtime.
- Other policy rejects remain durable realtime for now to keep the behavior
  conservative while we gather evidence.

## Consequences

- Reject floods should produce fewer outbox events.
- H5 reconnect remains correct because ordinary reject events did not carry a
  public state mutation. Clients recover from snapshot/history of state-changing
  events.
- Monitor rejects and flight recorder still see rejected bid rows from `bids`.
- The public auction `seq` does not advance for ordinary non-state rejects.
- Capacity claims remain forbidden until a fresh cloud run measures the effect.

## Required Validation

- Rejected bid HTTP response and idempotency replay stay accurate.
- Accepted bid, winner, price, seq, and order invariants are unchanged.
- Ordinary reject does not create `bid_rejected` auction/outbox rows.
- Diagnostic views still expose rejected bid rows.
- Fresh cloud PTS compares outbox produced/s and final backlog before and after
  the policy.
