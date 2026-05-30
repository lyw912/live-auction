# PTS Workload Manifest

> Status: current execution manifest, 2026-05-31.
> Read `docs/current/performance-correctness-contract.md` before running or interpreting any workload here.

This manifest separates current PTS-1B workloads from historical scripts. A file existing in `tests/pts/` does not mean it is current evidence.

Historical scripts are indexed in `tests/pts/HISTORICAL.md`. Old reset scripts
are opt-in only and are not current execution paths.

## Current Workloads

| ID | Purpose | JMX | Current use |
|---|---|---|---|
| `PTS-1B` | Final-second contention burst, 1000 users, one hot auction | `tests/pts/pts-1b-contention-burst-1000vu-1m.jmx` | Primary current performance/correctness workload |
| `PTS-1A` | Ordered accepted ladder, 1000 users | `tests/pts/pts-1a-accepted-ladder-1000vu-1m.jmx` | Control workload for accepted hot-path latency; not a replacement for contention |
| `SMOKE` | PTS connectivity and basic business chain | `tests/pts/live-auction-pts-smoke.jmx`, `tests/pts/live-auction-pts-business-smoke.jmx` | Harness validation only |

## Current Required Data

| File | Purpose |
|---|---|
| `docs/perf/pts/pts-1ab-1000vu-sessions.csv` | Current 1000-user PTS session pool |
| `tests/pts/pts_sessions.csv.example` | Example CSV shape only |

## Current Runtime Profile

PTS-1B must use the Redis/Kafka hot-engine profile:

```text
BID_ENGINE_MODE=redis_ledger
ADMISSION_ENABLED=false
REDIS_ADDR=localhost:6380
KAFKA_BROKERS=localhost:9092
```

Use `docs/current/runtime-profiles.md` for profile semantics. `.env.example` is not the PTS-1B profile.

## Current Run Sequence

Use these as the default current sequence:

```bash
L4B_PROFILE=pts-1b SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
BASE_URL=http://127.0.0.1:18080 bash tests/pts/preflight-l4b-pts-guards.sh before-<run-label>
# Run PTS with tests/pts/pts-1b-contention-burst-1000vu-1m.jmx and docs/perf/pts/pts-1ab-1000vu-sessions.csv.
BASE_URL=http://127.0.0.1:18080 bash tests/pts/collect-server-evidence.sh <report-id-or-label>
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh <report-id-or-label>
```

These scripts write new raw output to `docs/perf/pts/evidence/incoming/<label>`
unless `EVIDENCE_ROOT` is explicitly set. Move reviewed runs into
`current/` or `archive/*/` after classification.

When testing `PTS-1A`, use `L4B_PROFILE=pts-1a`. Do not cite `PTS-1A` as contention proof.

New PTS report reviews must use `docs/current/pts-run-review-template.md`.

## Current Utility Scripts

| Script | Current role |
|---|---|
| `tests/pts/reset-l4b-final-second-pressure.sh` | Current reset/seed for PTS-1A/PTS-1B |
| `tests/pts/preflight-l4b-pts-guards.sh` | Current preflight gate for Redis/Kafka/settlement protections |
| `tests/pts/collect-server-evidence.sh` | Current post-run server evidence collector |
| `tests/pts/verify-l4b-pts-correctness.sh` | Current correctness verifier |
| `tests/pts/fetch-pts-sampling-logs.sh` | Optional PTS sampling-log helper |
| `tests/pts/summarize-pts-sampling-logs.sh` | Optional sampling-log summarizer |
| `tests/pts/prepare-cloud-pressure.sh` | Shared seed/session helper used by reset scripts |

## Historical Or Bounded Workloads

These files are non-current. Do not use them for current PTS-1B success unless a current doc explicitly promotes and revalidates them:

| File | Classification | Note |
|---|---|---|
| `tests/pts/archive/current-adjacent/live-auction-l4b-final-burst-1000vu-1m.jmx` | `CURRENT_ADJACENT`/historical | Older full-check L4B final burst |
| `tests/pts/archive/current-adjacent/live-auction-l4b-final-burst-1000vu-2m.jmx` | `CURRENT_ADJACENT`/historical | Longer older L4B final burst |
| `tests/pts/archive/current-adjacent/live-auction-l4b-final-burst-accepted-1000vu-1m.jmx` | `CURRENT_ADJACENT`/historical | Older accepted profile |
| `tests/pts/archive/current-adjacent/live-auction-l4b-final-burst-bidonly-1000vu-1m.jmx` | `CURRENT_ADJACENT`/historical | Older bid-only profile |
| `tests/pts/archive/historical/live-auction-l4b-final-second-1000vu.jmx` | `HISTORICAL` | Legacy final-second profile |
| `tests/pts/archive/historical/live-auction-hotspot-pressure.jmx` | `HISTORICAL` | PG-lane/early hotspot pressure |
| `tests/pts/archive/historical/live-auction-accepted-pressure.jmx` | `HISTORICAL` | Earlier accepted pressure |
| `tests/pts/archive/historical/live-auction-core-pressure.jmx` | `HISTORICAL` | Earlier core pressure |
| `tests/pts/archive/historical/reset-pressure-data.sh` | `HISTORICAL` | Legacy generic reset |
| `tests/pts/archive/historical/reset-hotspot-pressure-data.sh` | `HISTORICAL` | Legacy hotspot reset |
| `tests/pts/archive/historical/reset-accepted-pressure-data.sh` | `HISTORICAL` | Legacy accepted reset |

The historical reset scripts above require `ALLOW_HISTORICAL_PTS=1`. If that
variable is missing, they should fail before mutating pressure data.

## Interpretation Rules

- HTTP `200` count is not accepted-bid count.
- Use business result fields: `ENGINE_ACCEPTED`, `ENGINE_REJECTED`, `ENGINE_SOLD`, `ENGINE_PAUSED`, `RECONCILING`, and `PROCESSING_RETRY_LATER`.
- Dominant `PROCESSING_RETRY_LATER`, vague `409`, or second-level pending states fail current PTS-1B UX even if settlement later converges.
- A PTS latency number without verifier output is not current success evidence.
- A PTS report review that omits runtime profile, `ENGINE_*` distribution, settlement status, verifier output, or fault-injection scope is incomplete.
