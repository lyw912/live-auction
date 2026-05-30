# Single-Hotspot Live Auction Redesign From First Principles

Date: 2026-05-30

Status: architecture redesign derived from `单热点调研.md`, not a patch to the
current implementation.

Supersedes earlier intermediate Route-B/hot-engine drafts. Use this document as
the architecture entry point.

Scope: high-value live auction in
`抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md`.

## Starting Point

The decisive sentence from `单热点调研.md` is:

```text
拍卖与秒杀的本质区别：
秒杀可容忍最终一致（超卖后补偿），
拍卖绝不能出现"两个买家同时看到自己是最高价"的脏读。

工业级实践中：
当前最高价与拍品状态必须强一致，
出价记录可最终一致。
```

This document treats that as the design root, not as a note to add later.

## Non-Negotiable Requirements

From the official brief and single-hotspot research, the system must satisfy:

| Requirement | Meaning |
|---|---|
| one current price | no split-brain price under concurrent bids. |
| one current winner | two users must never both see authoritative leading success. |
| one auction status | `ACTIVE/SOLD/ENDED/CANCELLED/RECONCILING` must not diverge. |
| atomic soft close | accepting a bid and extending `end_at` are one state transition. |
| cap is terminal | cap bid creates a single terminal sold decision and one order later. |
| server time authority | client countdown cannot decide close or bid legality. |
| idempotent bid | retry cannot create a second bid, second order, or different response. |
| durable success evidence | user-visible success must have a replayable record before returning. |
| eventually consistent audit | DB bid rows/order/outbox may lag, but only after durable decision. |
| explicit uncertainty | if safety cannot be proven, pause/reconcile instead of continuing. |

Therefore the core split is:

```text
strongly consistent live state:
  current_price, current_winner, status, end_at, engine_seq

eventually consistent materialization:
  bid rows, auction_events rows, outbox rows, order rows, history views

durability bridge:
  append-only Kafka decision log before HTTP success
```

## Why The Starting Options Are Insufficient

### DB Strong Lock

`SELECT ... FOR UPDATE` or optimistic `version` is the easiest proof of
correctness. It is also the easiest way to turn the official final-second
challenge into a row-lock queue.

Failure under the official scenario:

- 100+ users click in the final second;
- every accepted/rejected meaningful bid waits for DB transaction work;
- monitor/snapshot/outbox settlement compete for DB resources;
- tail latency becomes the product experience.

Decision: retain PostgreSQL as settlement/audit truth and fallback, but do not
use PG transaction as the live hot path if the goal is a high-performance
auction engine.

### Redis Pre-Deduction / Flash Sale

Redis `DECR stock -> MQ -> DB order` fits inventory. Auction is not inventory.

Auction decision mutates:

- current price;
- current winner;
- sequence;
- end time;
- sold/cap status;
- public rank;
- idempotency response.

Decision: borrow Redis atomicity and async persistence, but reject the inventory
pre-deduction model. Use a Redis-based state machine, not a counter gate.

### Actor / Queue Only

Per-auction actor is conceptually correct: one writer owns one auction state.
But pure queue-first changes the product contract:

- user may see only "queued";
- engine delays become a visible uncertainty state;
- gateway request/reply correlation becomes a new reliability surface.

Decision: actor semantics are correct, but for the course implementation use
Redis Lua as the per-auction single-writer state machine and keep HTTP
near-immediate. Option A Kafka-command-first remains the stronger future target.

### Batch Matching

Batching deforms live auction semantics. Cap, soft close, final-second ordering,
and proxy-bid equilibrium all become harder to explain.

Decision: do not batch same-auction decisions. Batch only downstream settlement
after the engine order is fixed.

## Chosen Architecture

The selected course-final architecture is:

```text
Route B+: Redis strongly consistent hot state + synchronous Kafka decision WAL

H5 bid
  -> gateway auth / ACL / schema
  -> completed idempotency replay
  -> H5/gateway debounce and admission
  -> Redis Lua single-auction decision
       - reads server-side auction state
       - validates rule and status
       - updates price/winner/status/end_at/engine_seq
       - writes idempotency result
       - writes pending decision
  -> Kafka append of the exact decision before HTTP success
       - key = auction_id
       - value includes engine_epoch, engine_seq, request_hash, response_json
  -> HTTP success only after Kafka ack
       - result = ENGINE_ACCEPTED / ENGINE_REJECTED / ENGINE_SOLD
       - settlement_status = PENDING
  -> settlement worker writes PostgreSQL bid/event/outbox/order idempotently
  -> WebSocket publishes engine decision and later settlement confirmation
  -> reconciler verifies Redis/Kafka/PostgreSQL/outbox/order
```

This is not pure Route A and not full Option A. It is a safety-hardened
intermediate design:

