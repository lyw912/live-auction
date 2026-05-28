# PTS L4b Redis/Kafka Ledger Engine Evidence

Date: 2026-05-29

Source docs: `1d31bf9 docs: add PTS1-Refactoring docs`

## Scope

Implemented L4b as the hot bidding path with separated failure domains:

- Redis Lua is the short atomic hot state machine. It validates auction status, winner, increment grid, cap, soft-close extension, idempotency, `engine_seq`, and `engine_epoch`.
- Kafka/Redpanda is the durable bid ledger. The Go engine synchronously appends the Lua decision with required acknowledgements before returning `ENGINE_ACCEPTED`, `ENGINE_REJECTED`, or `ENGINE_SOLD`.
- PostgreSQL remains final settlement/audit/order truth. The settlement worker consumes Kafka by consumer group, fences by `engine_epoch`, enforces contiguous `engine_seq`, and writes `bids`, `idempotency_records`, `auction_events`, `outbox_events`, orders, and scheduler jobs.
- Poison settlement has bounded retry, then writes `auction.dlq`, marks the PG settlement row with DLQ metadata, pauses the auction, and commits the Kafka offset so the partition can move.
- Reconciliation runs periodically and by operator signal. It compares Redis engine seq, PostgreSQL settled seq, failed/DLQ rows, and pauses on unsafe drift.
- The invariant verifier now checks Redis/Kafka engine settlement continuity, auction `engine_seq` coverage, DLQ/FAILED rows, and accepted/sold settlement coverage by bid/event rows.
- PC monitor exposes Redis engine pause/lag/settlement state.
- Control signals support `pause_redis_engine`, `resume_redis_engine`, and `reconcile_redis_engine`.

## Environment Note

Local testing runs Redis, Redpanda, PostgreSQL, and the app on one machine. That is a test topology only. Production requires Redis HA with no eviction and AOF policy, Kafka/Redpanda replicas with `acks=all`/ISR discipline, and PostgreSQL as the settlement truth.

Local infra:

```text
docker compose -f infra/docker-compose.yml up -d postgres redis redpanda
REDIS_ADDR=localhost:6380
KAFKA_BROKERS=localhost:9092
```

Redis is no longer the durable ledger in L4b. It is still required for the Lua state machine and is configured locally with `appendonly yes`, `appendfsync always`, and `noeviction` for failure testing.

## Failure Gates Covered

- Kafka append failure after Redis decision pauses the auction and returns `ENGINE_PAUSED`.
- Redis decision crash window is recoverable: reconciliation backfills Redis pending decisions into Kafka, deletes pending only after append, and settlement remains idempotent if Kafka already had the message.
- Duplicate Kafka delivery is idempotent; duplicate settlement does not create a second bid/order/event.
- Engine seq gap pauses settlement through `REDIS_ENGINE_LEDGER_GAP`.
- Stale `engine_epoch` pauses settlement through `REDIS_ENGINE_STALE_EPOCH`.
- Settlement retry is bounded; poison events go to DLQ and are recorded in PostgreSQL.
- Reconcile detects Redis-ahead/DB-ahead and DLQ settlement drift.
- H5 renders `ENGINE_ACCEPTED` / `ENGINE_SOLD` pending settlement distinctly instead of displaying DB-settled success.

## Validation

Focused Redis/Kafka ledger gate:

```text
REDIS_ADDR=localhost:6380 REDIS_STREAMS_ADDR=localhost:6380 go test ./internal/redisengine -count=1 -v
PASS
```

Invariant verifier gate:

```text
REDIS_ADDR=localhost:6380 REDIS_STREAMS_ADDR=localhost:6380 go test ./internal/invariant -count=1 -v
PASS
```

Related package gate:

```text
REDIS_ADDR=localhost:6380 REDIS_STREAMS_ADDR=localhost:6380 go test -p 1 ./internal/redisengine ./internal/gateway ./internal/invariant ./internal/reconcile ./internal/outbox ./internal/auction ./internal/realtime -count=1
PASS
```

Real Kafka-compatible ledger smoke against local Redpanda:

```text
KAFKA_INTEGRATION=1 KAFKA_BROKERS=localhost:9092 go test ./internal/redisengine -run TestKafkaLedgerRedpandaIntegration -count=1 -v
PASS
```

Recovery fix record:

- `docs/reviews/pts-l4b-kafka-ledger-judge-review-2026-05-29.md`
- `docs/evidence/pts-l4b-kafka-ledger-recovery-fix-2026-05-29.md`

No PTS latency number is claimed here. Redis/Kafka engine performance still needs the same-JMX before/after PTS profile with raw outputs, DB/Redis/Kafka/outbox/WS metrics snapshots, and invariant reports.
