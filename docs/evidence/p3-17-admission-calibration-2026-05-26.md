# P3-17 Admission Calibration

Date: 2026-05-26 Asia/Shanghai

Status: `DONE_WITH_WINDOWS_LOCAL_PROTECTION_DEFAULTS`

## Target

P3-R8 calibrates admission-on protection after the P3-R7 downstream ceiling sweep.

Goal:

- admission enabled;
- stable controlled 429 responses with `Retry-After`;
- no load-generator ceiling;
- downstream PostgreSQL/outbox remains below collapse;
- limits are explicitly local/protective, not public capacity claims.

## Changes

`tests/load/p3-admission-calibration.js` adds an open-model auction-wide admission workload:

- `constant-arrival-rate`;
- multi-user bidder pool to avoid measuring only one user limiter;
- counters for accepted, business rejected, `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, controlled rejection, `Retry-After`, and HTTP errors.

`tests/load/run-p3-local-stress.mjs` can run `p3-admission-calibration`.

Default local admission protection was changed to the calibrated conservative profile:

```text
BID_AUCTION_LIMIT_PER_SECOND=80
BID_AUCTION_MAX_IN_FLIGHT=32
```

`.env.example` now documents the local admission knobs. These are Windows-local release-track protection defaults and must be recalibrated before publishing capacity claims.

## Commands

Single-user limiter sanity:

```powershell
$env:P3_PROFILE='admission-on'
$env:P3_ARTIFACT_MODE='full'
$env:MANAGE_SERVER='1'
$env:WORKLOADS='bid-abuse'
$env:VUS='12'
$env:DURATION='30s'
$env:SLEEP_SECONDS='0'
$env:BID_USER_LIMIT_PER_SECOND='2'
$env:BID_IP_LIMIT_PER_SECOND='20'
$env:BID_AUCTION_LIMIT_PER_SECOND='80'
$env:BID_AUCTION_MAX_IN_FLIGHT='32'
$env:RAW_ROOT='docs/perf/raw/p3-r8-admission-user-limit-20260526-01'
pnpm exec node tests/load/run-p3-local-stress.mjs
```

Auction-wide explicit calibration:

```powershell
$env:P3_PROFILE='admission-on'
$env:P3_ARTIFACT_MODE='full'
$env:MANAGE_SERVER='1'
$env:WORKLOADS='p3-admission-calibration'
$env:RATE='120'
$env:DURATION='45s'
$env:PRE_ALLOCATED_VUS='160'
$env:MAX_VUS='500'
$env:USERS='512'
$env:BID_USER_LIMIT_PER_SECOND='20'
$env:BID_IP_LIMIT_PER_SECOND='10000'
$env:BID_AUCTION_LIMIT_PER_SECOND='80'
$env:BID_AUCTION_MAX_IN_FLIGHT='32'
$env:POST_WORKLOAD_OBSERVE_SECONDS='15'
$env:RAW_ROOT='docs/perf/raw/p3-r8-admission-auction-limit-120-20260526-01'
pnpm exec node tests/load/run-p3-local-stress.mjs
```

Default-profile verification:

```powershell
$env:P3_PROFILE='admission-on'
$env:P3_ARTIFACT_MODE='full'
$env:MANAGE_SERVER='1'
$env:WORKLOADS='p3-admission-calibration'
$env:RATE='120'
$env:DURATION='45s'
$env:PRE_ALLOCATED_VUS='160'
$env:MAX_VUS='500'
$env:USERS='512'
$env:BID_USER_LIMIT_PER_SECOND='20'
$env:BID_IP_LIMIT_PER_SECOND='10000'
Remove-Item Env:\BID_AUCTION_LIMIT_PER_SECOND -ErrorAction SilentlyContinue
Remove-Item Env:\BID_AUCTION_MAX_IN_FLIGHT -ErrorAction SilentlyContinue
$env:POST_WORKLOAD_OBSERVE_SECONDS='15'
$env:RAW_ROOT='docs/perf/raw/p3-r8-admission-default-auction-120-20260526-02'
pnpm exec node tests/load/run-p3-local-stress.mjs
```

Verification:

```powershell
go test -p 1 -count=1 ./internal/config ./internal/gateway
pnpm exec node tests/load/validate-k6-suite.mjs
```

## Raw Artifacts

| Run | Raw path | Purpose |
|---|---|---|
| Single-user limiter | `docs/perf/raw/p3-r8-admission-user-limit-20260526-01/` | Prove `RATE_LIMITED`/429/`Retry-After` works without downstream pressure. |
| Auction-wide explicit | `docs/perf/raw/p3-r8-admission-auction-limit-120-20260526-01/` | Prove auction admission protects at 120 offered rps with explicit 80/s and 32 in-flight limits. |
| Auction-wide defaults | `docs/perf/raw/p3-r8-admission-default-auction-120-20260526-02/` | Prove new default config exports and behaves as intended after fixing k6 custom counter attribution. |

## Results

| Profile | Admission | Iterations | Dropped | Controlled 429 | Retry-After | p99 ms | Downstream state |
|---|---:|---:|---:|---:|---:|---:|---|
| single-user 12 VU / 30s | on | 33756 | 0 | 33695 `RATE_LIMITED` | 33695 | 12.041 | outbox stayed published-only; lock wait sum about `0.161s`. |
| auction explicit 120 rps / 45s | on | 5400 | 0 | 1572 `BID_AUCTION_TOO_HOT` | 1572 | 171.629 | outbox drained by post-observe; lock wait sum about `25.788s`. |
| auction default 120 rps / 45s | on | 5400 | 0 | 1572 `BID_AUCTION_TOO_HOT` | 1572 | 234.234 | outbox drained by post-observe; lock wait sum about `29.699s`. |

The default-profile metrics exported:

```text
auction_admission_enabled 1
auction_admission_config_limit{kind="bid_auction_per_second"} 80
auction_admission_config_limit{kind="bid_auction_max_in_flight"} 32
```

## Attribution

Primary result:

- Admission calibration is effective for local release-track protection.
- Offered 120 rps stayed below the P3-R7 downstream collapse because the auction limiter rejected about 35 rps with controlled `BID_AUCTION_TOO_HOT`/429 and `Retry-After`.

Ruled out:

- k6/environment ceiling: no dropped iterations, no VU ceiling, no socket errors.
- downstream collapse: no sustained outbox backlog after observe, DB lock wait stayed far below R7 pressure.
- hidden admission-off mistake: metrics showed `auction_admission_enabled 1/1`.

## Review

The workload measures protection behavior, not downstream capacity.

The default limits are conservative local defaults. They are not capacity claims and must not be used in README/demo/slides as public throughput numbers.

## Verdict

`DONE_WITH_WINDOWS_LOCAL_PROTECTION_DEFAULTS`

P3-R8 closes for the current environment. Next useful work is P4 invariant verification and final Linux/native capacity calibration if public performance numbers are needed.
