# Evidence Index

> Date: 2026-05-24 Asia/Shanghai  
> Status: authoritative evidence map for P3/P4 reset.

## Classification

- `AUTHORITATIVE`: current decision input.
- `PARTIAL`: valid for a specific conclusion, but bounded by a known caveat.
- `HARNESS_ONLY`: proves scripts, seed, auth, or instrumentation can run; not bottleneck evidence.
- `SUPERSEDED`: replaced by newer evidence or policy.
- `RAW_LOCAL`: raw output exists but should be opened only through compact analysis or a named investigation.

## Authoritative Evidence

| Evidence | Classification | Current use |
|---|---|---|
| `docs/evidence/p2-01-real-session-boundary.md` | AUTHORITATIVE | P2 real session shortcut removed. |
| `docs/evidence/p2-02-room-membership-acl.md` | AUTHORITATIVE | Room membership/ACL is first-class for REST and WS. |
| `docs/evidence/p2-03-room-context-routing.md` | AUTHORITATIVE | Fixed `room_main` path removed from product flow. |
| `docs/evidence/p2-04-bid-admission-control.md` | AUTHORITATIVE | Admission exists as product protection, not as performance exploration. |
| `docs/evidence/p2-05-payment-provider-boundary.md` | AUTHORITATIVE | Payment provider mock has webhook/idempotency/reconciliation semantics. |
| `docs/evidence/p2-06-security-abuse-diagnostics.md` | AUTHORITATIVE | Security/abuse diagnostics have real producers. |
| `docs/evidence/p2-07-release-baseline-harness.md` | AUTHORITATIVE | Local baseline harness and final Linux guardrails exist. |
| `docs/evidence/p3-01-outbox-claim-fix-2026-05-24.md` | AUTHORITATIVE | Outbox claim bottleneck fixed for the tested local profile. |
| `docs/evidence/p3-02-relay-shard-ownership-2026-05-24.md` | AUTHORITATIVE | Relay shard ownership/failover is implemented with Windows-local evidence. |
| `docs/evidence/p3-03-local-stress-harness-2026-05-24.md` | AUTHORITATIVE_FOR_HARNESS | P3 runner isolation, zero-check detection, and workload management are fixed. |
| `docs/reviews/p3-04-centrifugo-judge-origin-2026-05-25.md` | AUTHORITATIVE | Original hostile Centrifugo comparison that triggered P3 realtime hardening. |
| `docs/evidence/p3-05-centrifugo-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | Bounded recovery, byte backpressure, stream epoch, outbox notify wakeup, and metrics implemented with focused tests. |
| `docs/adr/p3-01-centrifugo-borrowing-decision.md` | AUTHORITATIVE | Decision to borrow Centrifugo mechanisms without adding a second runtime transport. |
| `docs/reviews/p3-06-debezium-borrowing-review-2026-05-25.md` | AUTHORITATIVE | Hostile Debezium comparison: borrow CDC/outbox design discipline, do not rebuild runtime around Debezium. |
| `docs/adr/p3-02-debezium-borrowing-decision.md` | AUTHORITATIVE | Decision to keep Debezium/CDC evidence-gated and map borrowed ideas to project-owned outbox/recovery semantics. |
| `docs/evidence/p3-06-debezium-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | Debezium-borrowed envelope validation, relay watermarks, snapshot audit, control signals, and error classification implemented with focused tests. |
| `docs/reviews/p3-07-nats-jetstream-borrowing-review-2026-05-25.md` | AUTHORITATIVE | Hostile NATS/JetStream comparison: borrow delivery-state, ack/redelivery, dedupe, slow-consumer, monitoring, and snapshot/catchup design logic; do not rebuild runtime around a broker without measured internal messaging need. |
| `docs/evidence/p3-07-nats-jetstream-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | NATS/JetStream-borrowed delivery diagnostics implemented in app-owned outbox/realtime code with full backend and frontend typecheck verification. |
| `docs/reviews/p3-08-redis-lua-borrowing-review-2026-05-25.md` | AUTHORITATIVE | Hostile Redis Lua comparison: accept existing Lua GCRA admission and one-time WS ticket borrowing; keep PostgreSQL as auction money truth; keep Lua reservation/full rewrite evidence-gated behind PG hot-row proof, reconciliation ADR, and invariant tests. |
| `docs/evidence/p3-08-redis-lua-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | Redis Lua borrowed hardening implemented in app-owned code: script runner, stable script names, `EVALSHA` fallback discipline, script metrics/error classes, hash-tagged admission keys, and focused tests. |
| `docs/evidence/p3-09-raw-artifact-retention-cleanup-2026-05-25.md` | AUTHORITATIVE_FOR_EVIDENCE_HYGIENE | `docs/perf/raw` cleanup applied the current retention policy: keep evidence-referenced compact artifacts and delete old full logs, duplicate failed attempts, and unreferenced ignored raw directories. |
| `docs/evidence/p3-10-admission-off-harness-proof-2026-05-25.md` | AUTHORITATIVE_FOR_HARNESS | P3-R1 closed: committed downstream workloads prove `ADMISSION_ENABLED=false`, `auction_admission_enabled 0/0`, and zero admission reject delta in compact reports. |
| `docs/evidence/p3-11-multi-room-hot-cold-stress-2026-05-25.md` | AUTHORITATIVE | P3-R2 found shared bid-path DB/lock pressure: hot-room bid load degraded cold-room bid p95 from about `25ms` to about `506ms` without cross-room leak, cold WS error, or admission pollution. |
| `docs/evidence/p3-12-realtime-fanout-drilldown-2026-05-25.md` | AUTHORITATIVE | P3-R3 clean realtime drilldown: self-hub passed 300 watcher fanout, healthy-vs-slow isolation, and 100-VU reconnect recovery; higher profiles expose PG/recovery ceilings, not fanout failure. |
| `docs/evidence/p3-13-pg-hot-row-drilldown-2026-05-25.md` | AUTHORITATIVE | P3-R4 confirmed PostgreSQL hot auction row contention under clean admission-off bid pressure and implemented a conservative transaction-work reduction that lowered same-profile local p99 and lock/pool wait. Outbox pending remains the next bottleneck input. |
| `docs/evidence/p3-14-outbox-second-order-pressure-2026-05-25.md` | AUTHORITATIVE | P3-R5 found outbox relay watermark refresh as a second-order drain bottleneck and optimized batch drain to refresh watermarks once per touched shard. Relay drain improved about 4.6x in the tested post-observe window, but backlog still remained. |
| `docs/evidence/p3-15-architecture-go-no-go-2026-05-25.md` | AUTHORITATIVE | P3-R6 keeps the current release-track architecture: PostgreSQL bid truth, app-owned DB outbox relay, Redis projection/history, and self-hub realtime. Redis Lua reservation, Debezium/CDC, and NATS/JetStream runtime remain no-go for this P3 cycle without new ADR/invariant evidence. |
| `docs/evidence/p3-16-final-local-ceiling-sweep-2026-05-26.md` | AUTHORITATIVE | P3-R7 records the final Windows-local downstream ceiling table after P3-R4/P3-R5. It applies a small relay drain optimization, confirms outbox drain improvement, and classifies bid escalation beyond the clean profile as k6 VU ceiling caused by DB row-lock latency growth. |
| `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md` | AUTHORITATIVE | P3/P4 pressure protocol and admission-off policy. |
| `docs/design-v2-industrial/18-p3-p4-roadmap-reset.md` | AUTHORITATIVE | Current P3/P4 execution order and decision gates. |
| `docs/p3-decision-log.md` | AUTHORITATIVE | Current decisions, superseded evidence, and go/no-go gates. |

