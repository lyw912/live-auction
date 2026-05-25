# P3 Decision Log

> Date: 2026-05-24 Asia/Shanghai  
> Status: authoritative P3 decision surface after roadmap reset.  
> Governing reset: `docs/design-v2-industrial/18-p3-p4-roadmap-reset.md`.

## How To Use This Log

This file records P3/P4 decisions that should not be re-litigated in every work session. It does not replace raw evidence. It points to the evidence that currently governs each decision.

Decision states:

- `ACCEPTED`: use this rule until new evidence contradicts it.
- `EVIDENCE_GATED`: do not implement until the stated evidence appears.
- `SUPERSEDED`: do not use as current decision input.
- `BLOCKED`: cannot decide until a named harness or environment gap is fixed.

## Current Decisions

| ID | Decision | State | Evidence | Consequence |
|---|---|---|---|---|
| P3-D01 | PostgreSQL remains auction money truth. | ACCEPTED | `00-project-brief.md`, `12-engineering-rules.md`, P0/P1/P2 tests. | Redis, WebSocket, CDC, and clients cannot decide winner, price, cap, cancel, or hammer. |
| P3-D02 | Admission is disabled during downstream performance exploration. | ACCEPTED | `09-performance-and-benchmark.md`, `17-local-stress-and-p3-execution-plan.md`. | Use `ADMISSION_ENABLED=false`; reject any downstream bottleneck claim polluted by admission `429` or admission counters. |
| P3-D02A | The committed P3 downstream workload harness now proves admission-off cleanliness. | ACCEPTED | `docs/evidence/p3-10-admission-off-harness-proof-2026-05-25.md`. | Future downstream runs should read structured `admission_proof` first; any nonzero enabled value or reject delta is a harness gap, not subsystem bottleneck evidence. |
| P3-D03 | Windows local is direction/regression evidence only. | ACCEPTED | `docs/perf/windows-local-strategy.md`. | No final capacity, user count, p99, or p999 claim from Windows local runs. |
| P3-D04 | Earlier admission-raised pressure is useful but must be treated carefully. | ACCEPTED | `docs/evidence/p3-00-stress-attacker-round-1-2026-05-24.md`. | It found the outbox claim bottleneck, but future downstream evidence must use full admission-off proof. |
| P3-D05 | Outbox claim O(pending squared) bottleneck is fixed for the tested profile. | ACCEPTED | `docs/evidence/p3-01-outbox-claim-fix-2026-05-24.md`. | Do not jump to CDC/partitioning unless new outbox pressure shows backlog, lag, bloat, or claim/update pain again. |
| P3-D06 | PG hot-row pressure is a confirmed bid-path bottleneck, but the accepted release-track fix is conservative transaction-work reduction. | ACCEPTED | `docs/evidence/p3-13-pg-hot-row-drilldown-2026-05-25.md`. | Keep PostgreSQL truth, row lock, bid audit, auction seq, and outbox semantics. Do not introduce Redis Lua reservation or unaudited early-reject from P3-R4 alone. |
| P3-D07 | Relay shard ownership is implemented with Windows-local failover evidence. | ACCEPTED | `docs/evidence/p3-02-relay-shard-ownership-2026-05-24.md`. | Treat as correctness/failover evidence, not final multi-instance capacity proof. |
| P3-D08 | Self-hub is the only realtime runtime implementation. | ACCEPTED | `docs/evidence/p3-12-realtime-fanout-drilldown-2026-05-25.md`, `docs/design-v2-industrial/06-realtime-and-recovery.md`. | Do not keep alternate transport code, compose services, frontend protocol branches, or PoC harnesses in the mainline. Higher local pressure currently points to PG/recovery ceilings, not a reason to replace the self-hub. |
| P3-D09 | Synchronous 300-connect Windows failures are environment/connect-storm evidence, not self-hub fanout proof. | ACCEPTED | P3-01 realtime fanout attack evidence. | Use staggered setup for steady fanout tests; keep connection-storm as a separate product/backoff/admission scenario. |
| P3-D10 | Multi-room event isolation holds in the tested Windows-local round, but shared bid-path resources are not isolated. | ACCEPTED | `docs/evidence/p3-11-multi-room-hot-cold-stress-2026-05-25.md`. | No cross-room leak or cold WS failure appeared, but hot-room bid pressure degraded cold-room bid latency. Treat this as a P3-R4 PG/DB-pool isolation input, not as a realtime fanout failure. |
| P3-D11 | P4 invariant verifier may start before all P3 architecture choices are closed. | ACCEPTED | Reset analysis and P4 roadmap. | Implementing verifier early is allowed because it improves stress attribution and reviewer defense. |
| P3-D12 | Debezium/CDC is blocked until polling outbox is again the first measured bottleneck. | EVIDENCE_GATED | `docs/evidence/p3-01-outbox-claim-fix-2026-05-24.md`, `docs/evidence/p3-02-relay-shard-ownership-2026-05-24.md`. | Do not run Debezium or any CDC process in the mainline and do not replace `backend/internal/outbox/relay.go` until P3-R5 proves claim/update/table pressure remains after current fixes. |
| P3-D12A | Debezium may be cited only as selective design borrowing, not runtime integration. | ACCEPTED | `docs/reviews/p3-06-debezium-borrowing-review-2026-05-25.md`, `docs/adr/p3-02-debezium-borrowing-decision.md`, `docs/evidence/p3-06-debezium-borrowed-hardening-2026-05-25.md`. | Implemented claims: outbox envelope validation, offset/watermark diagnostics, snapshot lifecycle audit, control signals, and error-classification ideas. Forbidden claims: Debezium is integrated, Debezium improves current performance, or CDC replaces auction correctness. |
| P3-D13 | NATS/JetStream remains out of scope unless internal service messaging becomes a measured bottleneck. Selective design borrowing is accepted. | EVIDENCE_GATED | `18-p3-p4-roadmap-reset.md`, `docs/evidence/p3-01-outbox-claim-fix-2026-05-24.md`, `docs/evidence/p3-02-relay-shard-ownership-2026-05-24.md`, `docs/reviews/p3-07-nats-jetstream-borrowing-review-2026-05-25.md`, `docs/evidence/p3-07-nats-jetstream-borrowed-hardening-2026-05-25.md`. | Do not add external brokers, browser realtime replacements, or direct bid-path publishers. Allowed claims: borrowed delivery-state, ack/redelivery, dedupe, slow-consumer, monitoring, and snapshot/catchup design logic. Forbidden claims: NATS is integrated, JetStream improves current performance, or broker sequence replaces auction seq. Reconsider runtime adoption only through an ADR-backed change after measured need. |
| P3-D14 | Redis remains projection/cache/admission support; Lua reservation and Streams are evidence-gated. Existing Redis Lua admission/ticket borrowing is accepted. | EVIDENCE_GATED | `00-project-brief.md`, `04-data-and-storage.md`, `12-engineering-rules.md`, `18-p3-p4-roadmap-reset.md`, `docs/reviews/p3-08-redis-lua-borrowing-review-2026-05-25.md`. | Allowed claims: borrowed Redis Lua for bounded GCRA admission and one-time WS ticket consume. Forbidden claims: Redis is auction truth, Lua decides winner/price/order, or Lua reservation improves performance before PG hot-row evidence and reconciliation ADR. |
| P3-D15 | Outbox second-order pressure is confirmed and partially optimized through batched watermark refresh. | ACCEPTED | `docs/evidence/p3-14-outbox-second-order-pressure-2026-05-25.md`. | Keep DB-backed relay mainline for now. Do not introduce Debezium/CDC from P3-R5 alone; P3-R6 must decide keep/tune/parallelize with ADR-level evidence. |
| P3-D16 | Current release-track architecture stays: PostgreSQL bid truth, app-owned DB relay, Redis projection/history, and self-hub realtime. | ACCEPTED | `docs/evidence/p3-15-architecture-go-no-go-2026-05-25.md`. | Redis Lua reservation, Debezium/CDC, NATS/JetStream runtime, and self-hub replacement are no-go for this P3 cycle without a new ADR, invariant verifier evidence, and a contradictory bottleneck bundle. Proceed to P3-R7 local ceiling sweep and P3-R8 admission calibration. |
| P3-D17 | Final Windows-local downstream ceiling has been characterized for the current release-track architecture. | ACCEPTED | `docs/evidence/p3-16-final-local-ceiling-sweep-2026-05-26.md`. | Further local bid escalation above the clean profile is primarily k6 VU ceiling after DB row-lock latency growth. Proceed to admission-on calibration below the downstream cliff instead of adding Redis Lua reservation, CDC, broker runtime, or self-hub replacement. |
| P3-D18 | Local admission protection defaults are calibrated below the R7 downstream cliff. | ACCEPTED | `docs/evidence/p3-17-admission-calibration-2026-05-26.md`. | Use `BID_AUCTION_LIMIT_PER_SECOND=80` and `BID_AUCTION_MAX_IN_FLIGHT=32` as conservative Windows-local release-track defaults. Do not market these as capacity numbers; recalibrate on Linux/native before public claims. |
| P3-D19 | P4-R1 invariant verification is mandatory evidence for future mutating stress runs. | ACCEPTED | `docs/evidence/p4-01-invariant-verifier-2026-05-26.md`. | P3 runner now writes per-workload invariant JSON/Markdown and fails a workload when scoped invariants fail. Use scoped `auc_live` evidence for stress attribution; use unscoped verifier only as full database hygiene evidence. |
| P3-D20 | P4-R2 auction flight recorder is the forensic surface for contested auctions. | ACCEPTED | `docs/evidence/p4-02-auction-flight-recorder-2026-05-26.md`. | Use the host-only flight recorder API to explain one auction's rules, bids, events, outbox, order/payment, snapshots, and anomalies after stress or abuse runs. |
| P3-D21 | P4-R4 proxy bidding is deferred as a product-rule rewrite. | ACCEPTED | Official brief and `03-domain-model-and-rules.md` fixed-increment model. | Do not implement proxy/max-bid as an optimization. Reopen only through a separate product ADR with new rules and tests. |

