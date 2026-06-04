# S4 Fault Resilience Judge Defense

> Scope: current S4 local chaos evidence from 2026-06-04. This document explains
> what the workload means, what each number means to a bidder, and what can be
> claimed to judges. It is evidence interpretation, not a new benchmark.

## 1. Honest Headline

S4 is a bounded local chaos workload: 200 active bidders in one live auction
room, each looping `snapshot -> bid -> sleep about 1s` for 25s. At T+5s the
harness injects a 5s dependency fault. This is roughly 200 bid attempts per
second while a core component fails.

Current result: S4 P0/P1 pass for Redis, backend/settlement, PostgreSQL, Kafka,
Redis FLUSHALL, and Redis+Kafka. Bidder-facing RTO gates are 2s / 14s / 2s /
12s / 2s / 12s respectively. Full restore-start-to-final convergence is 19s /
29s / 18s / 28s / 20s / 28s. RPO=0 is proven by settlement and verifier gates,
with zero admission contamination. Production HA expansion details for Kafka
RF=3/minISR=2, Redis HA failover, multi-gateway reconnect, LB/NAT idle timeout,
real mobile weak network, and cross-AZ/region failures are maintained in
[production-ha-expansion-and-judge-defense.md](production-ha-expansion-and-judge-defense.md).

The P2 Redis partial-network run now has a current pass. Earlier latency-only
and timeout-only attempts were useful diagnostics but not pass evidence because
they did not reliably trip a client-visible fail-closed signature. The accepted
P2 run uses Toxiproxy `reset_peer` plus downstream timeout and records
`ENGINE_PAUSED` responses plus full convergence.

S4 also now has two focused P0 depth gates:

| Gate | Scale / fault model | Result |
|---|---|---|
| `07-relay-backpressure` | 600 Redis engine decisions in one auction, above the 512-entry relay batch ceiling | 600 stream entries, first 512 payloads valid seq 1..512, 600 ledger messages relayed, pending hash 0, next relay 0 |
| `08-settlement-idempotency` | one SOLD Kafka ledger message delivered to settlement 3 times | 1 settlement row, 1 SETTLED row, 1 bid, 1 order, 1 outbox event/delivery, 0 duplicate deliveries |

## 2. What "25s sustained bidding" means

| Field | Value |
|---|---|
| Tool | local Docker k6 + S4 shell harness |
| Runtime profile | `L1F_PROFILE=rto` |
| Scale | `K6_VUS=200` closed-loop active bidders |
| Per-user loop | read snapshot, compute a valid bid from current price, post `/bids`, sleep `SLEEP_MS=1000` |
| Duration | `K6_DURATION=25s` |
| Fault window | `FAULT_WINDOW_SECONDS=5`, injected after the initial ramp |
| Business meaning | 200 real active bidders keep trying to bid while one core dependency fails |

It is not "200 users total with one request each". It is 200 concurrent active
bidder loops. Because each VU sleeps about 1s after each attempt, the intended
pressure is about 200 bid attempts per second, minus normal request/runtime
overhead.

It is also not a final capacity ceiling. S4 is a resilience test: it asks whether
the system is safe and recovers under fault. S1/S2/S3 carry the throughput and
fanout capacity claims.

## 3. What the metrics mean to a bidder

| Metric | Meaning in the auction room |
|---|---|
| `decided` | the bidder got a final engine result, such as accepted, rejected, or sold |
| `paused` | Redis engine was unavailable or intentionally paused; the UI should tell the bidder to retry shortly |
| `reconciling` | Redis hot state was lost or incomplete; the system is rebuilding truth before accepting new bids |
| `http_errors` | backend process was down or connection failed; the bidder's request attempt failed |
| `accepted-in-window=0` | during a Redis truth-engine fault, no fake accepted bid was created |
| `settlements N/N` | every durable engine decision converged into settlement rows exactly once |
| `admission contamination=0` | the test was not accidentally rate-limited; failures are from the injected fault |
| `RTO gate` | user-visible recovery gate: new bid attempts are safe again, or paused/reconciling stops after Redis faults |
| `restore-start-to-final convergence` | back-office safety boundary: Redis pending, Kafka lag, settlement, and outbox have drained |

