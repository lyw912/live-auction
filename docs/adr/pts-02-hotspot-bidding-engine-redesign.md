# ADR PTS-02: Hotspot Bidding Engine Redesign

Date: 2026-05-28

Status: proposed

## Context

PTS-1 showed unacceptable tail latency for a single hot auction. The current PostgreSQL row-lock path preserves correctness, seq continuity, outbox delivery, idempotency, and order uniqueness, but it serializes all meaningful bids through the DB transaction. Under 1000 VU pressure this produced about 2s bid P99.

The project is judged on two core challenges:

- complex bidding rules: increment, cap, soft close, cancel, state machine, idempotency;
- millisecond realtime sync: consistent ranking, countdown, WebSocket recovery, no stale/wrong winner.

A DB-lock-only design is simple to defend but weak as a high-performance differentiator.

The official technical keywords are binding requirements:

- WebSocket long connections: room-level realtime propagation and recovery.
- Heartbeat keepalive: detect stale/slow clients and preserve resource bounds.
- Optimistic locking: CAS/version/fencing where it helps, with bounded retry only.
- Debounce/throttle: client and server load shaping before the hot serialization point.

## Decision

Adopt a staged architecture reset:

### Stage 1: PostgreSQL Truth Lane

Introduce an opt-in per-auction bounded execution lane before the existing `Repository.PlaceBid` transaction.

Properties:

- `auction_id` keyed queue;
- default worker count `1` per auction;
- bounded queue size and queue timeout;
- completed idempotency replay still bypasses queue;
- overload returns `BID_AUCTION_TOO_HOT` or `BID_RETRY_LATER` with `Retry-After`;
- metrics: queue depth, queue wait, queue reject, tx duration, lock wait, DB pool wait.

This stage preserves the existing product semantics: HTTP response is final DB truth.

This stage also formalizes the official "乐观锁、防抖节流" keywords:

- optimistic concurrency remains in PostgreSQL through row lock/versioned state transitions;
- H5 disables repeated clicks while a bid is pending;
- gateway GCRA limits user/IP/auction pressure;
- per-auction lane converts lock convoying into bounded queue wait and explicit retry.

### Stage 2: Redis Guard Before Redis Ledger

Add an experimental engine mode:

```text
BID_ENGINE_MODE=postgres_lane | redis_guard | redis_ledger
```

Use `redis_guard` before `redis_ledger`.

In `redis_guard` mode, Redis is a high-performance guard and projection layer, not auction truth:

```text
HTTP bid request
  -> auth/ACL/completed-idempotency replay
  -> Redis Lua guard
       - read projected current price/status/end_at/seq
       - reject clearly stale, too-low, ended, or inactive requests
       - shape obvious burst pressure
  -> PostgreSQL truth transaction
       - final rule check with FOR UPDATE/versioned state
       - update price/winner/end_at/order
       - write bid/audit/event/outbox/idempotency
  -> WebSocket/outbox delivery
```

Rules:

- Redis guard may reject only conservative conditions where stale projection cannot create a wrong business outcome.
- Redis guard must not declare winner, sold, order, or settled state.
- If Redis projection is missing, stale, or uncertain, fall through to PostgreSQL or return explicit retry according to policy.
- PostgreSQL remains the synchronous source of the HTTP final result.

Expected effect:

- good improvement when pressure includes stale low bids, ended/inactive bids, duplicate clicks, or abusive bursts;
- limited improvement when nearly all pressure is valid accepted bids, because PostgreSQL still serializes final state.

In `redis_ledger` mode, Redis becomes a hot engine with settlement:

```text
HTTP bid request
  -> auth/ACL/admission/idempotency precheck
  -> Redis Lua per-auction state machine
       - validate auction status, end_at, current price, increment, cap
       - enforce idempotency
       - increment engine_seq
       - update current price/winner/end_at/terminal state
  -> synchronously append accepted/rejected command result to Kafka
  -> return ENGINE_ACCEPTED / ENGINE_REJECTED / ENGINE_SOLD
  -> settlement worker replays ledger into PostgreSQL
       - insert bids
       - insert auction_events/outbox
       - update final auction row/order
       - complete DB idempotency
  -> reconciler verifies Redis state, ledger, DB state, and outbox
```

PostgreSQL remains final financial/audit settlement truth. Redis becomes hot-engine truth for live price/winner within a bounded reconciliation contract.

This mode is only acceptable with the full mechanism set:

- fencing token / `engine_epoch`;
- engine idempotency by request hash;
- monotonic `engine_seq`;
- durable ordered command log;
- PostgreSQL settlement worker;
- automatic reconciliation and repair/pause;
- invariant verifier after pressure and failure tests.

Without that full set, Redis must stay in `redis_guard` mode.

## Product Semantics Change

The API must not pretend Redis acceptance is already fully settled in PostgreSQL.

New visible states:

| State | Meaning | UI behavior |
|---|---|---|
| `ENGINE_ACCEPTED` | hot engine accepted and assigned seq | show leading/pending settlement |
| `SETTLED` | DB settlement committed | normal durable history |
| `ENGINE_REJECTED` | rejected by hot engine rules | show reason immediately |
| `RECONCILING` | engine and DB/ledger need repair | disable dangerous CTA, show recovering |
| `ENGINE_PAUSED` | engine cannot prove safe processing | stop bids, allow snapshot/result viewing |

For demo simplicity, Stage 1 keeps current response schema. Stage 2 may add fields rather than breaking existing clients:

