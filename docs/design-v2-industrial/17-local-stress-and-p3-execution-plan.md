# 17 · Local Stress And P3 Execution Plan

> Date: 2026-05-24 Asia/Shanghai  
> Status: accepted execution plan for Windows-first development and P3 scale work.

## Why This Exists

The current Windows test suite being mostly green means the correctness and smoke layers are healthy. It does not mean the system has no bottlenecks.

Use the `live-auction-v2-stress-attacker` skill for this work. It is the adversarial execution role for pressure rounds, bottleneck attribution, multi-round retesting, and quantitative before/after analysis.

That is not a contradiction. It means the previous gates were scoped to prevent functional regressions:

- backend tests prove auction invariants, idempotency, ACL, payment callback behavior, diagnostics, and degradation cases;
- route-mocked Playwright tests prove UI state contracts and browser edge states;
- live H5/PC smoke tests prove selected browser-to-backend flows;
- k6 smoke scripts prove the workload scripts, seed data, auth/membership assumptions, and basic metrics can run.

They do not automatically prove sustained hot-row contention, WebSocket fanout limits, outbox table pressure, memory growth, or final capacity. Most existing k6 scripts use short `constant-vus` smoke settings, and many frontend tests intentionally mock routes so the UI can be forced into rare states. Those choices are valid for regression testing, but insufficient for bottleneck discovery.

P3 must fix that gap. P3 is not "add distributed systems because it sounds advanced." P3 is the stage where the project repeatedly pushes the real backend until a specific subsystem bends, records why, then chooses the smallest architecture change that preserves correctness.

## Testing Layers

| Layer | Frequency | Environment | Purpose | Allowed Evidence | Not Allowed |
|---|---|---|---|---|---|
| PR smoke | every meaningful change | Windows local | catch correctness regressions, script drift, seed mistakes | Go tests, Playwright route-contract tests, live smoke, short k6 summaries | capacity claims |
| Local stress loop | on demand, before each P3 milestone, and after bid/outbox/realtime changes | Windows local | force bottleneck direction and regression trends | raw k6 output, Prometheus snapshots, DB/Redis/runtime diagnostics, invariant checker output | final p99/p999 or user-count claims |
| Bottleneck drilldown | when local stress shows saturation or growth | Windows local, focused workload | prove root cause: PG lock, outbox, WS fanout, Redis, runtime, UI | pg lock snapshots, explain/analyze, pprof, heap/goroutine deltas, outbox lag, WS queue metrics | redesign without evidence |
| Final capacity calibration | P5 only | Linux native or documented equivalent | publish capacity number if evidence supports it | 3 raw Linux runs per workload plus reviewed report | Windows-derived capacity number |

## Admission Policy During Performance Exploration

P3/P4/P5 performance exploration must disable admission, not raise ceilings.

Use a single explicit switch:

```text
ADMISSION_ENABLED=false
```

When this switch is false, the backend must bypass all admission paths:

- bid user/IP/auction Redis rate limits;
- bid local auction in-flight semaphore;
- WebSocket ticket admission;
- WebSocket connect admission.

This rule exists because the purpose of P3/P4/P5 exploration is to push pressure into PostgreSQL, Redis, outbox, realtime fanout, recovery, and runtime until the real bottleneck is visible. A guessed high limit is still a hidden ceiling. It is not acceptable performance evidence.

Every downstream-pressure run must prove admission is off:

- raw environment records `ADMISSION_ENABLED=false`;
- `/metrics` before and after includes `auction_admission_enabled 0`;
- before/after admission reject counters have zero delta;
- HTTP `429` from bid or WebSocket admission has zero delta.

If any admission rejection appears during a downstream-pressure run, the run is invalid for bottleneck attribution until rerun with admission fully disabled. It may be kept only as admission/protection evidence.

Admission is still a product feature. It is tested later as an overload-protection and weak-network/reconnect-backoff gate after capacity exploration has found the practical bottleneck and there is enough evidence to choose real limits.

## Script Authoring Attribution Rules

Every new P3 pressure script must be written with attribution in mind before the first run. A script that can only say pass or fail is not valid P3 bottleneck evidence.

Default new P3 performance scripts to downstream pressure:

