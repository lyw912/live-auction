# 18 · P3/P4 Roadmap Reset

> Date: 2026-05-24 Asia/Shanghai  
> Status: accepted reset for the next P3/P4 execution loop.  
> Trigger: mid-project evidence review after P2 completion, outbox claim fix, relay ownership work, and realtime/multi-room local stress smoke.

## Reset Thesis

P3/P4 is not being abandoned. The execution mode is being reset.

The project is past demo-hardening: P0/P1/P2 are implemented with evidence for real sessions, room ACL, room routing, admission, payment provider semantics, diagnostics, metrics, dashboards, and local stress harnesses. The next competitive gap is not "add Redis Lua" or "add another infrastructure component" by preference. The gap is to run a disciplined performance discovery loop:

```text
disable admission
-> prove pressure reaches the target subsystem
-> escalate until bottleneck, harness gap, or environment limit appears
-> attribute with narrow evidence
-> make the smallest correctness-preserving optimization
-> rerun the same workload
-> repeat until further improvement is not justified
-> only then set production admission limits
```

This reset supersedes any P3 plan that starts by adopting a transport, CDC, Redis authority, or partitioning change without a clean downstream-pressure evidence bundle.

## Governing Files

- `docs/design-v2-industrial/00-project-brief.md`: scoring surface and two core challenges.
- `docs/design-v2-industrial/09-performance-and-benchmark.md`: benchmark discipline and final Linux claim boundary.
- `docs/design-v2-industrial/10-test-gates.md`: required workload families.
- `docs/design-v2-industrial/12-engineering-rules.md`: truth, idempotency, outbox, WebSocket, and performance non-negotiables.
- `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`: local stress protocol.
- `docs/p3-decision-log.md`: current P3 decisions and go/no-go gates.
- `docs/evidence/index.md`: authoritative vs superseded evidence index.

## Current Implementation State

| Area | Current state | Reset interpretation |
|---|---|---|
| P0/P1/P2 | Complete per `docs/p2-progress.md` and P0/P1 evidence. | Product/security shortcuts are no longer the main blocker. |
| Admission | Implemented and useful as product protection. | Must be off during performance exploration. Re-enable only after finding the practical downstream ceiling. |
| Outbox claim | First real P3 bottleneck found and fixed. Claim query improved from `1584.153ms` to `14.165ms` under matching local pressure. | Keep evidence. Do not revisit CDC/partitioning unless backlog/claim/update becomes bottleneck again. |
| PG hot row | Still visible under 300 rps open-model hot-auction pressure after outbox fix. | Primary next bottleneck candidate, but Windows absolute p99 is not final capacity. |
| Relay ownership | DB shard lease ownership and owner-kill local pressure passed. | Good correctness evidence, not final horizontal capacity proof. |
| Realtime self hub | Staggered 300 watchers / 50 trigger rps local fanout passed; synchronous 300 connect storm hit Windows/local accept behavior; healthy-vs-slow probe passed for healthy clients with upstream pressure signal. | Self-hub remains the only runtime transport. Continue clean fanout, slow-consumer, reconnect, and Linux calibration. |
| Multi-room isolation | Harness smoke passed with cross-room leak rate 0. | Not adversarial enough. Must run hot/cold room pressure before claiming isolation. |
| P4 verifier/flight recorder | Not started. | May move earlier because it reduces stress attribution cost and makes judge-facing correctness evidence stronger. |

## Reset Rules

1. `ADMISSION_ENABLED=false` is mandatory for P3/P4/P5 downstream pressure.
2. A downstream-pressure run is invalid for bottleneck attribution if bid or WebSocket admission rejects move.
3. Windows local can prove bottleneck direction, regressions, harness gaps, and environment limits. It cannot publish final user count, p99, p999, or QPS.
4. Every pressure run must classify the first limit as one of: `BOTTLENECK_FOUND`, `HARNESS_GAP`, `ENV_LIMIT`, `NO_REGRESSION_WITH_CEILING`.
5. Use open-model arrival-rate workloads for sustained HTTP bid/outbox pressure.
6. Use session/VU workloads for connection count, reconnect storm, watcher fanout, and slow-consumer questions.
7. Do not compare admission-on protection runs with admission-off capacity discovery runs.
8. Do not use client reconnect jitter, admission, or backoff to hide downstream bottlenecks during performance exploration.
9. Do not adopt Redis Lua, CDC, NATS, or outbox partitioning unless the smallest local fix has failed or the evidence points directly to that layer.
10. P4 invariant verification can start before all P3 scale choices because it improves every later stress round.

