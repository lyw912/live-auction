# Historical PTS Scripts

> Status: historical script index, 2026-05-31.

This file marks old JMX/reset scripts as non-current execution surfaces and
prevents accidental use as current PTS-1B evidence.

Current PTS execution starts from:

- `tests/pts/MANIFEST.md`
- `docs/design/03-runtime-profiles.md`
- `docs/design/02-performance-correctness-contract.md`
- `docs/design/04-evidence-policy.md`

## Cleanup Decision

Historical reset scripts are blocked by default. Historical JMX files have been
physically moved under `tests/pts/archive/` because Alibaba PTS scenes and old
run reviews reference their names. They are not candidate files for current
PTS-1B unless a future current manifest explicitly promotes them.

Use `tests/pts/MANIFEST.md` for all current runs.

## Historical Reset Scripts

These scripts are blocked by default. Run them only with
`ALLOW_HISTORICAL_PTS=1` and label the output as historical or harness
exploration:

```text
ALLOW_HISTORICAL_PTS=1 bash tests/pts/archive/historical/reset-pressure-data.sh
ALLOW_HISTORICAL_PTS=1 bash tests/pts/archive/historical/reset-hotspot-pressure-data.sh
ALLOW_HISTORICAL_PTS=1 bash tests/pts/archive/historical/reset-accepted-pressure-data.sh
ALLOW_HISTORICAL_PTS=1 bash tests/pts/prepare-cloud-pressure.sh
```

Do not use those commands for current PTS-1B success evidence. If a historical
script is no longer referenced by an audit trail or active investigation, delete
it instead of adding another warning layer.

## Historical JMX Files

| File | Classification | Reason |
|---|---|---|
| `tests/pts/archive/historical/live-auction-core-pressure.jmx` | `HISTORICAL` | older downstream pressure profile |
| `tests/pts/archive/historical/live-auction-hotspot-pressure.jmx` | `HISTORICAL` | PG-lane / Redis-guard hotspot exploration |
| `tests/pts/archive/historical/live-auction-accepted-pressure.jmx` | `HISTORICAL` | earlier accepted pressure |
| `tests/pts/archive/historical/live-auction-l4b-final-second-1000vu.jmx` | `HISTORICAL` | legacy final-second workload |
| `tests/pts/archive/current-adjacent/live-auction-l4b-final-burst-1000vu-1m.jmx` | `CURRENT_ADJACENT` | older L4B burst, not current primary workload |
| `tests/pts/archive/current-adjacent/live-auction-l4b-final-burst-1000vu-2m.jmx` | `CURRENT_ADJACENT` | older longer L4B burst |
| `tests/pts/archive/current-adjacent/live-auction-l4b-final-burst-accepted-1000vu-1m.jmx` | `CURRENT_ADJACENT` | older accepted profile |
| `tests/pts/archive/current-adjacent/live-auction-l4b-final-burst-bidonly-1000vu-1m.jmx` | `CURRENT_ADJACENT` | older bid-only profile |

## Current Files

Use these for current PTS-1A/PTS-1B work:

```text
tests/pts/pts-1a-accepted-ladder-1000vu-1m.jmx
tests/pts/pts-1b-contention-burst-1000vu-1m.jmx
tests/pts/reset-l4b-final-second-pressure.sh
tests/pts/preflight-l4b-pts-guards.sh
tests/pts/collect-server-evidence.sh
tests/pts/verify-l4b-pts-correctness.sh
```
