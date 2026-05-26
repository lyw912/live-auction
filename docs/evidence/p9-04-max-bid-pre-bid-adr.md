# P9-S4 Max Bid And Pre-Bid ADR Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S4 Max Bid And Pre-Bid ADR<br>
> Decision: `docs/adr/p9-04-max-bid-pre-bid-decision.md`

## Changed

Added an accepted ADR for Max Bid and Pre-Bid as a product-rule extension, not as a hidden optimization.

The ADR defines:

- private `max_bid_intents` model;
- H5 user-scoped intent APIs;
- PostgreSQL row-lock transaction semantics for automatic bids;
- public/private/audit event boundaries;
- deterministic conflict and tie-break rules;
- H5/PC disclosure boundaries;
- abuse and fat-finger behavior;
- required P9-S5 implementation gates.

## Review

Plan review against v2 rules:

- PostgreSQL remains price, winner, bid, event, outbox, order, and idempotency truth.
- Redis and WebSocket remain projection/delivery only.
- Public outbox payloads do not include private max amounts.
- Automatic bidding is forbidden in the client and must run under the auction row lock.
- Every actual public price change still requires bid rows, auction events, outbox events, and seq continuity.
- No performance number or capacity claim is introduced.

## Validation

```text
git diff --check
```

Result: PASS.

Docs-only slice; no runtime code changed.

## Known Limits

- P9-S4 does not implement runtime Max Bid support.
- P9-S5 must implement each ADR sub-area as a separate commit with focused tests.
- A private WebSocket channel is not accepted in this ADR; user-scoped REST/snapshot disclosure comes first.
