# P3-06 Debezium Borrowing Review

Date: 2026-05-25 Asia/Shanghai

Status: `REVIEWED_BORROW_SELECTIVELY_IMPLEMENTED_DO_NOT_REBUILD`

Scope:

- Compare the current live-auction architecture against local Debezium source under `references/oss/sources/debezium`.
- Decide whether Debezium should be reused, reimplemented in part, or used as the basis of a full rewrite.
- Map the decision back to `docs/design-v2-industrial/00-project-brief.md`: official feature scope, two core challenges, and scoring terms.

## Judge Verdict

`PASS WITH DAMAGE`

Do not fully rebuild P3 around Debezium.

Debezium is a mature CDC framework. It is strongest at reading database change streams, preserving source offsets, coordinating snapshots and streaming, routing outbox records, applying backpressure between producer and consumer loops, and exposing operational state. It is not a replacement for this project's hardest auction logic:

```text
bid command -> gateway auth/rate/idempotency -> PostgreSQL row lock
            -> rule/state validation -> bid/order/event/outbox transaction
            -> relay projection -> Redis history/snapshot -> WebSocket recovery
```

The defensible P3 position is selective borrowing:

```text
I studied Debezium's CDC/outbox/snapshot/offset design, but I did not add a
Kafka Connect CDC subsystem because the project is judged on auction correctness
and recoverable realtime, not generic database replication. I borrowed the parts
that improve my own outbox and recovery model: offset/watermark discipline,
outbox envelope governance, bounded queues, snapshot lifecycle diagnostics,
operator signals, and explicit retriable/non-retriable failure handling.
```

## Debezium Proof Checked

Local source anchors:

| Debezium Area | Source Anchor | What It Proves |
|---|---|---|
| Outbox pattern | `documentation/modules/ROOT/pages/transformations/outbox-event-router.adoc:17`, `:120`, `:140`, `:427` | Debezium treats outbox as a way to avoid DB/event inconsistency, with event id, aggregate type/id, payload, routing key, and duplicate handling. |
| Bounded queue/backpressure | `debezium-connector-common/src/main/java/io/debezium/connector/base/ChangeEventQueue.java:39`, `:68`, `:70`, `:71`, `:147`, `:244` | Producer and poller are separated by a bounded queue with max item count and byte limit. |
| Offset context | `debezium-connector-common/src/main/java/io/debezium/pipeline/spi/OffsetContext.java:33`, `:59`, `:74`, `:80`, `:90`; `debezium-connector-postgres/src/main/java/io/debezium/connector/postgresql/PostgresOffsetContext.java:75`, `:83`, `:91`, `:103` | Debezium persists source position and snapshot state, then resumes from the last durable offset. |
| Snapshot then streaming | `debezium-connector-common/src/main/java/io/debezium/pipeline/ChangeEventSourceCoordinator.java:123`, `:149`, `:215`, `:225`, `:334`, `:359`, `:364` | Debezium explicitly coordinates snapshot and streaming phases, and updates signal context after phase transitions. |
| Signal actions | `debezium-connector-common/src/main/java/io/debezium/pipeline/signal/SignalProcessor.java:41`, `:57`, `:116`, `:147`, `:200`; `.../ExecuteSnapshot.java:41`, `.../PauseIncrementalSnapshot.java:16`, `.../ResumeIncrementalSnapshot.java:16` | Runtime control actions are separated from business writes and processed through enabled signal channels. |
| Error handling | `debezium-connector-common/src/main/java/io/debezium/pipeline/ErrorHandler.java:22`, `:51`, `:55`, `:64`, `:67`, `:136` | Producer failures are classified and propagated; retry policy is explicit. |
| Offset commit semantics | `debezium-api/src/main/java/io/debezium/engine/DebeziumEngine.java:126`, `:135`, `:140`, `:185`; `debezium-api/src/main/java/io/debezium/engine/spi/OffsetCommitPolicy.java:14`, `:64`; `documentation/modules/ROOT/pages/development/engine.adoc:615`, `:624` | At-least-once behavior and duplicate exposure after restart are expected; consumers must dedupe. |

## Current Project Proof Checked

