# Windows Local Performance And Attack Strategy

Date: 2026-05-24 Asia/Shanghai

Status: Accepted working policy for the long Windows development phase.

Detailed P3 cadence and drilldown rules: `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`. Use the `live-auction-v2-stress-attacker` skill when the goal is to actively create pressure, expose bottlenecks, attribute them, and quantify before/after changes.

## Rule

Windows local testing is required and valuable. It is not final capacity evidence.

During P3/P4/P5 performance exploration, admission must be fully disabled with `ADMISSION_ENABLED=false`. Do not raise ceilings as a substitute. The goal is to force pressure into the downstream subsystem and expose the real bottleneck. Runs with admission rejections are admission/protection evidence, not bottleneck evidence.

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
   Goal: compare before/after under the same laptop setup with admission disabled. Acceptable claims are directional, such as lower outbox lag, fewer slow consumers, or smaller lock waits under the same script.

3. Final Linux calibration.
   Goal: only at the release/final evidence phase, run native Linux 3-run raw outputs to decide whether any capacity number can be published. Admission remains disabled for capacity discovery; admission limits are chosen and validated after bottleneck exploration stops.

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
- for P3/P4/P5 downstream-pressure evidence, include raw proof that `auction_admission_enabled 0` before and after the workload and that admission rejection counters did not increase.

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

## Recurring Stress Rule

Short smoke checks are necessary but not sufficient. During active P3 work, before each P3 milestone, and whenever performance confidence matters, run the local stress loop defined in `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`.

The expected outcome is not always a bigger number. The expected outcome is a defensible verdict:

- `NO_REGRESSION`;
- `BOTTLENECK_FOUND`;
- `HARNESS_GAP`;
- `ENV_LIMIT`.

## Final Claim Boundary

Linux native 3-run evidence is moved to the final capacity/release gate. P2/P3 development may proceed with Windows local evidence as long as:

- no final capacity number is published;
- known bottlenecks and unproven capacity are documented;
- Linux final baseline remains in P5 before submission claims.
