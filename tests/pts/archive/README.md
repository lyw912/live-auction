# Archived PTS Workloads

> Status: archived script/JMX area, 2026-05-31.

This directory contains old PTS workloads that are kept only for audit,
diagnosis, or rerunning a historical investigation. It is not a current
execution surface.

Use `tests/pts/MANIFEST.md` for current PTS-1A/PTS-1B workloads.

## Layout

| Path | Meaning |
|---|---|
| `current-adjacent/` | Older Redis/Kafka L4B workloads that resemble the current route but are not the active manifest |
| `historical/` | PG-lane, Redis-guard, core-pressure, and earlier hotspot workloads |

Archived reset scripts require `ALLOW_HISTORICAL_PTS=1`. Any output from these
files must be labeled `CURRENT_ADJACENT`, `HISTORICAL`, or `HARNESS_ONLY`, never
`CURRENT_PASS`, unless a future current manifest explicitly promotes it.
