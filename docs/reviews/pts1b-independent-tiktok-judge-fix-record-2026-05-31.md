# PTS-1B Independent TikTok Judge Fix Record

> Date: 2026-05-31
> Source review: `docs/reviews/pts1b-independent-tiktok-judge-review-2026-05-31.md`
> Status: implementation fix record, not `CURRENT_PASS` performance evidence.

## Scope

This record documents the repair work for the independent hostile PTS-1B review. The fix target was the correctness boundary between Redis live decisions, Kafka durable fencing, PostgreSQL settlement/audit, and user-visible bid truth.

This document does not claim a current PTS-1B pass. A pass still requires the current 3-run PTS evidence pack and fault-injection evidence defined by `docs/current/evidence-policy.md`.

## Industrial References Checked

- Apache Kafka topic config documentation states that with `acks=all`, in-sync replicas must acknowledge the write, and if ISR is below `min.insync.replicas`, the producer receives a replica exception. Source: https://kafka.apache.org/43/configuration/topic-configs/
- Apache Kafka producer config documentation describes `acks=all` as waiting for the in-sync replica set. Source: https://kafka.apache.org/11/configuration/producer-configs/
- Kafka topic-level guidance documents the common production pattern of RF=3, `min.insync.replicas=2`, and producer `acks=all`. Source: https://kafka.apache.org/27/configuration/topic-level-configs/
- Redis persistence documentation defines RDB snapshots and AOF replay as Redis recovery mechanisms. This supports local Redis recovery hardening, but does not make Redis a cross-system durable decision WAL. Source: https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/

## Fixes Completed

### P0: Prevent PostgreSQL Settlement From Rewinding Redis Live State

Changed files:

- `backend/internal/redisengine/engine.go`
- `backend/internal/redisengine/engine_integration_test.go`

Failure found after PTS report `HA5YX7ZG`:

- PTS sampled 1000 bid requests and all returned HTTP `202`.
- Kafka contained duplicate messages for the same auction `engine_seq` with different request hashes, for example offsets `52` and `101` both carried `engine_seq=53`.
- PostgreSQL settlement then correctly detected `engine_seq request hash conflict`, paused the engine, and left the run non-terminal.

Root cause:

- Redis Lua increments `engine_seq` atomically inside one script invocation.
- `Worker.refreshRedisSettledState` was called after settlement and wrote the PostgreSQL settled snapshot back into the Redis hot-state hash.
- During contention, PostgreSQL settlement lags Redis live decisions. The refresh path therefore overwrote newer Redis `engine_seq` / winner / price fields with an older settled snapshot.
- Later Lua invocations read the rewound Redis state and reused already-issued `engine_seq` values.

Fix:

- Redis hot-state refresh from settlement is now a Redis-side conditional write.
- If current Redis `engine_seq` is greater than the PostgreSQL snapshot `engine_seq`, the refresh is skipped.
- If Redis state is missing while pending decisions still exist, the refresh is skipped instead of rebuilding from a partial settled snapshot.
- Settlement still updates PostgreSQL/checkpoint state; it no longer corrupts Redis live decision state.

Regression test:

- `TestRedisLedgerSettlementRefreshDoesNotRewindLiveRedisState` proves that when Redis live state is ahead of the first settled message, processing that settlement does not reduce Redis `engine_seq`, and the next bid does not reuse an old sequence.
- Verified with:

```bash
go test ./internal/redisengine -run TestRedisLedgerSettlementRefreshDoesNotRewindLiveRedisState -count=10
```

### P0: Kafka ACK Before User-Visible Engine Success

Changed files:

- `backend/internal/redisengine/engine.go`
- `backend/internal/auction/bid.go`
- `backend/internal/gateway/auction_handlers.go`
- `backend/internal/gateway/json.go`
- `backend/internal/gateway/json_test.go`
- `frontend/mobile-h5/src/main.tsx`

Behavior after fix:

- `ENGINE_ACCEPTED`, `ENGINE_REJECTED`, and `ENGINE_SOLD` are returned only after `durability_status = KAFKA_ACKED`.
- `kafka_append_status = UNKNOWN` no longer replays or returns an `ENGINE_*` success-like result.
- append lock busy / pending-order cases return explicit pending durability:

```json
{
  "result": "PROCESSING_RETRY_LATER",
  "decision_status": "PENDING_DURABILITY",
  "durability_status": "KAFKA_UNKNOWN",
  "settlement_status": "PENDING"
}
```

- `KAFKA_FAILED` maps to reconciling / fail-closed behavior.
- Pending durability responses do not expose `current_winner_id` as user truth.
- Gateway can return `202` with an explicit pending durability payload instead of replacing it with a vague error body.
- H5 treats `KAFKA_UNKNOWN` / `PENDING_DURABILITY` as confirmation pending and does not render accepted-leading or sold-final UX.

### P0/P1: Explicit Response Semantics

Changed files:

- `backend/internal/auction/bid.go`
- `backend/internal/redisengine/engine.go`
- `frontend/mobile-h5/src/main.tsx`
- `docs/current/performance-correctness-contract.md`

