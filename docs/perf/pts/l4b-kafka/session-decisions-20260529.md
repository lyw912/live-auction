# Session Decisions

Date: 2026-05-29

This note preserves the key decisions from the L4B/Kafka PTS preparation
discussion.

## Architecture Direction

- The PG-lane/Redis-guard progress is preserved on a backup branch.
- Current pressure work runs on `review/l4b-ledger`.
- Redpanda was removed from the active path; Apache Kafka is the local broker.
- The hot bid mode is `BID_ENGINE_MODE=redis_ledger`.
- Admission is disabled for downstream-pressure discovery:
  `ADMISSION_ENABLED=false`.

## Why PTS-1 Comes First

PTS-1 maps directly to the official auction challenge: many bidders act against
one hot auction near the critical moment. It is the first run because it stresses
the new Redis Lua + Kafka ledger path under the most business-relevant
contention.

PTS-2 is still required because a one-shot burst does not reveal sustained
throughput or backlog growth. PTS-2 should use RPS/step pressure or controlled
closed-model pacing to find the throughput knee.

## PTS-1 Timing Decision

The current PTS-1 is deliberately a one-shot burst validation:

- ramp to 1000 VUs early;
- hold them at a JMX barrier;
- release one bid per VU;
- keep remaining time for response and settlement observation.

The chosen `6 min / 1 min ramp / 5:30 barrier` is not a claim that the business
hammer window is 30 seconds. It is a paid-cloud validation profile that gives PTS
time to start all VUs and collect responses.

For a true last-second sniper/soft-close test, create PTS-1B:

- set auction `end_at` close to the barrier;
- open the barrier inside the final 1-5 seconds;
- keep `LoopController.loops=1`;
- assert `end_at` extension, no accepted bid after final close, and one terminal
  state.

## Looping Decision

The current JMX uses:

```text
LoopController.loops = 1
```

Each VU sends one bid after the barrier. This avoids accidentally turning PTS-1
into a 30-second sustained loop. Sustained loops belong to PTS-2.

## Correctness Gate Decision

Performance without correctness is a failed run. Every L4B PTS run needs two
layers:

1. preflight implementation/environment guard;
2. post-run invariant gate.

The P0 gate list includes:

- Redis pending decisions empty;
- Kafka/PG accepted counts match;
- settlement terminal;
- no bid/event seq gaps;
- no duplicate bid/order/engine sequence;
- `engine_epoch`/`engine_seq` monotonic;
- Kafka offset order matches engine order;
- no accepted bid after final `end_at`;
- increment grid valid;
- Redis no eviction;
- DLQ empty;
- no event payload cross-auction mismatch.

## Fault-Injection Boundary

Normal PTS cannot prove faults that are not injected. These remain required
separate workloads:

- Redis kill/restart during non-empty pending decisions;
- app crash after Redis decision before Kafka append;
- Kafka consumer rebalance under settlement lag;
- network partition/stale Redis primary;
- proxy/max-bid privacy and event-chain integrity.
