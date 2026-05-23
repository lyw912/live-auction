# P3 Progress

Date: 2026-05-24 Asia/Shanghai

Authoritative roadmap: `docs/design-v2-industrial/16-industrial-p2-p3-roadmap.md`

Execution plan: `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`

## Milestones

| ID | Deliverable | Status | Required Evidence |
|---|---|---|---|
| P3-00 | Local stress loop and bottleneck evidence discipline | BOTTLENECK_FOUND | `docs/evidence/p3-00-stress-attacker-round-1-2026-05-24.md` records the first real downstream-pressure stress-attacker result. The primary bottleneck is the outbox relay claim query/backlog path; PG hot-row locking is a secondary bottleneck. Raw artifacts live under `docs/perf/raw/p3-attack-20260524-035952/` |
| P3-01A | Outbox claim bottleneck fix | DONE | `docs/evidence/p3-01-outbox-claim-fix-2026-05-24.md` records the indexed claim fix and retest. Claim query dropped from `1584.153ms` to `14.165ms`; `auc_live` backlog after 30 seconds dropped from `8142` pending to `633` pending and later fully drained. |
| P3-01 | Realtime transport decision and adapter | NOT_STARTED | ADR comparing self hub, Centrifugo, Redis Pub/Sub, NATS; adapter tests; recovery semantics unchanged |
| P3-02 | Relay shard ownership for multi-instance backend | NOT_STARTED | DB lease per shard, owner failover test, duplicate publish bounded by client dedupe |
| P3-03 | Multi-room isolation | NOT_STARTED | hot/cold room k6 workload, no cross-room leak test, per-room latency/fanout metrics |
| P3-04 | Data path evolution decision | NOT_STARTED | measured evidence for PG lock, outbox hot table, snapshot pressure, or fanout bottleneck before Redis Lua/CDC changes |

## Notes

- P3 may not claim horizontal scale until two backend instances are tested with real relay ownership and realtime fanout.
- Centrifugo can replace the transport layer, but not PostgreSQL truth, outbox, idempotency, snapshot fallback, or diagnostics.
- Redis Lua and Debezium/CDC remain evidence-gated decisions, not default implementation work.
- Green smoke is not bottleneck evidence. P3 starts by running the recurring local stress loop from the execution plan and only then chooses transport, relay, or data-path changes.
- 2026-05-24 first P3-00 attempt exposed P2 harness/runtime gaps: load seed did not cover all k6 user families, scripts treated admission `429` as transport failure, and WS handlers did not reliably observe client close.
- 2026-05-24 repaired P3-00 smoke passed all committed workloads with real backend paths and saved raw artifacts under `docs/perf/raw/p3-00/`. This is now classified as harness validation only, not the real P3-00 bottleneck result.
- 2026-05-24 first real P3-00 stress-attacker round raised admission ceilings and ran downstream pressure. At target `300 rps / 45s`, k6 reached 800 VUs, dropped 5108 iterations, and p99 rose to `5.25s` without `429` domination. Outbox backlog grew to 8326 pending rows, drained only 184 rows after 30 seconds, and the current outbox claim query took `1584.153ms` with `31305491` rows removed by join filter.
- Future P3 rounds must separate `admission-on` from `downstream-pressure` profiles. Admission-on runs prove rate-limit/abuse protection and stable business `429`; downstream PG/outbox/WS bottleneck claims require admission ceilings to be explicitly raised or documented, then ruled out in attribution.
- User identities in load scripts must represent legitimate business actors. An outbox workload may use seeded bidders because bids create outbox events; a synthetic subsystem-named user such as `k6_outbox_*` is valid only if it is seeded with the required auth and room membership. Otherwise the result is ACL evidence, not subsystem pressure.
- P3-01/P3-02/P3-04 decisions are now blocked on the P0 outbox relay claim fix and retest. Do not use this Windows local run as a final capacity claim; use it as bottleneck direction and regression baseline.
- 2026-05-24 outbox claim fix unblocked the P3 relay path. The old `NOT EXISTS` anti-join under backlog was replaced by denormalized delivery fields, partial indexes, indexed predecessor checks, and batch relay draining. Retest raw artifacts live under `docs/perf/raw/p3-outbox-claim-fix-20260524-0448/`.
- After the fix, the same local downstream-pressure profile is still bid-path limited: k6 reaches `800` VUs and drops `5236` iterations, with p99 around `5.23s`. Treat this as PG hot-row/open-model pressure evidence, not as outbox claim evidence.
- Next loop should implement P3-02 relay shard ownership/failover, then attack watcher fanout and hot/cold multi-room isolation again. Do not claim multi-instance realtime until those gates pass.