`1200 HTTP errors` in the backend/settlement crash run means 1200 request
attempts failed while 200 VUs were looping. It does not mean 1200 distinct users
failed. In user terms, the same 200 active bidders saw failed submit attempts
until the backend restarted.

For S4 07, `600 decisions` means 600 accepted/rejected/sold engine decisions
already written into the Redis decision stream for one auction. It is not 600
WebSocket viewers and not 600 users watching silently. The business question is:
if a final-second burst or Kafka relay stall creates more durable decisions than
one relay tick can append, does the async back-office chain drain without losing
or duplicating decisions?

For S4 08, `3x delivery` means the same Kafka ledger message reaches the
settlement worker three times. This models at-least-once replay after crash,
offset commit failure, network retry, or consumer rebalance. The user-facing
question is: does the winner get one order/payment opportunity, or can replay
create a second order/charge/outbox notification?

## 4. P0 faults as real business incidents

### Redis SIGKILL

Real incident: Redis hot engine process dies during a live auction. Redis is the
single-writer truth path for deciding price, winner, sequence, and idempotency.

Expected user view: bids must not be accepted while Redis truth is unavailable.
Users see a short "auction engine recovering, retry shortly" state rather than
a fake accepted bid.

Current evidence:

| Field | Value |
|---|---|
| Label | `pts-1c-redis-20260604T203439` |
| Run scale | 200 active bidders, 25s, about 1 attempt/s each, 5s Redis fault |
| Overall client results | 3847 decided, 600 paused, 0 HTTP errors |
| Fault-window assertion | decided=0, paused=400, accepted-in-window=0 |
| Recovery | RTO gate 2s; restore-start-to-final convergence 19s |
| RPO proof | settlements 3847/3847, verifier PASS |

Judge wording:

> "Redis is the authoritative hot decision engine. During the 5s Redis SIGKILL
> window we returned zero accepted decisions and 400 paused responses, so bidders
> saw safe retry feedback instead of fabricated winners. After Redis restored,
> the user-visible RTO gate was 2s and all 3847 durable decisions settled exactly
> once."

### Backend / settlement crash

Real incident: the backend container is killed mid-auction. In this local
process model, the settlement worker lives in the same Go process, so the fault
models a process-level crash that can interrupt Kafka consumption before offset
commit.

Expected user view: while the backend is down, submit attempts fail. After
restart, users can bid again. Previously durable Kafka decisions must replay
without duplicate settlement or double charge.

Current evidence:

| Field | Value |
|---|---|
| Label | `pts-1c-settlement-20260604T205252` |
| Run scale | 200 active bidders, 25s, about 1 attempt/s each, 5s backend crash |
| Overall client results | 3800 decided, 1200 HTTP error attempts |
| Fault-window assertion | 1000 HTTP error attempts |
| Recovery | RTO gate 14s; restore-start-to-final convergence 29s |
| RPO/idempotency proof | 0 duplicate `(epoch, engine_seq)` settlement rows; 0 unsettled accepted bids |

Judge wording:

> "The 1200 errors are failed request attempts from 200 looping bidders, not
> 1200 failed users. The safety claim is replay correctness: after the backend
> restarted, Kafka redelivery produced zero duplicate `(epoch, engine_seq)`
> settlements and zero accepted bids left unsettled. That is the no-double-charge
> proof."

### PostgreSQL SIGKILL

Real incident: PostgreSQL is temporarily unavailable while the auction is live.
The risk is that the bid hot path secretly depends on PG and blocks all bidding,
or that Redis decisions are later lost.

Expected user view: live bidding can continue because Redis is the hot decision
engine. Settlement and durable PG writes catch up after PG returns. Payment and
final finance confirmation must wait for the convergence gate, not only the
foreground bid decision.

Current evidence:

