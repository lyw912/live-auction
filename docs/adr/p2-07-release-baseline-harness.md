# ADR P2-07 · Release Baseline Harness

Date: 2026-05-24 Asia/Shanghai

Status: Accepted

## Context

P2-07 requires Linux native 3-run evidence before any QPS, p99, fanout, online-user, or capacity claim. The current development environment is Windows, so producing final numbers here would violate `docs/design-v2-industrial/09-performance-and-benchmark.md`.

Existing P1 k6 scripts cover the core single-room workloads, but P2 also needs multi-room isolation and a repeatable runner that captures raw outputs and environment metadata.

## Decision

- Add `tests/load/multi-room-isolation.js` for hot-room bid traffic plus cold-room WebSocket observation.
- Extend `p1loadseed` so `room_main/auc_live` and `room_side/auc_side` are both active and all k6 users have memberships in both rooms.
- Add `tests/load/run-p2-linux-baseline.mjs` as the P2-07 execution harness.
- The harness supports smoke mode for local script validation, but final mode refuses non-Linux hosts and refuses `ulimit -n < 65535`.
- Final mode writes environment metadata, raw k6 JSON summaries, per-run logs, and a report draft under `docs/perf/`.

## Consequences

- The repository is ready for a real Linux P2-07 run without inventing local capacity numbers.
- P2-07 remains unclaimed until the Linux host runs `node tests/load/run-p2-linux-baseline.mjs --final` successfully and the raw outputs are reviewed.
- The baseline runner is intentionally stricter than ad hoc k6 commands because performance evidence is now part of release discipline.

## Follow-Up Gates

- Run final mode on Linux native with PostgreSQL, Redis, backend, and k6 boundaries documented.
- Review raw outputs for success criteria, errors, system metrics, DB/Redis/WS/runtime bottlenecks, and update the generated report.
- Only then decide whether P2-07 can move from `READY_FOR_LINUX_RUN` to `DONE`.
