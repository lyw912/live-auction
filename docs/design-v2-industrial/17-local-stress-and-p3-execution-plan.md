# 17 · Local Stress And P3 Execution Plan

> Date: 2026-05-24 Asia/Shanghai  
> Status: accepted execution plan for Windows-first development and P3 scale work.

## Why This Exists

The current Windows test suite being mostly green means the correctness and smoke layers are healthy. It does not mean the system has no bottlenecks.

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
| Local stress loop | weekly, before each P3 milestone, and after bid/outbox/realtime changes | Windows local | force bottleneck direction and regression trends | raw k6 output, Prometheus snapshots, DB/Redis/runtime diagnostics, invariant checker output | final p99/p999 or user-count claims |
| Bottleneck drilldown | when local stress shows saturation or growth | Windows local, focused workload | prove root cause: PG lock, outbox, WS fanout, Redis, runtime, UI | pg lock snapshots, explain/analyze, pprof, heap/goroutine deltas, outbox lag, WS queue metrics | redesign without evidence |
| Final capacity calibration | P5 only | Linux native or documented equivalent | publish capacity number if evidence supports it | 3 raw Linux runs per workload plus reviewed report | Windows-derived capacity number |

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

### Weekly Local Stress

Run the local stress loop even if no feature specifically asks for it. This is the recurring check that prevents "green smoke" from becoming false confidence.

Minimum weekly set:

- final-second bid burst;
- outbox burst;
- watcher fanout;
- slow consumer;
- reconnect storm;
- multi-room isolation;
- bid abuse.

Each weekly run must end with one of:

- no meaningful regression under the same local setup;
- bottleneck found and issue/ADR opened;
- test harness weakness found and fixed;
- environment limit reached and explicitly recorded.

### Before Each P3 Milestone

P3 work cannot start from a blank architecture preference. It must start from the latest local stress evidence:

- P3-01 realtime transport decision needs watcher fanout, slow consumer, reconnect storm, and runtime profiles.
- P3-02 relay shard ownership needs outbox burst, kill-owner/failover test, duplicate publish check, and relay lag metrics.
- P3-03 multi-room isolation needs hot/cold room load, per-room metrics, and cross-room invariant checks.
- P3-04 data path evolution needs measured PG lock, outbox, snapshot, or fanout evidence before Redis Lua, CDC, partitioning, or Centrifugo adoption is justified.

## Local Stress Workloads

| Workload | PR Smoke | Weekly Stress | Drilldown Trigger | Required Diagnostics |
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

- raw k6 summary and log;
- backend `/metrics` scrape before/during/after where feasible;
- DB lock/outbox SQL snapshots when bid or outbox is involved;
- Go runtime profile or metric snapshot when fanout, memory, or goroutine growth is involved;
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