## Go / No-Go Gates

### Debezium / CDC Outbox

Go only if all are true:

- polling outbox again becomes measured bottleneck after indexed claim, batch drain, shard leases, and table hygiene;
- evidence includes backlog, delivery lag, claim/update plan, and table bloat;
- ordering, DEAD gap notice, and diagnostics can be preserved.
- ADR defines topic/key/partition mapping, Debezium offset vs auction seq, bootstrap/snapshot mode, duplicate handling, replication slot/WAL loss recovery, local startup, and rollback.
- the app CDC relay adapter still writes Redis history/snapshot and publishes through the app realtime adapter; Debezium does not directly become browser realtime or auction truth.

No-go while:

- outbox drains under tested pressure;
- the first bottleneck is PG hot-row or load-generator/environment.
- the only argument is Debezium maturity, GitHub popularity, or generic CDC best practice without this project's outbox evidence.

### NATS / JetStream

Current accepted position:

- NATS/JetStream has been reviewed against local source in `docs/reviews/p3-07-nats-jetstream-borrowing-review-2026-05-25.md`.
- The project may cite selective design borrowing only: consumer delivery-state vocabulary, ack/redelivery/backoff/TERM-style poison handling, publish-message dedupe identity, slow-consumer pending-byte discipline, monitoring vocabulary, and snapshot/catchup failure thinking.
- The project does not run NATS, JetStream, a broker-backed fanout path, or a browser NATS client.

