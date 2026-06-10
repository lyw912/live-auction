# Scale-Out & Architecture Ceilings — judge Q&A prep

> Status: governing architecture-boundary narrative, 2026-06-02.
> Purpose: answer "your numbers are from one 8c32g box — how does this scale?"
> without either over-claiming or saying "just buy a bigger machine." This is the
> doc to read before being interrogated by senior architects.

## 1. The one framing that wins: "per-shard capacity"

Every single-node number we report is **one shard's worth of capacity**. The
system scales by adding shards, except for one deliberate point. State it up
front:

> "These numbers are per-node / per-shard. The only thing we deliberately do
> *not* scale horizontally is the per-auction decision sequencer — because a
> single auction needs a total order to guarantee 'highest valid amount wins.'
> Everything else scales by sharding, and here is exactly how."

## 2. Infra ceiling vs architecture ceiling

The empirical test: scale the box up — does the ceiling move proportionally
(infra) or plateau (architecture)? Scale out — does one *logical* resource stay
pinned while the rest spreads (architecture) or does load distribute (infra)?

| Bottleneck | Type | Scale-out path (the answer to give) |
|---|---|---|
| App CPU / RSS on one node | infra | horizontal WS/gateway nodes behind LB; report per-node limit |
| File descriptors | infra (mostly) | raise `ulimit -n`/`fs.nr_open`; also bound per-room connections |
| Redis command CPU/latency | infra (partly) | optimize key/data model first; Redis Cluster shards **only if keys shard by room/auction** |
| **Redis Pub/Sub propagation** | infra, but **not** by bigger app nodes | **Sharded Pub/Sub** (`SSUBSCRIBE`/`SPUBLISH`, Redis 7.0) — confine a room's messages to one shard; plain cluster pub/sub broadcasts to *all* nodes and scales *negatively* |
| Redis connection count | infra (carefully) | bounded pools + multiplexed/subscription topology; per-app-node fanout makes it worse |
| Kafka broker disk/network | infra | add brokers/partitions after measuring broker-side saturation |
| **Kafka consumer parallelism** | infra, **bounded by partitions + ordering key** | add partitions; consumer-group parallelism ≤ partition count; keying by `auction_id` preserves per-auction order |
| **One hot auction's decision order** | **ARCHITECTURE** | a single auction needs a single sequencer — solve with an efficient single-writer path + explicit limits; scale across auctions, **never split one auction** |
| WS fanout downlink | infra | room-sharded WS gateways + a cross-node fanout bus (Redis sharded pub/sub / NATS / dedicated realtime layer); one ordered decision stream feeds many gateway shards |

## 3. The single sequencer — why it is a *feature*, not a limit

- One auction's accept/reject decisions must be totally ordered so the winner is
  unambiguous. Total order ⇒ a single writer / single partition. This is the
  **single-writer principle** (LMAX Disruptor reaches tens of millions of ops/sec
  *because* it is single-threaded and lock-free).
- Our Redis single-writer Lua *is* that sequencer; `engine_seq` is the order; the
  Kafka per-`auction_id` partition is the durable fence.
- Therefore a bigger box **raises the single-writer's ops/sec** (good for us on
  8c32g), and the system scales by **partitioning auctions across sequencers** —
  N hot auctions → N partitions/shards → linear. One auction never needs to
  exceed one sequencer's capacity, and our S1 numbers show that capacity is ample
  for a single room's contention.

This is aligned with a common production pattern: keep the deadline-critical
decision path ordered and small, then drain/audit downstream asynchronously.
Do not present named industry products as proof unless the cited evidence is in
the run report; use them only as discussion examples.

## 4. Multi-Kafka without split-brain (the user's note)

When moving from one broker to a cluster, the anti-split-brain settings are the
talking points:

- **Quorum controller**: KRaft (or ZooKeeper) gives a single elected controller;
  a metadata quorum prevents two leaders for a partition.
- **`acks=all` + `min.insync.replicas ≥ 2`**: an ACK means the record is on a
  quorum of in-sync replicas, so a single broker loss cannot lose an acked
  decision (RPO=0 survives failover).
- **`unclean.leader.election.enable=false`**: never elect a non-ISR leader — that
  would resurrect a stale log and break the decision order.
- Keep `auction_id` as the partition key so per-auction order survives rebalances.

State these as the *configuration we would set*, and note the single-node test
deliberately runs one broker (the cold-start time is why F-kafka RTO is ~26 s; a
3-broker cluster would fail over faster — a falsifiable next test, not a hand-wave).

## 5. Prepared answers to the likely interrogation

> **"How do you support 100k users?"**
> Per-shard: one node holds ~10k connections (measured RAM/conn ≈ 20–30 KB). 100k
> = ~10 room-sharded gateway nodes behind an LB, each subscribed to the same
> ordered decision stream via sharded pub/sub. Fanout latency stays per-shard.

> **"Redis is single-threaded — what when it's the bottleneck?"**
> Two different limits. Decision CPU: the Lua engine is the per-auction sequencer;
> we scale by partitioning auctions, not by threading one auction. Pub/Sub
> propagation: we move to sharded pub/sub so a room's messages stay on one shard
> instead of broadcasting cluster-wide.

> **"Your Kafka consumer can't keep up."**
> Consumer parallelism is bounded by partitions. We add partitions and keep
> `auction_id` as the key so per-auction order holds; settlement workers scale up
> to the partition count. Beyond that, the ordering key is the limit, by design.
> The implementation exposes `REDIS_ENGINE_SETTLEMENT_WORKERS` for this
> cross-partition scale-out path. It improves many-store / many-auction drain,
> not one single hot auction, because one auction remains one ordered key.

> **"Why not parallelize settlement of one auction?"**
> Because `engine_seq`, winner, public seq, and order creation are serial state.
> Parallel workers on one auction can settle seq 42 before seq 41 unless a second
> sequencer is reintroduced, which just moves the bottleneck and risks wrong
> winner/payment. The valid single-auction optimization is **set-based batch
> writes inside the ordered transaction**: insert/update many rejected settlement
> rows, bid rows, idempotency rows, and checkpoint state with fewer PostgreSQL
> round trips while preserving the same `engine_seq` order.

> **"Why not stop persisting rejected decisions?"**
> Because a rejected bid is still a final user-visible decision. It must be
> idempotently replayable and auditable with the exact reject reason and
> decision-time basis. The current implementation already uses batch/set-based
> settlement where safe, and S2 decision/reject-heavy evidence proves convergence
> for the scoped workload. The next optimization is a narrower partitioned
> rejected-decision audit schema or COPY ingest, not deleting reject evidence.

> **"This is all one machine — isn't it just a toy?"**
> The single-node run is the per-shard capacity proof. We report where the node
> ceiling is (S2/S3), classify each ceiling as resource vs architecture (this
> table), and name the next falsifiable test. The only non-horizontal point is the
> per-auction sequencer, and that is a correctness choice, not a scaling failure.

## 6. The honest sentence for any saturation

When a run hits a limit, say exactly this shape — never "just infrastructure":

> "This run hit a single-node / one-broker limit at X. That limit is a
> {resource ceiling → add a node/partition/shard | architecture ceiling → the
> per-auction sequencer, scaled by partitioning auctions}. The next falsifiable
> test is Y."
