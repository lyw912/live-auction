# Windows Local Performance And Attack Strategy

Date: 2026-05-24 Asia/Shanghai

Status: Accepted working policy for the long Windows development phase.

## Rule

Windows local testing is required and valuable. It is not final capacity evidence.

Use Windows local runs to find and fix:

- correctness bugs: multiple winners, multiple orders, seq gaps, idempotency drift;
- degradation bugs: Redis down, hot-auction overload, slow consumers, reconnect storms, outbox poison;
- bottleneck direction: PostgreSQL lock wait, outbox lag, goroutine leaks, Redis latency, WebSocket queue growth;
- script and deployment defects: incomplete seed data, missing room membership, foreign-key cleanup order, missing metrics;
- structural performance issues: too much work inside one transaction, repeated serialization per WebSocket client, weak outbox indexes.

Do not use Windows local runs to claim:

- maximum Linux connection count;
- stable p99/p999;
- cloud CPU/disk/kernel capacity;
- multi-machine SUT/load-generator performance;
- final "supports N users" or production-like capacity.

## Development Layers

1. Local correctness and attack testing.
   Goal: every workload runs, invariants are checked, failures are diagnosable.

2. Local relative optimization.
   Goal: compare before/after under the same laptop setup. Acceptable claims are directional, such as lower outbox lag, fewer slow consumers, or smaller lock waits under the same script.

3. Final Linux calibration.
   Goal: only at the release/final evidence phase, run native Linux 3-run raw outputs to decide whether any capacity number can be published.

## When To Test Each Suspected Bottleneck Locally

| Area | Test during | Local workload | Evidence to record | Local verdict allowed |
|---|---|---|---|---|
| PostgreSQL hot auction row | after bid/rule/idempotency changes | `final-second-bid-burst.js`, focused backend concurrency tests | accepted/rejected mix, retry-later, lock/tx/pool metrics, invariant check | bottleneck direction only |
| Outbox relay/table pressure | after bid/payment/scheduler/outbox changes | `outbox-burst.js`, relay integration tests | backlog, delivery lag, DEAD count, retry count, table bloat query if available | bottleneck direction only |
| WebSocket fanout | after realtime/client recovery changes | `watcher-fanout.js`, `slow-consumer.js` | connection success, messages, slow closes, queue/goroutine/RSS metrics | bottleneck direction only |
| Reconnect and snapshot pressure | after recovery/history/snapshot changes | `reconnect-storm.js` | recovered count, snapshot source mix, DB snapshot rebuild count | bottleneck direction only |
| Multi-room isolation | after room routing/admission/fanout changes | `multi-room-isolation.js` | cross-room leak rate, cold-room errors, room-scoped anomaly/metrics | isolation behavior, not capacity |
| Abuse/rate-limit posture | after gateway/rate-limit changes | `bid-abuse.js` | rate-limited/too-hot/retry-after distribution, anomaly rows | behavior distribution only |

## Required Local Evidence For Substantial Changes

For changes touching bid, outbox, realtime, recovery, room routing, payment, or admission control:

- run relevant backend integration tests;
- run relevant k6 smoke workload if the backend can run locally;
- capture raw output under `docs/perf/raw/windows-local/` or summarize in `docs/evidence/`;
- state clearly that the result is Windows local smoke or relative comparison;
- run or update an invariant check when the workload mutates money state.

## Test-Attacker Link

When designing or reviewing local workloads, use the `live-auction-v2-tiktok-test-attacker` scenario catalog:

- malicious clients;
- duplicate and stale idempotency;
- forged room/auction pairs;
- Redis/DB failure modes;
- slow consumers and reconnect storms;
- payment callback duplication and late callbacks;
- diagnostics visibility.

Route-mocked Playwright tests are UI contract checks. They do not prove backend behavior under attack or load.

## Final Claim Boundary

Linux native 3-run evidence is moved to the final capacity/release gate. P2/P3 development may proceed with Windows local evidence as long as:

- no final capacity number is published;
- known bottlenecks and unproven capacity are documented;
- Linux final baseline remains in P5 before submission claims.
