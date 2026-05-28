# PTS Hotspot Architecture Reset Index

Date: 2026-05-28

Status: master entry for PTS-1 hotspot redesign, implementation, testing, and final defense

## Why This Exists

PTS-1 proved that the current single-auction path is correct but too slow for the core challenge. A bid P99 around 2s is not acceptable for a live auction system whose official challenge is high-concurrency, realtime bidding.

This folder now has several focused documents. This README is the entry point so implementation and testing do not drift.

## Read Order

Read in this order:

1. `单热点调研.md`
   - User-provided research baseline.
   - Frames the core single-hotspot problem and the DB lock vs Redis/actor/queue tradeoff.

2. `docs/perf/pts/pressure-test-plan.md`
   - Existing PTS strategy and historical run interpretation.
   - Key fact: report `9VY7W7BF` passed correctness but exposed ~2265ms bid P99.

3. `docs/perf/pts/pts1-hotspot-optimization-plan.md`
   - Existing conservative next-step plan.
   - Useful for PG lane metrics and before/after comparison.

4. `docs/perf/pts/hotspot-industrial-research-2026-05-28.md`
   - Industrial research synthesis.
   - Explains why Redis is not automatically forbidden, and what machinery makes it safe.

5. `docs/adr/pts-02-hotspot-bidding-engine-redesign.md`
   - Architecture decision record.
   - Defines `postgres_lane`, `redis_guard`, and `redis_ledger` modes.

6. `docs/perf/pts/hotspot-redesign-roadmap-2026-05-28.md`
   - Development roadmap and branch strategy.
   - Defines M0-M5 implementation/test order.

7. `docs/perf/pts/scoring-dimension-decomposition-2026-05-28.md`
   - Word-by-word decomposition of official scoring terms.
   - Use this for final report, slides, and judge defense.

8. `docs/perf/pts/full-pressure-runbook.md`
   - Concrete PTS execution steps.
   - Use after implementation, not before design decisions.

## Current Decision

The project should not keep patching the current hot path indefinitely.

Decision:

| Track | Mode | Purpose | Integration policy |
|---|---|---|---|
| Stable rescue | `postgres_lane` | Reduce 2s tail quickly without changing truth semantics. | Mainline baseline improvement. |
| Redis guard | `redis_guard` | Use Redis Lua as prefilter/projection to protect PG without deciding winner/order. | Same codebase; can merge before Redis ledger if correctness tests pass. |
| Aggressive differentiator | `redis_ledger` | Move hot bid decision out of PG row lock using Redis Lua + ledger + settlement + reconciliation. | Same codebase behind feature flag until failure gates pass. |
| Realtime proof | WS fanout/reconnect | Prove room-scoped long connection, heartbeat, slow-client, 1000+ watcher route. | Mainline delivery plane for both modes. |

2026-05-29 update: L4b is implemented in mainline as Redis Lua hot state + Kafka/Redpanda durable bid ledger + PostgreSQL settlement truth. Local Docker Redis is exposed as `localhost:6380` and local Redpanda as `localhost:9092`. Evidence: `docs/evidence/pts-l4b-redis-ledger-engine-2026-05-29.md`.

These are layers, not competing branches. Temporary branches can be used for review, but the target architecture is integrated.

## Development Sequence

The implementation is layered. Each step should update backend, UI, and tests together:

| Layer | Main purpose | Backend work | UI work | Tests/evidence |
|---|---|---|---|---|
| L1 Admission/debounce | reduce useless pressure | gateway GCRA/retry-after | pending disable/cooldown | rapid click, 429/retry state |
| L2 PG lane | reduce DB convoy | per-auction bounded lane | retry-too-hot state | queue tests, PTS before/after |
| L3 WebSocket recovery | realtime correctness | heartbeat, room routing, gap snapshot | reconnecting/recovered/stale states | 1000 WS, slow consumer |
| L4a Redis guard | protect PG without truth shift | Lua guard, projection freshness | fast reject/retry only | false-reject/stale tests |
| L4b Redis ledger | extreme hot path | Lua, command log, settlement, reconciler | engine accepted/settled/paused | crash/replay/reconcile |
| L5 Diagnostics/evidence | judge proof | metrics, monitor APIs | PC diagnostic panels | evidence index |

### Step 0: Freeze Baseline Evidence

Do not start another paid PTS run until evidence paths are recorded:

- `docs/perf/pts/evidence/after-9VY7W7BF-pts1-hotspot-review/analysis-summary.md`
- `docs/perf/pts/evidence/before-pts1-hotspot-20260528-1647/`
- `docs/perf/pts/evidence/after-9VY7W7BF-pts1-hotspot-review/`

Expected output:

- baseline report ID;
- git SHA;
- PTS JMX path;
- observed p95/p99;
- DB pool/row-lock attribution;
- invariant status.

### Step 1: Implement `postgres_lane`

Goal:

- convert hidden DB row-lock/pool convoying into bounded per-auction queue wait and explicit retry.

Implementation checklist:

- per-auction bounded lane keyed by `auction_id`;
- idempotency completed replay before queue admission;
- worker count default `1` per auction;
- queue size and queue timeout config;
- return `BID_AUCTION_TOO_HOT` or `BID_RETRY_LATER` with `Retry-After`;
- metrics:
  - `auction_bid_queue_depth`;
  - `auction_bid_queue_wait_seconds`;
  - `auction_bid_queue_rejected_total`;
  - `auction_bid_tx_seconds`;
  - existing lock/pool metrics.

Tests:

- unit test queue admission and timeout;
- integration test idempotent replay bypasses full queue;
- integration test full queue returns retryable error;
- existing auction correctness tests still pass;
- PTS-1 before/after using same JMX.

### Step 2: Slim The PG Transaction

Goal:

- reduce lock hold time without changing correctness.

Checklist:

- accepted/cap/end/cancel/order/outbox remain in same DB transaction;
- ordinary non-state rejects remain auditable but do not produce unnecessary room-wide durable realtime;
- no network publish inside transaction;
- measure transaction duration before and after.

Tests:

- bid accepted writes bid/event/outbox/idempotency in one tx;
- ordinary reject stored and idempotent;
- policy reject durable realtime still works;
- outbox order remains monotonic.

### Step 3: Implement `redis_guard`, Then `redis_ledger`

Goal:

- build the real high-performance path without lying about correctness.

Start with `redis_guard`:

```text
HTTP bid
  -> Redis Lua guard rejects clearly invalid/stale pressure
  -> PostgreSQL remains final synchronous truth
```

This is useful because it reduces rejected/stale pressure without moving winner/order truth. It will not eliminate the accepted-bid row-lock ceiling.

Hot path:

```text
HTTP bid
  -> auth / ACL / admission / request hash
  -> Redis Lua state machine
  -> append ledger entry
  -> return ENGINE_ACCEPTED / ENGINE_REJECTED / ENGINE_SOLD
  -> async settlement to PostgreSQL + outbox
  -> reconciler verifies Redis / ledger / DB
```

Minimum implementation:

- manual bid only;
- no proxy/max-bid in Redis v1;
- no real payment changes;
- Kafka/Redpanda ledger required for L4b; local single-node Redpanda is test topology only;
- Redis keys hash-tagged by `{auction_id}`;
- `engine_seq` and `engine_epoch`;
- Redis idempotency key by client bid id + request hash;
- Kafka ledger behind the command-log interface;
- settlement worker idempotently writes DB rows;
- reconciler can pause auction on gaps or poison events.

Required product states:

- `ENGINE_ACCEPTED`;
- `SETTLED`;
- `ENGINE_REJECTED`;
- `RECONCILING`;
- `ENGINE_PAUSED`.

### Step 4: WebSocket Long-Connection Proof

Official keyword coverage:

- WebSocket 长连接;
- 心跳保活;
- 房间级路由隔离;
- 单直播间 1000+ 用户同时在线.

Implementation/test checklist:

- ticket/subprotocol browser-compatible auth;
- room and auction ACL on connect;
- server ping/pong heartbeat;
- write deadline;
- bounded client queue;
- slow-consumer close;
- reconnect with `last_seq`;
- gap notice -> snapshot fallback;
- no cross-room event leak;
- fanout metrics and close reason metrics.

Load proof:

- one hot room;
- 1000 live WS connections;
- concurrent low/medium bid traffic;
- 3-5 minutes duration;
- record connection success, fanout lag, close reasons, RSS, goroutines, FDs.

No final `1000+` claim without this evidence.

### Step 5: Final PTS And Invariant Gates

Run:

- original PTS-1 JMX against baseline or saved baseline evidence;
- same JMX against `postgres_lane`;
- Redis-engine pressure profile;
- WS fanout/reconnect profile;
- invariant checker after every destructive run.

Required invariant result:

- no wrong winner;
- no wrong current price;
- no duplicate bid for same idempotency key;
- no duplicate order/payment transition;
- seq continuity or explicit gap notice;
- outbox published or marked DEAD with anomaly.

## Testing Matrix

| Area | Required tests | Blocks |
|---|---|---|
| PG lane | queue full, timeout, replay bypass, metrics | merge of `postgres_lane` |
| PG transaction | accepted/reject/cap/cancel/order/outbox/idempotency | any release |
| Redis guard | conservative reject, stale projection fallback, no winner/order mutation | merge of `redis_guard` |
| Redis engine | Lua rule matrix, idempotency, cap, soft close, terminal race | `redis_ledger` demo |
| Settlement | crash/retry/unique conflict/poison/gap | `redis_ledger` merge |
| Reconciliation | Redis ahead, DB ahead, stale epoch, stream gap | `redis_ledger` merge |
| WebSocket | auth, heartbeat, slow consumer, reconnect, gap snapshot | 1000+ online claim |
| Frontend | debounce, pending, retry, reconciling, paused, winner/loser | final demo |
| PTS | same JMX before/after, raw evidence, metrics snapshots | performance claim |

## Final Defense Narrative

Use this wording:

```text
PTS-1 proved the correct PostgreSQL row-lock path hit a single-hotspot latency ceiling.
We did not hide the result. We split the solution into a safe PostgreSQL lane rescue and an aggressive Redis-ledger hot engine.
The Redis path is not a naked cache: it has Lua atomic state transition, idempotency, engine sequence, fencing epoch, durable ledger, PostgreSQL settlement, outbox, reconciliation, and paused/recovering product states.
WebSocket is tested as a room-scoped long-connection system with heartbeat, slow-client close, reconnect, and gap recovery.
Every performance claim is tied to PTS/k6 raw evidence and invariant verification.
```

## Do Not Do

- Do not claim `1000+` online from HTTP-only PTS.
- Do not claim accepted-bid TPS from mostly rejected pressure.
- Do not make Redis decide winner without a ledger and reconciliation.
- Do not use infinite optimistic-lock retries under one hot auction.
- Do not hide overload by dropping requests without explicit business response and metrics.
- Do not show optimistic frontend success before server/engine acceptance.
