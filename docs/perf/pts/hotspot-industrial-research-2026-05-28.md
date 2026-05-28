# Single Hotspot Bidding Industrial Research

Date: 2026-05-28

Status: research baseline for PTS-1 architecture reset

## Executive Finding

PTS-1 exposed the real core problem: the current path is correct but not competitive for a hot live auction. A P99 above 2s means the system is behaving like a database-serialized back office workflow, not a millisecond bidding engine.

The official project keywords must remain first-class design inputs, not presentation words:

- WebSocket long connections are the realtime delivery plane, not just a demo channel.
- Heartbeat keepalive is the liveness and weak-network boundary for every live room.
- Optimistic locking is still useful, but only at the correct layer: DB version/CAS for low-contention writes, Redis `WATCH`/Lua CAS or engine epoch/fencing for hot state, never infinite retry under a single hotspot.
- Debounce/throttle is a full-stack load-shaping mechanism: client click debounce, gateway admission, per-auction queue, and fanout coalescing.

The industrial answer is not "Redis is forbidden" or "DB lock is sacred". The real rule is sharper:

- The hot path needs one deterministic serialization point per auction.
- That serialization point must have a replayable log, idempotency, fencing, and reconciliation.
- PostgreSQL can remain final settlement/audit truth without being the synchronous latency bottleneck for every accepted bid.
- Clients must not be lied to: the product must distinguish hot-engine accepted, settled, sold, cancelled, and reconciliation-paused states.

## Source Material

Local project evidence:

- `单热点调研.md`
- `docs/perf/pts/pts1-hotspot-optimization-plan.md`
- `docs/perf/cloud-server/09-current-code-reconciliation.md`
- `docs/design-v2-industrial/00-project-brief.md`
- `docs/design-v2-industrial/09-performance-and-benchmark.md`
- `docs/design-v2-industrial/10-test-gates.md`

External reference points:

- Redis Lua/Functions: scripts execute server-side and can atomically operate on Redis data, but slow scripts block other clients. https://redis.io/docs/latest/develop/programmability/
- Redis transactions: `MULTI/EXEC/WATCH` provide serialized command execution and optimistic locking semantics. https://redis.io/docs/latest/develop/using-commands/transactions/
- Redis Streams: append-only ordered streams with consumer groups, replay, configurable retention, and at-least-once processing. https://redis.io/docs/latest/develop/data-types/streams/
- Redis persistence: AOF/RDB choices trade durability, latency, and recovery behavior. https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/
- Transactional outbox: store the message in the same database transaction that updates business entities. https://microservices.io/patterns/data/transactional-outbox.html
- Kafka design: ordering is strongest within a single topic partition, which maps naturally to keying by `auction_id`. https://kafka.apache.org/documentation/
- PostgreSQL concurrency: MVCC plus row/table/advisory locks provide explicit conflict control, but a hot row remains serialized. https://www.postgresql.org/docs/current/mvcc-intro.html
- Flink checkpoints: state and stream positions can be restored to consistent snapshots for replayable settlement pipelines. https://nightlies.apache.org/flink/flink-docs-stable/docs/concepts/stateful-stream-processing/

## What The Current PTS Result Means

Observed PTS-1:

| Signal | Meaning |
|---|---|
| Bid P99 around 2265ms | Requests are waiting behind the single auction serialization point. |
| Snapshot P99 around 1688ms | Read/recovery path is competing with write pressure and DB pool/CPU. |
| WS ticket P99 around 1119ms | Non-bid APIs are also affected by shared resource pressure. |
| Seq continuous, outbox delivered | Correctness path works. |
| DB pool wait and auction row-lock wait dominated | PostgreSQL synchronous hot row is the bottleneck. |

The current implementation is defensible for correctness. It is not defensible as an "extreme performance" differentiator unless we either:

1. explicitly cap traffic and explain admission behavior as a product decision; or
2. move the hot serialization point out of PostgreSQL and build the missing correctness machinery.

## Industrial Solution Spectrum

| Option | Serialization Point | User Response | Performance Potential | Correctness Burden | Fit |
|---|---|---:|---:|---:|---|
| A. PG row lock | PostgreSQL row | synchronous final result | low-medium | low | correctness baseline |
| B. PG lane/actor before DB | in-process queue then PG | synchronous final result or fast retry | medium | medium | quick PTS tail reduction |
| C. Redis Lua + DB sync-after | Redis Lua | synchronous engine result | high | high | realistic PTS breakthrough |
| D. Redis Stream/Kafka command log + per-auction engine | durable log + actor | accepted/pending then finalized | high | high | industrial target |
| E. Batch matching | batch window | delayed result | high | high/product-changing | poor fit for live auction |

