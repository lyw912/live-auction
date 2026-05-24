# P3-07 NATS / JetStream Borrowing Review

Date: 2026-05-25 Asia/Shanghai

Status: `REVIEWED_BORROW_SELECTIVELY_DO_NOT_REBUILD`

Scope:

- Compare the current live-auction architecture against local NATS Server source under `references/oss/sources/nats-server`.
- Decide whether NATS + JetStream should be reused, reimplemented in part, or used as the basis of a full rewrite.
- Map the decision back to `docs/design-v2-industrial/00-project-brief.md`: official feature scope, two core challenges, and scoring terms.

## Judge Verdict

`PASS WITH DAMAGE`

Do not rebuild P3 around NATS + JetStream.

NATS + JetStream is a mature internal messaging and durable stream substrate. It is strongest at subject-based routing, durable streams, explicit consumer ack/redelivery policy, duplicate publish detection, slow-consumer handling, stream/consumer monitoring, and clustered replication/snapshot/catchup. It is not a replacement for this project's hardest auction path:

```text
bid command -> gateway auth/rate/idempotency -> PostgreSQL row lock
            -> rule/state validation -> bid/order/event/outbox transaction
            -> relay projection -> Redis history/snapshot -> WebSocket recovery
```

The defensible P3 position is selective borrowing:

```text
I studied NATS + JetStream's consumer, dedupe, slow-consumer, monitoring, and
replication/snapshot design, but I did not add a broker runtime because the
project is judged on auction correctness and browser recovery, not generic
service messaging. I borrowed the ideas that fit my domain-owned pipeline:
explicit delivery state, retry/backoff/dead-letter semantics, duplicate IDs,
consumer lag/ack-pending diagnostics, bounded pending bytes, and recovery
snapshot thinking. PostgreSQL still decides price, winner, terminal state, and
idempotency.
```

## NATS / JetStream Proof Checked

Local source anchors:

| NATS / JetStream Area | Source Anchor | What It Proves |
|---|---|---|
| Consumer delivery policy | `server/consumer.go:88`, `:92`, `:95`, `:96`, `:97`, `:98`, `:105`, `:120` | JetStream makes deliver policy, ack policy, ack wait, max delivery, backoff, max ack pending, and inactive threshold explicit consumer config. |
| Ack/Nak/Term semantics | `server/consumer.go:330`, `:386`, `:391`, `:647`, `:650`, `:670` | Delivery outcome is explicit: consumers ack, nak, or terminate; defaults are bounded and backoff can override ack wait/max deliver. |
| Consumer observability | `server/consumer.go:3471`, `:3485`, `:3491`; `server/store.go:396`, `:397`, `:449`, `:453` | Consumer info tracks redelivered and pending state, and the store persists redelivery metadata. |
| Publish dedupe | `server/jetstream_test.go:20782`, `:20787`, `:20798` | `Nats-Msg-Id` duplicate publish detection is tested; duplicate is visible in publish ack. |
| Max delivery reporting | `server/jetstream_test.go:19483`, `:19485`, `:19534`, `:19556`, `:19557`, `:19558` | Max delivery, ack wait, and redelivery reporting are treated as first-class tests. |
| Slow consumer handling | `server/client.go:155`, `:1813`, `:1853`, `:1862`, `:1889`, `:2513`, `:2520`, `:2534` | Slow consumers are detected by timeout and pending bytes and aggregated for monitoring. |
| Monitoring surface | `server/server.go:3010`, `:3011`, `:3017`, `:3019`, `:3020`, `:3115`, `:3117`, `:3131`, `:3135`, `:3137` | NATS exposes explicit server, connection, account, JetStream, and health monitoring endpoints. |
| Stream/source state | `server/stream.go:342`, `:343`, `:394`, `:395`, `:2947`, `:3062`, `:4308`, `:5684` | Streams expose state, mirror/source info, inbound queues, and source processing. |
| Raft/snapshot/catchup | `server/raft_test.go:3830`, `:3831`, `:3842`, `:3938`, `:3967`, `:4083`, `:4084`, `:4088`, `:4191`, `:4201`, `:4206`, `:4275` | Clustered durability depends on quorum, snapshot, catchup, compaction, and corruption/gap handling. |

