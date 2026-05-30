# PTS L4b Kafka Ledger Recovery Fix

Date: 2026-05-29

Triggered by: `docs/reviews/pts-l4b-kafka-ledger-judge-review-2026-05-29.md`

## Scope

This record closes the P0 recovery issues found in the L4b hostile review.

Implemented changes:

- Redis pending decisions are now recoverable into Kafka during reconciliation.
- Reconciliation orders pending decisions by `engine_seq`, appends them to the configured bid ledger, and deletes the Redis pending field only after append succeeds.
- Settlement no longer refuses existing Kafka ledger entries merely because the auction engine is paused. Pause remains a protection against new hot-engine bid decisions.
- H5 renders `ENGINE_ACCEPTED` and `ENGINE_SOLD` with pending settlement as `ENGINE_PENDING`, with CTA disabled and settlement-specific copy.
- H5 converts pending settlement to normal leading/settled UI only after settled outbox events arrive.
- Kafka auto topic creation is disabled by default. The Kafka integration test creates its test topics explicitly.

## Crash Window Policy

The Redis Lua engine intentionally records every decision under `bid:{auction_id}:engine:pending` before the Go process appends to Kafka.

Crash cases:

| Failure point | Recovery behavior |
|---|---|
| Before Redis Lua commits | No decision exists; client retries with same idempotency key. |
| After Redis Lua commits, before Kafka append | Reconciler reads Redis pending decision, appends it to Kafka, deletes pending after append, and pauses until DB catches up. |
| After Kafka append, before pending delete | Reconciler may append duplicate ledger message; settlement is idempotent by `auction_id + engine_epoch + engine_seq` and request hash. |
| Kafka append repeatedly fails | Reconciler pauses the auction and records `REDIS_ENGINE_PENDING_KAFKA_RECOVERY_FAILED`. |
| Settlement poison | Worker DLQs, records settlement metadata, pauses auction, and commits the Kafka offset so the partition can move. |

## Validation

Executed focused commands:

```powershell
cd backend
go test ./internal/redisengine -run "TestReconcileBackfillsKafkaLedgerFromRedisPendingCrashWindow|TestReconcileRecoversRedisDecisionWithoutKafkaAck|TestKafkaSettlement" -count=1
ok   live-auction/backend/internal/redisengine

go test ./internal/invariant -count=1
ok   live-auction/backend/internal/invariant
```

Executed related-package gate:

```powershell
cd backend
go test -p 1 ./internal/redisengine ./internal/gateway ./internal/invariant ./internal/reconcile ./internal/outbox ./internal/auction ./internal/realtime -count=1
ok   live-auction/backend/internal/redisengine
ok   live-auction/backend/internal/gateway
ok   live-auction/backend/internal/invariant
ok   live-auction/backend/internal/reconcile
ok   live-auction/backend/internal/outbox
ok   live-auction/backend/internal/auction
ok   live-auction/backend/internal/realtime
```

Executed frontend/doc hygiene gates:

```powershell
cd ..
pnpm.cmd --filter mobile-h5 build
PASS

git diff --check
PASS
```

## Known Limits

- This is correctness and recovery evidence, not latency evidence.
- Kafka topic replication/min-ISR is a deployment precondition. The application disables auto topic creation by default but does not yet inspect broker topic configs.
- Local Apache Kafka is single-node functional evidence only.
- The Redis pending recovery path depends on Redis retaining pending state. Production Redis must use no-eviction and durable/HA configuration.
