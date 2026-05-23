# ADR P3-00 · Local Stress Discipline Before Scale Changes

Date: 2026-05-24 Asia/Shanghai

Status: Accepted

## Context

The project has many green Windows tests: backend correctness tests, route-mocked Playwright UI contract tests, live browser smoke tests, and short k6 smoke scripts.

Those tests are useful, but they answer limited questions. They prove that important paths still work and that known correctness invariants are not trivially broken. They do not prove that PostgreSQL hot-row contention, outbox relay/table pressure, or WebSocket fanout can sustain heavier load.

The gap is a testing-discipline gap, not proof that prior work was fake. Earlier phases optimized for correctness and demo completeness. P3 needs a different standard: recurring pressure, measurement, attribution, and only then architecture changes.

## Decision

Adopt `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md` as the operating plan for P3.

P3 adds a P3-00 discipline gate before transport, relay, or data-path changes:

- short smoke still runs for PR-level confidence;
- weekly Windows-local stress runs intentionally push bid/outbox/realtime/recovery/multi-room workloads beyond smoke settings;
- bottleneck drilldowns must capture raw k6 output, backend metrics, DB/Redis/runtime diagnostics, and invariant results where applicable;
- route-mocked Playwright tests remain allowed only as UI contract coverage;
- final capacity numbers still require P5 Linux native 3-run evidence.

## Consequences

- A green PR smoke result no longer supports the phrase "no bottleneck found."
- P3 work cannot choose Centrifugo, relay sharding, CDC, Redis Lua reservation, or outbox partitioning from preference alone.
- A failed local stress run is useful if it identifies a bottleneck, harness gap, or environment limit.
- Windows local evidence may justify relative optimization and architecture direction, but not final capacity claims.

## Review Rule

P3 reviews must ask:

1. Which workload was pushed beyond smoke?
2. What did the real backend, PostgreSQL, Redis, and WebSocket path do?
3. Did invariants still hold?
4. Was the bottleneck attributed to PG lock, outbox, WS fanout, Redis, runtime, UI, harness, or laptop environment?
5. Is the proposed change the smallest correctness-preserving response to that evidence?

If the answer is missing, the P3 change is not ready.
