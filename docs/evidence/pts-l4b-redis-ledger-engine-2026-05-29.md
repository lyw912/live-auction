# PTS L4b Redis Ledger Engine Evidence

Date: 2026-05-29

Source docs: `1d31bf9 docs: add PTS1-Refactoring docs`

## Scope

Implemented L4b as the real hot bidding path:

- Redis Lua atomically validates bid status, winner, increment grid, cap, soft-close extension, idempotency, `engine_seq`, and `engine_epoch`.
- The same Lua script appends every engine result to a Redis Stream ledger with hash-tagged keys.
- HTTP bid responses now expose `ENGINE_ACCEPTED`, `ENGINE_REJECTED`, or `ENGINE_SOLD` with `settlement_status=PENDING`.
- Settlement worker consumes Redis Streams with `XREADGROUP`, fences by `engine_epoch`, enforces contiguous `engine_seq`, and writes PostgreSQL `bids`, `idempotency_records`, `auction_events`, `outbox_events`, orders, and scheduler jobs.
- Reconciliation compares Redis engine seq with PostgreSQL settlement seq and pauses on unsafe drift.
- PC monitor exposes Redis engine pause/lag/settlement state.
- Control signals support `pause_redis_engine`, `resume_redis_engine`, and `reconcile_redis_engine`.

## Environment Note

L4b requires Redis Streams/XADD, therefore Redis 5+ is mandatory. This workspace had an old Windows Redis 3.2 service on `localhost:6379`; it does not support XADD and correctly causes the engine to fail closed with `ENGINE_PAUSED`.

The project Docker Redis was moved to host port `6380`:

```text
docker compose -f infra/docker-compose.yml up -d redis
REDIS_ADDR=localhost:6380
REDIS_STREAMS_ADDR=localhost:6380
```

The running container Redis version used for L4b verification:

```text
redis_version:7.4.9
```

## Validation

Focused Redis ledger gate:

```text
REDIS_STREAMS_ADDR=localhost:6380 go test ./internal/redisengine -run TestRedisLedger -count=1 -v
PASS
```

Covered:

- engine accepted response before DB settlement;
- settlement commit into PostgreSQL;
- engine rejected low bid and settlement;
- cap bid creates SOLD and exactly one order;
- duplicate client bid replay returns the same engine result;
- reconciliation pauses on DB-behind-Redis before settlement and reports OK after replay;
- existing auction `seq` backfill prevents Redis engine seq collisions;
- unsupported PostgreSQL-only rules such as fat-finger confirmation fail closed by pausing the engine instead of silently processing with simplified Lua.

Related package gate:

```text
REDIS_ADDR=localhost:6380 REDIS_STREAMS_ADDR=localhost:6380 go test -p 1 ./internal/redisengine ./internal/gateway ./internal/outbox ./internal/reconcile ./internal/auction ./internal/realtime -count=1
PASS
```

Broader backend gate:

```text
REDIS_ADDR=localhost:6380 REDIS_STREAMS_ADDR=localhost:6380 go test -p 1 ./...
```

Result: PASS.

No PTS latency number is claimed here. Redis ledger performance still needs the same-JMX PTS profile with settlement lag and invariant evidence.