Added public bid response fields:

- `decision_status`
- `durability_status`
- `settlement_status`
- `decision_basis`

`decision_basis` includes:

- `previous_price_cents`
- `required_min_price_cents`
- `current_price_cents`
- `reason`
- `engine_seq`

This makes low-reject fairness auditable at the response and ledger payload layer.

### P0/P1: Redis Loss Recovery Resume Procedure

Changed files:

- `backend/internal/redisengine/engine.go`
- `backend/internal/gateway/monitor_handlers.go`
- `frontend/pc-console/src/main.tsx`
- `docs/current/fault-injection-runbook.md`
- `docs/current/evidence-policy.md`

`resume_redis_engine` no longer increments `engine_epoch` and deletes Redis state as a shortcut.

New resume behavior:

1. run reconcile preflight;
2. drain pending Redis decisions to Kafka or fail closed;
3. load `auction_engine_checkpoints`;
4. verify checkpoint hash and compare checkpoint state against PostgreSQL settlement state;
5. rebuild Redis hot-state fields from the verified snapshot;
6. run reconcile postflight;
7. unpause only when postflight is `OK`;
8. return and persist `rto_ms` in the control-signal result.

PC diagnostics now surface last recovery RTO/status through Redis Engine diagnostics.

### P0/P1: Kafka Local vs Production Durability Posture

Changed files:

- `infra/docker-compose.yml`
- `infra/README.md`
- `infra/docker-compose.kafka-production-example.yml`
- `tests/pts/preflight-l4b-pts-guards.sh`

Local compose remains single broker, but is explicitly labeled functional-only evidence.

Added production example posture:

- 3 Kafka brokers;
- `auction.bid-events` / `auction.dlq` RF=3;
- `min.insync.replicas=2`;
- producer path already uses `RequiredAcks: kafka.RequireAll`;
- unclean leader election disabled;
- auction-id message keying retained for per-auction partition ordering.

Preflight now separates:

- P0: Kafka topic exists, partition count is explicit, writer uses all ACKs, ISR health is visible.
- P1: production broker-failure durability requires RF>=3 and min ISR>=2. Local RF=1 cannot be cited as production durability proof.

## Tests And Validation

Passed:

- `go test ./internal/redisengine -run TestRedisLedgerSettlementRefreshDoesNotRewindLiveRedisState -count=10`
- `bash -n tests/pts/verify-l4b-pts-correctness.sh`
- `go test ./... -run TestNonExistent -count=0`
- `go test ./internal/gateway -run TestWriteBidAdmissionResult -count=1`
- `pnpm --filter mobile-h5 build`
- `pnpm --filter pc-console build`
- `docker compose -f infra/docker-compose.kafka-production-example.yml config`
- `git diff --check`

Blocked:

- `go test ./internal/auction ./internal/gateway ./internal/redisengine`
- `go test ./internal/redisengine -count=1`

Blocker details:

- Tests could not connect to PostgreSQL at `127.0.0.1:5432`; the error was `connectex: No connection could be made because the target machine actively refused it`.
- `docker ps` showed `live-auction-postgres` running and mapped to `0.0.0.0:5432->5432/tcp`, so this needs a local Docker/port/proxy follow-up before treating integration tests as verified.
- Kafka container startup was blocked because `live-auction-redpanda` already owned port `9092`; the existing process was not stopped during this repair.
- On the current Linux/Docker environment, the focused Redis rewind regression passes, but the full redisengine package still has pre-existing scheduling-sensitive pending-append tests that can fail with `pendingResponses = 0`; this is a test harness stability issue and should be hardened before using full-package green as a release gate.

## Remaining Evidence Required

Before claiming `CURRENT_PASS`:

- restore local PostgreSQL connectivity and rerun backend integration tests;
- run the current reset before any new PTS-1B attempt so PostgreSQL rows, Redis bid keys, and Kafka topics from `HA5YX7ZG` cannot pollute the next result;
- run current reset/preflight from `tests/pts/MANIFEST.md`;
- run PTS-1B three times with current JMX/CSV, but do not treat HTTP `202` RTT
  as user-visible decision p99;
- update or wrap the PTS sampler so a `202` response is followed with the same
  `client_bid_id` / idempotency key until final `ENGINE_*` or timeout, or label
  the result as ingress-only;
- collect `ENGINE_*`, HTTP, durability, and settlement distributions;
- report `accept_latency_ms`, `final_decision_latency_ms`,
  `settlement_latency_ms`, `pending_ratio`, and `timeout_ratio`;
- run `tests/pts/verify-l4b-pts-correctness.sh`;
- execute Redis loss, Kafka timeout/restart, settlement worker crash, PostgreSQL disruption, and reconnect storm fault gates;
- classify evidence according to `docs/current/evidence-policy.md`.

## Current Verdict After Fix

The original P0 design flaw of showing Redis-only success before Kafka durability has been repaired in code. Redis resume is now a controlled checkpoint-based recovery path instead of a manual unpause shortcut.

The project still must not claim PTS-1B pass until the integration tests and current PTS/fault evidence are rerun successfully in a clean environment.