## Current Project Proof Checked

| Current Area | Source Anchor | Verdict |
|---|---|---|
| Bid truth path | `backend/internal/auction/bid.go:125`, `:144`, `:162`, `:182`, `:662`, `:678`, `:831`, `:850`, `:894` | Stronger than JetStream for this domain. JetStream can deliver messages; it cannot decide legal bid increments, cap sold, cancel/end races, or idempotency result. |
| Atomic event/outbox append | `backend/internal/auction/repository.go:484`, `:486`, `:500`, `:506`, `:519` | Strong. Auction seq increment, event row, outbox row, and delivery row are written in the PostgreSQL transaction. |
| Ordered relay and poison behavior | `backend/internal/outbox/relay.go:195`, `:231`, `:244`, `:248`, `:257`, `:391`, `:425`, `:462`, `:486`, `:558`, `:576`, `:609` | Good. Existing relay already has claim state, shard lease ownership, same-auction head-of-line guard, classified errors, DEAD, gap notice, and watermarks. |
| Browser recovery | `backend/internal/realtime/server.go:192`, `:203`, `:211`, `:219`, `:253`, `:323`, `:335`, `:346`, `:349`, `:356`, `:373`, `:387`, `:421` | Good. Browser path uses scoped tickets, room ACL, history replay, seq-gap snapshot fallback, singleflight/semaphore bounded snapshot rebuild, and Redis snapshot projection. |
| P3 governing decision | `docs/p3-decision-log.md` P3-D13 | NATS/JetStream is already evidence-gated unless internal service messaging or relay delivery becomes a measured bottleneck. |

## Borrowing Decision Matrix

| NATS / JetStream Mechanism | Direct Reuse? | Borrow/Reimplement? | Project Landing | Priority |
|---|---:|---:|---|---|
| NATS core pub/sub runtime | No | No | Browser realtime remains app-owned WebSocket hub; direct browser NATS is out of scope. | No-go |
| JetStream durable stream | No | Maybe later | Only if current outbox/internal delivery is a measured bottleneck and ADR preserves PostgreSQL outbox truth. | Defer |
| Subject taxonomy | No | Yes | Document event subject/key convention if a future broker adapter exists: `auction.{room_id}.{auction_id}.events`, but domain ordering remains `auction_id + seq`. | P3/P4 doc |
| `Nats-Msg-Id` dedupe | No | Yes | Map to existing `outbox_events.event_key + seq + payload_sha256`; add future `broker_message_id = outbox_id` only if broker adapter is built. | Already partly / future |
| Consumer ack / ack wait / max deliver | No | Yes | Current `outbox_delivery.status`, `attempts`, `max_attempts`, `locked_until`, `last_error_class`, and `DEAD` are the project-owned equivalent. Borrow naming/diagnostic clarity. | Already partly |
| Ack pending / redelivery diagnostics | No | Yes | Extend monitor vocabulary around `ready_count`, `publishing_count`, `dead_count`, oldest ready age, attempts, and retry class. | Already partly |
| `+TERM` poison handling | No | Yes | Current non-retriable `PAYLOAD_INVALID` goes DEAD immediately and emits gap notice. Use JetStream as design justification. | Already implemented through Debezium borrowing |
| Slow consumer pending-byte policy | No | Already borrowed | Current self hub tracks queue messages and queue bytes, closes slow consumers, and exports queue metrics. | Already implemented through Centrifugo borrowing |
| Monitoring endpoints | No | Yes | Keep PC monitor and `/api/monitor/*` as domain-specific equivalent of varz/connz/jsz/healthz. Add broker-shaped metrics only when useful. | Already partly |
| Raft replicated stream | No | Conceptual only | Do not copy Raft. Borrow the lesson: recovery requires explicit snapshot/catchup/gap handling and invariant proof. | P4 verifier |
| Mirrors/sources | No | No for P3 | Useful only if cross-region or cross-service event distribution becomes scope. | P5+ |

## Full Rebuild Assessment

Full rebuild around NATS + JetStream is not justified for P3.

Reasons:

1. JetStream delivers messages after a publish. It does not solve bid legality, cap reachability, automatic extension, cap/cancel/end races, one order per auction, payment idempotency, or H5 recovery safety states.
2. The project already has a transactional outbox. Publishing directly from bid/cancel/end handlers to NATS would violate the engineering rule: PostgreSQL must commit truth before external delivery.
3. JetStream sequence is not the auction sequence. The project requires app-owned `auction_id + seq` so the client can detect gaps and recover by snapshot.
4. JetStream ack/redelivery is at-least-once. The project still needs dedupe, head-of-line policy, poison behavior, and snapshot fallback.
5. Adding NATS introduces a second operational subsystem: server config, streams, consumers, storage, dedupe windows, durable names, retention, monitoring, broker-down behavior, local startup, and rollback.
6. Current evidence points elsewhere: after the outbox claim fix, the next bottleneck candidate is PG hot-row/open-model pressure, plus clean realtime/multi-room drilldowns. NATS does not remove single-auction serialization.
7. Rebuild would invalidate P0/P1/P2 evidence and require a new failure bundle: broker down, consumer crash, duplicate delivery, same-auction ordering, poison, restart, backpressure, and diagnostics.

Rebuild only becomes reasonable if all are true:

- current DB-lease outbox relay or internal service delivery becomes the measured first bottleneck after P3-R5;
- the monolith is intentionally split into services that need durable internal messaging;
- PostgreSQL outbox remains commit truth before broker publish;
- ADR defines subjects, streams, retention, duplicate window, message ID, durable consumers, ack/redelivery/backoff/TERM mapping, ordering, metrics, local startup, and rollback;
- tests prove broker down, consumer crash, duplicate publish/delivery, same-auction order, poison delivery, broker restart, and bounded backpressure;
- browser recovery still uses app-owned `auction_id + seq`, Redis history/snapshot, gap notice, and snapshot fallback.

## Concrete Borrowing Plan

### P3: Documentation And Defensibility

1. Keep this review as the NATS/JetStream comparison evidence.
2. Update `docs/p3-decision-log.md` so the current accepted claim is selective borrowing only.
3. In interview/demo docs, claim only:

```text
Borrowed NATS/JetStream's delivery-state, ack/redelivery, dedupe, slow-consumer,
monitoring, and snapshot/catchup design logic; did not integrate NATS runtime.
```

4. Do not claim NATS performance advantage, horizontal scale, or broker durability unless a future ADR and test bundle actually integrate it.

### P3/P4: Low-Risk Improvements Worth Borrowing

These are useful without adding NATS:

1. Delivery lifecycle vocabulary:
   - Make monitor tables explicitly show `PENDING`, `PUBLISHING`, `FAILED`, `PUBLISHED`, `DEAD`, attempts, max attempts, next attempt, locked owner, and error class.
   - This is the project equivalent of JetStream consumer delivery/ack state.

2. Duplicate message identity:
   - Keep `outbox_id`, `event_key`, `auction_id`, `seq`, and `payload_sha256`.
   - If a broker adapter is ever introduced, use `outbox_id` or `event_key + seq` as the broker message id; never use JetStream sequence as domain order.

3. Redelivery and poison diagnostics:
   - Surface attempts histogram and oldest retry age beside watermarks.
   - Treat non-retriable payload failures like JetStream `TERM`: immediately DEAD + anomaly + gap notice.

4. Consumer lag model:
   - Keep `ready_count`, `publishing_count`, `dead_count`, `oldest_ready_age_ms`, last published outbox/auction/seq.
   - Add per-shard lag trend only if P3-R5 shows outbox second-order pressure.

5. Slow-consumer discipline:
   - Current queue byte/message caps are correct.
   - P3-R3 should prove healthy-vs-slow isolation and runtime profile before any transport replacement claim.

6. Snapshot/catchup thinking:
   - Do not copy Raft.
   - Use NATS's snapshot/catchup failure mindset to justify P4 invariant verifier and auction flight recorder: every gap must be explainable and recoverable.

### Defer Until Proven

- NATS server process in `docker-compose`.
- JetStream streams for auction events.
- NATS client SDK in backend mainline.
- Browser NATS/WebSocket replacement.
- Broker-backed fanout.
- Cross-service event consumers.
- JetStream mirrors/sources or multi-region topology.

## Mapping To Official Scope

