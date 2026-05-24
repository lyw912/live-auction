# P3-06 Debezium-Borrowed Outbox Hardening Evidence

Date: 2026-05-25 Asia/Shanghai

Status: `AUTHORITATIVE`

Origin:

- Review: `docs/reviews/p3-06-debezium-borrowing-review-2026-05-25.md`
- ADR: `docs/adr/p3-02-debezium-borrowing-decision.md`
- Local OSS source: `references/oss/sources/debezium`

## Decision

Implemented selective Debezium borrowing inside the existing app-owned outbox/recovery architecture.

No Debezium runtime, Kafka Connect, Debezium Server, WAL reader, Kafka topic, Redis Stream truth path, or browser CDC client was added.

PostgreSQL remains auction truth. Redis/WebSocket remain projection and delivery.

## Borrowed Points Implemented

| Borrowed Debezium Idea | Project Implementation | Why It Matters To Judges |
|---|---|---|
| Outbox envelope governance | `outbox_events` has `event_schema_version`, `event_key`, and `payload_sha256`; relay validates before publish. | Prevents silent schema/hash drift and lets the candidate explain event contract evolution instead of "JSON blob relay". |
| Offset/watermark visibility | `outbox_relay_watermarks` records per-shard last published outbox/auction/seq, ready age, ready/publishing/dead counts. | Maps Debezium offset visibility to domain offsets without adding WAL CDC. |
| Snapshot lifecycle notifications | `snapshot_rebuild_events` records REQUESTED/STARTED/COMPLETED/FAILED with source, duration, stale flag, error class/message. | Makes recovery auditable; diagnostic panels have a real producer. |
| Runtime signals | `system_control_signals` supports host-only force snapshot rebuild, retry dead outbox, pause/resume relay shard. | Borrows Debezium's SignalProcessor pattern for operator actions while keeping bid path clean. |
| Retriable/non-retriable errors | `outbox_delivery` stores `last_error_class`, `last_error_retriable`, `last_error_at`; `PAYLOAD_INVALID` goes DEAD immediately. | Distinguishes poison payload from transient Redis/timeouts and creates explainable dead-letter behavior. |

## Code Landed

| Area | Files |
|---|---|
| Migration | `backend/migrations/202605250002_debezium_borrowed_outbox_diagnostics.sql` |
| Outbox writers | `backend/internal/auction/repository.go`, `backend/internal/scheduler/scheduler.go` |
| Relay behavior | `backend/internal/outbox/relay.go` |
| Monitor APIs | `backend/internal/gateway/monitor_handlers.go`, `backend/internal/gateway/router.go` |
| Tests | `backend/internal/outbox/relay_integration_test.go`, `backend/internal/gateway/monitor_integration_test.go` |

## Design Details That Must Survive Grilling

### Payload Hash

The hash is generated in PostgreSQL from the stored JSONB text:

```sql
encode(digest(convert_to($payload::jsonb::text, 'UTF8'), 'sha256'), 'hex')
```

Reason: if Go hashed the pre-insert marshaled bytes, PostgreSQL JSONB normalization could change key order/spacing before relay reads the row. The implemented hash is over the database representation that the relay later scans. This is the defensible version.

### Watermarks

Watermark refresh is shard-scoped after publish/failure. It does not run a full `outbox_delivery` aggregation on every event.

Reason: Debezium-style offset visibility is valuable, but adding it must not reintroduce a hot-table publish bottleneck.

### Signals

Signals are separate from bid/cancel/end transactions and processed by relay before delivery batches.

Reason: operator recovery actions must not be part of the money path. Host API validates signal/target combinations before insert; relay validates again before execution.

### Failure Classification

`PAYLOAD_INVALID` is non-retriable and DEAD immediately. Redis/timeouts are retriable and keep the normal attempt budget.

Reason: retrying a poison event wastes capacity and blocks same-auction head-of-line. Infrastructure failures can recover.

## Verification

Commands run:

