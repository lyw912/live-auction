# PTS Workspace Index

> Status: bounded PTS workspace index, 2026-05-31.

This folder is not the current architecture authority. Start current PTS-1B work from:

1. `docs/current/README.md`
2. `docs/current/performance-correctness-contract.md`
3. `docs/current/evidence-policy.md`
4. `docs/current/runtime-profiles.md`
5. `docs/current/fault-injection-runbook.md`
6. `docs/current/pts1b-readiness-checklist.md`
7. `tests/pts/MANIFEST.md`

## Current Active Inputs

| Path | Use |
|---|---|
| `docs/perf/pts/evidence/README.md` | raw evidence layout and archive rules |
| `docs/perf/pts/l4b-kafka/report-review-index.md` | report review classification |
| `docs/perf/pts/l4b-kafka/final-burst-1000vu-runbook.md` | current-adjacent runbook; verify against `tests/pts/MANIFEST.md` before use |
| `docs/perf/pts/l4b-kafka/pts1b-observability-runbook-2026-05-30.md` | observability helper for PTS-1B |
| `docs/perf/pts/pts-1ab-1000vu-sessions.csv` | current PTS-1A/PTS-1B 1000-user session data source |
| `docs/perf/pts/generate-pts-sessions.sql` | session CSV generation helper |

## Historical Or Current-Adjacent Background

These documents explain route changes and failed/partial runs. They are not final success proof:

- `docs/perf/pts/pressure-test-plan.md`
- `docs/perf/pts/pts1-hotspot-optimization-plan.md`
- `docs/perf/pts/hotspot-industrial-research-2026-05-28.md`
- `docs/perf/pts/hotspot-redesign-roadmap-2026-05-28.md`
- `docs/perf/pts/full-pressure-runbook.md`
- `docs/perf/pts/scoring-dimension-decomposition-2026-05-28.md`
- `docs/perf/pts/l4b-kafka/*.md`
- `docs/perf/pts/archive/data/*.csv`

Read them only after the current contract above, and label any citation as current-adjacent, historical, or superseded.

## Root Hygiene

Do not put binaries, server logs, pid files, raw run folders, or one-off smoke output in this directory.

Raw PTS output goes to:

```text
docs/perf/pts/evidence/incoming/<label>/
```

After review:

- `CURRENT_PASS` evidence may move to `docs/perf/pts/evidence/current/`.
- useful failing/partial/historical evidence moves to `docs/perf/pts/evidence/archive/*`.
- non-evidence smoke/noise is deleted.

## Current PTS-1B Pass Definition

A run is not successful by report ID or HTTP `200` count alone. It must prove:

- 1000 final-second hot-auction bid attempts under the current manifest/profile;
- user-visible `ENGINE_*` decision p99 <= 50ms;
- global highest valid amount wins;
- every low reject is justified by decision-time state;
- idempotency survives duplicate concurrent requests;
- Redis/Kafka/PostgreSQL fault gates pass or fail closed with measured RTO;
- evidence records env/profile, git SHA, JMX/CSV, metrics, verifier output, and reviewed raw paths.