| Official Scoring Term | NATS / JetStream Borrowing Landing |
|---|---|
| 完整工程链路 | Keep the existing item/rule/bid/order/payment/history/diagnostics chain. NATS does not replace it. |
| 竞拍数据采集 | Improve event delivery diagnostics: attempt count, retry age, dead-letter reason, consumer lag vocabulary. |
| 数据治理 | Reinforce explicit event identity: `outbox_id`, `event_key`, `auction_id`, `seq`, schema version, payload hash, stream epoch, snapshot version. |
| 后端服务 | Preserve row lock, state machine, idempotency, scheduler, transactional outbox. Borrow delivery-state and retry semantics after commit. |
| 接口网关 | No NATS role. Gateway still owns auth, ACL, schema, idempotency probe, and admission. |
| 前端交互 | H5 still sees server-authoritative auction events and snapshot recovery, not a generic broker protocol. |
| 系统可用性 | Borrow broker-down/consumer-crash/poison/retry drills as future failure gates if a broker adapter is introduced. |
| 性能 | No number from NATS. Only claim evidence-gated architecture discipline and smaller operational surface today. |
| 稳定性 | Bounded pending bytes, retry budgets, DEAD/TERM-like poison handling, duplicate/gap handling. |
| 可观测性 | Domain-specific monitor pages mirror the useful parts of varz/connz/jsz: health, connection pressure, delivery lag, dead letters. |
| 核心挑战优化 | Realtime recovery becomes more explainable; auction correctness remains in PostgreSQL transactions. |
| 独特思考 | Mature broker ideas are reduced to domain-owned delivery semantics instead of outsourcing the core challenge. |

## Mapping To Two Core Challenges

### 复杂竞拍规则

NATS/JetStream should not be used as the selling point here.

The answer must stay:

- PostgreSQL row lock serializes executable bid/cancel/end paths.
- Rule validation, fixed increment, cap sold, automatic extension, idempotency, and one-order uniqueness happen before any broker/delivery layer.
- Broker-style delivery can only carry already-committed truth.

### 毫秒级实时同步

NATS/JetStream helps as a design reference:

- Consumer ack model -> project `outbox_delivery` lifecycle.
- `Nats-Msg-Id` dedupe -> project `outbox_id/event_key/seq/payload_sha256`.
- Ack pending/redelivery info -> project relay watermarks, ready/publishing/dead counts, attempts.
- Slow consumer handling -> project WS queue count/byte budget and slow close.
- Snapshot/catchup -> project bounded history replay, snapshot fallback, and P4 invariant verifier.
- Monitoring endpoints -> project PC diagnostics and `/api/monitor/*` real producers.

## Competitive Position Against Direct NATS + JetStream Users

If another project directly uses NATS + JetStream, do not argue that NATS is bad. Argue scope and proof:

1. Direct JetStream proves durable messaging, not auction correctness. The evaluator still needs to see row locking, idempotency, cap/cancel/end race tests, and one order per auction.
2. JetStream is at-least-once with ack/redelivery. A good integration still needs dedupe and idempotent consumers. This project makes dedupe explicit with `auction_id + seq` and `outbox_id`.
3. JetStream sequence is broker-local. It cannot replace domain sequence unless they prove subject/stream partitioning exactly matches auction order and recovery semantics.
4. NATS adds an operational surface: stream retention, duplicate windows, durable consumers, storage, broker restart, monitoring, and local startup. This project intentionally kept the runtime smaller while P3 evidence points to PG hot-row/fanout/multi-room attribution.
5. Browser realtime is not automatically solved by NATS. A browser still needs auth, ACL, reconnect, gap detection, snapshot fallback, disabled dangerous CTA, and UI state rules.
6. If the competitor uses NATS well, acknowledge it. The differentiation is not "they are wrong"; it is "they solved generic messaging, while this project proves auction-domain correctness and recoverable browser state with fewer moving parts."

## Interview Drill