Go only if all are true:

- service-to-service/internal messaging split is intentionally introduced, or current relay/internal delivery becomes a measured bottleneck;
- PostgreSQL outbox remains the commit truth before any broker publish;
- app-owned `auction_id + seq`, Redis history/snapshot, `DEAD`, gap notice, snapshot fallback, and diagnostics are preserved;
- ADR defines subjects, streams, retention, duplicate window, app message id, durable consumers, ack/redelivery/backoff/`Term` mapping, ordering, metrics, local startup, and rollback;
- tests prove broker down, consumer crash, duplicate delivery, same-auction order, poison delivery, broker restart, and bounded backpressure.

No-go while:

- the system remains a modular monolith with DB-lease outbox relay as the mainline;
- outbox drains under tested pressure and the first bottleneck is PG hot-row, browser fanout, admission, or environment;
- the reason is only "industrial messaging", GitHub maturity, or desire to add infrastructure;
- the proposed integration publishes directly from bid/cancel/end handlers or treats JetStream sequence as auction seq;
- the target problem is browser realtime, where the self-hub remains the scoped implementation.

### Redis Lua Reservation

Current accepted position:

- Redis Lua has been reviewed against local Redis source in `docs/reviews/p3-08-redis-lua-borrowing-review-2026-05-25.md`.
- The project may cite selective implemented borrowing only: GCRA admission scripts for user/IP/auction request protection and one-time WebSocket ticket consume.
- PostgreSQL remains the auction money truth. Redis Lua admission can fail open with anomaly; Redis ticket consume can fail closed. Neither path decides winner, price, cap, cancel, end, order, auction seq, or idempotency response.

