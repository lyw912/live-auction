# Document Map And Cleanup Policy

> Status: cleanup guide, 2026-05-31.

## Keep Immutable

| Path | Policy |
|---|---|
| `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md` | Immutable official source |
| `docs/references/official-brief-images/` | Immutable downloaded official images |
| `单热点调研.md` | Keep as important research source; not a direct governing design |

## Current Authority

| Path | Use |
|---|---|
| `docs/current/README.md` | first read for all new work |
| `docs/current/architecture.md` | current architecture contract |
| `docs/current/performance-correctness-contract.md` | PTS-1B correctness, latency, and fault gates |
| `docs/current/evidence-policy.md` | current evidence classification and pass/fail rules |
| `docs/current/runtime-profiles.md` | env/profile split for demo vs PTS-1B |
| `docs/current/pts-run-review-template.md` | required template for new PTS report reviews |
| `docs/current/fault-injection-runbook.md` | current Redis/Kafka/PostgreSQL fault gates |
| `docs/current/chaos-test-runbook.md` | current S4-core and Toxiproxy execution runbook |
| `docs/current/fault-test-matrix.md` | layered future S4 fault matrix beyond S4-core |
| `docs/current/pts1b-readiness-checklist.md` | final pre-run PTS-1B checklist |
| `docs/current/document-map.md` | cleanup and conflict policy |
| `docs/archive/evidence-era-map.md` | historical evidence classification |
| `docs/archive/progress-history-map.md` | historical progress/decision-log classification |
| `docs/archive/progress/` | physically archived phase/progress ledgers |
| `docs/adr/README.md` | ADR classification and supersession index |
| `docs/current/test-strategy/` | judge-facing test narrative: scenarios→metrics→PTS playbook→fault structure→report template (authority for *why/what-to-show*) |
| `tests/pts/MANIFEST.md` | current PTS workload/script manifest (implementation index) |
| `tests/pts/HISTORICAL.md` | historical PTS script/JMX index and opt-in rules |
| `docs/perf/pts/evidence/README.md` | raw PTS evidence directory policy |
| `docs/perf/pts/l4b-kafka/report-review-index.md` | historical/current-adjacent PTS report review classification |
| `README.md` | public project entry; must point to current architecture |
| `docs/setup_guide.md` | local setup; must distinguish normal demo from PTS-1B profile |
| `docs/demo/known-limits.md` | demo/product limits; must not contradict current hot-bid contract |
| `.env.example` | conservative local demo profile |
| `.env.pts1b.example` | PTS-1B Redis/Kafka hot-engine profile |

## Archived Or Background

| Path | Use | Caveat |
|---|---|---|
| `docs/design-v2-industrial/00-project-brief.md` | project scope and scoring decomposition | official summary inside repo, not immutable source |
| `docs/design-v2-industrial/07-frontend-ux.md` | UI state and workflow baseline | resolve bid-contract conflict through `docs/current` |
| `docs/design-v2-industrial/19-extreme-bidding-atmosphere.md` | atmosphere/experience design | keep if it does not fake server truth |
| `docs/design-v2-industrial/20-ui-ux-redesign.md` | UI/UX redesign direction | keep if it does not conflict with recovery/decision semantics |
| `docs/design-v2-industrial/08-observability-and-ops.md` | diagnostics/runbook ideas | update PG-only assumptions as work continues |
| `docs/design-v2-industrial/12-engineering-rules.md` | engineering discipline | keep discipline, supersede PG-lane truth wording |
| `docs/reviews/README.md` | review archive entry point | individual reviews are scoped to their original era |
| `docs/adr/README.md` | ADR index | ADRs are not all current hot-path authority |

## Public/Demo Entry Policy

These files are likely to be read by reviewers before deep docs:

- `README.md`
- `docs/setup_guide.md`
- `docs/demo/demo-flow.md`
- `docs/demo/known-limits.md`
- `docs/demo/p10-no-mock-auction-demo.md`

They must not say PostgreSQL is the only current bid truth without qualifying the current Redis/Kafka hot path. They should distinguish:

- normal local demo/product flow;
- current PTS-1B Redis/Kafka hot-engine profile;
- historical PG-lane evidence.

Runtime profile rules are in `docs/current/runtime-profiles.md`.

## Superseded For Hot-Bid Architecture

These files are historical/background only for hot-bid work. They must not be used as current hot-path authority:

- `docs/design-v2-industrial/02-architecture.md`
- `docs/design-v2-industrial/03-domain-model-and-rules.md`
- `docs/design-v2-industrial/04-data-and-storage.md`
- `docs/design-v2-industrial/05-api-contracts.md`
- `docs/design-v2-industrial/09-performance-and-benchmark.md`
- `docs/design-v2-industrial/10-test-gates.md`
- `docs/design-v2-industrial/11-implementation-plan.md`
- `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`
- `docs/design-v2-industrial/18-p3-p4-roadmap-reset.md`
- `docs/archive/progress/p3-decision-log.md`
- old `docs/archive/progress/p*-progress.md` execution logs.

Read `docs/archive/progress-history-map.md` before citing any of these progress files.

## Evidence Policy

Do not keep evidence in current entry paths just because it once mattered. Delete non-evidence smoke/noise, and physically archive evidence that is useful for diagnosis or audit.

- Current evidence proves the current Redis/Kafka/PG contract.
- Historical evidence explains why earlier routes were rejected or where bottlenecks appeared.
- Failed pressure runs are useful if they have clear workload, artifact, and bottleneck notes.
- Raw runtime artifacts under docs should not be treated as curated documentation.
- Physical evidence layout is `docs/perf/pts/evidence/current`, `incoming`, and local-only `archive/*`.
- Raw PTS archive directories are ignored by Git by default; commit reviewed reports and indexes, not bulk sampler/metrics dumps.

## Naming Policy For Future Docs

Use names that expose the scope:

- `current-*` for governing docs.
- `historical-*` or archive maps for old phases.
- include report ID only for run reviews, not for architecture docs.
- avoid new ambiguous `p3`, `phase`, `l4`, or `route-b` docs unless they are under an explicitly historical directory.

## Completed Cleanup Decisions

- Misleading current-authority labels in `docs/evidence/index.md` are superseded.
- PTS evidence artifacts are physically archived under `docs/perf/pts/evidence/archive/*`.
- Bulk raw PTS archive artifacts are removed from the committed documentation tree; archive directories are local-only unless a reviewed report justifies a narrow exception.
- Old root-level progress logs are physically archived under `docs/archive/progress/`.
- New raw PTS output goes to `docs/perf/pts/evidence/incoming/`.
- PTS run reviews must use `docs/current/pts-run-review-template.md`.
- Fault gates and final readiness are governed by `docs/current/fault-injection-runbook.md` and `docs/current/pts1b-readiness-checklist.md`.
