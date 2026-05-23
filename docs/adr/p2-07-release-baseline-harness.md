# ADR P2-07 · Local Baseline And Release Harness

Date: 2026-05-24 Asia/Shanghai

Status: Accepted

## Context

P2-07 originally required Linux native 3-run evidence before moving on. That is too expensive for the long development phase and would delay useful local hardening. The updated rule is: Windows local evidence is required for correctness, attack, and bottleneck-direction work; Linux native 3-run evidence is deferred to the final capacity/release gate before any QPS, p99, fanout, online-user, or capacity claim.

Existing P1 k6 scripts cover the core single-room workloads, but P2 also needs multi-room isolation and a repeatable runner that captures raw outputs and environment metadata.

## Decision

- Add `tests/load/multi-room-isolation.js` for hot-room bid traffic plus cold-room WebSocket observation.
- Extend `p1loadseed` so `room_main/auc_live` and `room_side/auc_side` are both active and all k6 users have memberships in both rooms.
- Add `tests/load/run-p2-linux-baseline.mjs` as the final capacity execution harness.
- The harness supports smoke mode for local script validation, but final mode refuses non-Linux hosts and refuses `ulimit -n < 65535`.
- Final mode writes environment metadata, raw k6 JSON summaries, per-run logs, and a report draft under `docs/perf/`.
- Add `docs/perf/windows-local-strategy.md` to define what Windows local tests can and cannot prove.

## Consequences

- P2-07 can be considered done when local harness, workloads, seed data, and documentation are in place and validated.
- Final Linux capacity evidence remains unclaimed until a Linux host runs `node tests/load/run-p2-linux-baseline.mjs --final` successfully and the raw outputs are reviewed.
- The final runner is intentionally stricter than ad hoc k6 commands because published performance evidence is release discipline.

## Follow-Up Gates

- During local development, run the relevant Windows smoke workload after bid/outbox/realtime/recovery/admission changes and record honest local evidence.
- At final release, run final mode on Linux native with PostgreSQL, Redis, backend, and k6 boundaries documented.
- Review raw outputs for success criteria, errors, system metrics, DB/Redis/WS/runtime bottlenecks, and update the generated report.
- Only after final Linux review may README/demo/slides include capacity numbers.