| Question | Defensible Answer | Code/Evidence To Show |
|---|---|---|
| Why not use NATS + JetStream directly? | It delivers messages but does not decide legal bids, winners, cap sold, or idempotency. Current bottleneck evidence does not point to internal messaging. | `backend/internal/auction/bid.go`, `docs/p3-decision-log.md` P3-D13 |
| What exactly was borrowed? | Consumer delivery-state thinking, ack/redelivery/backoff/TERM semantics, dedupe message id, slow-consumer pending-byte discipline, monitoring vocabulary, snapshot/catchup mindset. | This review; current outbox/realtime code |
| What is your JetStream equivalent of ack pending? | `outbox_delivery` states plus relay watermarks: ready, publishing, dead, last published outbox/auction/seq, oldest ready age. | `backend/internal/outbox/relay.go:609` |
| What is your message id? | Domain message identity is `outbox_id`, plus `event_key`, `auction_id`, `seq`, and `payload_sha256`; JetStream seq would not be auction seq. | `backend/internal/auction/repository.go:506` |
| What happens on duplicate delivery? | Relay is at-least-once; clients dedupe by `auction_id + seq`, gaps force snapshot, idempotency prevents duplicate money effects. | `backend/internal/realtime/server.go:323` |
| When would you adopt JetStream? | Only after internal service split or measured relay/internal delivery bottleneck, with ADR and broker-down/consumer-crash/poison/order tests. | `docs/p3-decision-log.md` NATS gate |
| Why are you stronger than a team that just says "we used NATS"? | I can show the transaction that creates truth, the outbox delivery state, recovery behavior, and tests. NATS alone is infrastructure, not domain proof. | `backend/internal/auction/repository.go:484`, `backend/internal/outbox/relay.go`, P0/P3 evidence |

## Implementation Follow-Up

Implemented after the review:

- `backend/internal/gateway/monitor_handlers.go` exposes broker-style delivery diagnostics from app-owned tables: `delivery_message_id`, `delivery_state`, `max_attempts`, `redelivery_count`, `ack_deadline_at`, `retry_age_ms`, ack-pending/retrying/redelivered shard counts, and slow-consumer pending-byte/message counts.
- `backend/internal/outbox/relay.go` records retrying, ack-pending, redelivered, and oldest retry age metrics per shard while keeping PostgreSQL outbox as truth.
- `backend/internal/realtime/hub.go` and `backend/internal/realtime/server.go` record slow-consumer pending-byte or pending-message reasons and queue pressure in real `user_activity_events`.
- `frontend/pc-console/src/main.tsx` surfaces outbox watermarks, snapshot rebuilds, and control signals beside the existing outbox/recovery diagnostics.
- Tests were added/updated in `backend/internal/outbox/relay_integration_test.go`, `backend/internal/realtime/server_integration_test.go`, and `backend/internal/gateway/monitor_integration_test.go`.

## Required Follow-Up Before Claiming This In Demo

- [DONE] Record this NATS/JetStream hostile comparison review.
- [DONE] Update P3 decision/evidence surfaces to classify NATS as selective borrowing, not runtime integration.
- [DONE] Implement the low-risk NATS/JetStream borrowing in project-owned code: delivery-state vocabulary, message identity diagnostics, redelivery/ack-pending visibility, TERM-style poison proof, slow-consumer queue pressure, and PC diagnostic visibility.
- [P3-R3] Prove clean self-hub fanout, healthy-vs-slow isolation, reconnect, and runtime profile before final realtime-runtime claims.
- [P3-R5] Recheck outbox second-order pressure before reconsidering broker-backed internal delivery.
- [P3-R6] If evidence points to broker adoption, write an ADR and implement a broker adapter behind PostgreSQL outbox, not in the bid transaction.

## Allowed And Forbidden Claims

Allowed:

- "I reviewed NATS + JetStream and selectively borrowed delivery-state, dedupe, redelivery, slow-consumer, monitoring, and snapshot/catchup design logic."
- "The project does not run NATS. It keeps a smaller app-owned transactional outbox because the hard part is auction correctness plus browser recovery."
- "NATS would become reasonable only after measured internal messaging need or service split."

Forbidden:

- "The project uses NATS."
- "NATS proves our realtime performance."
- "JetStream makes bid ordering correct."
- "JetStream sequence is our auction sequence."
- "Broker durability replaces PostgreSQL outbox or auction_events."
- "A full NATS rebuild is better without measured relay/internal messaging bottleneck evidence."