- it fixes the current Redis-only success hole;
- it avoids synchronous PostgreSQL transaction work in the live response path;
- it gives every successful response an immutable ordered decision record;
- it leaves an upgrade path to Kafka-command-first actor processing.

## State Ownership

### Redis Hot State Owns Live Truth

Redis hot engine owns, for the active auction window:

- `status`;
- `current_price_cents`;
- `current_winner_id`;
- `end_at_ms`;
- `extend_count`;
- `accepted_bid_count`;
- `public_seq` for accepted/sold events;
- `engine_seq` for all executable decisions;
- idempotency response for bid requests;
- `engine_epoch` fencing token.

All of these must be updated atomically in one Lua decision for a bid/cap/soft
close path.

### Kafka Owns Durable Decision Proof

Kafka decision log owns:

- immutable decision record;
- engine order;
- replay input for settlement;
- evidence that a user-visible engine success existed before HTTP returned.

Partitioning contract:

- producer key is `auction_id`;
- partition strategy is `hash(auction_id) % partition_count`;
- consumer group is `settlement-workers`;
- Kafka assigns each partition to one active consumer in the group;
- multiple auctions may share one partition, but a single auction's order stays
  on that partition;
- PostgreSQL settlement still enforces `engine_seq == db_engine_seq + 1`, so a
  reordered or duplicated message cannot silently mutate auction truth.

If Kafka ack is missing, there is no successful bid from the platform's point of
view, even if Redis briefly mutated state.

### PostgreSQL Owns Settlement And Audit Truth

PostgreSQL owns:

- settled bid rows;
- final auction row convergence;
- auction event rows;
- outbox delivery rows;
- order/payment mock state;
- settlement checkpoint/watermark;
- anomaly records.

PostgreSQL can lag the live engine, but it cannot contradict it silently. If it
cannot settle a durable decision, the auction/result becomes anomalous or paused.

## Success Boundary

The HTTP success boundary is exactly:

```text
Redis Lua decision succeeded
AND
Kafka decision append ack succeeded
```

Not enough:

- Redis Lua success alone;
- Redis AOF success assumption;
- pending hash exists;
- goroutine scheduled to write Kafka;
- PostgreSQL eventually might settle.

This boundary is the answer to the high-value jewelry concern:

> A user-visible successful bid has an immutable ordered decision record before
> the response leaves the backend.

## Failure-First Design

### Redis Failure

| Timing | Behavior |
|---|---|
| before decision | return retry/unavailable; no success. |
| during Lua | script error pauses auction; no success. |
| after Lua before Kafka ack | no HTTP success yet; recover pending if possible; otherwise pause/reconcile. |
| after Kafka ack | decision can be replayed from Kafka; rebuild Redis from checkpoint/log. |

Redis persistence reduces recovery loss, but it is not the success contract.

### Kafka Failure

| Timing | Behavior |
|---|---|
| before append | no success. |
| append timeout/unknown | do not return success; mark decision uncertain; pause/reconcile until idempotency is resolved. |
| acked then backend crash | retry returns same decision from Redis/Kafka/settlement idempotency. |
| consumer/settlement unavailable | live result remains pending settlement; alert by lag threshold. |

Kafka is the durable-log boundary. If it cannot acknowledge, the correct
high-value behavior is retry/pause, not optimistic success.

### PostgreSQL Failure

| Timing | Behavior |
|---|---|
| before settlement | live engine may continue only within bounded settlement lag policy. |
| settlement lag over threshold | PC/H5 diagnostics show pending/reconciling; optionally pause auction. |
| unique conflict | treat as idempotent replay if payload matches; otherwise anomaly. |
| order creation failure | retry settlement; do not mark order paid/complete. |

PostgreSQL being down must not erase a Kafka-acked decision, but prolonged lag
must be visible and bounded.

### WebSocket Failure

WebSocket delivery is not truth. It is a recoverable projection:

- events carry `auction_id + seq`;
- clients dedupe old seq;
- gap triggers snapshot;
- recovering disables bid CTA;
- slow clients are closed;
- settlement confirmation may arrive after engine decision.

## Reconciliation Model

The reconciler is not optional. It is the cost of moving live state out of
PostgreSQL.

It must verify:

- Redis `engine_seq` equals latest Kafka decision seq or is explainably ahead
  only inside a non-acked recovery window;
- every HTTP success has a Kafka decision;
- every Kafka accepted/sold decision has or will get one PostgreSQL bid row;
- accepted public seq is contiguous;
- final current winner equals highest accepted engine decision under rules;
- cap creates exactly one order;
- idempotency response JSON matches stored decision;
- outbox published or has explicit retry/dead/gap state.

If verification fails:

```text
auction.status = RECONCILING / ENGINE_PAUSED
new bids rejected with retry/recovery code
operator diagnostics expose exact missing seq / offset / trace_id
clients snapshot instead of trusting local stream
```

## Settlement Lag Policy

Settlement lag is not user-facing live correctness, but it is financial/audit
risk. Route B defines a budget:

| Lag | Meaning | Action |
|---:|---|---|
| `< 1s` | healthy target | no user-visible warning. |
| `1s-5s` | degraded | PC diagnostic warning; H5 still pending settlement. |
| `> 5s` | unsafe backlog | throttle or pause new high-value bids for affected auction. |
| poison/dead | cannot prove settlement | pause/reconcile and emit anomaly/gap. |

The exact thresholds must be validated by pressure tests. The policy exists so
async settlement cannot grow without bound.

Kafka `acks=all` is part of the same bounded-risk policy. If ISR shrinks below
the broker/topic `min.insync.replicas`, Kafka should reject or time out writes.
The auction engine must return pending/reconciling and pause instead of
downgrading to async or weaker acks. Preflight and PTS evidence must include
topic partition count, replication factor, ISR health, and consumer-group lag.

## Why This Is Not Just "补丁 Route B"

The old current implementation was:

```text
Redis decision
  -> HTTP success
  -> async Kafka
```

The redesigned system changes the contract:

```text
Redis decision
  -> durable Kafka decision
  -> HTTP engine success
  -> async PostgreSQL materialization
```

This is a different trust model:

- Redis-only success is forbidden.
- Kafka ack is the WAL boundary.
- PostgreSQL materialization is allowed to lag but is measured and bounded.
- uncertainty pauses the auction.
- the UI has distinct pending/engine/settled/reconciling states.

## Implementation Implications

Minimum code changes:

1. Move Kafka append into the HTTP path after Redis Lua decision.
2. Return success only after append success.
3. On append failure, do not return engine success; pause/reconcile and leave
   pending decision for recovery.
4. Ensure duplicate retry after unknown append can resolve by `client_bid_id`.
5. Add `ENGINE_ACCEPTED` vs `SETTLED` UI wording and state.
6. Add settlement lag metrics and pause threshold.
7. Add failure tests for Redis/Kafka/PG crash windows.
8. Add H5 local in-flight bid lock tests so rapid taps cannot create multiple
   requests before React re-renders the disabled CTA.
   If a bid response is lost, H5 must keep the original bid request identity and
   retry with the same `client_bid_id` / `Idempotency-Key` instead of generating
   a fresh business decision.
9. Add dedicated soft-close and cap race gates: one old-window extension,
   exactly one cap `ENGINE_SOLD`, one order, and no equal-cap loser classified
   as `BID_TOO_LOW`.

Better next iteration:

1. Add PostgreSQL settlement checkpoint:

```sql
auction_engine_checkpoints (
  auction_id text primary key,
  engine_epoch bigint not null,
  engine_seq bigint not null,
  decision_topic text not null,
  decision_partition int not null,
  next_decision_offset bigint not null,
  state_hash text not null,
  snapshot_json jsonb not null,
  updated_at timestamptz not null
)
```

2. Add Redis rebuild from PG checkpoint + Kafka decision log.
3. Add topic preflight for replication/min-ISR/auto-create policy.
4. Add invariant checker gates into every PTS run.
5. Add PTS-1C soft-close sniper and cap-race evidence artifacts separate from
   generic final-second throughput.

## Judge Defense In One Minute

> We started from the single-hotspot rule: auction differs from seckill because
> two users must never both see themselves as the highest bidder. Therefore
> current price, winner, status, and soft-close time are strongly consistent in a
> single Redis Lua auction engine. Bid rows and order settlement can be async,
> but only after the exact engine decision has been appended to Kafka. HTTP
> success is behind Kafka ack, so user-visible success is never Redis-only. If
> Redis, Kafka, or PostgreSQL is uncertain, the auction enters pending,
> reconciling, or paused state instead of lying to the user. This keeps the live
> path faster than PG lane while preserving high-value auction auditability.

## Sources

Local:

- `单热点调研.md`
- `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md`

External:

- Redis persistence:
  https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/
- Redis Lua:
  https://redis.io/docs/latest/develop/programmability/eval-intro/
- Redis transactions / WATCH:
  https://redis.io/docs/latest/develop/using-commands/transactions/
- Kafka documentation:
  https://kafka.apache.org/documentation/
- kafka-go package documentation:
  https://pkg.go.dev/github.com/segmentio/kafka-go
- Transactional outbox pattern:
  https://microservices.io/patterns/data/transactional-outbox.html
- Event sourcing pattern:
  https://martinfowler.com/eaaDev/EventSourcing.html
- Redlock/fencing critique:
  https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html