| Current Area | Source Anchor | Verdict |
|---|---|---|
| Atomic auction event/outbox append | `backend/internal/auction/repository.go:480`, `:484`, `:499`, `:505`, `:512` | Strong. Domain mutation, seq increment, auction event, outbox row, and delivery row are in the same DB transaction. |
| Bid truth path | `backend/internal/auction/bid.go` via `PlaceBid`, `lockAuctionForBid`, `evaluateAndApplyBid` | Stronger than Debezium for this domain. Debezium can only observe committed changes; it cannot decide legal bid ordering, cap sold, or idempotency. |
| Ordered relay claim | `backend/internal/outbox/relay.go:211`, `:232`, `:264`, `:301`, `:370`, `:403`, `:413` | Good. It has shard leases, head-of-line guard, stream epoch, dead-letter anomaly, and gap notice. |
| Recovery and bounded snapshot | `backend/internal/realtime/server.go:323`, `:349`, `:371`, `:383`, `:425`, `:442` | Good. Recovery falls back to bounded DB snapshot path with semaphore and stale snapshot behavior. |
| Byte backpressure | `backend/internal/realtime/hub.go:30`, `:55`, `:103`, `:148`; `backend/internal/realtime/server.go:317` | Already matches a Debezium/Centrifugo-style bounded resource discipline. |
| P3 borrowed hardening | `docs/evidence/p3-05-centrifugo-borrowed-hardening-2026-05-25.md` | Existing evidence proves the project can borrow mature OSS ideas without replacing the runtime. Debezium should follow the same documentation pattern. |

## Borrowing Decision Matrix

| Debezium Mechanism | Direct Reuse? | Borrow/Reimplement? | Project Landing | Priority |
|---|---:|---:|---|---|
| Debezium PostgreSQL connector / WAL reader | No | Only if polling outbox becomes proven bottleneck | Future P4/P5 ADR after `outbox-hot-table` evidence | Defer |
| Kafka Connect / Debezium Server runtime | No | No | Adds Kafka/offset/schema-history operations outside P3 scope | No-go |
| Outbox Event Router schema discipline | No | Yes | Implemented explicit outbox envelope/version contract, event key semantics, and payload hash validation | P3 code |
| Offset and watermark model | No | Yes | Treat `auction_id + seq + stream_epoch + snapshot_version` as project-owned offset model; implemented per-shard relay watermarks/lag diagnostics | P3 code |
| Bounded ChangeEventQueue | No | Already partly borrowed | Keep queue count + byte caps; consider relay internal publish queue only if relay fanout blocks | Already/P4 |
| Snapshot lifecycle and notifications | No | Yes | Implemented snapshot rebuild status events: requested, started, completed, failed | P3 code |
| Runtime signals | No | Yes | Implemented admin-only DB-backed signals for force snapshot, retry dead outbox, pause/resume relay shard | P3 code |
| Retriable/non-retriable error classification | No | Yes | Implemented outbox failure rows/anomalies with stable error class and retry decision | P3 code |
| Incremental snapshot chunks/watermarks | No | Narrowly | Useful only for large historical replay/rebuild; not needed for active auction H5 snapshot | Defer |

## Full Rebuild Assessment

Full rebuild around Debezium is not justified for P3.

Reasons:

1. Debezium observes committed DB changes; it does not solve final-second bid serialization, cap/cancel/end races, or one-order-per-auction. Those must remain in `auction` code.
2. Debezium has at-least-once semantics. Its own engine documentation says restart can expose duplicate records depending on offset flush and batch behavior. That means this project would still need `auction_id + seq` dedupe and snapshot recovery.
3. Kafka Connect, schema history, replication slots, WAL retention, offset store, connector lifecycle, and SMT routing are an operational subsystem. P3 design explicitly prefers a modular monolith and says performance numbers need local evidence before claims.
4. The official non-goals exclude microservice expansion. A Debezium rebuild shifts attention from the two core challenges to CDC platform operations.
5. Current implementation has project-owned tests for bid/outbox/recovery gates. Replacing it would invalidate evidence and require a new failure bundle.

Rebuild only becomes reasonable if all are true:

- `outbox-hot-table` or relay polling is proven as the bottleneck under reproducible Linux baseline.
- The project needs cross-service event consumption beyond browser realtime.
- Kafka or another durable broker is accepted as a new explicit runtime dependency.
- A new ADR defines offset storage, duplicate handling, schema history, replication slot operations, and recovery drills.