## Official Keywords As Engineering Requirements

| Keyword | Industrial interpretation | Project implementation target |
|---|---|---|
| WebSocket 长连接 | Long-lived room connection with explicit room/auction isolation and recoverable event stream. | Keep current room-scoped WS, add fanout-pressure gates, seq gap handling, snapshot fallback, and settlement-state events for Redis engine mode. |
| 心跳保活 | Detect dead/slow clients, release resources, avoid stale UI. | Server ping/pong, write deadline, slow-consumer close, reconnect with `last_seq`, metrics for close reason and reconnect source. |
| 乐观锁 | CAS/version control, not unbounded retry. | PG `version`/row lock baseline; Redis engine uses atomic Lua or `WATCH`-style CAS semantics plus `engine_epoch` fencing; retries are bounded and measured. |
| 防抖节流 | Shape useless pressure before it hits the serialization point. | H5 click debounce and pending disable; gateway user/IP/auction GCRA; per-auction lane; WS fanout coalescing for high-frequency internal events. |

## Why "Redis Decides Price/Winner" Is Not Automatically Wrong

The old red line says Redis cannot be truth because DB is the source of truth. That is too simplistic.

Redis can be the hot-path authority if the design makes these statements true:

- Every accepted mutation has a durable command/event record before or atomically with the state transition.
- Every mutation has a monotonic `engine_seq` and fencing token.
- PostgreSQL settlement can replay from the log and detect divergence.
- Client-visible success is named accurately: `ENGINE_ACCEPTED`, `SETTLED`, `SOLD`, not a fake DB-committed result.
- On Redis/engine uncertainty, bidding enters `RECONCILING` or `PAUSED`, not "best effort continue".

Redis is unsafe only when used as a naked cache with asynchronous DB writes and no ledger. Redis is viable when treated as a deterministic in-memory state machine backed by a durable event ledger and reconciler.

## Why Naked Redis Lua Is Still Not Enough

A single Lua script can atomically check current price, validate increment, update winner, extend `end_at`, and emit a Redis Stream entry. That solves lock contention but creates new failure questions:

| Failure | Bad naive result | Industrial mitigation |
|---|---|---|
| Process crashes after Redis accepted, before DB write | DB misses accepted bid | Redis Stream/Kafka ledger replay; settlement worker idempotently inserts into DB. |
| Redis loses acknowledged write | ghost accepted bid disappears | AOF policy, WAIT/replica durability where available, bounded "engine accepted but not settled" state, reconciliation pause. |
| DB settlement fails | Redis/DB divergence | settlement retry, dead-letter, invariant verifier, user-facing paused state for affected auction. |
| duplicate client retry | duplicate accepted bid | idempotency key stored in engine state and DB; same request returns same result. |
| cancel/end races bid | wrong winner or reversible terminal | same per-auction state machine serializes bid/cancel/end as commands. |
| Redis failover/old leader | stale writer overwrites | fencing epoch on every command; DB settlement rejects old epoch. |
| WS/outbox misses event | UI gap | history replay, snapshot, gap notice, outbox/stream bridge. |

## Recommended Direction

Use a three-tier plan:

1. Short-term PTS rescue: add per-auction bounded lane in front of the existing PG path. This reduces DB pool/row-lock convoying and makes overload explicit. It is not the final architecture.
2. Redis guard path: use Redis Lua as a high-performance prefilter/projection layer. It rejects clearly invalid or stale pressure and protects PG, but it does not maintain winner/order truth.
3. Strategic differentiator: implement a Redis Lua + durable ledger hot engine as an opt-in `BID_ENGINE_MODE=redis_ledger` profile, with PostgreSQL settlement and reconciliation. This is the route that can credibly beat DB-lock-only competitors, but only after the full correctness mechanism exists.

Kafka is not a required dependency for the first Redis-ledger implementation. In the current single-machine deployment, Redis Streams or a PostgreSQL ledger/outbox table is a more pragmatic command log. Kafka should remain a compatible future implementation of the command-log interface, not a mandatory Docker dependency for PTS-1.

Realtime delivery is a parallel P0 track, not a later UI polish task:

- WebSocket must carry accepted/rejected policy events, settlement events, pause/reconcile events, gap notices, and snapshots.
- Heartbeat and reconnect must be tested under the same hot-auction pressure, because a fast bid engine is useless if users see stale ranking.
- Debounce/throttle must be visible to users as pending/retry states, not silent dropped clicks.

The final defense to judges is:

- We first measured the real bottleneck.
- We kept the correct PG baseline.
- We built a high-performance hot engine without losing auditability.
- We can prove every accepted bid by replaying the engine log into PostgreSQL and checking invariants.
