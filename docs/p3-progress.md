# P3 Progress

Date: 2026-05-23 Asia/Shanghai

Authoritative roadmap: `docs/design-v2-industrial/16-industrial-p2-p3-roadmap.md`

## Milestones

| ID | Deliverable | Status | Required Evidence |
|---|---|---|---|
| P3-01 | Realtime transport decision and adapter | NOT_STARTED | ADR comparing self hub, Centrifugo, Redis Pub/Sub, NATS; adapter tests; recovery semantics unchanged |
| P3-02 | Relay shard ownership for multi-instance backend | NOT_STARTED | DB lease per shard, owner failover test, duplicate publish bounded by client dedupe |
| P3-03 | Multi-room isolation | NOT_STARTED | hot/cold room k6 workload, no cross-room leak test, per-room latency/fanout metrics |
| P3-04 | Data path evolution decision | NOT_STARTED | measured evidence for PG lock, outbox hot table, snapshot pressure, or fanout bottleneck before Redis Lua/CDC changes |

## Notes

- P3 may not claim horizontal scale until two backend instances are tested with real relay ownership and realtime fanout.
- Centrifugo can replace the transport layer, but not PostgreSQL truth, outbox, idempotency, snapshot fallback, or diagnostics.
- Redis Lua and Debezium/CDC remain evidence-gated decisions, not default implementation work.
