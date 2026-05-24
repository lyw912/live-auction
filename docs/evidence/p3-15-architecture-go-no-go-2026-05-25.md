# P3-15 Architecture Go/No-Go

Date: 2026-05-25 Asia/Shanghai

Status: `KEEP_CURRENT_ARCHITECTURE_WITH_LIMITS`

## Target

P3-R6 closes the architecture decision gate after the clean P3-R1 through P3-R5 loop.

The decision must answer:

- keep PostgreSQL row-lock bid truth or introduce Redis Lua reservation;
- keep app-owned DB relay or introduce Debezium/CDC, NATS, or another broker runtime;
- keep self-hub browser realtime or reopen transport scope;
- continue optimizing locally or move to admission calibration.

## Governing Evidence

| Evidence | Decision input |
|---|---|
| `docs/evidence/p3-10-admission-off-harness-proof-2026-05-25.md` | Downstream pressure harness now proves admission disabled and rejects admission pollution. |
| `docs/evidence/p3-11-multi-room-hot-cold-stress-2026-05-25.md` | Hot room did not leak to cold room, but shared bid-path DB/lock pressure degraded cold bid latency. |
| `docs/evidence/p3-12-realtime-fanout-drilldown-2026-05-25.md` | Self-hub passed clean watcher fanout, healthy-vs-slow, and reconnect rounds; higher rounds point to PG/recovery ceilings, not fanout runtime failure. |
| `docs/evidence/p3-13-pg-hot-row-drilldown-2026-05-25.md` | PostgreSQL hot auction row was confirmed and improved through conservative transaction-work reduction. |
| `docs/evidence/p3-14-outbox-second-order-pressure-2026-05-25.md` | Outbox relay watermark refresh was confirmed and improved through batch watermark refresh. Backlog still remained under the tested input rate. |
| `docs/reviews/p3-06-debezium-borrowing-review-2026-05-25.md` and `docs/adr/p3-02-debezium-borrowing-decision.md` | Debezium/CDC borrowing accepted; runtime integration stays evidence-gated. |
| `docs/reviews/p3-07-nats-jetstream-borrowing-review-2026-05-25.md` | NATS/JetStream borrowing accepted; runtime integration stays evidence-gated. |
| `docs/reviews/p3-08-redis-lua-borrowing-review-2026-05-25.md` | Redis Lua admission/ticket borrowing accepted; Redis reservation stays evidence-gated. |

## Decision Matrix

| Option | Decision | Reason |
|---|---|---|
| PostgreSQL row-lock bid truth | KEEP | P3-R4 proved contention, then reduced transaction work without changing correctness semantics. No evidence yet justifies splitting auction truth. |
| Redis Lua reservation | NO-GO FOR CURRENT RELEASE TRACK | Would require a new reconciliation ADR, Redis-loss semantics, cap/cancel/end race proof, idempotency replay proof, and invariant verifier. P3-R4 did not prove conservative PG truth is insufficient. |
| App-owned DB outbox relay | KEEP | P3-R5 proved second-order relay pressure, then improved it materially through app-owned batch watermark refresh. |
| Debezium/CDC runtime | NO-GO FOR CURRENT RELEASE TRACK | P3-R5 does not prove polling relay is unsalvageable; CDC would add offsets, WAL slots, duplicate delivery, bootstrap, and ops without removing auction seq/gap/recovery obligations. |
| NATS/JetStream runtime | NO-GO FOR CURRENT RELEASE TRACK | Current problem is not service-to-service messaging. Browser realtime remains self-hub, and DB outbox remains commit truth. |
| Self-hub realtime | KEEP | P3-R3 clean rounds found no self-hub failure at the tested ceiling. |
| Parallel relay / batch claim | DEFER TO FUTURE ADR | P3-R5 shows backlog remains, but current release track should first run final local ceiling sweep and admission calibration. Parallel delivery must prove same-auction ordering, DEAD gap behavior, shard ownership, and recovery invariants. |
| Admission calibration | GO NEXT | P3-R4/P3-R5 define downstream cliffs well enough for P3-R7 local ceiling sweep and P3-R8 admission-on protection tuning. |

