# PTS Evidence Directory Policy

> Status: raw evidence manifest, 2026-05-31.

This directory stores raw and semi-raw PTS evidence. It is intentionally not the current evidence authority. Start from:

- `docs/current/evidence-policy.md`
- `tests/pts/MANIFEST.md`
- `docs/archive/evidence-era-map.md`

## Directory Layout

| Path | Purpose |
|---|---|
| `current/` | reserved for future `CURRENT_PASS` evidence only |
| `incoming/` | new raw output from collection scripts before review/classification |
| `archive/` | optional local-only archive root for classified raw evidence |
| `archive/current-adjacent/` | local-only recent Redis/Kafka/PTS-1A/PTS-1B partial, failing, or diagnostic runs |
| `archive/historical/` | local-only PG-lane, Redis-guard, core-pressure, and old hotspot runs |
| `archive/harness-only/` | local-only preflight, before snapshots, and script/harness proof |

The root of this directory should not accumulate run folders. New collection
scripts write to `incoming/<label>`; after review, move the folder to
`current/`, a local-only `archive/*/` directory, or delete it. Do not commit raw
sampler exports, Prometheus dumps, server snapshots, or ad hoc report JSON just
because they were useful during an investigation; commit the reviewed report or
index entry instead.

## How To Read This Directory

1. Identify the report/run label you need.
2. Check whether it is `CURRENT_PASS`, `CURRENT_FAILING`, `CURRENT_ADJACENT`, `HISTORICAL`, `HARNESS_ONLY`, or `RAW_ARTIFACT`.
3. Open only that run's curated files first:
   - `l4b-correctness.txt`
   - `l4b-*-gates.tsv`
   - `metrics.prom`
   - `postgres-summary.txt`
   - `redis-info.txt`
   - `pts-report-details.json` or `report-details.json`
   - `analysis-summary.md` when present
4. Do not bulk-read every run directory.

## Recent Current-Adjacent Runs

| Label/report | Classification | Current use |
|---|---|---|
| `archive/current-adjacent/UF5DX7GG` | `CURRENT_ADJACENT`/investigation | Use to inspect latest business distribution and verifier state; not final success by name alone |
| `archive/current-adjacent/MT50X7MG` | `CURRENT_ADJACENT`/bottleneck evidence | Useful for gateway/Redis stage bottleneck analysis; not final success |
| `archive/current-adjacent/after-0Z57X76G-pts1b` | `CURRENT_ADJACENT`/partial | Correctness direction; did not prove p99 50ms or full fault gates |
| `archive/current-adjacent/after-R652X74G-pts1b-fixed` | `CURRENT_ADJACENT`/partial | Kafka/settlement fence investigation; not final success |

Deleted as non-evidence:

| Label | Reason |
|---|---|
| UF5DX7GG no-expected smoke directory | verifier smoke without expected unique-bid constraint; deleted instead of retained |

## Historical Runs

Most `before-*`, `after-*`, and bare report-ID directories were created during PG-lane, Redis-guard, early L4B, route-B, or failure-investigation phases. If retained locally, they live under `archive/historical/`, `archive/current-adjacent/`, or `archive/harness-only/`. Do not cite them as current proof without a matching current run review.

## Repository Policy

`docs/perf/pts/evidence/archive/**` is ignored by Git except for directory
markers. This is intentional: old raw evidence already polluted normal search
and model context. Preserve durable lessons in:

- `docs/current/evidence-policy.md`
- `docs/archive/evidence-era-map.md`
- `docs/perf/pts/l4b-kafka/report-review-index.md`
- a focused run review under `docs/perf/pts/l4b-kafka/`

If a raw artifact must be committed for audit, put it behind a narrowly named
review directory and explain why an index/report is insufficient.

## Current Evidence Naming

For future current runs, prefer:

```text
before-pts1b-current-YYYYMMDD-HHMM
after-REPORTID-pts1b-current-YYYYMMDD-HHMM
fault-redis-loss-REPORTID-YYYYMMDD-HHMM
fault-kafka-timeout-REPORTID-YYYYMMDD-HHMM
```

Avoid new ambiguous names such as `after-fix`, `final-ready`, `l4b-good`, or `review`.
