# Hotspot Redesign Roadmap And Score Mapping

Date: 2026-05-28

Status: proposed execution plan

Supersession note, 2026-05-29: current L4b intentionally uses Kafka/Redpanda as the required durable ledger. Earlier roadmap text preferring Redis Streams first is superseded by the Redis hot state + Kafka ledger + PostgreSQL settlement implementation.

## Goal

Turn PTS-1 from a weak point into the project's strongest differentiator:

- no 2s hidden tail under a single hot auction;
- no wrong winner, wrong price, duplicate bid/order, or missing event;
- clear proof that the system handles the official "100+ users final-second bid" challenge and has a credible route to 1000+ online watchers.

The roadmap explicitly covers the official technical keywords:

| Keyword | Roadmap owner |
|---|---|
| WebSocket 长连接 | fanout/reconnect workload, room routing, seq recovery |
| 心跳保活 | ping/pong, slow-client close, reconnect state machine |
| 乐观锁 | PG version/row lock baseline, Redis Lua/CAS, engine epoch fencing |
| 防抖节流 | H5 pending disable, gateway GCRA, auction lane, fanout coalescing |

## Integration Strategy

These tracks are not mutually exclusive. They should converge into one industrial architecture:

| Layer | Purpose | Integration policy |
|---|---|---|
| PostgreSQL baseline | Keep existing correctness and settlement truth. | Always retained as fallback. |
| PG lane | Reduce DB lock/pool convoying and expose overload. | Merge into mainline because it improves the baseline path. |
| Redis guard | Optional Redis prefilter/projection that protects PG without deciding winner/order. | Integrate behind `BID_ENGINE_MODE=postgres_lane|redis_guard|redis_ledger`; can merge earlier than ledger. |
| Redis ledger engine | Optional hot decision engine for extreme hotspot mode. | Integrate behind `BID_ENGINE_MODE=postgres_lane|redis_guard|redis_ledger`; default stays conservative until gates pass. |
| WebSocket fanout/reconnect | Realtime delivery plane for both PG and Redis modes. | Merge into mainline; not a competing branch. |
| Scoring/evidence docs | Explain why the architecture wins. | Keep with implementation evidence. |

Short-lived feature branches are still useful for code review, but the architecture should not fork into disconnected products. The intended final shape is one codebase with:

```text
default path: PostgreSQL truth + per-auction lane + WebSocket recovery
guard path: Redis guard + PostgreSQL truth + WebSocket recovery
extreme path: Redis ledger hot engine + PostgreSQL settlement + same WebSocket recovery
```

If time is short, ship the default path with `postgres_lane` and keep `redis_ledger` disabled but demonstrable with evidence. If `redis_ledger` passes failure gates, enable it for dedicated hotspot profiles.

## Layered Target Architecture

The final implementation should be planned as layers that compose, not as isolated tasks:

| Layer | Name | Responsibility | Main modules | UI surface | Runtime switch |
|---|---|---|---|---|---|
| L0 | Evidence and baseline | Preserve PTS-1 facts and invariant gates. | `docs/perf/pts`, `tests/pts`, invariant checker | none, docs only | n/a |
| L1 | Admission and debounce | Stop useless pressure before serialization. | H5 bid dock, gateway GCRA, local pending state | pending disable, retry copy, cooldown | `ADMISSION_ENABLED`, limit envs |
| L2 | PostgreSQL lane | Bound hot-auction DB concurrency while preserving final DB truth response. | gateway/auction lane, repository metrics | same existing bid result UI plus retry-too-hot state | `BID_ENGINE_MODE=postgres_lane` |
| L3 | Realtime delivery | Room-scoped WebSocket, heartbeat, recovery, slow-client isolation. | realtime hub/server, Redis history/snapshot | reconnecting, recovered, stale/gap, live rank updates | always on |
| L4a | Redis guard | Redis Lua prefilter/projection that protects PG without deciding winner/order. | redisx scripts, projection updater, gateway guard | fast too-low/ended/retry feedback, no fake success | `BID_ENGINE_MODE=redis_guard` |
| L4b | Redis ledger engine | Hot auction state machine in Redis Lua with durable ledger and async settlement. | redisx scripts, bid engine, command log, settlement worker, reconciler | engine accepted, pending settlement, settled, paused/reconciling | `BID_ENGINE_MODE=redis_ledger` |
| L5 | Diagnostics and judge evidence | Prove bottleneck removal and correctness. | monitor APIs, metrics, PTS/k6 reports | PC diagnostics: queue/engine/settlement/WS panels | n/a |

