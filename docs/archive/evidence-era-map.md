# Evidence Era Map

> Status: historical classification map, 2026-05-31.

This project changed architecture several times while chasing the PTS-1B target. Old evidence is valuable for explaining the engineering journey, but it must not be used to prove the current system unless explicitly revalidated.

## Eras

| Era | Main idea | Typical docs/evidence | Current use |
|---|---|---|---|
| P0/P1/P2 foundation | PostgreSQL truth, outbox, Redis projection, H5/PC workflows | `docs/evidence/p0-*`, `p1-*`, `p2-*` | product completeness, baseline correctness, UI/recovery background |
| P3/P4 PG-lane pressure | PostgreSQL row lock remains auction truth; optimize transaction/outbox/admission | `docs/evidence/p3-*`, `p4-*`, `docs/archive/progress/p3-decision-log.md` | historical bottleneck proof; not current hot-path architecture |
| PTS L1-L3 | admission/debounce, PG lane, realtime delivery | `docs/evidence/pts-l1-*`, `pts-l2-*`, `pts-l3-*` | shows incremental hardening; not PTS-1B final proof |
| PTS L4a Redis guard | Redis prefilter protects PG but PG remains final decision | `docs/evidence/pts-l4a-*` | historical route; useful for cache/guard correctness ideas |
| Early L4b Redis ledger | Redis engine + ledger exploration before final fence/UX contract | `docs/evidence/pts-l4b-redis-ledger-engine-*` | historical implementation basis; must be checked against current contract |
| Kafka fence/recovery fixes | Redis decisions fenced through Kafka and settled to PG | `docs/evidence/pts-l4b-kafka-ledger-recovery-fix-*`, `docs/perf/pts/l4b-kafka/*` | current-adjacent, but individual reports may show failed p99/UX/fault gaps |
| Current balanced contract | Redis hot state + Kafka decision WAL/fence + PG settlement/audit + fail-closed recovery | `docs/current/*` plus future verified PTS/fault evidence | current authority |

## Current Evidence Manifests

| Path | Purpose |
|---|---|
| `docs/current/evidence-policy.md` | Defines current evidence classes and pass/fail rules |
| `tests/pts/MANIFEST.md` | Defines active vs historical PTS scripts |
| `docs/perf/pts/evidence/README.md` | Defines how to read raw PTS evidence directories |
| `docs/perf/pts/l4b-kafka/report-review-index.md` | Classifies individual L4B report reviews |
| `docs/reviews/README.md` | Classifies historical review folders and caveats |

## Report IDs Mentioned In Recent Work

| Report/run | Classification | Note |
|---|---|---|
| `0Z57X76G` | current-adjacent failed/partial evidence | useful correctness direction but did not prove p99 50ms or full fault gates |
| `R652X74G` | current-adjacent failed/partial evidence | useful for comparing Kafka/settlement fence behavior; not final success |
| `UF5DX7GG` | current-adjacent investigation evidence | use only with its verifier and server evidence; one HTTP `200` alone is not business-result proof |
| `MT50X7MG` | current-adjacent bottleneck evidence | server metrics showed hot path/gateway/Redis stage bottlenecks; not final success |

## Rules For Using Historical Evidence

- Label every citation as current, current-adjacent, historical, harness-only, or superseded.
- Never combine latency from one architecture with correctness proof from another as if it were one system.
- Never use a report review as success evidence if the review says p99, UX, or fault injection failed.
- Archive failed evidence if it identifies a real bottleneck or prevents repeating a bad route; delete smoke/noise that cannot support diagnosis.