| Field | Value |
|---|---|
| Label | `pts-1c-pg-20260604T205623` |
| Run scale | 200 active bidders, 25s, about 1 attempt/s each, 5s PG fault |
| Overall client results | 4841 decided, 0 paused/errors |
| Fault-window assertion | decided=1000, paused=0 |
| Recovery | RTO gate 2s; restore-start-to-final convergence 18s |
| RPO proof | zero unsettled accepted bids after PG recovery; verifier PASS |

Judge wording:

> "PG was unavailable for 5s, but the Redis hot path still returned 1000
> decisions during that window and did not pause bidders. After PG recovered,
> settlement caught up in the 19s full-convergence window with zero accepted bids
> left unsettled. That proves PG is not on the hot bid decision path, while RPO
> remains zero. Payment/final confirmation should open only after convergence."

### Kafka SIGKILL

Real incident: the message broker used for durable replay/settlement is
temporarily unavailable. In the current local S4 profile this is a single Kafka
container, not a replicated three-broker cluster.

Expected user view: foreground bidding can continue because Redis is still the
hot decision engine. Back-office settlement, payment readiness, finance export,
and final winner confirmation must wait until Kafka returns and the relay drains.

Current evidence:

| Field | Value |
|---|---|
| Label | `pts-1c-kafka-20260604T205824` |
| Run scale | 200 active bidders, 25s, about 1 attempt/s each, 5s Kafka fault |
| Overall client results | 5000 decided, 0 paused/errors |
| Fault-window assertion | decided=1000, paused=0 |
| Bidder RTO gate | 12s |
| Kafka ready after restore start | 16s |
| Restore-end-to-final convergence | 12s |
| Restore-start-to-final convergence | 28s |
| RPO proof | Redis pending drained, Kafka lag 0, verifier PASS |

Judge wording:

> "Kafka failure did not block foreground bidding: during the 5s Kafka fault
> window users still received 1000 Redis engine decisions and saw no pauses or
> HTTP errors. The tradeoff is back-office finality. In the current local
> single-broker profile, Kafka readiness took 16s from restore start and full
> settlement convergence took 28s from restore start. Therefore payment and final
> finance confirmation are gated behind a settlement-confirming state until
> pending and lag drain to zero."

## 5. P1/P2 depth coverage

| Fault | Evidence label | What it proves | RTO gate | Final convergence |
|---|---|---|---:|---:|
| Kafka SIGKILL | `pts-1c-kafka-20260604T205824` | Redis hot path continues; relay drains after Kafka restart | 12s | 28s |
| Kafka SIGKILL, independent k6 | `s4-p1-kafka-independent-20260604T202510` | same Kafka fault while pressure comes from separate VPC k6 host; avoids self-load criticism | client p99 43.04ms | 5000/5000 settled, Kafka lag 0, Redis pending 0, verifier PASS |
| Redis FLUSHALL | `pts-1c-redis-flush-20260604T210256` | Redis data loss is detected; system enters `RECONCILING`, rebuilds, and avoids seq replay | 2s | 20s |
| Redis+Kafka SIGKILL | `pts-1c-both-20260604T210834` | correlated dependency failure still fail-closes accepted decisions and drains pending work | 12s | 28s |
| Redis partial network via Toxiproxy | `pts-1c-partial-20260604T224626` | P2 proxy-path fault reached clients as fail-closed responses and then converged | 8s | 4000 decisions, 200 paused, fault-window paused=200, restore-start-to-final=25s, verifier PASS |
| Relay backpressure | `tests/chaos/07-relay-backpressure.sh` | backlog above 512-entry relay batch ceiling drains over multiple passes without silent loss or duplicate cursor replay | n/a | 600/600 relayed |
| Settlement idempotency | `tests/chaos/08-settlement-idempotency.sh` | same Kafka decision delivered 3x still creates one settlement/order/outbox business effect | n/a | 3x -> 1 effect |

### Independent k6 vs local S4

The local S4 runs and the independent-k6 Kafka run prove different things:

| Run type | What it proves | What it does not prove |
|---|---|---|
| Local S4 runner | full server-side chaos choreography, reset/seed, fault timing, convergence gates, verifier evidence | load generator shares the service host, so a skeptical reviewer can ask about self-load effects |
| Independent VPC k6 | client pressure is generated from a separate ECS and uses the service private IP; k6 host CPU/RSS/TCP metrics show the generator is not the bottleneck | it still uses the same single-node service-side Kafka/Redis/PG topology unless the service environment is changed |

For `s4-p1-kafka-independent-20260604T202510`, k6 ran from the VPC private path,
covered the 20:25:15-20:25:20 Kafka fault window, and recorded
`bid_fault_window_decided_total=1000`, `http_req_failed=0`, 5000 final decisions,
15 accepted / 4985 rejected, p99 43.04ms, and admission contamination 0. Server
evidence then showed 5000/5000 settled, Kafka lag 0, Redis pending 0, Redis
stream length 5000, outbox `PUBLISHED=34`, and verifier P0/P1 PASS.

The earlier `s4-p1-kafka-independent-20260604T202032-invalid-prefault` run is
explicitly invalid because k6 finished before the fault window.

### Redis partial-network diagnostics

The accepted P2 partial-network evidence is
`pts-1c-partial-20260604T224626`. The backend Redis address was routed through
Toxiproxy at `localhost:16379`. The toxic was `reset_peer` on upstream and
downstream plus a downstream timeout. The run produced 4000 final decisions,
200 `ENGINE_PAUSED` responses, 0 reconciling responses, 0 HTTP errors, and 0
admission contamination. Inside the 5s fault window, clients saw 192 decisions
and 200 paused responses. Layer-C gates passed:
`fault_observed_by_clients`, `payment_finality_convergence_gate`, and
`recovery_rto_within_profile_target` with `recovery_rto=8s <= 45s`. Full
convergence was 25s from restore start, with Redis pending 0, Kafka lag 0,
stream length 4000 matching settlements, and verifier PASS.

Earlier `pts-1c-partial-20260604T220112` is not pass evidence. It used a
latency-heavy toxic, raised HTTP tail latency, but left clients receiving only
final decisions with no paused/reconciling/error signature. A timeout-only
variant was also insufficient because existing Redis pooled connections were not
reliably forced into the fail-closed path. That is not "cheating"; it is the
difference between testing slow Redis and testing partial connection loss. The
documented pass gate now uses the fault model that actually exercises the
fail-closed Redis client behavior.

### Relay backpressure: why 600 over 512 matters

Real incident: a hot auction has a final-second burst, or Kafka is slow for a
short window, so the Redis decision stream contains more entries than one relay
batch can append.

Current evidence:

| Field | Value |
|---|---|
| Gate | `tests/chaos/07-relay-backpressure.sh` |
| Scale | 600 Redis engine decisions in one auction |
| Batch ceiling | `relayBatchSize=512` |
| Stream proof | 600 stream entries; first 512 payloads valid with seq 1..512 |
| Relay proof | 600 ledger messages after multi-pass drain |
| Duplicate proof | next relay returns 0 after cursor catch-up |
| Final pending proof | Redis pending hash 0 |

Judge wording:

> "This gate proves our async relay is not a hidden single-batch bottleneck. A
> 600-decision burst exceeds the 512-entry relay batch ceiling, but the stream
> cursor drains all 600 over multiple passes and a later relay does not append
> duplicates. We also hardened the relay so malformed or mismatched stream
> records fail the pass instead of being skipped while the cursor advances."

Boundary: this is not a maximum throughput result. It is a correctness/backlog
gate. `pending hash=0` is not enough alone; finality must still use stream
cursor, Kafka lag, open settlement, open outbox, and payment convergence together.

### Settlement idempotency: why 3x Kafka redelivery matters

Real incident: Kafka is at-least-once. A worker can crash after a PG write but
before offset commit, or a rebalance/network retry can redeliver the same
message.

Current evidence:

| Field | Value |
|---|---|
| Gate | `tests/chaos/08-settlement-idempotency.sh` |
| Fault model | call settlement on the same SOLD ledger message 3 times |
| Settlement proof | 1 `redis_engine_settlements` row, 1 `SETTLED` row |
| Business proof | 1 bid row, 1 order |
| Realtime/outbox proof | 1 outbox event, 1 outbox delivery, 0 missing/duplicate deliveries |

Judge wording:

> "We do not claim Kafka magically gives exactly-once auction settlement. We
> assume at-least-once delivery and make the business effect idempotent. Three
> deliveries of the same SOLD decision still produce one settlement row, one bid,
> one order, and one outbox delivery. That is the no-double-charge proof."

## 6. Why this should convince judges

S4 does not rely on "the container restarted" as evidence. Every scenario must
pass three kinds of gates:

| Gate type | What it checks |
|---|---|
| User-visible fault signature | clients actually saw the intended fault effect: paused, reconciling, HTTP error, or continued decisions |
| Safety invariant | no accepted bid during Redis truth loss, no duplicate settlement, no unsettled accepted bid, no admission contamination |
| Recovery/convergence | RTO gate within target, Redis pending drained, Kafka lag zero, outbox open zero, settlement complete |

Industry framing: AWS Well-Architected treats RTO and RPO as explicit recovery
objectives, not guesses; Google SRE frames reliability as user-journey SLIs/SLOs;
AWS FIS emphasizes stop conditions around steady-state alarms. S4 follows the
same shape locally: define steady state, inject a bounded fault, restore, and
verify user-visible recovery plus data correctness.

The industrial comparison must stay conservative:

| Claim | Safe? | Reason |
|---|---|---|
| "RPO=0, no duplicate settlement, no dirty accepted bids" | yes | directly verified by gates |
| "PG is not on the foreground bid path" | yes | PG fault window still had 1000 decisions and 0 pauses |
| "Kafka 12s is production leader election/rebalance time" | no | current S4 Kafka is single broker, RF=1, no multi-broker leader failover |
| "Redis FLUSHALL 2s proves production cache rebuild is always 2s" | no | current run is local, single auction, small state; production cluster/replica recovery needs separate evidence |
| "The design uses checkpoint rebuild" | yes | `resumeRedisEngine -> rebuildRedisFromCheckpoint -> writeRedisStateSnapshot` restores Redis hot state from checkpoint/current PG state |
| "Relay backlog can silently skip bad stream entries" | no after hardening | malformed/missing/mismatched stream records now fail the relay pass instead of advancing cursor |
| "Settlement is exactly-once because Kafka is exactly-once" | no | the defensible claim is at-least-once delivery plus idempotent business effect |

## 7. Business fallback and optimization stance

Kafka 12s bidder RTO / 28s full convergence does not require an immediate Redis
engine or Kafka relay fix for the graduation S4 P0/P1 gate, because foreground
bidding remains correct and RPO=0. The current worst full convergence is the
settlement-crash run at 29s. These numbers do require the product story to be
explicit about payment/finality gating.

Current implementation already has a basic order-existence gate: the mobile H5
winner state shows `ENGINE_SOLD_PENDING` / "订单同步中" and disables the payment
CTA until a `payableOrderID` exists; the backend payment path loads the order by
ID and verifies the winner before payment.

Current implementation also has a backend payment convergence gate: `PayMock`
blocks a new payment initiation with `PROCESSING_RETRY_LATER` while the auction
has open Redis-engine settlement rows or unpublished outbox deliveries. This is
covered by integration tests:

| Proof | What it proves |
|---|---|
| `TestPaymentWaitsForSettlementConvergence` | an order can exist, but payment is blocked until auction outbox has published |
| `TestPaymentWaitsForOpenRedisEngineSettlement` | payment is blocked while a Redis-engine settlement row is still `PROCESSING` |

S4 runner now also emits `payment_finality_convergence_gate`: payment/final
confirmation is considered safe only when `open_settlements=0`,
`open_outbox=0`, `redis_pending=0`, and `kafka_lag=0`.