Important dependency rule:

```text
L1 + L2 + L3 can ship together as the safe mainline.
L4a depends on L1 + L2 + L3 and can merge earlier because PG remains truth.
L4b depends on L1 + L3 + L5 and must preserve invariant evidence before it is enabled by default.
```

## Milestones

### M0: Evidence Lock

Deliverables:

- preserve current PTS-1 raw evidence;
- record environment, JMX, git SHA, DB/Redis settings;
- write "why 2s P99 is unacceptable" note in final materials.

Exit criteria:

- baseline can be reproduced or at least directly compared.

Layer coverage:

| Layer | Work |
|---|---|
| L0 | Freeze baseline evidence and current bottleneck attribution. |
| L5 | Create evidence index and final defense wording. |
| UI | No product UI change; only document current latency risk honestly. |

### M1: Admission, Debounce, And PostgreSQL Lane Rescue

Implementation:

- H5 bid button enters `pending` immediately after click and ignores repeated clicks until response/timeout.
- H5 displays retryable overload separately from normal bid rejection.
- gateway keeps completed idempotency replay before any limiter/queue.
- per-auction bounded queue;
- idempotency replay before queue;
- one worker per hot auction by default;
- fast retry when queue too deep or wait budget exceeded;
- metrics for queue depth/wait/reject and transaction duration.
- H5 bid button remains pending-disabled until server response or timeout.
- gateway still applies user/IP/auction GCRA throttling before DB pressure.

Expected result:

- admitted bid P99 materially lower than 2265ms;
- DB pool wait and row-lock wait reduced;
- overload appears as explicit retryable business response, not silent 2s waiting.

Risk:

- accepted throughput may not rise much; this is tail control, not engine breakthrough.

Layer coverage:

| Layer | Backend | UI | Test evidence |
|---|---|---|---|
| L1 | GCRA remains; add clear retry-after semantics. | pending disable, cooldown, retry text. | rapid-click test: one in-flight bid; retry state rendered. |
| L2 | per-auction lane before `PlaceBid`; queue metrics. | no optimistic success; show `BID_AUCTION_TOO_HOT` as retryable. | queue full/timeout/replay bypass tests; PTS before/after. |
| L5 | metrics added to `/metrics` and PC monitor candidate. | optional PC queue pressure badge. | metrics snapshots saved with PTS. |

### M2: Transaction Slimming

Implementation:

- measure locked transaction section;
- remove any non-essential work from lock window;
- ensure ordinary non-state rejects do not produce full-room durable realtime events;
- keep accepted/cap/end/cancel/order/outbox in the same transaction.

Expected result:

- lower DB lock hold time;
- reduced outbox pressure from ordinary rejects.

Layer coverage:

| Layer | Backend | UI | Test evidence |
|---|---|---|---|
| L2 | shorten locked DB section; keep accepted/cap/order/outbox atomic. | unchanged result semantics. | bid accepted/reject/cap/cancel/order integration tests. |
| L3 | avoid flooding room WS with ordinary non-state rejects. | current user still sees HTTP reject; room only sees stateful events. | rejected event policy tests; outbox lag comparison. |
| L5 | add tx duration and reject distribution interpretation. | PC diagnostics reject distribution remains true. | DB lock/tx metrics before/after. |

### M3: Redis Guard Then Redis Ledger Prototype

Implementation:

- `BID_ENGINE_MODE=redis_guard` first;
- Redis Lua guard for clearly invalid bids using projected current price/status/end_at;
- if guard is uncertain or projection is stale, fall through to PostgreSQL or return explicit retry;
- guard never declares winner/order/sold;
- measure guard reject rate and PG lock reduction;
- then add `BID_ENGINE_MODE=redis_ledger`;
- Lua script for manual bid only: active status, end time, increment, cap, soft close, idempotency;
- append ledger entry to Redis Stream in same script;
- return `ENGINE_ACCEPTED`/`ENGINE_REJECTED`;
- settlement worker consumes stream and writes PostgreSQL rows/outbox;
- reconciling/paused state for gaps and poison settlement.
- WS emits engine accepted/rejected, settled, reconciling, paused, and gap notice events.
- heartbeat/reconnect path is tested while Redis engine pressure is running.

Deliberate exclusions:

- max/proxy bid in Redis;
- multi-region;
- real payment;
- Flink stream processing; settlement remains app-owned.
- multi-node production Kafka proof; local Redpanda is a functional test topology.

Expected result:

- hot accepted/rejected decision moves from DB row lock to Redis single-threaded atomic script;
- DB work becomes async settlement, so HTTP P99 can drop sharply if Redis is healthy and script is short.

Concrete components:

| Component | Responsibility | First implementation scope |
|---|---|---|
| `BidEngine` interface | Hide PG lane vs Redis ledger decision path. | `PlaceBid(ctx, request) -> BidResponse` with mode dispatch. |
| `PostgresEngine` | Existing repository path plus lane. | Default mode. |
| `RedisGuard` | Reject clearly invalid/stale pressure before PG truth transaction. | No winner/order mutation. |
| `RedisLedgerEngine` | Lua state transition and ledger append. | Manual bid only. |
| Redis Lua script | Atomic rule validation, idempotency, current price/winner/end_at, cap sold, pending-decision marker. | No proxy/max-bid loop. |
| `AuctionCommandLog` | Kafka/Redpanda durable ledger behind a small interface. | Local Redpanda for functional gates; production requires replicated brokers. |
| Settlement worker | Consume ledger and write PostgreSQL bid/event/outbox/idempotency/order. | At-least-once, idempotent. |
| Reconciler | Compare Redis state, ledger, DB, outbox. | pause on gap/poison/divergence. |
| Gateway response adapter | Preserve old response where possible, add settlement fields. | `ENGINE_ACCEPTED`, `ENGINE_REJECTED`, `ENGINE_SOLD`. |

Layer coverage:

| Layer | Backend | UI | Test evidence |
|---|---|---|---|
| L4a | Redis guard and projection freshness. | fast reject/retry; no fake success. | guard reject correctness; stale projection fail-open/fail-closed policy tests. |
| L4b | Redis Lua, stream ledger, settlement, reconciler. | new bid states: engine accepted, pending settlement, settled. | Lua rule matrix; crash/replay; duplicate retry; settlement idempotency. |
| L3 | publish engine and settlement events over same room WS. | price updates from engine seq; settlement badge disappears after settled. | WS gap/recovery while settlement lags. |
| L5 | engine latency, Redis script latency, settlement lag, reconciliation metrics. | PC diagnostics engine mode/lag/pause panel. | Redis-engine PTS profile and invariant checker. |

### M4: Reconciliation And Failure Proof

Required failure cases:

- Redis accepted, settlement worker killed, restart replays and settles.
- duplicate client retry returns same engine result.
- DB unique conflict is treated as idempotent settlement, not fatal.
- settlement poison pauses auction and emits anomaly/gap.
- Redis restart from persistence either restores state or pauses for rebuild.
- stale engine epoch cannot overwrite DB settlement.

Exit criteria:

- invariant checker passes after every failure run.

Layer coverage:

| Layer | Backend | UI | Test evidence |
|---|---|---|---|
| L4b | pause/rebuild/replay logic; stale epoch rejection. | `RECONCILING` and `ENGINE_PAUSED` disable bid CTA. | Redis ahead/DB ahead/missing stream/stale epoch tests. |
| L3 | gap notice and snapshot after pause/recovery. | recovering state then authoritative snapshot. | reconnect after gap and paused-auction tests. |
| L5 | anomaly events and repair report. | PC monitor shows incident, source, repair status. | reconciliation report artifact. |

### M5: PTS And Judge Evidence

Run before/after:

- original PTS-1 JMX against `main`;
- same JMX against `postgres_lane`;
- Redis-engine JMX profile with settlement lag and reconciliation metrics;
- WebSocket fanout/reconnect profile separately.
- H5 debounce/throttle manual trace: rapid clicks produce one pending request plus clear retry/pending states.
- heartbeat drill: dead client is closed; healthy client receives the same auction seq without stale ranking.

Evidence table:

| Metric | Baseline | PG lane | Redis ledger | Required interpretation |
|---|---:|---:|---:|---|
| bid p99 | current ~2265ms | lower | much lower target | same environment only |
| DB pool wait | high | lower | settlement-only | prove pressure moved |
| row lock wait | high | lower | near-zero on HTTP path | prove bottleneck removed |
| Redis latency | current low | low | must stay low | script not too slow |
| settlement lag | n/a | n/a | bounded | DB catches up |
| invariant failures | 0 | 0 | 0 | non-negotiable |

Layer coverage:

| Layer | Evidence |
|---|---|
| L1 | rapid-click/debounce trace, rate-limit counters, retry-after correctness. |
| L2 | PTS-1 PG lane before/after, DB lock/pool/tx metrics. |
| L3 | 1000 WS live-connection run, heartbeat close, reconnect recovery, fanout lag. |
| L4a | Redis guard reject rate, false-reject safety, PG lock/pool reduction. |
| L4b | Redis engine p99, Redis latency, settlement lag, replay/reconciliation proof. |
| L5 | final evidence index and judge defense table. |

## Score Mapping

Detailed word-by-word scoring decomposition is maintained in:

- `docs/perf/pts/scoring-dimension-decomposition-2026-05-28.md`

| Official scoring item | Evidence this roadmap creates |
|---|---|
| Complete engineering chain | bid engine, settlement, outbox, WS, UI state, monitor, reconciliation |
| Availability | queue backpressure, paused/reconciling mode, crash replay |
| Performance | direct before/after PTS evidence, bottleneck attribution |
| Stability | bounded queues, ledger replay, idempotency, slow-client gates |
| Data consistency | engine seq, DB settlement, invariant verifier, unique constraints |
| Observability | engine/queue/settlement/reconciliation metrics and anomalies |
| Technical depth | comparison of PG lock, actor lane, Redis Lua, ledger, outbox, Kafka/Flink evolution |
| Innovation | hot-path Redis state machine with product-honest settlement states |
| H5 realtime experience | WebSocket long connection, heartbeat recovery, immediate engine feedback, debounce/pending states |
| PC diagnostics | show hot auction queue/engine state, settlement lag, reconciliation incidents |

## Architecture Gates Before Merge

Do not merge `redis_ledger` unless all are true:

- A single command log can replay every accepted bid in order.
- Redis state can be rebuilt or the auction can be safely paused.
- PostgreSQL settlement is idempotent.
- Client never sees settled/order state before DB commit.
- All terminal transitions are serialized through one engine path.
- Invariant checker proves one winner/order and seq continuity.

## Recommended Next Step

Implement M1 first because it is low-risk and gives immediate PTS improvement. In parallel, implement M3 on a separate branch and do not let it block stable correctness work.

The final project story should not be "we avoided Redis because it is risky". It should be:

```text
We measured the DB hot row ceiling, shipped a safe queue rescue, then built a Redis-ledger hot engine with replay, fencing, reconciliation, and product-visible settlement states. That is the difference between a demo and an industrial bidding system.
```
