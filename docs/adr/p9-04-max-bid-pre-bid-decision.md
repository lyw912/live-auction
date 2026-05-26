# ADR P9-04 · Max Bid And Pre-Bid Architecture

Date: 2026-05-27 Asia/Shanghai

Status: Accepted

## Context

The current v2 auction core is manual fixed-increment bidding. `P3-D21` deferred proxy bidding because it changes product semantics and must not be introduced as a hidden optimization.

P9 reopens the topic as an explicit product decision: bidders may set a private maximum willingness-to-pay before or during an auction, and the server may place automatic bids on their behalf. This is not a performance shortcut. It is a new auction rule surface that must preserve the existing invariants:

- PostgreSQL remains the only authority for price, winner, bid rows, events, outbox, idempotency, and orders.
- Public WebSocket payloads must never expose a user's max amount.
- Client code must never simulate automatic bidding or decide winner/hammer.
- Every public price change still has an auditable accepted bid row, auction event, and outbox record.

## Decision

Accept Max Bid and Pre-Bid for P9-S5, implemented as private bidder intents plus server-side settlement inside the existing auction transaction boundary.

### Product Semantics

- `Max Bid` is a private user intent: "bid for me up to this integer-cent ceiling."
- `Pre-Bid` is the same private intent created while an auction is `SCHEDULED`; it becomes executable only when the auction becomes `ACTIVE`.
- A max bid is not an immediate public bid at the max amount.
- Automatic bidding advances only by the frozen auction increment grid and never above cap.
- A user may have at most one active intent per auction.
- Users can create, update, or cancel their own active intent until it is exhausted, cancelled, or the auction is terminal.
- Intent amounts are binding for automatic bids once the server executes them.

### Privacy Model

- Store max amounts only in a private PostgreSQL table, never in `auction_events.payload_json`, public `outbox_events.payload_json`, Redis room history, public snapshots, room leaderboard rows, or PC public auction lists.
- H5 may read only the current user's intent through an authenticated user-scoped endpoint.
- PC host views may show aggregate pre-bid readiness only: count of active intents and highest executable public starting pressure bucket if implemented later. They must not show user max amounts or ranked private ceilings.
- Diagnostics may expose individual intent IDs and user IDs only in host-only flight recorder/audit surfaces. Raw max amount visibility is restricted to debug/audit evidence and must not be part of normal host controls.

### Data Model

Add `max_bid_intents`:

```sql
max_bid_intents (
  id text primary key,
  auction_id text not null references auctions(id),
  user_id text not null references users(id),
  max_amount_cents bigint not null,
  status text not null,
  source text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  cancelled_at timestamptz,
  exhausted_at timestamptz,
  last_applied_seq bigint,
  version bigint not null default 0,
  unique (auction_id, user_id)
)
```

Allowed `status`: `ACTIVE`, `CANCELLED`, `EXHAUSTED`, `TERMINAL`.

Allowed `source`: `PRE_BID`, `MAX_BID`.

All amounts use integer cents. Application constants must define enum-like values.

### API Model