## External Research Baseline

Private TikTok Shop, Whatnot, Taobao, and eBay internals are not public. The project should use public product behavior, official infrastructure docs, mature open-source systems, and papers as design pressure, not as borrowed performance proof.

| Source | What it shows | Design implication for this project |
|---|---|---|
| TikTok Shop LIVE Shopping and LIVE Manager | Live commerce is product-set, room, showcase, operations, and post-event analytics oriented. | PC/H5 must remain room/product/operator centered, not only a bid API benchmark. |
| Whatnot live auction help | Many live auctions are short, and 30 seconds or less is normal. | Final-second bid burst is the main workload, not an edge case. |
| eBay automatic bidding | Proxy/max-bid is a credible auction model but changes semantics. | Keep proxy bidding as optional P4/P5 ADR, not hidden scope creep. |
| PostgreSQL explicit locks and `pg_locks` | Row locks are explicit, observable serialization tools. | PG row lock remains defensible money truth while lock wait is measured honestly. |
| k6 open vs closed models and constant-arrival-rate | Open-model arrival rate keeps offered load independent of SUT response time while VUs are available. | Use arrival-rate tests for hot bid/outbox saturation; treat dropped iterations as load-generator/env evidence. |
| AWS Builders Library on fairness/admission | Admission and throttling improve availability after the system's limits are understood. | Set admission limits after capacity discovery, not before bottleneck exploration. |
| Debezium outbox event router | Mature CDC-based outbox routing from a dedicated outbox table. | Candidate only if polling outbox remains a measured bottleneck after indexed claim, batching, shard ownership, and table hygiene. |
| NATS/JetStream | Mature internal messaging substrate with persistence options. | More relevant to internal service messaging than browser WebSocket recovery; not P3 default. |
| Google SRE monitoring | Monitoring should be symptom-oriented and support fast diagnosis. | Diagnostics must explain lock, outbox, fanout, reconnect, admission, and room-isolation failures. |
| Jepsen-style analyses | Correctness claims should be checked against histories and failure behavior. | P4 invariant verifier is a differentiator, not polish. |
| The Tail at Scale | Tail latency and stragglers dominate user-visible latency at scale. | Optimize p99/p999 causes, bounded queues, backpressure, and overload behavior; do not market averages. |

Infrastructure candidates are intentionally evidence-gated:

| Candidate | Usefulness | Adoption gate |
|---|---|---|
| NATS / JetStream | Internal eventing and durable streams. | Need for service-to-service messaging appears after monolith boundary is intentionally split. |
| Debezium | PostgreSQL CDC and outbox routing. | Polling outbox remains bottleneck after local fixes and Linux confirmation. |
| Redis | Rate limit state, projection/cache, optional reservation prototype. | Never becomes auction truth without reconciliation ADR. |
| Prometheus / Grafana / k6 | Existing measurement stack. | Keep as evidence baseline; improve workload and attribution quality. |

Do not cite exact GitHub star counts in final material unless rechecked on the release date.

## Performance Discovery Loop

### Phase A: Harness Sanity

Goal: prove the test can reach the subsystem and is not blocked by seed/auth/ACL/admission/env.

Required:

- real backend, PostgreSQL, Redis, outbox relay, and WebSocket path when relevant;
- `P3_PROFILE=downstream-pressure`;
- `ADMISSION_ENABLED=false`;
- before/after `/metrics` proof that `auction_admission_enabled 0`;
- zero delta for admission reject counters;
- nonzero k6 checks and iterations;
- invariant checker for workloads that mutate money state.

Exit:

- `HARNESS_READY`, or a concrete harness fix.

### Phase B: Escalation

Goal: find the first real limit.

Escalate one dimension at a time:

- HTTP bid arrival rate;
- duration;
- hot auction count;
- room count;
- watcher connection count;
- trigger event rate;
- reconnect staleness;
- slow-consumer ratio;
- relay worker/shard count.

Stop when one appears:

- backend bottleneck metric moves clearly;
- k6 dropped iterations or Windows connect refusal appears before backend saturation;
- invariants fail;
- no regression at the planned ceiling.

### Phase C: Attribution

Minimum attribution matrix:

| Suspect | Required evidence |
|---|---|
| PG hot row | lock wait, tx duration, pool wait, retry-later, active lock snapshots, invariant result. |
| Outbox | backlog, delivery lag, retry/DEAD, claim/update plan, dead/live tuple ratio, shard ownership state. |
| WebSocket fanout | fanout lag, write errors, slow closes, queue depth, goroutines, heap/RSS, CPU profile if needed. |
| Reconnect/snapshot | history hit/miss, snapshot rebuild count, semaphore saturation, DB query count, retry-after. |
| Multi-room isolation | hot vs cold latency/fanout, room-tagged metrics, cross-room invariant, global pool contention. |
| Redis | command latency, memory, evictions, blocked clients, fail-open/fail-closed behavior. |
| Runtime | CPU, heap, GC, goroutines, FDs, pprof sample. |
| Environment | k6 CPU/VU ceiling, dropped iterations, local connect refusal, Docker/Windows resource signals. |

### Phase D: Smallest Fix

Preferred optimization order:

1. Remove accidental O(n^2), repeated serialization, unbounded queues, and missing indexes.
2. Reduce transaction work while preserving one PG truth transition.
3. Add batching, sharding, lease ownership, or fairness where the measured queue demands it.
4. Add transport/CDC/data-path components only when local fixes cannot address the measured limit.
5. Document any semantic change with an ADR before implementation.

### Phase E: Rerun

Retest with the same:

- workload name;
- scale/duration;
- seed/data shape;
- admission setting;
- machine/environment;
- script SHA;
- invariant checker.

Only then report a before/after delta.

### Phase F: Set Admission

Admission limits are configured after the practical downstream ceiling is known and further optimization is not justified for the release candidate.

Admission target:

- preserve DB/outbox/WS latency below the release SLO;
- shed per-user/IP abuse before hot auction global pressure;
- shed room-scoped pressure before whole-service collapse;
- expose `Retry-After` and reason-specific error codes;
- produce diagnostics that explain what was protected.

## P3 Reset Roadmap

| Order | New milestone | Goal | Exit gate |
|---:|---|---|---|
| 1 | P3-R0 decision/evidence reset | Freeze decisions, evidence index, and clean pressure policy. | This document, `docs/p3-decision-log.md`, and `docs/evidence/index.md` exist. |
| 2 | P3-R1 admission-off harness proof | Ensure all downstream workloads can run with `ADMISSION_ENABLED=false` and fail if admission moves. | Compact report proves admission off before/after and zero admission reject delta. |
| 3 | P3-R2 hot/cold multi-room adversarial stress | Prove one hot room does not corrupt or silently starve cold rooms. | Hot/cold latency/fanout metrics, cross-room invariant, bottleneck verdict. |
| 4 | P3-R3 clean realtime fanout and slow-consumer drilldown | Validate the self-hub under fanout/slow/reconnect pressure without Windows connect-storm noise. | Clean downstream-pressure fanout, slow-consumer, reconnect, runtime profile verdict. |
| 5 | P3-R4 PG hot-row drilldown | Quantify how much of hot auction tail is row lock vs transaction work vs pool vs load-generator. | Lock/tx/pool profile and a chosen optimization or explicit "keep PG" decision. |
| 6 | P3-R5 outbox second-order pressure | Recheck outbox after claim fix and relay ownership under longer burst and multi-room traffic. | No backlog growth, or measured table/claim/update bottleneck with next fix. |
| 7 | P3-R6 architecture go/no-go | Decide CDC/partition, Redis Lua prototype, or keep current architecture. Realtime transport remains self-hub unless scope is explicitly reopened. | ADR per chosen change, with falsification workload. |
| 8 | P3-R7 final local ceiling sweep | Repeat the loop until local changes stop improving the current release candidate. | Known local bottleneck table and no hidden admission. |
| 9 | P3-R8 admission limit calibration | Re-enable admission and set product limits below observed downstream cliff. | Admission-on protection workloads prove controlled `429`/business rejects and stable downstream metrics. |

## Architecture Decision Gates

### Self Hub Runtime

Keep self hub as the only runtime transport while:

- clean fanout reaches the local/Linux planned ceiling without memory/goroutine growth;
- slow consumers are closed without healthy-client degradation;
- reconnect storm is bounded by history/snapshot controls;
- multi-instance story is honestly limited or tested through current relay ownership and deployment evidence.

Do not add alternate realtime transports because:

- synchronous Windows connect storm fails before the Go handler;
- high-star status is attractive;
- "industrial" sounds like "more infrastructure";
- a PoC exists outside the mainline.

### Polling Outbox vs CDC/Partitioning

Keep polling outbox if:

- indexed claim, batch draining, shard ownership, and table hygiene keep backlog bounded;
- outbox lag is not the first user-visible bottleneck.

Consider partitioning if:

- backlog is bounded by auction/room/shard but table/index bloat or update churn becomes measurable.