Required product fallback:

- After auction end, show `settlement_confirming` / `订单同步中` until full
  convergence passes or the order is safely materialized.
- Do not open payment, finance export, or final winner payment CTA from stale PG
  state while Kafka lag / Redis pending / open settlement / open outbox is non-zero.
- If convergence exceeds the local hard ceiling, freeze payment and alert ops.

Optimization path if this were production:

| Priority | Change | Why |
|---|---|---|
| P0 product gate | keep backend payment convergence gate and S4 `payment_finality_convergence_gate`; add an end-to-end H5 test that Kafka/PG fault keeps CTA disabled until convergence | prevents paying from incomplete settlement state and proves the UI/order guard under Kafka/PG faults |
| P1 infra | run Kafka with 3 brokers, replication factor 3, `min.insync.replicas=2`, `acks=all`, unclean leader election disabled | current single broker is functional evidence, not HA |
| P1 observability | expose "settlement_confirming_seconds", Kafka lag, Redis pending, open settlement/outbox on the host/payment UI | makes the back-office convergence delay visible and defensible |
| P1 backlog metric | expose relay backlog age/cursor lag, not only pending count | AWS queue-backlog guidance treats backlog age/drain as the key async latency signal |
| P2 performance | tune relay/settlement batch and consumer restart behavior only after measuring which segment dominates | current Kafka run is dominated by local broker readiness plus drain; tune with evidence |

## 8. Production HA Questions A ByteDance-Style Judge Will Ask

The source assignment is a Douyin E-commerce live auction system, not a toy
graduation demo. The correct defense is to separate what is implemented now from
what is the production expansion path. This section gives the short S4-specific
answers; the full topology contract, boundary cases, and next-test matrix are in
[production-ha-expansion-and-judge-defense.md](production-ha-expansion-and-judge-defense.md).

### Kafka: why single broker locally, and what production would require

Current S4 uses one local Kafka container with `replication-factor=1` and
`min.insync.replicas=1`. This is enough to prove functional behavior:
Redis decisions remain replayable, Kafka outage does not corrupt settlement, and
relay drains after recovery. It is not production HA.

Production path:

| Layer | Production setting | Reason |
|---|---|---|
| Brokers | 3+ brokers across failure domains | one broker loss should not stop the log |
| Topic replication | `replication.factor=3` | every partition has leader + followers |
| Write quorum | `min.insync.replicas=2` + producer `acks=all` | Kafka docs describe this as the typical durability setup: a majority must persist the write, otherwise producer gets a replica error |
| Safety | `unclean.leader.election.enable=false` | avoid electing an out-of-sync replica and losing committed decisions |
| Partitioning | key by `auction_id` | one auction remains ordered; different rooms scale across partitions |
| Consumer capacity | settlement workers <= partition count, with idempotent `(auction_id, engine_seq)` settlement | at-least-once replay becomes exactly-once business effect |

Why not implement this in local S4 now: cost and environment. A real 3-broker
Kafka profile needs more memory/ports/startup time and changes the fault being
tested from "broker unavailable" to "broker loss with leader election". That is a
valid P2 production-readiness test, but it is not required to prove the current
assignment's core high-concurrency and correctness story. The repository already
contains `infra/docker-compose.kafka-production-example.yml` with the RF=3 /
min-ISR=2 shape so the expansion path is concrete, not hand-wavy.

### Redis: why no Sentinel/Cluster locally, and what production would require

Current S4 uses one Redis container with AOF enabled and `noeviction`. This proves
the bid engine's fail-closed behavior and checkpoint rebuild logic. It is not a
Redis HA benchmark.

Production path:

| Layer | Production setting | Reason |
|---|---|---|
| Primary/replica | Redis primary with at least one replica | reduce downtime and allow failover |
| Failover | Sentinel or managed Redis HA | detect primary failure and promote a replica |
| Client support | Redis client configured for Sentinel/managed endpoint updates | clients must reconnect to the promoted primary |
| Durability | AOF enabled; tune fsync according to latency/durability budget | Redis persistence is required if Redis is part of the recovery chain |
| Stronger replication | optional `WAIT` for selected writes | Redis docs warn this reduces loss probability but does not make Redis a CP/strong-consistency system |
| Business safety | retain fail-closed + checkpoint rebuild even with HA | HA failover is not a substitute for correctness gates |

Why not implement this in local S4 now: a Sentinel/Cluster test changes the
environment and failure model, and the current project already has the stronger
business invariant: if Redis truth is unavailable or state is suspect, bids fail
closed and payment waits for settlement convergence. For a production handoff,
the next concrete test would be "Redis primary killed, Sentinel promotes replica,
client reconnects, no accepted bids during uncertainty, resume after checkpoint
verification."

### Fail-fast / fail-closed

This is implemented and should be defended explicitly. Google SRE guidance on
cascading failures recommends rejecting early and cheaply under overload or
degraded dependencies. In this project, the equivalent is:

| Condition | Behavior |
|---|---|
| Redis truth unavailable/state missing | `ENGINE_PAUSED` / `RECONCILING`, no fabricated accepted bid |
| Kafka/settlement not converged at payment time | `PROCESSING_RETRY_LATER`, payment remains blocked |
| H5 has no order yet | "订单同步中" / CTA disabled |
| Redis/Kafka/PG recovered | only then full-convergence gate allows finality/payment |

This is the right tradeoff for high-value live auction: late confirmation is
acceptable; wrong winner or early payment is not.

## 9. Claim boundary

What S4 proves:

- Under a 200-active-bidder local chaos workload, current code fails closed for
  Redis truth-engine loss.
- Settlement replay is idempotent under backend/worker crash: RPO=0 and no
  duplicate settlement rows.
- PG is not on the Redis hot decision path; decisions continue during a 5s PG
  outage and settle after recovery.
- Kafka outage does not block Redis decisions; relay drains after restart.
- Redis data loss cannot silently restart engine sequence from 1 after durable
  history/checkpoint exists.
- Toxiproxy partial Redis latency is implemented as a P2 harness but is not
  current pass evidence; the latest run raised latency without tripping the
  client-visible fail-closed signature.
- Relay backlog above the 512-entry batch ceiling drains 600/600 decisions over
  multiple passes, and bad stream payloads no longer advance the cursor silently.
- Kafka redelivery is handled as at-least-once input: 3 deliveries of the same
  SOLD decision produce one settlement/order/outbox business effect.

What S4 does not prove:

- It is not a 1000/2000 WS fanout capacity result. S3 covers fanout.
- It is not a maximum bid throughput result. S1/S2 cover bid performance.
- It is not multi-AZ production HA. It is local single-node chaos evidence with
  production-shaped invariants.
- It is not a production Kafka/Redis HA benchmark. Current S4 uses one Kafka
  broker, one Redis container, and one PostgreSQL container.
- It does not prove Kafka's own exactly-once transaction semantics. The claim is
  application-level idempotent settlement under at-least-once redelivery.
- It does not prove sustained backlog capacity. S4 07 is a correctness gate;
  relay backlog age/drain under larger load should be measured before a
  production capacity claim.

## 10. Sources Used For Industrial Framing

- Apache Kafka producer configuration: `acks=all` and idempotence:
  <https://kafka.apache.org/41/configuration/producer-configs>
- Apache Kafka broker/topic configuration: `min.insync.replicas` and unclean
  leader election:
  <https://kafka.apache.org/41/configuration/broker-configs>
- Redis Streams:
  <https://redis.io/docs/latest/develop/data-types/streams/>
- AWS Builders Library on queue backlogs:
  <https://aws.amazon.com/builders-library/avoiding-insurmountable-queue-backlogs>
- AWS transactional outbox pattern and duplicate-message/idempotent-consumer
  guidance:
  <https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html>
- Google SRE Handling Overload:
  <https://sre.google/sre-book/handling-overload/>