```json
{
  "result": "ENGINE_ACCEPTED",
  "engine_seq": 123,
  "settlement_status": "PENDING",
  "current_price_cents": 65000,
  "current_winner_id": "user_42"
}
```

## Redis State Model

Key hash tag: `{auction_id}` for cluster-safe co-location.

```text
auction:{id}:state
  status
  current_price_cents
  current_winner_id
  end_at_ms
  start_price_cents
  increment_cents
  cap_price_cents
  extend_window_ms
  extend_by_ms
  max_extend_count
  extend_count
  engine_seq
  engine_epoch
  accepted_bid_count

auction:{id}:idem:{client_bid_id}
  request_hash
  result_json
  engine_seq
  expires_at

```

The Lua script must be short and deterministic. It must not call network, sleep, or do unbounded loops. Proxy/max-bid settlement is not included in the first Redis hot engine; it remains PostgreSQL path until a separate atomic algorithm is designed.

Optimistic locking in this stage means:

- Redis Lua atomic compare/update for the common path;
- optional Redis `WATCH/MULTI/EXEC` only for non-Lua administrative CAS paths;
- `engine_epoch` as fencing token so stale engine instances cannot settle or overwrite newer state;
- bounded retry with `BID_RETRY_LATER`, never infinite CAS loops.

## Settlement Contract

The settlement worker consumes the ledger in order per auction. Current L4b uses Kafka as the required durable ledger, not Redis Streams. Redis holds hot state and short idempotency replay only.

```text
AuctionCommandLog.Append(ctx, auctionID, entry)
AuctionCommandLog.Fetch(ctx, consumer_group)
AuctionCommandLog.Ack(ctx, auctionID, entryID)
```

Implementation policy:

| Implementation | When to use | Why |
|---|---|---|
| Kafka | Current L4b runtime. | Dedicated append-only ledger, consumer group replay, and physical separation from Redis hot state. |
| PostgreSQL ledger/outbox table | Emergency fallback design only. | Strong single-DB durability, fewer moving parts, but higher DB write cost and reintroduces hot DB pressure. |
| Redis Streams | Rejected for this upgraded L4b runtime. | It couples hot state and historical ledger in the same Redis failure domain. |

On the local one-machine deployment, Kafka does not remove host-level failure domain. It is still used to prove runtime boundaries and failure gates; production must use replicated brokers, ISR, DLQ monitoring, and replay/reconciliation operations.

Required DB guarantees:

- `engine_epoch, engine_seq` unique per auction;
- idempotent insert by `client_bid_id` and request hash;
- partial unique order constraint still guarantees one order per sold auction;
- outbox rows are written in the same PostgreSQL transaction as DB state updates;
- stale epoch writes are rejected.

If settlement reaches a gap or poison event, mark the auction `RECONCILING`, emit a gap notice, and stop accepting new hot-engine bids for that auction until repaired.

## Reconciliation

The reconciler runs three checks:

| Check | Source A | Source B | Action |
|---|---|---|---|
| live state | Redis state | latest settled DB event | catch settlement lag/divergence |
| ledger continuity | Stream/Kafka seq | DB `auction_events.engine_seq` | replay or pause |
| terminal uniqueness | Redis terminal state | DB auction/order | repair or incident |

No manual-only reconciliation is acceptable. Manual intervention can be a last resort, but the system must first produce machine-readable anomalies and replay plans.

## Why This Beats A Conservative Competitor

| Dimension | DB-only competitor | Proposed design |
|---|---|---|
| P99 under hotspot | row-lock convoy; seconds under pressure | hot memory serialization; DB async settlement |
| correctness story | strong but slow | strong with replay, fencing, reconciliation |
| judge deep-dive | "we used row lock" | can discuss engine seq, ledger replay, failure repair |
| product honesty | final response only | explicit engine/settlement states |
| observability | DB metrics/outbox | engine, ledger, settlement, reconciliation, outbox |
| rollback | easy | mode switch back to PG lane |

## Rejected Alternatives

### Redis Lua Direct DB Async Without Ledger

Rejected. It is fast but cannot prove recovery after process crash, Redis data loss, or DB write failure.

### Optimistic Lock Infinite Retry

Rejected. It moves row-lock waiting into conflict retry and makes tail latency and DB CPU worse under a single hot object.

### Batch Bidding

Rejected for the main live auction path. It changes fairness and final-second semantics. It may be used later for analytics or feed price refresh, not bid acceptance.

### Kafka/Flink First

Kafka is now accepted for the L4b durable ledger because Redis Stream would keep hot state and historical ledger in one Redis failure domain. Flink remains deferred; settlement is still an app-owned consumer group that writes PostgreSQL truth idempotently.

## Rollback

Rollback is mode-based:

```text
BID_ENGINE_MODE=postgres_lane
```

Redis hot-engine state is then read-only diagnostic evidence. PostgreSQL path continues to process bids with the existing correctness invariants.

## Required Gates

- Unit: Lua rule matrix, idempotency matrix, cap/soft-close/cancel ordering.
- Integration: engine accept then settlement commit; crash before settlement; settlement retry; duplicate retry; terminal uniqueness.
- Reconciliation: Redis ahead of DB, DB ahead of Redis, missing stream entry, stale epoch.
- Load: PTS-1 before/after under same JMX; redis latency; settlement lag; DB/outbox lag.
- WebSocket: long connection fanout, heartbeat timeout, slow-consumer close, reconnect with `last_seq`, gap-to-snapshot recovery.
- Frontend: click debounce, pending settlement, retry throttle copy, reconciling, paused, gap recovery, winner/loser after settlement.