Go only if all are true:

- clean pressure evidence shows PG row lock is the release-blocking bottleneck;
- final Linux or strong local drilldown confirms the issue is not outbox, pool sizing, transaction work, or k6/Windows;
- ADR proves reconciliation, cap/cancel/end races, Redis-loss behavior, TTL/eviction behavior, and auditability;
- DB settlement remains final and every Redis reservation is reconciled to bids, auction_events, outbox_events, and idempotency_records;
- invariant verifier proves one winner, one order, price match, seq continuity, and no unreconciled accepted reservation after crash/race/load tests.

No-go while:

- correctness story is stronger than unproven speedup;
- no reconciliation ADR exists;
- performance need is only speculative;
- the proposed change lets Redis decide winner, price, cap, cancel, end, order, or idempotency result.

### Redis Streams

Go only if all are true:

- current Redis list history or relay/internal delivery is a measured bottleneck;
- Redis Streams are introduced as projection/recovery/internal-delivery infrastructure, not auction truth;
- PostgreSQL outbox remains the commit truth before any stream append used for delivery;
- app-owned `auction_id + seq`, Redis/DB snapshot fallback, `DEAD`, gap notice, and diagnostics are preserved;
- ADR defines stream keys, trim policy, stream ID vs auction seq, consumer group ownership, ack/reclaim, pending-entry handling, duplicate delivery, Redis restart/down behavior, and rollback.

No-go while:

- the current bounded Redis list history works and history miss falls back to DB snapshot;
- outbox drains and relay ownership works under tested pressure;
- the goal is to replace PostgreSQL outbox, auction_events, or `outbox_delivery`;
- stream ID or consumer-group state is treated as domain ordering or audit truth;
- Pub/Sub is proposed as durable realtime.

### Admission Limit Calibration

Go only after:

- practical downstream limit is known;
- current release candidate has stopped improving materially;
- limits include evidence-based headroom;
- admission-on tests prove stable `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, `Retry-After`, and diagnostics.

No-go during:

- PG, outbox, WS, reconnect, Redis, or runtime bottleneck discovery.

## Superseded Or Demoted Evidence

| Evidence | Current classification | Reason |
|---|---|---|
| Early P3-00 smoke under admission-on/default limits | HARNESS_ONLY | Useful for seed/auth/script confidence, not downstream bottleneck attribution. |
| Admission-raised P3-00 bottleneck run | PARTIAL_AUTHORITY | Valid for discovering outbox claim and PG pressure direction; superseded by the full `ADMISSION_ENABLED=false` rule for future runs. |
| Synchronous 300 watcher Windows connection failure | ENV_LIMIT_SIGNAL | Failure happened at connection setup/local accept boundary; not proof that steady fanout failed. |
| Multi-room downstream-pressure smoke | HARNESS_SMOKE | Cross-room leak rate 0 is useful, but not adversarial hot/cold isolation proof. |

## Next Required Decisions

| Order | Decision to close | Required evidence |
|---:|---|---|
| 1 | Is any further bid-path or relay redesign justified after admission calibration? | Linux or stronger local evidence plus ADR/invariant proof; current P3 evidence keeps the existing architecture. |
| 2 | Can stress evidence become machine-checkable instead of manually interpreted? | Closed for P4-R1/P4-R2 by `docs/evidence/p4-01-invariant-verifier-2026-05-26.md` and `docs/evidence/p4-02-auction-flight-recorder-2026-05-26.md`; next P4 work can build repeatable risk simulation on top. |
