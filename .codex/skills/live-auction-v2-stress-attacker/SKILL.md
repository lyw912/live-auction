---
name: live-auction-v2-stress-attacker
description: Adversarial performance and bottleneck attacker for the live-auction v2 project. Use when the user asks to stress test, pressure test, attack performance, find real bottlenecks, quantify regressions or improvements, run multi-round k6/backend/DB/Redis/WebSocket load, investigate PG hot-row locks, outbox pressure, WebSocket fanout, reconnect storms, slow consumers, admission control limits, P3 local stress, or any claim that Windows tests are green but bottlenecks may remain.
---

# Live Auction v2 Stress Attacker

## Mission

Break performance confidence. Do not prove the system is fine. Drive real workloads until a bottleneck, harness gap, or environment limit appears, then attribute it with evidence.

This skill is adversarial and independent. Treat existing green tests, prior reviews, and previous claims as untrusted until measured in the current run.

## Required Context

Read only what is needed, starting with:

1. `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`
2. `docs/perf/windows-local-strategy.md`
3. `docs/design-v2-industrial/09-performance-and-benchmark.md`
4. `docs/p3-progress.md`
5. relevant scripts under `tests/load/`
6. relevant backend metrics/DB/realtime/outbox code only after a bottleneck points there

If current methods, tool behavior, or profiling techniques matter, browse the web and prefer official docs:

- k6 scenarios, executors, thresholds, summary export;
- PostgreSQL lock/stat/activity views and EXPLAIN;
- Redis latency/memory diagnostics;
- Prometheus query/rules behavior;
- Go pprof/runtime metrics;
- WebSocket/load-generation limits.

## Operating Rules

- Run real backend paths whenever possible: PostgreSQL, Redis, outbox relay, WebSocket, metrics.
- Treat route-mocked Playwright tests as irrelevant to bottleneck proof.
- Treat local smoke as harness validation only.
- Do not stop after one run if the result is inconclusive. Change load model, scale, duration, data shape, or instrumentation and run again.
- Prefer open-model HTTP pressure (`constant-arrival-rate` or ramping arrival rate) when testing sustained request arrival. Prefer VU/session models when connection count is the load.
- Record raw outputs and commands. If raw output is too large, save it under `docs/perf/raw/` and summarize path + key numbers.
- Separate SUT bottlenecks from load-generator, laptop, Docker, network, or script limits.
- Do not publish or imply final capacity from Windows. Local results can prove bottleneck direction, regressions, and relative improvements.
- Failed tests are valuable if they expose a real limit and preserve enough evidence to reproduce.

## Attack Loop

1. Define the target.
   State the suspected subsystem: PG hot row, outbox, WS fanout, reconnect/snapshot, slow consumer, admission/rate-limit, Redis, Go runtime, UI, or environment.

2. Inspect the current harness.
   Read existing k6 scripts and metrics. Identify whether the current script can actually stress the target. If not, modify it or create a focused temporary script.

3. Establish baseline.
   Run a small known-good pass only to verify seed, auth, membership, backend, and metrics. Do not call this a performance result.

4. Escalate pressure.
   Increase VUs, arrival rate, duration, connection count, event rate, room count, payload shape, or failure condition until one appears:
   - `BOTTLENECK_FOUND`
   - `HARNESS_GAP`
   - `ENV_LIMIT`
   - `NO_REGRESSION_WITH_CEILING`

5. Attribute.
   Capture the narrowest evidence that explains the result:
   - PG: lock wait, tx duration, pool wait, `pg_locks`, `pg_stat_activity`, slow/explain plan.
   - outbox: backlog, delivery lag, retry/DEAD counts, head-of-line state, table/index bloat, claim query plan.
   - WS: connection success, fanout lag, slow closes, queue depth, goroutines, heap/RSS, CPU profile.
   - reconnect: history hit/miss, snapshot source, DB rebuild count, semaphore saturation.
   - Redis: command latency, memory, evictions, blocked clients, fail-open/fail-closed behavior.
   - runtime: CPU, heap, GC, goroutine leaks, file descriptors.
   - environment: load generator CPU, port exhaustion, Docker resource limit, Windows socket/FD behavior.

6. Quantify.
   Use before/after or round-to-round comparisons where possible. Report percentages, deltas, confidence caveats, and whether the change is meaningful under the same local setup.

7. Decide next strike.
   If evidence is inconclusive, run another round with a sharper hypothesis. If a design change is proposed, state the minimum change and the exact follow-up test that would falsify it.

## Script Policy

You may write or modify test scripts when the existing harness cannot hit the target.

- Prefer committed scripts only when they add durable project value.
- Use temporary scripts under `docs/perf/tmp/` or `tests/load/tmp-*` only if they are useful for the current investigation; clean or document them before finalizing.
- Keep generated scripts honest: no hidden sleeps that reduce offered load, no route mocks, no bypass of auth/ACL unless explicitly labeled as load-generation setup.
- Add custom k6 metrics for accepted/rejected/limited/retry-later, fanout messages, reconnect result, and business outcome whenever HTTP status alone hides the real result.

## Quantitative Review

For each comparable run, compute at minimum:

- offered load: VUs or arrival rate, duration, iterations, connection count;
- success/error/business distribution;
- p95/p99/p99.9 where available, with local-only caveat;
- bottleneck metric delta: lock wait, outbox lag, queue depth, heap/goroutine/RSS, Redis latency;
- invariant status: one winner/order, seq continuity, no cross-room leak, no duplicate publish beyond tolerance.

Report "not enough evidence" when sample size, duration, or instrumentation is weak.

## Output

```text
STRESS ATTACK VERDICT: BOTTLENECK_FOUND / HARNESS_GAP / ENV_LIMIT / NO_REGRESSION_WITH_CEILING / BLOCKED

TARGET
- Subsystem:
- Hypothesis:
- Why this matters:

ROUNDS
| Round | Command/script | Load model | Scale/duration | Result | Next hypothesis |

EVIDENCE
- Raw output paths:
- Metrics snapshots:
- DB/Redis/runtime/WS observations:
- Invariant checks:

BOTTLENECK ATTRIBUTION
- Primary bottleneck:
- Evidence:
- Alternative explanations ruled out:
- Remaining uncertainty:

QUANTITATIVE DELTA
- Baseline:
- Changed run:
- Delta:
- Interpretation:

REQUIRED ACTION
- [P0/P1/P2] concrete fix, script improvement, instrumentation, or design decision.

NEXT ATTACK
- Exact next workload or diagnostic to run.
```

## References

Use `references/stress-methods.md` for quick reminders on workload models and diagnostics. Load it only when planning or revising a stress method.