## Concrete Borrowing Plan

### P3: Documentation and Defensibility

1. Add an ADR: "Debezium Borrowing Decision".
2. Record this review as evidence.
3. In demo/interview docs, claim only:

```text
Borrowed Debezium's outbox/offset/snapshot/backpressure/error-handling design
logic; did not integrate Debezium runtime.
```

4. Do not claim CDC performance advantage.

### P3: Implemented Low-Risk Code Improvements

1. Outbox envelope governance:
   - Added `event_schema_version`, `event_key`, and `payload_sha256` to `outbox_events`.
   - Event key is `auction_id`; ordering remains `auction_id + seq`.
   - Relay rejects unsupported schema version, empty key/hash, invalid JSON, or hash mismatch before publish.

2. Relay watermark diagnostics:
   - Track per-shard `last_published_outbox_id`, `last_published_seq`, `oldest_ready_age_ms`, `ready_count`, `publishing_count`, and `dead_count`.
   - Refresh only the current shard to avoid a new publish-time full-table aggregation bottleneck.
   - Expose diagnostics through `/api/monitor/outbox/watermarks`.

3. Snapshot lifecycle evidence:
   - Emit producer-backed rows for snapshot rebuild requested/started/completed/failed.
   - Add duration, source, stale flag, error class, and error message fields.
   - Expose diagnostics through `/api/monitor/snapshots`.

4. Error classification:
   - Store `last_error_class`, `last_error_retriable`, and `last_error_at` in `outbox_delivery`.
   - Classes: `REDIS_UNAVAILABLE`, `PUBLISH_TIMEOUT`, `PAYLOAD_INVALID`, `UNKNOWN`.
   - Non-retriable payload invalid goes DEAD immediately; retriable infra errors still use normal attempts/backoff.

5. Admin signals:
   - Added `system_control_signals` table outside bid/cancel/end transactions.
   - Supported signals: `force_snapshot_rebuild`, `retry_dead_outbox`, `pause_relay_shard`, `resume_relay_shard`.
   - Expose host-only `GET/POST /api/monitor/signals`.
   - Relay processes pending signals before batch delivery, borrowing Debezium's SignalProcessor pattern while staying app-owned.

Implementation evidence:

- `docs/evidence/p3-06-debezium-borrowed-hardening-2026-05-25.md`
- `goose -dir backend\migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up`
- `go test ./internal/outbox`
- `go test ./internal/gateway ./internal/auction`
- `go test -p 1 ./internal/scheduler`

### Defer Until Proven

- WAL/CDC relay replacement.
- Debezium embedded engine.
- Kafka Connect or Debezium Server.
- Incremental snapshot chunking for active auction state.

## Mapping To Official Scope

| Official Scoring Term | Debezium Borrowing Landing |
|---|---|
| 完整工程链路 | Keep the current item/rule/bid/order/payment/history/diagnostics chain. Debezium does not replace it. |
| 竞拍数据采集 | Improve event envelope, relay watermarks, snapshot lifecycle, and failure events. |
| 数据治理 | Make event id, aggregate key, seq, stream epoch, snapshot version, schema version, and replay boundary explicit. |
| 后端服务 | Preserve row lock, state machine, idempotency, scheduler, and transactional outbox. Borrow offset/error discipline around relay. |
| 接口网关 | No Debezium role. Gateway still owns auth, ACL, schema, idempotency probe, and rate limit. |
| 前端交互 | H5 still recovers by `auction_id + seq` and snapshot, not by an opaque CDC client. |
| 系统可用性 | Add relay lag/watermark diagnostics and classified retry/dead-letter behavior. |
| 性能 | No number from Debezium. Only claim reduced operational risk and clearer bottleneck visibility until baseline proves more. |
| 稳定性 | Bounded queue, bounded snapshot, retry classification, and explicit duplicate/gap handling. |
| 可观测性 | Snapshot and relay progress become producer-backed diagnostics, matching Debezium notification discipline. |
| 核心挑战优化 | Realtime recovery gets stronger without weakening final-second bid correctness. |
| 独特思考 | Mature CDC ideas are reduced to domain-owned auction offsets instead of outsourcing the core. |