- run through `tests/load/run-p3-local-stress.mjs` unless there is a documented reason not to;
- assume `P3_PROFILE=downstream-pressure`;
- keep `ADMISSION_ENABLED=false` for PG, outbox, WebSocket fanout, reconnect, Redis, and runtime pressure;
- use `P3_PROFILE=admission-on` only when the target is admission/protection behavior itself.

Each script must expose enough counters or logs to classify the first limiting factor:

- admission/protection: HTTP `429`, `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, WebSocket ticket/connect admission, retry-after, and admission counter deltas;
- environment/load generator: k6 `dropped_iterations`, max-VU exhaustion, local connect refusal, socket or ephemeral-port exhaustion, timeouts before backend saturation, and Docker/Windows resource ceilings;
- system bottleneck: PG lock/pool/transaction growth, outbox backlog/lag/retry/dead rows, WebSocket queue/fanout/slow-close growth, Redis latency, Go CPU/heap/goroutine growth, or auction invariant failure.

The P3 runner records environment signals in `summary.json` for every workload. If those signals appear, the verdict must consider `ENV_LIMIT` before making an architecture decision. If admission signals appear during a downstream-pressure run, the verdict is admission pollution or harness gap, not PG/outbox/WS bottleneck.

Raw artifact retention must stay small. The default runner mode is `P3_ARTIFACT_MODE=minimal`: keep compact analysis, environment metadata, k6 aggregate summaries, and before/after metrics; keep full logs, during-sample metrics, DB snapshots, and readyz dumps only for failed workloads. Use `P3_ARTIFACT_MODE=full` only for focused drilldown. Generated binaries must stay outside `docs/perf/raw`.

For attribution and trend comparison, read compact outputs first:

- per-run `analysis-compact.json` and `analysis-compact.md`;
- aggregate `docs/perf/raw/p3-artifact-index.json` generated by `tests/load/analyze-p3-artifacts.mjs`.

Do not load every raw file for routine analysis. Open raw k6 JSON, Prometheus snapshots, DB snapshots, or logs only when the compact report points to a specific workload and candidate bottleneck.

## Stress Attacker Execution Protocol

The stress-attacker workflow is a protocol, not an instruction to read all evidence.

1. Target one subsystem.
   Pick exactly one primary target for the round: PG hot row, outbox relay/table, WebSocket fanout, slow consumers, reconnect/snapshot, Redis, Go runtime, admission/protection, or environment.

2. Prove the harness can reach that target.
   Use real backend paths, seeded users, real room membership, real PostgreSQL, real Redis, and real WebSocket where relevant. If the current script cannot drive the target, write or adjust a focused script before drawing conclusions.

3. Keep downstream pressure free of admission.
   For PG/outbox/WS/reconnect/Redis/runtime pressure, use `P3_PROFILE=downstream-pressure` and `ADMISSION_ENABLED=false`. If admission counters or admission `429` move, the result is admission pollution or harness gap.

4. Rule out environment before architecture conclusions.
   Check k6 dropped iterations, max-VU exhaustion, connect refusal, socket/ephemeral-port exhaustion, client-side timeouts, Docker resource limits, and Windows-local connection behavior. If these move before backend bottleneck metrics move, report `ENV_LIMIT`.

5. Use a controlled load model.
   Use open-model arrival rate for sustained HTTP bid/outbox pressure so slow responses do not hide offered load. Use VU/session models for connection-count questions such as watchers, reconnect, and slow consumers.

6. Compare like with like.
   Before/after comparisons must keep the same workload, profile, admission setting, seed shape, scale, duration, and local environment. If any of these changed, label the comparison as directional only.

7. Read compact evidence first.
   Run `pnpm exec node tests/load/analyze-p3-artifacts.mjs`, read `analysis-compact.md` / `analysis-compact.json`, and inspect only the raw artifacts named by the compact report for the suspected bottleneck.

8. Escalate instrumentation only when needed.
   Stay in `P3_ARTIFACT_MODE=minimal` for smoke, regression checks, routine attribution, and before/after comparison. Switch to `P3_ARTIFACT_MODE=full` only for one focused drilldown round when compact evidence is inconclusive or points to a subsystem that needs full logs, during-sample metrics, DB snapshots, or profiles.

9. End with a falsifiable next step.
   A P3 stress result must end in one of `BOTTLENECK_FOUND`, `HARNESS_GAP`, `ENV_LIMIT`, or `NO_REGRESSION_WITH_CEILING`, plus the exact next workload or diagnostic that could disprove the attribution.

## Why Short Smoke Does Not Expose The Bottlenecks

### PG Hot Row

The auction truth path deliberately serializes money writes through PostgreSQL. Small tests prove the serialization is correct. They do not prove the hot row is acceptable under sustained late-bid traffic.

To expose the bottleneck, the test must create enough overlapping bid attempts against the same active auction that row-lock wait, transaction duration, pool wait, and `BID_RETRY_LATER` become visible. A green smoke with low VUs only says the lock path works.

### Outbox Table Pressure

The outbox design protects committed events from process crashes. Small tests prove events are written and relayed. They do not prove the table can sustain bursty claim/update cycles without lag, dead tuples, bloat, or head-of-line blocking.

To expose the bottleneck, the relay must run while accepted/rejected bid traffic keeps creating outbox rows. Evidence must include backlog, max delivery lag, retry/DEAD counts, table/index size, dead/live tuple ratio, and the claim query plan when lag grows.

### WebSocket Fanout

Route-mocked WebSocket UI tests prove browser recovery behavior. They do not allocate hundreds or thousands of real connections, exercise write loops, or reveal repeated serialization per client.

To expose the bottleneck, the test must hold real WebSocket connections open, trigger real auction events, include slow clients, and record fanout lag, write failures, slow-close counts, queue depth, goroutines, heap, and RSS.

## Mock Policy

Mocks are not banned. Undisclosed mocks are banned.

Allowed:

- route-mocked Playwright tests for deterministic UI states, rare errors, recovery gaps, and visual assertions;
- fake local payment provider, because it now uses provider IDs, signed webhook semantics, idempotent event processing, and reconciliation;
- mock auth headers only under `APP_ENV=test` or explicit `ALLOW_MOCK_AUTH=true`.

Required:

- every route-mocked UI claim must stay labeled as UI contract coverage;
- every high-risk mocked UI scenario needs at least one backend or live-smoke counterpart before final demo claims;
- P3 stress evidence must hit the real backend, real PostgreSQL, real Redis, and real WebSocket path;
- k6 may use test users or gated mock auth only when the evidence says so.
- P3/P4/P5 downstream-pressure evidence must use `ADMISSION_ENABLED=false`. Do not describe a run as downstream pressure if admission limits were merely raised.

## Cadence

### Every PR Or Work Cycle

Run the relevant correctness tests and at least one workload from `docs/perf/windows-local-strategy.md` when touching bid, outbox, realtime, recovery, room routing, payment, or admission control.

Minimum evidence:

- command;
- commit;
- Windows/local environment label;
- raw output or evidence summary;
- known limits;
- whether the run was smoke, relative comparison, or bottleneck drilldown.

### Adversarial Local Stress

Run the local stress loop whenever performance confidence matters, before each P3 milestone, and after meaningful bid/outbox/realtime/recovery changes. Do not wait for a calendar boundary. This is the recurring check that prevents "green smoke" from becoming false confidence.

Minimum adversarial set when running the full loop:

- final-second bid burst;
- outbox burst;
- watcher fanout;
- slow consumer;
- reconnect storm;
- multi-room isolation;
- bid abuse.

Each adversarial run must end with one of:

- no meaningful regression under the same local setup;
- bottleneck found and issue/ADR opened;
- test harness weakness found and fixed;
- environment limit reached and explicitly recorded.

During P3/P4/P5 performance exploration, the local stress runner must fail the run if admission is not fully disabled or if admission rejection counters increase.

### Before Each P3 Milestone

P3 work cannot start from a blank architecture preference. It must start from the latest local stress evidence:

- P3-01 realtime transport decision needs watcher fanout, slow consumer, reconnect storm, and runtime profiles.
- P3-02 relay shard ownership needs outbox burst, kill-owner/failover test, duplicate publish check, and relay lag metrics.
- P3-03 multi-room isolation needs hot/cold room load, per-room metrics, and cross-room invariant checks.
- P3-04 data path evolution needs measured PG lock, outbox, snapshot, or fanout evidence before Redis Lua, CDC, partitioning, or Centrifugo adoption is justified.

All P3 milestone bottleneck evidence must be downstream-pressure evidence with `ADMISSION_ENABLED=false`, unless the milestone explicitly says it is testing admission/protection behavior.

## Local Stress Workloads

| Workload | PR Smoke | Adversarial Stress | Drilldown Trigger | Required Diagnostics |
|---|---:|---:|---|---|
| final-second bid burst | 5-20s, low VU | 2-5 min, escalating VU or arrival rate | retry-later grows, p99 jumps, pool wait grows, accepted rate collapses | bid latency, lock wait, tx duration, pool wait, `pg_locks`/`pg_stat_activity`, invariant checker |
| outbox burst | 5-20s, low VU | 2-5 min, relay active | backlog monotonic growth, delivery lag, DEAD or retry spike | outbox lag, backlog, retries, DEAD, dead/live tuples, claim query explain |
| watcher fanout | 5-20s, low VU | 2-5 min, many real sockets | message lag, close spike, heap/goroutine growth | fanout lag, queue depth, slow close, heap, RSS, goroutine, CPU profile |
| slow consumer | 5-20s | 2-5 min, mixed healthy/slow clients | healthy clients degrade or queues grow | slow-close reason, per-client queue, hub locks, heap/goroutine delta |
| reconnect storm | 5-20s | 2-5 min, stale `last_seq` mix | DB snapshot rebuilds saturate, retry-after grows | recovery source, snapshot semaphore, DB queries, cache hit/miss |
| multi-room isolation | 5-20s | 2-5 min, one hot room plus cold room | cold room latency/fanout collapses or cross-room leak | room tags for metrics, cross-room invariant, per-room admission/fanout |
| bid abuse | 5-20s | 1-3 min with low limits | limiter distribution or fail-open behavior unclear | rate-limited/too-hot/retry-after, Redis-down anomaly, user/IP/auction counters |

The exact local numbers are intentionally not fixed in design. The run should scale until one of three things happens: a bottleneck appears, the laptop/environment limit appears, or the workload reaches a documented ceiling without regression. Record which happened.

## Open Versus Closed Load Model

Use both models deliberately:

- closed model (`constant-vus`) is useful for smoke, WebSocket sessions, and "how does the system behave with N connected clients?";
- open model (`constant-arrival-rate` or `ramping-arrival-rate`) is required when the question is "can the system sustain this request arrival rate even as responses slow down?"

For bid and outbox HTTP pressure, P3 should add arrival-rate variants so slow responses do not quietly reduce offered load. For watcher fanout, slow consumer, and reconnect scenarios, VU/session-based models remain useful because the number of live connections is itself the load.

## Evidence Bundle

Every local stress or drilldown bundle should contain:

- compact analysis report and aggregate index for routine attribution;
- raw k6 aggregate summary;
- backend `/metrics` scrape before/after by default;
- full logs, during-sample metrics, DB lock/outbox SQL snapshots, readyz dumps, and Go runtime profiles only for failed workloads or `P3_ARTIFACT_MODE=full` drilldown;
- invariant checker output for workloads that mutate auction money state;
- short verdict: `NO_REGRESSION`, `BOTTLENECK_FOUND`, `HARNESS_GAP`, or `ENV_LIMIT`.

## P3 Explained

P3 turns the project from "one backend process can demonstrate the auction" into "the realtime path can survive measured pressure and a second backend instance without lying about correctness."

In practical terms:

1. Pressure the current self hub and outbox relay until the evidence says whether they are enough.
2. If self hub fails fanout/recovery/slow-consumer gates, introduce a realtime adapter such as Centrifugo while keeping PostgreSQL, outbox, snapshot, and diagnostics in the app.
3. If a second backend can run, add relay shard ownership so only one owner claims a shard and failover is observable.
4. Prove hot/cold room isolation so one popular room does not collapse unrelated rooms.
5. Only then discuss data-path changes such as partitioned outbox, CDC, or Redis Lua reservation.

## External References Used

- k6 scenarios and executors: https://grafana.com/docs/k6/latest/using-k6/scenarios/
- k6 open vs closed models: https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/
- k6 constant arrival rate executor: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-arrival-rate/
- PostgreSQL lock monitoring and `pg_locks`: https://www.postgresql.org/docs/current/monitoring-locks.html
- Prometheus recording and alerting rules: https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/
- Go `net/http/pprof` and `runtime/metrics`: https://pkg.go.dev/net/http/pprof and https://pkg.go.dev/runtime/metrics
