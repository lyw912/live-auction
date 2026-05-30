# Route B+ Phase 4 Reconciliation Checkpoint Evidence

Date: 2026-05-30

Scope:

- `backend/internal/redisengine`
- `backend/migrations/202605300002_redis_engine_checkpoints.sql`

Design inputs:

- `docs/perf/pts/l4b-kafka/single-hotspot-redesign-from-first-principles-2026-05-30.md`
- `docs/perf/pts/l4b-kafka/route-b-implementation-plan-2026-05-30.md`

## Implemented

Phase 4 now has a durable settlement checkpoint instead of a documentation-only
recovery claim.

- Added `auction_engine_checkpoints`.
- Settlement writes the checkpoint in the same PostgreSQL transaction that marks
  the Kafka decision `SETTLED`.
- The checkpoint stores:
  - auction id;
  - engine epoch and engine seq;
  - decision topic, partition, and next offset;
  - canonical state hash;
  - canonical snapshot JSON.
- Reconciler now checks:
  - non-terminal settlement rows;
  - engine-seq gaps;
  - accepted public seq gaps;
  - winner/price drift;
  - sold/order uniqueness;
  - bid idempotency response drift;
  - outbox coverage;
  - missing, lagging, invalid, or hash-drifted checkpoints.
- Any detected invariant violation pauses only the affected auction with a
  specific reason and anomaly payload.

The implementation keeps Kafka's at-least-once consumer model explicit:
settlement remains idempotent by business identity and offsets are committed
only after successful processing. This follows the Kafka consumer contract that
committed offsets are the consumer restart point, so committing before durable
processing can skip work.

## Tests Added

- `TestReconcileHealthyActiveAuctionWithoutAcceptedBids`
- `TestReconcileDetectsAcceptedPublicSeqGap`
- `TestReconcileDetectsWinnerPriceDrift`
- `TestReconcileDetectsOutboxCoverageMissing`
- `TestKafkaSettlementWritesEngineCheckpoint`
- `TestReconcileDetectsMissingEngineCheckpoint`
- `TestReconcileDetectsEngineCheckpointHashDrift`

Existing Phase 1-3 tests continue to cover:

- Kafka append before HTTP success;
- pre-settlement Redis idempotency replay;
- duplicate Kafka message idempotency;
- same engine seq / same offset / same client bid conflict handling;
- stale epoch pause;
- transient future seq without committing offset;
- retry-to-DLQ and reconcile pause.

## Verification

Local database migration:

```bash
/root/go/bin/goose -dir backend/migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
```

Focused package test:

```bash
go test ./internal/redisengine
```

Full backend test:

```bash
go test ./...
```

Before the full backend run, stale `auc_engine_%` ACTIVE fixtures from previous
redisengine integration runs were cleaned because the shared monitor test only
returns a bounded active-auction page:

```bash
docker exec live-auction-postgres psql -U live_auction -d live_auction -c "update auctions set status='ENDED', updated_at=now() where id like 'auc_engine_%' and status='ACTIVE';"
```

## Review Notes

- The checkpoint is not written for synthetic Redis-stream style messages that
  do not carry a real topic/partition/offset. Rebuild-from-Kafka claims require
  Kafka offset metadata.
- `MemoryLedger` now assigns a process-unique partition to each ledger instance
  so integration tests model Kafka's global uniqueness of `(topic, partition,
  offset)` across auctions.
- No performance number is claimed here. Phase 6 still requires formal PTS
  100% sampling logs, server histograms, Redis/Kafka/PostgreSQL evidence, and
  correctness verifier output after pressure/failure drills.

## External References

- Apache Kafka consumer offset and at-least-once notes:
  https://kafka.apache.org/42/javadoc/org/apache/kafka/clients/consumer/KafkaConsumer.html
- Kafka log/consumer offset distribution background:
  https://kafka.apache.org/40/implementation/distribution/
- kafka-go writer API reference:
  https://pkg.go.dev/github.com/segmentio/kafka-go
