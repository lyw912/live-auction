# 03 · Domain Model And Rules

## Entities

| Entity | Meaning |
|---|---|
| Room | live room owned by host |
| Item | auctionable product |
| Auction | one item under a rule set and state machine |
| AuctionRule | frozen rule version used by bids |
| Bid | accepted or executable rejected bid attempt |
| AuctionEvent | ordered authoritative event for clients/replay |
| OutboxEvent | committed delivery record |
| Order | SOLD auction winner payment mock |
| SchedulerJob | durable timed command |
| ChatMessage | social/non-critical room message |
| UserActivityEvent | analytics/user behavior/audit |
| SystemAnomalyEvent | operational issue |

## Auction States

```text
DRAFT
  -> SCHEDULED
  -> ACTIVE
  -> SOLD
  -> ENDED
  -> CANCELLED
```

Order states:

```text
SOLD creates ORDER_PENDING
ORDER_PENDING -> PAID
ORDER_PENDING -> ORDER_EXPIRED
```

`is_narrating` is a separate focus flag. It does not affect bid legality except for H5 focus rules.

## Legal Transitions

| From | To | Actor | Guard |
|---|---|---|---|
| DRAFT | SCHEDULED | host | valid rules |
| SCHEDULED | DRAFT | host | not started |
| SCHEDULED | ACTIVE | host/scheduler | no other ACTIVE in room |
| ACTIVE | SOLD | bid/end job | cap bid or end with winner |
| ACTIVE | ENDED | end job | end_at reached and no winner |
| ACTIVE | CANCELLED | host | cancel commits before terminal |
| DRAFT/SCHEDULED | CANCELLED | host | allowed |
| SOLD/ENDED/CANCELLED | any | none | terminal, reject |

## Rule Validation

```text
start_price_cents >= 0
increment_cents > 0
cap_price_cents IS NULL OR cap_price_cents >= start_price_cents + increment_cents
cap_price_cents IS NULL OR (cap_price_cents - start_price_cents) % increment_cents == 0
duration_seconds BETWEEN 30 AND 86400
extend_window_seconds BETWEEN 10 AND 30
extend_by_seconds BETWEEN 10 AND 30
max_extend_count BETWEEN 1 AND 10
fat_finger_threshold_cents IS NULL OR fat_finger_threshold_cents > increment_cents
```

If cap is invalid, return:

```json
{
  "code": "INVALID_AUCTION_RULE_CAP_UNREACHABLE",
  "violations": ["cap_price_cents must satisfy (cap-start) % increment == 0"],
  "suggested_caps": [30000, 40000]
}
```

## Bid Validation Under Row Lock

Order:

1. auction exists and belongs to room.
2. status is ACTIVE.
3. `server_now <= end_at`.
4. user is not current winner.
5. amount is above current price or start threshold.
6. amount matches increment grid.
7. amount <= cap.
8. fat-finger guard.
9. allocate seq and mutate.

Reject priority is defined in `05-api-contracts.md`.

## 0 Start

If `start_price_cents = 0` and no accepted bid:

```text
minimum accepted amount = increment_cents
```

If start is non-zero and no accepted bid:

```text
minimum accepted amount = start_price_cents + increment_cents
```

## Increment Grid

For no accepted bid:

```text
(amount - start_price_cents) % increment_cents == 0
amount >= start_price_cents + increment_cents
```

For existing bid:

```text
(amount - current_price_cents) % increment_cents == 0
amount >= current_price_cents + increment_cents
```

## Cap

Rules:

- `amount == cap_price_cents` -> `ACCEPTED_SOLD`.
- `amount > cap_price_cents` -> `BID_ABOVE_CAP`.
- cap must be reachable from start/increment at rule creation.
- cap bid wins over extension.

## Extension

Inside final window:

```text
if end_at - server_now <= extend_window_seconds
  and extend_count < max_extend_count:
    new_end_at = max(current_end_at, server_now + extend_by_seconds)
```

If cap bid also happens, `auctions.end_at` is not updated because auction becomes SOLD. The computed extension candidate may be stored only in event payload for audit.

## Cancel Races

All money transitions lock the same auction row. Commit order is authority:

- cancel commits first -> later bid/end job rejects.
- cap bid/end commits first -> later cancel rejects.
- impossible final state: `SOLD + CANCELLED`.

## END_AUCTION Race With Extension

`END_AUCTION` job must re-read after lock:

```text
if now < auction.end_at:
  reschedule to auction.end_at and do not hammer
```

Never trust stale job `run_at`.

## Rule Edit Race

Rules freeze at SCHEDULED. Concurrent PATCH and START must produce either:

- PATCH first, then START uses new frozen rule;
- START first, PATCH returns `RULE_FROZEN_AFTER_SCHEDULED`.

No mid-edit state.

## Narrating

Rules:

- one room has at most one `is_narrating=true`.
- if room has ACTIVE auction, narrating must point to that same auction.
- when ACTIVE ends/cancels, app clears narrating explicitly.
- no DB trigger side effects.

## Deposit

Deposit exists only to complete official H5/order UI.

```text
raw = amount_cents * deposit_bps / 10000
floor = min(deposit_floor_cents, amount_cents / 2)
cap = min(deposit_cap_cents, amount_cents / 2)
deposit = min(max(raw, floor), cap)
```

Default:

- deposit_bps = 1000
- floor = 10000 cents
- cap = 100000000 cents

State:

- order created -> HELD
- pay mock success -> REFUNDED
- order expire -> FORFEITED

## Derived Values

| Value | Source |
|---|---|
| accepted_bid_count | incremented in bid transaction, reconciled from bids |
| round_count_total | equals accepted bid count for display |
| participant_count | approximate from distinct bidders + presence, not money-critical |
| current winner display | masked server-side |

Add reconciliation checker for accepted_bid_count vs accepted bids.
