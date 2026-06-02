# Fault Test Matrix

> Status: planning matrix, 2026-06-02. This document separates S4 core fault
> tests from S5/UX, fanout, multi-room, abuse, and infra follow-ups.

## External Basis

- Grafana k6 `constant-vus` is a closed-loop executor: fixed VUs iterate for a
  duration. A `sleep(1s)` pacing models active user cadence, while removing it
  turns the workload into a synthetic backlog generator.
  <https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-vus/>
- AWS Fault Injection Service planning requires steady-state behavior and stop
  conditions in business or technical metrics before injecting faults.
  <https://docs.aws.amazon.com/fis/latest/userguide/getting-started-planning.html>
- AWS Well-Architected defines RTO as the maximum acceptable service restoration
  delay selected from business impact, not a tool default.
  <https://docs.aws.amazon.com/wellarchitected/latest/reliability-pillar/disaster-recovery-dr-objectives.html>
- Kafka batch and offset-commit tuning can improve drain throughput, but it
  changes the at-least-once replay window and must be tested as a production
  code change, not hidden in the test runner.
  <https://docs.confluent.io/kafka/design/efficient-design.html>

## RTO Policy

For this project, a live auction is an interactive, deadline-sensitive workflow:
users are watching a price ladder and reacting inside seconds. A ten-minute
backlog drain is unacceptable UX even if it is eventually correct. The current
judge-facing RTO scale is therefore:

| Band | Meaning | Use |
|---|---|---|
| `<=10s` | excellent | user sees a short transient interruption; suitable headline for Redis/PG/backend-ready paths |
| `<=30s` | acceptable | user-visible recovery is still bounded in a live session; suitable for Kafka replay on one local machine |
| `<=45s` | hard local ceiling | allows Docker single-container restart and single-worker replay overhead; not a product ambition |
| `>45s` | fail for judge-facing RTO | keep only as backlog-drain evidence, not UX recovery evidence |

The target is intentionally stricter than generic managed-database failover
windows because this is an in-room bidding interaction, not a background
reporting workload. It is also honest about the local machine: one Kafka broker
restarted by Docker is slower than a production rolling broker replacement or a
network-partition chaos test through a proxy.

## Current S4 Core Scope

S4-core proves the bid decision pipeline fails closed or recovers correctly while
200 active bidders keep bidding with 1s pacing.

| Layer | Current scale | Faults | Must prove | Current status |
|---|---:|---|---|---|
| `S4-core` | 200 VU, 25s, 1s pacing, 5s fault window | Redis SIGKILL, Kafka SIGKILL, Redis+Kafka, Redis FLUSHALL, PostgreSQL SIGKILL, backend crash | fault reached clients; no phantom accepts; Redis pending/Kafka relay/PG settlement/outbox converge; final invariants pass | implemented and passing |

Do not add WebSocket fanout, multi-room traffic, malicious clients, or
multi-broker infrastructure failover into S4-core. They are different proof
questions and would make failures harder to attribute.

## Future Fault Layers

| Layer | Question | Suggested workload | Faults | Pass signals | Local limitation |
|---|---|---|---|---|---|
| `S4-1000` | Does the same core fault proof hold under peak bid concurrency? | 1000 bid VU, short duration, paced or bounded arrival; no WS/read background | same six S4-core faults | all S4-core P0 gates; RTO reported separately; hot-path p99 is diagnostic, not SLA during fault | may consume the whole single machine; run only after S1 pass |
| `S4-WS` | Do WebSocket clients recover/fan out after decision-path faults? | 100-500 bid VU plus 500-2000 WS viewers; no multi-room | backend crash, Redis reconnect, Kafka lag, outbox relay pause | reconnect success, no seq gaps after replay, bounded fanout lag, slow-client close reasons visible | local browser/WS FD limits cap scale |
| `S4-multi-room` | Does one hot room fault or backlog isolate from other rooms? | 1 hot room plus 5-20 side rooms, lower per-room VU | Kafka lag, outbox backlog, Redis pause for one auction, backend restart | side-room p99 and WS lag do not collapse; no cross-room auction/seq leakage | single Redis/Kafka means infra faults affect all rooms; use logical pause/backlog for isolation proof |
| `UX-F` | What does a real user see during weak network or recovery? | Playwright/H5 mobile flows with throttling and reconnect | offline, slow 3G, WS reconnect, snapshot gap, backend restart | visible state transitions are explicit; CTA disabled/enabled correctly; stale price/winner not shown as final truth | not a capacity test |
| `ABUSE-F` | Can hostile clients exploit faults or replay windows? | focused integration/fuzz scripts, low VU | duplicate idempotency with different body, forged room/auction, stale client_seen_seq, fake winner/current price, repeated pay/confirm | explicit rejection; no state mutation; no hidden accepted bid; audit evidence | should be automated in backend tests first |
| `INFRA-F` | How does real clustered infra fail over? | staging or multi-container lab | Kafka broker loss, Redis Sentinel/Cluster failover, PostgreSQL primary failover | client reconnect, no data loss beyond RPO, bounded RTO by component, no duplicate settlement | not fully provable on the current single-machine single-broker stack |

## Coverage Notes

- Slow consumers, weak network, mobile UI recovery, and reconnect storms belong
  in `S4-WS`, `S5`, or `UX-F`, not S4-core. They need fewer bid VUs but more protocol
  state assertions.
- Multi-room isolation belongs in `S4-multi-room`. Use logical per-auction
  pauses/backlogs locally; reserve Redis/Kafka process death for global infra
  failure.
- Malicious request combinations belong in `ABUSE-F`; they are correctness and
  security tests, not load tests.
- Kafka multi-broker, Redis Sentinel/Cluster, and PostgreSQL primary failover
  belong in `INFRA-F`. On this local stack, document the gap instead of claiming
  clustered HA.

## Current Optimization Position

The latest S4-core breakdown shows:

- Redis/PG/backend readiness paths can produce a first post-fault decision in
  sub-second to low-second time.
- Kafka and correlated Redis+Kafka are dominated by local Docker Kafka restart
  (`16-17s`) plus single-worker settlement replay/drain (`20-25s` under the
  200 VU rto profile).
- Runner overhead was reduced by removing the rto-profile fixed recovery grace,
  lowering convergence polling to 1s, and keeping P0 convergence gates intact.

Further RTO optimization should target measured production code paths:

- settlement consumer batching or commit batching, with duplicate/replay tests;
- multiple settlement workers only if partitioning preserves auction order;
- network-partition fault injection with Toxiproxy so Kafka does not require a
  cold Docker broker restart;
- production-like Kafka multi-broker and Redis/PG failover labs.

Do not optimize by lowering correctness gates or ignoring Kafka lag, Redis
pending decisions, open outbox, or unsettled rows.
