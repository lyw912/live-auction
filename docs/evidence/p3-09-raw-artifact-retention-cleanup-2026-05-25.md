# P3-09 Raw Artifact Retention Cleanup

Date: 2026-05-25 Asia/Shanghai

Status: `DONE`

## Why

`docs/perf/raw` had accumulated old full logs, during-run metrics, duplicate failed attempts, and ignored raw directories from before the P3 artifact retention policy was written. This contradicted the current policy in:

- `docs/perf/raw/README.md`;
- `docs/evidence/index.md` Raw Artifact Policy;
- `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`.

The current rule is to keep compact summaries, environment metadata, k6 aggregate JSON, and evidence-referenced diagnostics, while deleting full logs and old unreferenced raw runs.

## Retention Rules Applied

Kept:

- P1/P2 small historical raw evidence referenced by evidence/perf docs.
- `docs/perf/raw/p3-artifact-index.json`.
- Evidence-referenced P3 diagnostics:
  - `p3-00/summary.json`, `environment.json`, `metrics-after.prom`, and DB/anomaly snapshots;
  - selected files under `p3-attack-20260524-035952/` named by `p3-00-stress-attacker-round-1`;
  - selected files under `p3-outbox-claim-fix-20260524-0448/` named by `p3-01-outbox-claim-fix`;
  - shard lease and owner-kill proof files named by `p3-02-relay-shard-ownership`;
  - compact JSON/summary/environment-style files for P3-01/P3-03 referenced local stress runs.

Removed:

- 527 old raw files, including full `*.log`, `server*.log`, during-sample metrics, duplicate failed attempts, and unreferenced pre-policy run directories.
- Empty directories left after file cleanup.

## Remaining Shape

After cleanup, `docs/perf/raw` keeps only a small evidence-oriented surface:

- top-level P1/P2 raw evidence files;
- compact P3 index;
- selected P3 raw directories that are referenced by evidence docs;
- no generated executables;
- no full server/k6 log bundles from old ignored runs.

## Caveats

- Some historical evidence docs still cite old raw directory names as run identifiers. Where the directory was not retained, the evidence table remains the source of record for the summarized result.
- Windows-local raw output remains directional/regression evidence only. This cleanup does not upgrade any capacity claim.
