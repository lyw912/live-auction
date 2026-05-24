# ADR P3-02 · Debezium Borrowing Decision

Date: 2026-05-25 Asia/Shanghai

Status: Accepted · Implemented Selective Borrowing

Origin:

- `docs/reviews/p3-06-debezium-borrowing-review-2026-05-25.md`
- Local source: `references/oss/sources/debezium`

## Context

P3 is evaluating mature open-source systems before making data-path changes. Debezium is a mature CDC framework with PostgreSQL connectors, offset storage, snapshot/streaming coordination, outbox routing, bounded event queues, signal processing, and explicit error handling.

The project design still says:

- PostgreSQL is auction money truth.
- Redis and WebSocket are projection/delivery.
- Performance or scale changes need measured evidence.
- Debezium/CDC is evidence-gated until the outbox relay is again a measured bottleneck.

## Decision

Do not integrate Debezium, Kafka Connect, Debezium Server, or a WAL CDC relay into the P3 runtime.

Borrow Debezium design logic selectively:

1. Outbox envelope and schema governance.
2. Offset/watermark discipline mapped to `auction_id + seq + stream_epoch + snapshot_version`.
3. Bounded queue and byte-budget backpressure.
4. Snapshot lifecycle diagnostics.
5. Operator signal pattern for non-bid-path recovery actions.
6. Retriable vs non-retriable outbox failure classification.

## Rationale

Debezium is valuable, but it observes committed database changes. It does not decide:

- whether a bid is legal;
- whether cap price sells;
- whether cancel/end/bid races produce one terminal state;
- whether payment/order idempotency is correct;
- whether H5 is allowed to bid during recovery.

Those remain project-owned auction-domain responsibilities.

The current implementation already has a domain transactional outbox: auction state changes append `auction_events`, `outbox_events`, and `outbox_delivery` rows in the same DB transaction. Replacing this with a CDC runtime would add offset storage, schema history, replication slots, WAL retention, connector lifecycle, and duplicate handling without removing the need for project-owned seq/gap/snapshot semantics.

## Implementation Landing

| Borrowed Idea | Project Landing |
|---|---|
| Outbox event id/key/payload discipline | `outbox_events` now stores `event_schema_version`, `event_key`, and `payload_sha256`; relay validates them before Redis/WS publish. |
| Source offset and snapshot markers | Treat `auction_id + seq + stream_epoch + snapshot_version` as the domain offset model; expose per-shard relay watermark diagnostics. |
| ChangeEventQueue limits | Keep existing WS queue count/byte limits; add relay internal queue only if publish/fanout blocks under evidence. |
| Snapshot notifications | `snapshot_rebuild_events` records requested/started/completed/failed rebuild phases with source, duration, and error class. |
| SignalProcessor | `system_control_signals` supports host-only `force_snapshot_rebuild`, `retry_dead_outbox`, `pause_relay_shard`, and `resume_relay_shard`; it is processed outside bid/cancel/end transactions. |
| ErrorHandler | `outbox_delivery` stores stable error class, retriable flag, error time, and non-retriable payload failures go DEAD immediately. |

Implemented files:

- `backend/migrations/202605250002_debezium_borrowed_outbox_diagnostics.sql`
- `backend/internal/auction/repository.go`
- `backend/internal/scheduler/scheduler.go`
- `backend/internal/outbox/relay.go`
- `backend/internal/gateway/monitor_handlers.go`
- `backend/internal/gateway/router.go`
- `backend/internal/outbox/relay_integration_test.go`
- `backend/internal/gateway/monitor_integration_test.go`

Evidence:

- `docs/evidence/p3-06-debezium-borrowed-hardening-2026-05-25.md`

## Go Criteria For Actual CDC

Reopen Debezium/WAL CDC only if all are true:

- longer/multi-room outbox pressure proves current polling/shard-lease relay is the first bottleneck;
- evidence includes backlog, lag, claim/update plans, and table bloat;
- cross-service event distribution is in scope;
- ADR defines topic/key/partition mapping, Debezium offset vs auction seq, snapshot/bootstrap mode, duplicate handling, replication slot/WAL loss recovery, local startup, rollback, and diagnostics;
- app-owned Redis history/snapshot and WebSocket recovery semantics remain intact.

## No-Go Conditions

Do not adopt Debezium while:

- PG hot-row contention, browser fanout, admission, or local environment is the first bottleneck;
- the reason is Debezium maturity rather than project evidence;
- the design makes CDC, Kafka offsets, or Redis stream IDs authoritative for auction price/winner/order;
- it requires replacing P0/P1/P2 evidence without a new failure bundle.

## Interview Position

```text
I studied Debezium's CDC and outbox implementation, but I did not use it as a
black-box runtime because this project is judged on auction correctness and
recoverable realtime. I borrowed the parts that fit the domain: outbox envelope
governance, offset/watermark thinking, bounded queues, snapshot lifecycle,
operator signals, and explicit failure classification. The auction result still
comes from PostgreSQL row-lock transactions, and realtime clients recover by
auction seq or snapshot.
```

## Claims Allowed

Allowed:

- Debezium was reviewed and used as a design reference.
- The project borrows CDC/outbox offset discipline in app-owned code.
- Full Debezium runtime is intentionally deferred behind measured outbox evidence.

Forbidden:

- The project uses Debezium.
- Debezium improves this project's current performance numbers.
- CDC replaces auction correctness logic.
- WAL CDC is superior without a measured relay bottleneck.
