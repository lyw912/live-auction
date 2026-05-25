# P3-16 Final Local Ceiling Sweep

Date: 2026-05-26 Asia/Shanghai

Status: `ENV_LIMIT_WITH_BOTTLENECK_TABLE_AND_SMALL_OUTBOX_OPTIMIZATION`

## Target

P3-R7 characterizes the current Windows-local ceiling after P3-R4 hot-row optimization and P3-R5 outbox watermark batching.

This is not final capacity evidence. It is local direction/regression evidence with `ADMISSION_ENABLED=false`.

## Commands

Baseline bid ceiling:

```powershell
$env:P3_PROFILE='downstream-pressure'
$env:P3_ARTIFACT_MODE='full'
$env:MANAGE_SERVER='1'
$env:WORKLOADS='p3-bid-pressure'
$env:RATE='160'
$env:DURATION='45s'
$env:PRE_ALLOCATED_VUS='220'
$env:MAX_VUS='800'
$env:RAW_ROOT='docs/perf/raw/p3-r7-bid-ceiling-160-20260525-01'
pnpm exec node tests/load/run-p3-local-stress.mjs
```

Outbox ceiling and after-runs:

```powershell
$env:P3_PROFILE='downstream-pressure'
$env:P3_ARTIFACT_MODE='full'
$env:MANAGE_SERVER='1'
$env:WORKLOADS='outbox-burst'
$env:VUS='200'
$env:DURATION='45s'
$env:SLEEP_SECONDS='0'
$env:POST_WORKLOAD_OBSERVE_SECONDS='30'
$env:WORKLOAD_TIMEOUT_MS='180000'
pnpm exec node tests/load/run-p3-local-stress.mjs
```

Bid escalation:

```powershell
$env:P3_PROFILE='downstream-pressure'
$env:P3_ARTIFACT_MODE='full'
$env:MANAGE_SERVER='1'
$env:WORKLOADS='p3-bid-pressure'
$env:RATE='220'
$env:DURATION='45s'
$env:PRE_ALLOCATED_VUS='320'
$env:MAX_VUS='800'
$env:POST_WORKLOAD_OBSERVE_SECONDS='15'
$env:WORKLOAD_TIMEOUT_MS='180000'
$env:RAW_ROOT='docs/perf/raw/p3-r7-bid-ceiling-220-20260526-01'
pnpm exec node tests/load/run-p3-local-stress.mjs
```

Verification:

```powershell
go test -p 1 -count=1 ./internal/outbox ./internal/gateway ./internal/auction ./internal/realtime
pnpm exec node tests/load/validate-k6-suite.mjs
pnpm exec node tests/load/analyze-p3-artifacts.mjs
git diff --check
```

## Raw Artifacts

| Run | Raw path | Purpose |
|---|---|---|
| R7 bid 160 | `docs/perf/raw/p3-r7-bid-ceiling-160-20260525-01/` | Clean bid pressure baseline after earlier optimizations. |
| R7 outbox 200 before | `docs/perf/raw/p3-r7-outbox-ceiling-200-20260526-01/` | Clean outbox ceiling before this round's relay trim. |
| R7 outbox claim trim | `docs/perf/raw/p3-r7-outbox-batch-claim-trim-200-20260526-01/` | Same profile after removing per-event lease and shard-id lookup work from batch processing. |
| R7 outbox batch 256 | `docs/perf/raw/p3-r7-outbox-drain-batch-256-200-20260526-01/` | Same profile after raising default drain batch from 64 to 256. |
| R7 bid 220 | `docs/perf/raw/p3-r7-bid-ceiling-220-20260526-01/` | Escalation until k6/open-model ceiling appears. |

All downstream runs reported `auction_admission_enabled 0/0` and zero admission reject delta.

## Changes

`backend/internal/outbox/relay.go` now:

- checks/renews relay shard ownership once per `ProcessBatch` instead of once per event;
- returns `d.shard_id` from the claim query instead of issuing a second shard lookup query per event;
- raises `DefaultDrainBatch` from `64` to `256`.

The relay still claims, publishes, and marks each event serially. Same-auction head-of-line, DEAD/gap behavior, Redis projection, and watermark semantics are unchanged.

## Results

| Profile | Iterations | Dropped | Env signals | p99 ms | Outbox final pending | Post-observe published delta | Notes |
|---|---:|---:|---|---:|---:|---:|---|
| bid 160 | 7200 | 0 | none | 233.811 | not primary | not primary | Clean current bid baseline. |
| outbox 200 before | 9102 | 0 | none | 1549.434 | 4205 | about 4715 | Clean pressure; backlog remained after 30s observe. |
| outbox 200 claim trim | 8580 | 0 | none | 1679.970 | 3474 | about 5093 | Relay drain improved, request p99 remained lock-dominated. |
| outbox 200 batch 256 | 8987 | 0 | none | 1576.129 | 3176 | about 5614 | Best drain result in the same local profile. |
| bid 220 | 9260 | 640 | `k6_vu_ceiling`, `dropped_iterations` | 4491.451 | 6145 after 15s observe | about 3651 | Backend slowed enough that k6 reached 800 VUs; do not treat as clean capacity. |

## Attribution

Primary current local bottleneck:

- PostgreSQL hot auction row serialization remains the request-side ceiling.
- Evidence: R7 bid 220 reached `800` k6 VUs and dropped `640` iterations with admission disabled and no HTTP errors. DB activity samples repeatedly showed tuple/transactionid locks on the bid `SELECT ... FOR UPDATE` path. `auction_bid_lock_wait_seconds_sum` was about `843s` over `9260` bid requests.

Second-order bottleneck:

- Single app-owned relay drain remains behind sustained outbox input, but this round improved the drain path without changing architecture.
- Evidence: under the same 200 VU outbox profile, post-observe published delta improved from about `4715` to about `5614`, and final pending fell from `4205` to `3176`.

Ruled out for this round:

- Admission: every downstream run recorded admission disabled and zero admission reject delta.
- Load script envelope: HTTP error rate stayed zero and checks passed in the outbox profiles.
- Redis publish latency as primary cause: R7 bid 220 showed Redis outbox pipeline latency sum about `0.438s` over `3159` samples, far below DB lock wait.
- Self-hub fanout: these workloads did not show a realtime/fanout failure; latest clean realtime evidence remains P3-R3.

## Review

Code review against `12-engineering-rules.md`, `10-test-gates.md`, `06-realtime-and-recovery.md`, and `09-performance-and-benchmark.md` found no new correctness issue.

Remaining risk:

- `ProcessBatch` still publishes events one-by-one to preserve ordering. Parallel relay or bulk publish remains a future ADR/invariant-verifier problem, not a silent P3 optimization.
- Windows-local numbers remain direction/regression evidence only.

## Verdict

`ENV_LIMIT_WITH_BOTTLENECK_TABLE_AND_SMALL_OUTBOX_OPTIMIZATION`

The current local environment has reached a useful downstream-pressure ceiling for this release-track architecture:

- bid escalation past the clean 160 rps profile now runs into k6 VU ceiling because backend latency grows under DB row-lock pressure;
- outbox drain can be tuned, and this round tuned it, but the remaining request-side cliff is still PostgreSQL hot-row serialization;
- no evidence justifies Redis Lua reservation, Debezium/CDC, NATS/JetStream, or self-hub replacement in this P3 loop.

## Next

P3-R8 admission calibration:

- re-enable admission;
- set user/IP/auction/in-flight limits below the R7 downstream cliff;
- prove stable `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, `Retry-After`, and diagnostics without downstream backlog collapse.