## Partial Evidence

| Evidence | Classification | Keep for | Caveat |
|---|---|---|---|
| `docs/evidence/p3-00-stress-attacker-round-1-2026-05-24.md` | PARTIAL | Discovery of outbox claim O(pending squared) bottleneck and PG hot-row direction. | Used raised admission ceilings; future downstream evidence must use `ADMISSION_ENABLED=false`. |
| `docs/evidence/p3-01-realtime-fanout-attack-2026-05-24.md` | PARTIAL | Realtime self-hub direction, connection-storm classification, slow-consumer harness. | Needs clean admission-off reruns and Linux calibration before final transport decision. |
| `docs/perf/p2-07-linux-baseline-round-1.md` | PARTIAL | Baseline harness guardrail. | Not final P5 3-run capacity evidence. |
| `docs/perf/windows-local-k6-smoke-2026-05-23.md` | PARTIAL | Early local workload smoke. | Not adversarial P3 bottleneck evidence. |

## Harness-Only Evidence

| Evidence | Classification | Reason |
|---|---|---|
| Early `docs/perf/raw/p3-00/` bundles | HARNESS_ONLY | Proved seed/auth/scripts could run; admission polluted downstream attribution. |
| `docs/perf/raw/p3-local-stress-202605240620/` | HARNESS_ONLY | Admission-on smoke, useful for protection behavior and runner sanity. |
| `docs/perf/raw/p3-local-stress-202605240623/` | HARNESS_ONLY | Downstream realtime/isolation smoke; not adversarial enough. |

## Raw Artifact Policy

Default behavior:

1. Run `pnpm exec node tests/load/analyze-p3-artifacts.mjs`.
2. Read `docs/perf/raw/p3-artifact-index.json` and the relevant `analysis-compact.*` files.
3. Open at most the raw files named by the compact report for the suspected bottleneck.

Do not bulk-read `docs/perf/raw/**`.

Keep:

- compact reports;
- summaries;
- evidence markdown;
- raw paths referenced by authoritative evidence.

Ignore unless investigating:

- old raw bundles not referenced by this index;
- full logs from smoke runs;
- raw files from runs classified as harness-only.

Historical cleanup:

- `docs/evidence/p3-09-raw-artifact-retention-cleanup-2026-05-25.md` records the 2026-05-25 cleanup of pre-policy raw artifacts.

Clean or archive later only after confirming no evidence document references the raw path.

## Evidence Still Missing

| Gap | Why it matters | Next evidence |
|---|---|---|
| PG hot-row attribution after outbox fix | Closed by P3-R4 for Windows-local direction evidence. | `docs/evidence/p3-13-pg-hot-row-drilldown-2026-05-25.md`; final Linux capacity still separate. |
| Outbox second-order pressure | Closed for current Windows-local direction evidence; optimized watermark refresh and later tuned R7 batch drain, but backlog still remains under high local input. | `docs/evidence/p3-14-outbox-second-order-pressure-2026-05-25.md`; `docs/evidence/p3-16-final-local-ceiling-sweep-2026-05-26.md`. |
| P4 invariant verifier | Stress evidence still relies too much on manual interpretation. | CLI report for seq, terminal state, order, winner, idempotency, cross-room leak, outbox coverage. |
| Final Linux 3-run capacity baseline | Required before any public capacity claim. | P5 Linux native baseline with environment and raw output. |
