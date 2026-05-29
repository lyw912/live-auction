# PTS L4a Redis Guard Stale Window Repair

Date: 2026-05-29

Context: PTS report `1L29X7UG` showed the high-lane `redis_guard` profile was
still dominated by PostgreSQL row-lock and DB-pool waiting:

- `auction_bid_redis_guard_total{outcome="STALE"}` was `347259`;
- `auction_bid_lock_wait_seconds_sum` was about `40973s`;
- `db_pool_empty_acquire_wait_seconds_total` was about `236216s`.

## Change

- Redis guard now treats a stale projection as usable only for the monotonic
  safe case: if `amount_cents <= projected current_price_cents`, the request is
  rejected before PostgreSQL with `BID_TOO_LOW`.
- Stale bids that might still win continue to the PostgreSQL truth path.
- Accepted PostgreSQL bids refresh the Redis guard projection immediately after
  commit using a short best-effort Redis script.
- If the immediate refresh fails, a bounded background retry attempts two more
  short refreshes. Failure still does not change the committed bid result.
- The refresh script uses `seq` fencing, so an older post-commit refresh cannot
  overwrite a newer projection from another accepted bid or the outbox relay.
- The post-commit refresh uses a context that is not canceled by a disconnected
  HTTP client. Outbox relay remains the durable projection repair path.
- The PTS cloud preparation default returns to one bid-lane worker per auction
  with bounded queueing. The previous high-lane profile remains available only
  as an explicit diagnostic override.

## Explicit Non-Changes

- PostgreSQL remains authoritative for price, winner, seq, SOLD, order, bid
  rows, idempotency, auction events, and outbox.
- Redis does not become a ledger or settlement engine.
- Kafka is not introduced.
- SOLD order creation is not moved out of the bid transaction because the v2
  correctness gate requires terminal state, order uniqueness, event, outbox,
  and idempotency to commit atomically.
- `synchronous_commit` is not changed by default. It may be tested only as a
  labeled database experiment with crash-loss tolerance documented.

## Next Validation

Run `tests/pts/live-auction-hotspot-pressure.jmx` with the bounded-lane
`redis_guard` profile and compare against `1L29X7UG`:

- guard `STALE` vs `REJECT` vs `ALLOW`;
- `auction_bid_redis_guard_projection_update_total`;
- `auction_bid_queue_wait_seconds`;
- `auction_bid_queue_rejected_total`;
- `db_pool_empty_acquire_wait_seconds_total`;
- `auction_bid_lock_wait_seconds`;
- `auction_bid_tx_seconds`;
- outbox lag/backlog;
- seq continuity, single winner/order, and full outbox publication.