## Accepted Release-Track Architecture

For the current release track:

```text
HTTP bid
  -> auth / ACL / idempotency replay probe / admission protection
  -> PostgreSQL transaction with auction row lock
  -> bid/order/idempotency/auction_events/outbox commit
  -> app-owned outbox relay with shard leases and batch watermark refresh
  -> Redis history/snapshot projection
  -> app-owned self-hub WebSocket delivery
  -> client recovery by auction seq or snapshot
```

This remains a modular monolith. PostgreSQL is money truth. Redis and WebSocket are projection/delivery only.

## Explicit No-Go Statements

### Redis Lua Reservation

Do not implement Redis Lua bid reservation in this P3 cycle.

Required before reopening:

- P4 invariant verifier covering seq continuity, winner/price/order match, idempotency replay, and no unreconciled reservation;
- ADR for Redis key topology, TTL, eviction, failover, BUSY/script timeout, reconciliation, and rollback;
- crash/race/load tests for bid/cancel/end/cap, Redis down/restart, duplicate bid, expired reservation, and DB settlement mismatch;
- Linux or stronger local evidence showing conservative PostgreSQL truth is release-blocking after transaction-work optimization and admission calibration.

### Debezium / CDC

Do not integrate Debezium, Kafka Connect, Debezium Server, WAL CDC, or a CDC relay runtime in this P3 cycle.

Required before reopening:

- evidence that app-owned relay remains the first bottleneck after batch watermark refresh, table hygiene, and admission tuning;
- claim/update plans, table bloat/dead tuple evidence, backlog and delivery-lag evidence;
- ADR for topic/key/partition mapping, offset storage, bootstrap/snapshot, duplicate handling, replication slot/WAL loss recovery, local startup, rollback, and diagnostics;
- tests for connector crash, duplicate delivery, same-auction order, poison, restart, and gap notice.

### NATS / JetStream

Do not integrate NATS or JetStream runtime in this P3 cycle.

Required before reopening:

- explicit service split or measured internal messaging bottleneck;
- ADR for subjects, streams, retention, duplicate window, message id, durable consumers, ack/redelivery/backoff/TERM mapping, local startup, rollback, and monitoring;
- tests for broker down, consumer crash, duplicate publish/delivery, same-auction order, poison, broker restart, and bounded backpressure.

### Self-Hub Replacement

Do not replace the app-owned self-hub while P3-R3 remains the latest clean realtime evidence.

Required before reopening:

- clean fanout/slow/reconnect evidence showing self-hub is the primary bottleneck, not bid path, recovery DB work, admission, or Windows connection setup.

## Residual Risks

| Risk | Current handling | Next action |
|---|---|---|
| Single hot auction remains serialized | Accepted as correctness-first PG truth. | P3-R7 local ceiling sweep, P3-R8 admission calibration, future Redis reservation only with ADR/invariants. |
| Single embedded relay still drains below tested 200 bid/s input | Batch watermark refresh improved drain but backlog remained. | P3-R7 characterize ceiling; future parallel relay/batch claim only with ordering tests. |
| Windows local numbers are not capacity claims | All evidence docs label local-only direction/regression evidence. | P5 Linux native 3-run baseline before public numbers. |
| Stress evidence still needs machine-checked invariants | Manual evidence is improving but not enough. | P4-R1 invariant verifier remains high value. |

## Next

P3-R7 final local ceiling sweep:

- repeat focused bid and outbox pressure with clean admission-off proof;
- record current local bottleneck table after P3-R4/P3-R5 optimizations;
- avoid new architecture work unless a new first bottleneck contradicts this decision.

P3-R8 admission calibration:

- re-enable admission;
- set user/IP/auction/in-flight limits below the downstream cliff;
- prove stable `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, `Retry-After`, and diagnostics without downstream collapse.
