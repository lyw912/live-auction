# Local Raw Performance Artifacts

This directory is intentionally git-ignored except for this README.

P3/P4/P5 local stress runs can create large transient artifacts: k6 summaries, Prometheus snapshots, DB snapshots, logs, and temporary binaries. Do not commit raw run directories by default.

Default retention:

- keep `analysis-compact.json`, `analysis-compact.md`, `summary.json`, `environment.json`, per-workload k6 `*.json`, and before/after metrics for the current investigation;
- keep full logs, during-sample metrics, DB snapshots, and readyz dumps only for failed runs or when `P3_ARTIFACT_MODE=full`;
- never keep generated `*.exe` files under `docs/perf/raw`;
- promote only reviewed, small evidence summaries to `docs/evidence/` or a named report under `docs/perf/`.

For low-token analysis, start with:

```powershell
pnpm exec node tests/load/analyze-p3-artifacts.mjs
```

Read raw files only after the compact index points to a specific workload and candidate bottleneck.
