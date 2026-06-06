# Current Architecture Contract

> Status: governing architecture contract for hot bidding, 2026-06-05.

## Why PG Lane Is Not The Hot Path

The PostgreSQL row-lock lane was a defensible correctness baseline. It serializes money state in one transaction and is easy to audit. It is not a viable PTS-1B hot path because 1000 final-second bidders compete on the same auction row. Queueing, pool tuning, shorter transactions, and admission reduce collapse but cannot turn one-row serialization into sub-50ms p99 for a burst of meaningful decisions.

Therefore PostgreSQL is no longer the synchronous decision point for the optimized manual-bid hot path. It is still mandatory for settlement, orders, audit, diagnostics, and recovery.

## Hot Path Components

| Component | Current role | Must not do |
|---|---|---|
| Gateway | auth, room/ACL, admission, request decoding, fast idempotency, response contract | perform repeated PG reads on every hot bid if Redis/cache can safely answer |
| Redis Lua engine | atomic live decision, current price/winner/end, engine_seq, engine idempotency | depend on stale PG snapshots during the hot decision |
| Redis Stream | synchronous `ENGINE_DURABLE` decision log/idempotency replay boundary; all Lua KEYS use `{auctionID}` hash tag (Cluster-ready) | be treated as safe if AOF/no-eviction/retention is not proven |
| Kafka | ordered relay/WAL; default `KAFKA_ACKED` response boundary via group-commit latch | be optional for post-run correctness, relay, and fault evidence |
| PostgreSQL | settlement truth, audit truth, order truth, long-term query truth | be the contended synchronous row-lock decision point for PTS-1B |
| Settlement worker | replay Kafka decisions to PG and outbox | invent decisions not present in the engine/WAL |
| Reconciler | compare Redis/Kafka/PG/outbox and repair or pause | hide uncertainty from users/operators |
| WebSocket/outbox | deliver server-authoritative state and recovery hints | fabricate winner/price from client state |

## Decision Rules

The engine must be able to explain every decision from local decision-time state:

- Accept if amount is on grid, meets current minimum, auction is active, user is allowed, idempotency is fresh, cap/end rules allow it, and the engine can record the decision.
- Reject as low only if amount is strictly below the current required price/minimum at the moment of the Redis Lua decision.
- Reject duplicate only when request hash matches an existing idempotency record; mismatched hash must be a conflict, not replay.
- If cap is reached, the engine must transition terminal state once and fence later bids.
- If required state is missing or uncertain, pause/reconcile or route through an explicitly documented safe fallback. Do not guess.

## Redis Loss Is Not "Just Rebuild Later"

For high-value auctions, rebuild is a recovery mechanism, not a user-facing excuse. If Redis state is missing, stale, or lost:

1. Stop accepting new hot-engine bids for the affected auction.
2. Mark the auction `RECONCILING` or `ENGINE_PAUSED`.
3. Rebuild Redis from durable Kafka high-water and PostgreSQL settlement state.
4. Verify engine epoch, engine_seq, current price, winner, terminal status, and pending settlement coverage.
5. Resume only after the verifier proves the rebuilt state is safe.

During rebuild, users must not see fake success. They should see a bounded recovering/paused state and be prevented from placing dangerous bids until the auction can prove safety.

## Durability Layers

The system explicitly separates four durability layers, each with its own response
field and evidence requirement:

| Layer | Field value | Meaning | Recovery path |
|---|---|---|---|
| Redis-AOF-local | `ENGINE_DURABLE` | Decision in Redis Stream + idem key, AOF `appendfsync always` | AOF replay on restart; `rebuildRedisFromCheckpoint` on disk loss |
| Kafka append acknowledged | `KAFKA_ACKED` | AppendBatch `acks=all` confirmed by broker | Kafka RF=3 / minISR=2 for production quorum; rebuild from ledger on Redis disk loss |
| PG settled | `settlement_status=SETTLED` | PostgreSQL has applied the decision | Idempotent settlement replay from Kafka |
| PG order truth | order exists | Payment/order lifecycle closed | Idempotent order/payment flow |

**Exactly-once semantics:** The Kafka producer (`segmentio/kafka-go Writer`) operates
at-least-once (no idempotent producer; KIP-185 not implemented in this library). The
consumer achieves **effectively exactly-once** via the idempotent consumer pattern:
PG unique constraints (`orders.auction_id UNIQUE`, `bids UNIQUE(auction_id,user_id,client_bid_id)`)
and `engine_seq` CAS ensure duplicate Kafka deliveries produce no additional business
effect. PostgreSQL is the **exactly-once boundary**; Kafka is the at-least-once WAL
(by design). This is the industry-standard pattern (at-least-once + idempotent consumer
= effectively exactly-once).

`KAFKA_ACKED` is the **default synchronous response boundary**
(`BID_ENGINE_RESPONSE_DURABILITY=kafka_ack`). The handler waits for the relay's
group-commit batch confirmation via an in-process latch. Kafka fault behavior is
protected by two layers: fail-fast wakeup (relay signals `false` immediately on
AppendBatch failure) and circuit breaker (`kafkaRelayUnhealthy atomic.Bool`,
zero-cost skip when open). Neither degrades decision correctness — both degrade
only to the Redis-AOF-local `ENGINE_DURABLE` boundary.

`ENGINE_DURABLE` remains an explicit low-latency diagnostic boundary
(`BID_ENGINE_RESPONSE_DURABILITY=redis_aof`). It is sufficient to return a final
business decision when Redis is configured with AOF `appendfsync always`, but the
run still cannot be called correct until Kafka relay, PG settlement, and outbox
delivery converge.

See `docs/current/runtime-profiles.md#kafka-ack-response-durability-boundary` for
the full architecture.

## Kafka/Settlement Boundary

The system must distinguish:

- live decision: what the Redis engine decided;
- engine durability: whether the decision is recorded in Redis hot state, Redis Stream, and idempotency replay state as `ENGINE_DURABLE`;
- relay durability: whether the decision has been appended/fenced in Kafka (`KAFKA_ACKED`);
- settlement: whether PostgreSQL has applied the decision;
- delivery: whether outbox/WebSocket/snapshots have exposed it.

The default contract returns final user-visible `ENGINE_*` decisions at the
`KAFKA_ACKED` boundary when Kafka is healthy, with bounded fallback to
`ENGINE_DURABLE` on timeout/fail-fast/circuit-open. Either way, the run cannot be
called correct until Redis pending decisions, Kafka relay, PG settlement, and
outbox delivery have converged or the auction has failed closed.

## Unsupported Or Paused Paths

Until a dedicated algorithm exists, complex PG-only features such as proxy/max-bid settlement must either:

- stay on a separately documented PostgreSQL path with explicit performance limitations; or
- be disabled/paused for the Redis hot-engine auction profile.

Do not silently mix PG-only private bidding semantics into the Redis engine without a new invariant proof.
