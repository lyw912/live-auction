# P3-10 Admission-Off Harness Proof

Date: 2026-05-25 Asia/Shanghai

Status: `DONE`

## Why

P3 downstream bottleneck work must prove pressure is not being stopped by admission controls. Earlier P3 runs found useful bottleneck direction, but the reset policy requires every downstream workload to show:

- `ADMISSION_ENABLED=false`;
- `auction_admission_enabled 0` before and after each workload;
- zero admission rejection counter delta;
- nonzero workload checks on real backend paths.

This evidence closes P3-R1. It does not claim capacity.

## Harness Changes

Updated `tests/load/run-p3-local-stress.mjs`:

- records structured `admission_proof` in `analysis-compact.json`;
- includes enabled before/after and reject deltas in `analysis-compact.md`;
- fails downstream-pressure workloads if admission is enabled or any admission rejection counter moves;
- supports current k6 summary-export shape where metric values may be direct metric fields rather than nested under `values`;
- computes check rates from `passes`/`fails` when k6 does not emit a direct rate;
- cleans common local dev ports after managed runs on Windows.

Updated `tests/load/analyze-p3-artifacts.mjs`:

- carries `admission_proof` into `p3-artifact-index.json`;
- classifies admission pollution as `HARNESS_GAP`;
- points follow-up readers to admission proof before raw logs.

Updated `tests/load/bid-abuse.js`:

- keeps the limit-required threshold for admission-on abuse tests;
- disables that threshold when `ADMISSION_ENABLED=false`, because downstream-pressure runs intentionally bypass admission to reach PG/outbox/realtime.

## Run

Command profile:

```text
P3_PROFILE=downstream-pressure
ADMISSION_ENABLED=false
P3_ARTIFACT_MODE=minimal
MANAGE_SERVER=1
ISOLATE_WORKLOADS=1
RAW_ROOT=docs/perf/raw/p3-r1-admission-proof-20260525-03
VUS=3
DURATION=5s
WORKLOAD_TIMEOUT_MS=180000
pnpm exec node tests/load/run-p3-local-stress.mjs
```

Raw compact report:

```text
docs/perf/raw/p3-r1-admission-proof-20260525-03/analysis-compact.md
docs/perf/raw/p3-r1-admission-proof-20260525-03/analysis-compact.json
docs/perf/raw/p3-r1-admission-proof-20260525-03/environment.json
```

## Result

All committed P3 workloads passed the admission-off proof round:

| Workload | Status | Enabled before/after | Admission reject delta | Iterations | Dropped |
|---|---:|---:|---:|---:|---:|
| preflight | PASS | 0/0 | all zero | 1 | 0 |
| final-second-bid-burst | PASS | 0/0 | all zero | 231 | 0 |
| outbox-burst | PASS | 0/0 | all zero | 480 | 0 |
| watcher-fanout | PASS | 0/0 | all zero | 9 | 0 |
| slow-consumer | PASS | 0/0 | all zero | 9 | 0 |
| reconnect-storm | PASS | 0/0 | all zero | 15 | 0 |
| multi-room-isolation | PASS | 0/0 | all zero | 246 | 0 |
| bid-abuse | PASS | 0/0 | all zero | 633 | 0 |
| p3-ws-fanout-pressure | PASS | 0/0 | all zero | 26 | 0 |
| p3-slow-consumer-pressure | PASS | 0/0 | all zero | 26 | 0 |
| p3-ws-connection-storm | PASS | 0/0 | all zero | 0 script iterations, 6 checks and 3 WS sessions | 0 |
| p3-healthy-vs-slow-consumer | PASS | 0/0 | all zero | 100 | 0 |

The post-run port check showed only `TIME_WAIT` entries on `18080`; no active owner process remained on the checked local ports.

## Interpretation

Verdict: `NO_REGRESSION_OR_NEEDS_DOMAIN_METRICS` for P3-R1.

This proves the harness can run the downstream workload set without hidden admission ceilings. It does not prove multi-room isolation under adversarial load, self-hub fanout limits, PG hot-row attribution, or outbox second-order behavior. Those remain the next P3 rounds.

## Next

Start P3-R2 hot/cold multi-room adversarial stress:

- increase hot-room bid/fanout pressure while keeping cold-room watchers active;
- keep `ADMISSION_ENABLED=false`;
- capture per-room hot/cold response, fanout, and cross-room leak metrics;
- classify the result as bottleneck, harness gap, environment limit, or no regression with a local ceiling.