```powershell
goose -dir backend\migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
go test ./internal/outbox
go test ./internal/gateway ./internal/auction
go test -p 1 ./internal/scheduler
```

Results:

- Migration applied through version `202605250002`.
- `internal/outbox` passed.
- `internal/gateway` passed.
- `internal/auction` passed.
- `internal/scheduler` passed with `-p 1` after an earlier Windows-local linker memory failure when multiple packages were tested together.

New focused test coverage:

| Test | What It Proves |
|---|---|
| `TestRelayPublishesPendingOutboxToRedisInOrder` | Redis envelope now carries schema version, event key, payload, and payload hash while preserving seq order. |
| `TestRelayInvalidEnvelopeDeadLettersWithoutRetry` | Corrupt hash is detected before publish and becomes `PAYLOAD_INVALID` DEAD with gap notice. |
| `TestRelayPoisonMarksDeadAndWritesAnomaly` | Transient Redis failure is classified as retriable `REDIS_UNAVAILABLE`; DEAD anomaly/gap notice still happen when attempts are exhausted. |
| `TestRelayRebuildSnapshotWritesRedisSnapshot` | Snapshot rebuild writes Redis snapshot and audit phases REQUESTED/STARTED/COMPLETED. |
| `TestRelayProcessSignalsRebuildsSnapshotAndRetriesDeadOutbox` | Control signals actually rebuild snapshot and move a DEAD delivery back to PENDING. |
| `TestRelayProcessSignalsPauseAndResumeShard` | Control signals can pause a shard lease even when no lease exists and later resume it by removing the paused lease. |
| `TestMonitorRoutesReturnRealDBRowsAndRequireHost` | New watermark/snapshot/signal monitor routes are host-only and backed by DB rows; invalid signal requests return 400. |

## Mapping To Official Scope And Scoring

| Official / Design Term | Improvement |
|---|---|
| 完整工程链路 | Event/outbox/recovery/diagnostics chain now has envelope governance, watermarks, and operator recovery. |
| 竞拍数据采集 | Outbox events include explicit key/version/hash and can be audited like CDC records. |
| 数据治理 | Schema version, key, payload hash, domain seq, stream epoch, and snapshot version are explicit. |
| 后端服务 | No weakening of row-lock bid correctness; all borrowing stays after DB commit in outbox/recovery. |
| 系统可用性 | DEAD, retryable/non-retryable errors, and operator retry/rebuild actions are visible and executable. |
| 稳定性 | Poison events stop retrying; infra failures remain retryable; gaps still force snapshot recovery. |
| 可观测性 | Monitor APIs expose outbox rows, relay watermarks, snapshot lifecycle, and control signals from real producers. |
| 核心挑战优化 | Realtime recovery becomes more diagnosable without replacing auction correctness with generic CDC. |
| 独特思考 | Mature Debezium ideas were reduced to domain-owned offsets instead of importing a large runtime outside scope. |

## Allowed Claims

- "I reviewed Debezium and selectively borrowed its outbox, offset/watermark, snapshot lifecycle, signal, and error-classification ideas."
- "The project does not run Debezium. It keeps a smaller app-owned transactional outbox because the hard part is auction correctness plus browser recovery."
- "The borrowed mechanisms are implemented and tested in the relay/monitor path."

## Forbidden Claims

- "The project uses Debezium."
- "Debezium improved our performance numbers."
- "CDC makes bid ordering correct."
- "WAL CDC is better than the current relay without fresh outbox bottleneck evidence."

## Competitive Answer Against Direct Debezium Users

If another team directly uses Debezium, the fair answer is:

```text
Their solution proves generic CDC extraction. Mine proves auction-domain correctness and recovery with a smaller runtime. Debezium still emits at-least-once records, so they must separately prove dedupe, same-auction order, poison handling, snapshot recovery, and bid/cancel/end correctness. I borrowed the parts that matter here: envelope governance, durable offsets, snapshot lifecycle, signals, and error classification, but kept PostgreSQL row-lock transactions as the auction truth.
```