## Mapping To Two Core Challenges

### 复杂竞拍规则

Debezium should not be used as the selling point here. The answer must stay:

- PostgreSQL row lock serializes executable bid/cancel/end paths.
- Rule validation, cap sold, extension, idempotency, and one-order uniqueness happen before any outbox/CDC delivery.
- Debezium-like outbox only carries already-committed truth.

### 毫秒级实时同步

Debezium helps as a design reference:

- Offset model -> project `auction_id + seq + stream_epoch`.
- Snapshot lifecycle -> project Redis snapshot plus bounded DB rebuild.
- At-least-once delivery -> project client dedupe and gap snapshot.
- Bounded queue -> project queue count and byte budget.
- Error notification -> project anomaly and diagnostic producers.

## Competitive Position Against Direct Debezium Use

If another project directly uses Debezium, do not argue that the self relay is universally better. Argue scope and proof:

1. Direct Debezium proves CDC extraction, not auction correctness. The evaluator still needs to see bid row locking, idempotency, cap/cancel/end races, and one order per auction.
2. Direct Debezium is at-least-once. A crash can replay duplicates unless the consumer has dedupe. This project makes dedupe explicit with `auction_id + seq`.
3. Direct Debezium usually introduces Kafka Connect/server, offsets, schema history, replication slots, and WAL retention. That is mature, but it is a larger operational surface than P3 needs.
4. Debezium outbox routing is useful for cross-service event distribution. This project is a single-process demo with browser realtime, so a DB-owned outbox relay is easier to inspect and test.
5. This project can point to one transaction that writes auction truth plus outbox rows, then to relay/recovery tests. A black-box CDC integration must still prove every downstream gap, duplicate, and snapshot path.
6. If the competitor uses Debezium well, acknowledge it. The differentiation is not "they are wrong"; it is "they solved a broader CDC problem, while this project solved the auction-specific correctness and recovery problem with a smaller, testable runtime."

## Interview Drill

| Question | Defensible Answer | Code/Evidence To Show |
|---|---|---|
| Why not use Debezium directly? | It cannot decide legal bids or winners; it only observes committed changes. P3 does not need Kafka Connect/CDC operations. | `backend/internal/auction/bid.go`, `repository.go:480` |
| What exactly was borrowed? | Outbox envelope discipline, offset/watermark thinking, bounded queues, snapshot lifecycle, signal/error handling patterns. | This review; future Debezium ADR |
| How do you handle Debezium-style duplicates? | Relay is at-least-once; clients dedupe by `auction_id + seq`, and gaps force snapshot. | `docs/design-v2-industrial/06-realtime-and-recovery.md`, `server.go:323` |
| What is your offset? | Not WAL LSN. Domain offset is `auction_id + seq`, guarded by stream epoch and snapshot version. | `relay.go:301`, `relay.go:336`, `server.go:391` |
| When would you adopt WAL/CDC? | Only after outbox polling/hot table is a measured bottleneck and cross-service event distribution is in scope. | `docs/design-v2-industrial/01-scope-and-roadmap.md` P2 candidate note |
| What if NOTIFY is lost? | It is only wakeup; polling remains correctness fallback. | `relay.go:78`, `relay.go:114`, P3 Centrifugo ADR |

## Required Follow-Up Before Claiming This In Demo

- [DONE] Add `docs/adr/p3-02-debezium-borrowing-decision.md`.
- [DONE] Add this review to the evidence index and P3 progress log.
- [DONE] Implement Debezium-specific hardening in app-owned code: envelope validation, watermark diagnostics, snapshot audit, admin signals, and error classification.
- [P5] Consider CDC/WAL only after final Linux baseline proves the current polling relay is the bottleneck.

## Allowed And Forbidden Claims

Allowed:

- "I reviewed Debezium and borrowed its CDC/outbox design discipline."
- "I kept Debezium out of the runtime because the project needs auction-domain correctness more than a generic CDC platform."
- "The current system uses a transactional outbox and domain sequence; this is the part Debezium's outbox pattern is meant to protect."

Forbidden:

- "The project uses Debezium."
- "Debezium proves our realtime performance."
- "CDC would make bids correct."
- "WAL CDC is better than the current relay without measured outbox bottleneck evidence."