Add authenticated H5 APIs:

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/auctions/{id}/max-bid-intent` | Read current user's private intent. |
| PUT | `/api/auctions/{id}/max-bid-intent` | Create or update current user's private intent. |
| DELETE | `/api/auctions/{id}/max-bid-intent` | Cancel current user's private intent. |

`PUT` requires `Idempotency-Key` and body:

```json
{
  "max_amount_cents": 90000,
  "client_seen_seq": 41
}
```

The request hash scope is:

```text
scope_type = max_bid_intent
scope_id = auction_id
user_id = current user
idempotency_key = Idempotency-Key
request_hash = sha256("max-bid-intent:v1|auction_id|user_id|idempotency_key|max_amount_cents")
```

The existing `idempotency_records.scope_type` constraint must be expanded explicitly before using this scope.

### Transaction Semantics

Every automatic bid is executed only inside the same PostgreSQL row lock used by manual bids, start, end, and cancel.

When a manual bid is accepted or an auction starts:

1. Probe completed idempotency before rate/admission for the user-facing request.
2. Enter the auction transaction and set lock/statement timeouts.
3. Lock the auction row.
4. Validate current auction status, time, cap, increment, and current winner under the lock.
5. Apply the triggering manual bid if present.
6. Load competing active max-bid intents for the auction under the same transaction.
7. Compute the minimal sequence of public accepted bids needed to represent proxy competition.
8. Insert each accepted bid row with `source = AUTO_MAX_BID` and an internal client bid id derived from intent id plus auction seq.
9. Mutate auction price/winner/seq/accepted count for each public price change.
10. Write matching `auction_events` and `outbox_events` for public price changes.
11. Mark exhausted intents when their max no longer beats the public minimum next bid or terminal cap.
12. Complete idempotency for the triggering request before commit.

The algorithm must be deterministic for equal inputs. Tie-break order is:

1. Higher max amount wins.
2. Earlier intent `created_at` wins when max amounts tie.
3. Lexicographic intent id is the final stable tie-breaker.

### Event Model

Public events:

- Continue to publish `bid_accepted`, `auction_extended`, and `auction_sold` only for committed public state changes.
- Public payloads may include `bid_source = "AUTO_MAX_BID"` only if it does not identify a user's private max.
- Public payloads must include current public price, masked current winner, seq, end_at/server time as usual.

Private/user-scoped events:

- `max_bid_intent_created`
- `max_bid_intent_updated`
- `max_bid_intent_cancelled`
- `max_bid_applied`
- `max_bid_outbid`
- `pre_bid_activated`

These are delivered through user-scoped REST snapshot fields first. A dedicated private WS channel requires a separate transport design; it must not reuse public room broadcast with private payloads.

Audit events:

- Flight recorder can show that an automatic bid was applied and which intent id caused it.
- Normal host UI must not reveal max amount.

### Conflict Rules

- Manual bid by current winner remains `REJECTED_SELF_LEADING`.
- Creating/updating an intent below the current minimum executable amount returns `MAX_BID_TOO_LOW`.
- Creating/updating an intent above cap returns `MAX_BID_ABOVE_CAP`.
- Intent amount must be on the frozen increment grid from start/current price rules; otherwise return `MAX_BID_INCREMENT_MISMATCH`.
- A user with an active winning manual or automatic bid may keep an intent, but the server must not bid against themselves.
- Cancel commits before bid/start settlement means the intent is ignored.
- Bid/start settlement commits before cancel means any already written automatic bid stands.
- Terminal auction rejects create/update/cancel with `AUCTION_NOT_ACTIVE` or `AUCTION_TERMINAL` as appropriate.
- Cap bid still wins over extension; automatic cap bid creates the same SOLD/order path as manual cap bid.

### UI Disclosure

H5:

- Max Bid entry lives in a secondary sheet near Custom Bid, not in the primary CTA.
- Copy must say the max amount is private and the server will bid by increments up to that amount.
- H5 must show whether the current user's intent is active, applied, outbid, exhausted, cancelled, or terminal.
- During disconnected/recovering/pending, create/update/cancel controls are disabled.
- H5 must never show optimistic "max bid placed"; it waits for the committed API response or snapshot.

PC:

- Seller Studio can enable/display pre-bid readiness only after backend rule fields exist.
- Host UI may show neutral aggregate readiness and audit links.
- Host UI must not show user private max amounts or rank bidders by private ceiling.

### Abuse And Fat-Finger Behavior

- Intent create/update uses the same authenticated user and room membership checks as manual bidding.
- Admission may protect intent mutation APIs, but completed idempotent retries must bypass limiter counting.
- Fat-finger applies to the max amount. If the max exceeds the rule threshold, the server returns a confirm token before storing/updating the intent.
- Confirm tokens resolve the original max amount server-side and use the same idempotency key.
- Repeated intent churn can create host-only anomalies, but it must not create public shame labels.
- Automatic bids count as binding bid attempts and are included in risk/audit surfaces.

## Alternatives

| Option | Pros | Cons | Decision |
|---|---|---|---|
| Keep Max Bid deferred | Preserves simpler fixed-increment model | Leaves P9 advanced auction UX incomplete | Rejected for P9 after explicit ADR |
| Client-side proxy bidding | Easy UI demo | Violates PG truth, leaks private max, breaks recovery | Rejected |
| Redis/Lua proxy settlement | Could reduce hot-row work in theory | Moves money truth out of PostgreSQL without reconciliation proof | Rejected |
| Store max as normal bid amount | Simple schema | Leaks private willingness-to-pay and changes auction economics | Rejected |
| PostgreSQL private intents with row-lock settlement | Preserves truth, audit, recovery, privacy | More transaction complexity and tests | Accepted |

## Consequences

- P9-S5 must be split into separate commits: schema/repository, API, transaction integration, event model, H5 disclosure, PC audit, and abuse/fat-finger tests.
- The bid transaction becomes more complex and must receive focused unit/integration/concurrency coverage before UI claims.
- Public outbox and Redis history remain safe because they carry only public accepted price changes.
- Performance may degrade under many competing intents; no capacity claim is allowed until a fresh bottleneck bundle exists.
- Existing fixed-increment manual bidding remains the compatibility baseline.

## Evidence

- `03-domain-model-and-rules.md` requires PostgreSQL row-lock serialization for bid validation.
- `04-data-and-storage.md` defines PostgreSQL as auction/idempotency/event truth and Redis as projection.
- `06-realtime-and-recovery.md` requires committed events/outbox for recoverable client state.
- `19-extreme-bidding-atmosphere.md` requires Max Bid privacy and server atomic settlement.
- `20-ui-ux-redesign.md` places Max Bid as a secondary bid mode with privacy/complexity disclosure.
- `P3-D21` allowed reopening proxy bidding only through a separate product ADR with new rules and tests.

## Tests / Gates

Required before claiming runtime Max Bid support:

- DB migration test: one active intent per user/auction, enum constants, integer-cent constraints.
- Repository tests: create/update/cancel, terminal rejection, status transitions, idempotent update replay.
- API tests: ACL, auth, idempotency hash mismatch, current-user-only reads.
- Transaction tests: manual bid triggers automatic response, pre-bid activates on start, equal max tie-break, self-leading does not bid against self.
- Cap tests: automatic cap bid creates one SOLD state and one order.
- Concurrency tests: manual bid vs auto intent vs cancel/end produces continuous seq and exactly one terminal state.
- Outbox tests: every public automatic price change has event/outbox and no private max in payload.
- Recovery tests: H5 snapshot can explain current user's intent without public leakage.
- UI tests: H5 disables controls in pending/recovering/disconnected and shows no optimistic success.
- Abuse/fat-finger tests: high max requires confirm token, repeat churn is bounded/audited.

## Rollback

- Disable Max Bid APIs behind config and hide H5/PC entry points.
- Leave existing accepted bid rows/events immutable.
- Mark remaining `ACTIVE` intents as `TERMINAL` for terminal auctions or `CANCELLED` for rollback before activation.
- Do not rewrite auction history, public events, orders, or outbox rows.