Consider Debezium CDC if:

- polling claim/update remains bottleneck after local fixes;
- the outbox schema is stable;
- ops complexity is acceptable for the release scope;
- ordering, DEAD gap semantics, and diagnostics can be preserved.

### PG Row Lock vs Redis Lua Reservation

Keep PG row lock if:

- correctness remains the top differentiator;
- measured tail comes mainly from unavoidable single-auction serialization;
- final Linux capacity is enough for the scoped release.

Consider Redis Lua reservation only if:

- clean Linux/local evidence shows PG hot-row lock is the release-blocking bottleneck;
- reconciliation ADR proves Redis loss cannot create a winner;
- cap/cancel/end races still converge to one terminal DB state;
- every reservation is auditable against bids/events/idempotency.

### Admission Strategy

Keep admission off for capacity discovery.

Turn admission on only after:

- downstream ceiling is known;
- bottlenecks have been optimized or consciously accepted;
- limits are derived from evidence with headroom;
- admission-on tests prove stable product behavior and diagnostics.

## P4 Reset Roadmap

P4 should not wait until every scale decision is finished. P4 tools make the next stress rounds cheaper and more defensible.

| Order | Milestone | Why now | Exit gate |
|---:|---|---|---|
| 1 | P4-R1 invariant verifier | Converts stress output into machine-checkable correctness evidence. | CLI verifies seq continuity, one terminal, one order, winner/price match, idempotency, no cross-room leak. |
| 2 | P4-R2 auction flight recorder | Gives reviewers and developers a forensic timeline for one contested auction. | PC diagnostic detail shows rules, bids/rejects, outbox, WS recovery, payment, anomalies. |
| 3 | P4-R3 risk simulator | Turns abuse/chaos scenarios into repeatable product-risk evidence. | Scripts output expected outcome, DB invariant result, user-visible state, metrics/anomalies. |
| 4 | P4-R4 optional proxy-bid ADR | Only after core gates are strong; proxy bidding changes domain semantics. | ADR accepts or rejects proxy bidding with separate model and tests. |

P4-R1 may be started before P3-R3 because it strengthens every performance run.

## Windows Safety Plan

To avoid local Windows lockups and false bottlenecks:

- default `P3_ARTIFACT_MODE=minimal`;
- isolate workloads with `MANAGE_SERVER=1`;
- keep initial stress windows short, then scale duration;
- use `WORKLOADS=...` for one target at a time;
- treat `connectex` refusal, k6 dropped iterations, max VU exhaustion, and zero checks as `ENV_LIMIT` or `HARNESS_GAP`;
- stagger WebSocket connections when the target is steady fanout rather than connection storm;
- run full artifact mode only for one focused drilldown;
- stop before repeated OS-level connection refusal contaminates later workloads;
- keep final capacity calibration for Linux native P5.

## Reviewer Defense Position

The defensible story after this reset is:

```text
This project does not claim TikTok-scale capacity from a Windows laptop.
It implements the hard auction correctness path, removes demo shortcuts,
uses admission as protection rather than as benchmark camouflage,
pushes real backend paths until bottlenecks appear,
optimizes only with before/after evidence,
and preserves machine-checkable auction invariants under stress.
```

That is stronger than adopting a fashionable component without proof.

## References

- TikTok Shop LIVE Shopping: https://seller-us.tiktok.com/university/essay?knowledge_id=6927759780628226
- TikTok Shop LIVE Manager: https://seller-us.tiktok.com/university/essay?knowledge_id=1195537245292331
- Whatnot live auction help: https://help.whatnot.com/hc/en-us/articles/9779931101837-Start-an-auction-during-a-show
- eBay automatic bidding: https://www.ebay.com/help/buying/bidding/automatic-bidding?id=4014
- PostgreSQL explicit locking: https://www.postgresql.org/docs/current/explicit-locking.html
- PostgreSQL `pg_locks`: https://www.postgresql.org/docs/current/view-pg-locks.html
- k6 open vs closed models: https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/
- k6 constant arrival rate: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-arrival-rate/
- AWS Builders Library fairness and admission control: https://aws.amazon.com/builders-library/fairness-in-multi-tenant-systems
- Debezium outbox event router: https://debezium.io/documentation/reference/stable/transformations/outbox-event-router.html
- Google SRE monitoring: https://sre.google/sre-book/monitoring-distributed-systems
- Go pprof: https://pkg.go.dev/net/http/pprof
- Jepsen analyses: https://jepsen.io/analyses
- The Tail at Scale: https://cacm.acm.org/research/the-tail-at-scale/
