# Current Evidence Policy

> Status: governing evidence policy, 2026-06-05.

This file defines what can be cited as current evidence after the PG-lane to Redis/Kafka reset.

## Evidence Classes

| Class | Meaning | Can prove current PTS-1B? |
|---|---|---|
| `CURRENT_PASS` | Same current architecture, current JMX/CSV, current reset/preflight/verify gates, no known P0 gaps | Yes |
| `CURRENT_FAILING` | Same current architecture but p99, UX, correctness, or fault gate failed | No, but can prove a bottleneck |
| `CURRENT_ADJACENT` | Close to current architecture but contract, script, or implementation differed | No, useful for diagnosis only |
| `HISTORICAL` | PG-lane, Redis-guard, early Redis-ledger, old L4B, or old phase evidence | No |
| `HARNESS_ONLY` | Proves seed, auth, PTS connectivity, scripts, or metrics collection | No |
| `RAW_ARTIFACT` | Raw logs/metrics/report details without curated interpretation | No |

## Minimum Current PTS-1B Evidence Pack

A run cannot be called current PTS-1B success unless it has all of these:

- report ID;
- git SHA and dirty-tree note;
- runtime profile and env source (`.env.example`, `.env.pts1b.example`, or scripted reset);
- JMX path from `tests/pts/MANIFEST.md`;
- CSV path and session count;
- reset command and preflight label;
- PTS report details or full sampler export;
- server evidence under `docs/perf/pts/evidence/incoming/<label>/` before review, then under `current/` or `archive/*/` after classification;
- `ENGINE_*` distribution, HTTP status distribution, durability status, and settlement status;
- normal final hot-path decisions in the default profile use `durability_status=KAFKA_ACKED`; bounded `ENGINE_DURABLE` fallback is acceptable only when relay/settlement convergence later proves no data loss;
- Redis Stream/pending state, Kafka relay lag/DLQ, PostgreSQL settlement coverage, and outbox backlog after convergence;
- Redis Engine recovery evidence when claiming fault readiness, including `resume_redis_engine` signal result `rto_ms`, preflight/postflight status, checkpoint hash, and Redis Engine diagnostics `last_recovery_rto_ms`;
- correctness verifier output from `tests/pts/verify-l4b-pts-correctness.sh`;
- Redis/Kafka/PostgreSQL health evidence;
- explicit fault-injection evidence if claiming resilience;
- final classification: `CURRENT_PASS`, `CURRENT_FAILING`, `CURRENT_ADJACENT`, `HISTORICAL`, `HARNESS_ONLY`, or `RAW_ARTIFACT`.

New PTS report reviews must use `docs/current/pts-run-review-template.md`.

## Current Pass Criteria

All must hold:

- user-visible `ENGINE_*` decision p99 <= 60ms for default `kafka_ack` PTS-1B evidence, or <= 50ms for explicit `redis_aof` low-latency evidence;
- final `ENGINE_*` decision responses are `KAFKA_ACKED` >= 99% in default `kafka_ack` mode, with bounded `ENGINE_DURABLE` fallback <= 1% and proven convergence;
- 1000 intended unique bids are classified;
- final winner is the highest valid amount;
- every low reject is justified against decision-time current/required price;
- no unresolved pending append, DLQ, engine pause, or settlement gap remains;
- Redis/Kafka/PostgreSQL fault gates pass or the claim is explicitly scoped to no-fault performance only;
- no dominant `PROCESSING_RETRY_LATER`, vague `409`, or seconds-long pending UX.

## Historical Evidence Rules

- A failed report can be valuable, but cite it as a failed report.
- Do not retroactively reclassify a timeout failure as pass by changing the
  target after the run. If the new target is 110s, rerun with a 110s timeout and
  cite that evidence separately.
- Do not combine "fast" from one architecture and "correct" from another.
- Do not cite old `AUTHORITATIVE` labels in `docs/evidence/index.md` without checking `docs/archive/evidence-era-map.md`.
- Do not bulk-read raw evidence directories. Use the manifest first, then open only the run needed for the current question.
- Raw archive directories are local-only by default. Commit reviewed summaries/indexes instead of bulk sampler logs, metrics dumps, or transient server snapshots.

## Current Evidence Locations

| Purpose | Path |
|---|---|
| Current success/failure contract | `docs/current/performance-correctness-contract.md` |
| Active PTS workload manifest | `tests/pts/MANIFEST.md` |
| Historical PTS script index | `tests/pts/HISTORICAL.md` |
| Runtime profile guide | `docs/current/runtime-profiles.md` |
| Current PTS run review template | `docs/current/pts-run-review-template.md` |
| Fault injection runbook | `docs/current/fault-injection-runbook.md` |
| PTS-1B readiness checklist | `docs/current/pts1b-readiness-checklist.md` |
| Raw PTS evidence directory policy | `docs/perf/pts/evidence/README.md` |
| L4B report review index | `docs/perf/pts/l4b-kafka/report-review-index.md` |
| Review archive index | `docs/reviews/README.md` |
| Historical era map | `docs/archive/evidence-era-map.md` |
| Historical broad evidence index | `docs/evidence/index.md` |
