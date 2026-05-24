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
| `docs/evidence/p3-04-centrifugo-judge-origin-2026-05-25.md` | AUTHORITATIVE | Original hostile Centrifugo comparison that triggered P3 realtime hardening. |
| `docs/evidence/p3-05-centrifugo-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | Bounded recovery, byte backpressure, stream epoch, outbox notify wakeup, and metrics implemented with focused tests. |
| `docs/adr/p3-01-centrifugo-borrowing-decision.md` | AUTHORITATIVE | Decision to borrow Centrifugo mechanisms without adding a second runtime transport. |
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

Clean or archive later only after confirming no evidence document references the raw path.

## Evidence Still Missing

| Gap | Why it matters | Next evidence |
|---|---|---|
| Admission-off proof for all downstream P3 workloads | Avoids hidden ceilings and false bottlenecks. | Runner-enforced `ADMISSION_ENABLED=false`, before/after metrics, zero admission reject delta. |
| Hot/cold multi-room adversarial pressure | `multi-room-isolation` is only smoke-level today. | Hot room bid/fanout pressure plus cold room latency/fanout and cross-room invariant. |
| Clean realtime fanout/slow-consumer/reconnect drilldown | Self-hub needs cleaner downstream-pressure evidence before final runtime claims. | Staggered fanout, healthy-vs-slow, reconnect storm, pprof/runtime metrics, environment classification. |
| PG hot-row attribution after outbox fix | Current bid path still saturates locally. | Lock/tx/pool metrics, pprof, transaction-work breakdown, invariant checker. |
| Outbox second-order pressure | Claim query fixed, but table/update/bloat under longer load is not closed. | Longer outbox burst, multi-room outbox pressure, bloat/dead tuple/lag evidence. |
| P4 invariant verifier | Stress evidence still relies too much on manual interpretation. | CLI report for seq, terminal state, order, winner, idempotency, cross-room leak, outbox coverage. |
| Final Linux 3-run capacity baseline | Required before any public capacity claim. | P5 Linux native baseline with environment and raw output. |
