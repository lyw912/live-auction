# S4 — 故障韧性 / Fault Injection & Recovery

> Maps to: brief rubric 系统可用性(异常兜底) + 稳定性(数据一致性); 加分 "分布式锁解决幂等性，绝对不允许一笔出价扣两次钱".
> Headline: **M5 RTO ≤ 30 s (local) + RPO = 0** + zero phantom accepts + zero duplicate settlement.
> Tool: **local k6 + Toxiproxy + SIGKILL** (0 VUM — never PTS).
> Source-of-truth: `tests/pts/run-pts-1c-concurrent-fault.sh`, `tests/pts/L1-component/pts-1c-k6-concurrent-fault.js`, `tests/chaos/*`.

## 1. Why local, not PTS

Fault correctness is **structural, not statistical**: the Redis single-writer Lua
serializes every decision atomically regardless of VU count, so the fail-closed
races that matter appear at 200 VU just as at 1000. PTS adds cost and distributed
IPs that buy nothing here. 200 concurrent local bidders + Toxiproxy + SIGKILL is
the right, free instrument.

## 2. Frame every fault as a chaos experiment (not a demo)

A credible experiment has four parts (Principles of Chaos Engineering + AWS FIS):

```
1. STEADY STATE (numeric SLI, measured over a window):
     decision success ≥ 99%  AND  bid decision p99 ≤ 50 ms,  over a trailing 30 s window
2. HYPOTHESIS: "injecting <fault> for 5 s keeps the system SAFE (fail-closed or continue),
     and it returns to steady state within RTO target, losing zero accepted bids."
3. INJECT: bounded blast radius (one dependency, one 5 s window), with an abort/stop condition.
4. VERIFY + ROLLBACK: did steady state hold/recover? scripted restore (toxic remove / restart).
```

This is what turns "we have HA" into evidence. The load model is already aligned
to a user story (200 active bidders, 1 attempt/s, 5 s fault, recover in seconds —
not a manufactured 60k-decision backlog; that is the `L1F_PROFILE=backlog` drain
test, never cited as user-facing RTO).

## 3. The minimal fault set (each maps to a brief requirement)

| ID | Fault (inject) | Hypothesis / required behavior | Brief tie | Tier | Measured |
|---|---|---|---|---|---|
| **F-redis** | Redis SIGKILL | fail-closed: `ENGINE_PAUSED`, **zero accepted decisions** during fault | 异常兜底 | **P0** | RTO 2 s ✅ |
| **F-settle** | backend/settlement worker SIGKILL | Kafka redelivers; **zero duplicate settlement rows** | 绝不扣两次钱 | **P0** | RTO 2 s, 0 dup ✅ |
| **F-pg** | PostgreSQL SIGKILL | hot path keeps deciding (PG-independent); **zero unsettled** after recovery | 数据一致性 | **P0** | RTO 3 s ✅ |
| **F-kafka** | Kafka SIGKILL | hot path continues `ENGINE_DURABLE`; Redis pending/relay lag is visible; relay drains after restart | durability | P1 | RTO 12 s ✅ |
| **F-flush** | Redis `FLUSHALL` (state lost, process lives) | detect loss → `RECONCILING` → controlled rebuild → resume | 缓存状态丢失 | P1 | RTO 2 s ✅ |
| **F-both** | Redis + Kafka SIGKILL | correlated failure: fail-closed, no accepted decisions in window | resilience | P1 | RTO 12 s ✅ |
| **F-partial** | Toxiproxy Redis `reset_peer` + timeout | weak-network/partial outage should reach clients as `ENGINE_PAUSED`/`RECONCILING`/HTTP error | depth | P2 | current reset/timeout toxic PASS; latency-only/timeout-only attempts remain diagnostic failures |
| **F-relay-backpressure** | Redis decision stream exceeds relay batch ceiling | relay drains backlog over multiple passes; no silent loss/duplicate cursor advance | async backlog safety | P0 depth | 600/600 relayed ✅ |
| **F-settlement-idempotency** | same Kafka ledger message delivered 3x | at-least-once replay has exactly one settlement/order/outbox business effect | 绝不扣两次钱 | P0 depth | 3x -> 1 effect ✅ |

P0 alone tells the minimum credible story: *correct under peak contention (S1) +
fail-closed + never double-charges + never loses an accepted bid.*

## 3.1 Current Evidence Snapshot

All rows below are current local evidence from 2026-06-04, `L1F_PROFILE=rto`,
`K6_VUS=200`, `K6_DURATION=25s`, `SLEEP_MS=1000`, `FAULT_WINDOW_SECONDS=5`.
This means 200 concurrent active bidders in one hot auction, each loop doing:
snapshot -> bid -> sleep about 1 second. It models a live room where 200 users
keep retrying manually/automatically at roughly 1 bid attempt per second while
one dependency is broken for 5 seconds.

| Fault | Evidence label | Client-visible during run | Fault-window assertion | Bidder RTO gate | Full convergence / RPO assertion |
|---|---|---:|---|---:|---|
| Redis SIGKILL | `pts-1c-redis-20260604T203439` | 3847 decided, 600 paused | window decided=0, paused=400, accepted-in-window=0 | 2s | restore-start-to-final=19s; settlements 3847/3847; verifier PASS |
| backend/settlement crash | `pts-1c-settlement-20260604T205252` | 3800 decided, 1200 HTTP errors | window HTTP errors=1000 | 14s | restore-start-to-final=29s; 0 duplicate `(epoch,seq)`; 0 unsettled accepted; verifier PASS |
| PostgreSQL SIGKILL | `pts-1c-pg-20260604T205623` | 4841 decided, 0 paused/errors | window decided=1000, paused=0 | 2s | restore-start-to-final=18s; zero unsettled accepted after PG recovery; verifier PASS |
| Kafka SIGKILL | `pts-1c-kafka-20260604T205824` | 5000 decided, 0 paused/errors | window decided=1000; hot path Kafka-independent | 12s | Kafka ready after restore 16s; restore-start-to-final=28s; Redis pending drained; Kafka lag 0; verifier PASS |
| Redis FLUSHALL | `pts-1c-redis-flush-20260604T210256` | 1000 decided, 3974 paused, 26 reconciling | window decided=0, paused=974, reconciling=26, accepted-in-window=0 | 2s | restore-start-to-final=20s; settlements 1000/1000; Redis stream len 0 is expected under data-loss profile; verifier PASS |
| Redis+Kafka SIGKILL | `pts-1c-both-20260604T210834` | 3800 decided, 600 paused | window decided=200 non-accepted decisions, paused=400, accepted-in-window=0 | 12s | Kafka ready after restore 16s; restore-start-to-final=28s; pending drained; verifier PASS |
| Redis partial Redis network via Toxiproxy | `pts-1c-partial-20260604T224626` | 4000 decided, 200 paused, 0 reconciling, 0 HTTP errors | window decided=192, paused=200, reconciling/errors=0 | 8s | restore-start-to-final=25s; settlements 4000/4000; Redis stream len 4000; Redis pending 0; Kafka lag 0; verifier PASS |
| Relay backpressure | `tests/chaos/07-relay-backpressure.sh` | focused backend gate, no k6 | 600 Redis decisions exceed 512 relay batch ceiling | n/a | 600 stream entries; first page 512 valid payloads seq 1..512; ledger 600; pending hash 0; next relay 0 |
| Settlement idempotency | `tests/chaos/08-settlement-idempotency.sh` | focused backend gate, no k6 | same ledger message settled 3 times | n/a | 1 settlement row, 1 SETTLED row, 1 bid, 1 order, 1 outbox event/delivery; 0 duplicate deliveries |

User-facing interpretation:

| Metric | What it means in the auction room |
|---|---|
| `decided` | the bidder received a final engine decision for that bid attempt (`ENGINE_ACCEPTED`, `ENGINE_REJECTED`, or `ENGINE_SOLD`) |
| `paused` / `reconciling` | the bidder's attempt did not create a bid; UI should show "engine recovering, retry shortly" |
| `http_errors` | process-level crash/down; user cannot submit until backend is back |
| `fault_window_decided=0` for Redis faults | while Redis truth engine is unavailable/lost, the system refuses to fabricate accepted bids |
| `fault_window_decided=1000` for PG/Kafka faults | Redis hot path can still decide while PG/Kafka are temporarily unavailable |
| `settlements=N/N` and verifier PASS | every durable engine decision converged to PostgreSQL settlement exactly once |

Pressure-reached evidence:

| Proof | Meaning |
|---|---|
| k6 total decisions/paused/errors | clients were actively submitting bids during the run, not just after recovery |
| fault-window counters | the injected 5s fault overlapped real bid attempts; for example the independent Kafka run `s4-p1-kafka-independent-20260604T202510` recorded `bid_fault_window_decided_total=1000` |
| service convergence gates | Redis pending, Kafka lag, open settlement rows, and open outbox rows returned to zero |
| verifier gates | RPO=0, no phantom accepts, no duplicate `(epoch, engine_seq)` settlement effect |

Do not treat a clean k6 exit as the S4 proof by itself. The load-generator proves
overlap and user-visible symptoms; the server evidence proves safety. A
Kafka/PG fault may have `fault_window_decided=1000` and still be unsafe if
settlement/outbox never converges. Conversely, a Redis fault is expected to show
paused/reconciling/fail-closed responses rather than continued accepts.

### 3.1.1 S4 07/08 focused gates

These two gates are intentionally not 25s k6 chaos runs. They attack two
correctness holes that a senior reviewer will ask about after seeing the Redis
Stream -> Kafka -> PostgreSQL architecture.

| Gate | Test logic | Real business incident | Current measured proof | User/business meaning |
|---|---|---|---|---|
| `07-relay-backpressure` | Create 600 Redis engine decisions in one auction, above the relay batch ceiling of 512, then reset the relay cursor and drain through `relayAuctionLogBatch` until no work remains. | Final-second bid burst or Kafka relay stall creates more durable Redis decisions than one relay tick can append to Kafka. | 600/600 bids returned `DECIDED + ENGINE_DURABLE`; Redis Stream length=600; first 512 stream entries have valid payloads and seq 1..512; relay eventually appends 600 ledger messages; final pending hash=0; next relay returns 0. | Bidders can receive decisions while back-office relay catches up; payment/finality must still wait for convergence. The important safety point is no silent skipped stream entry and no duplicate append after cursor advance. |
| `08-settlement-idempotency` | Take one SOLD Kafka ledger message and call settlement three times with the same payload. | Kafka consumer redelivers after worker crash, offset commit failure, rebalance, or transient network issue. | After 3 deliveries: exactly 1 `redis_engine_settlements` row, 1 `SETTLED` row, 1 bid row, 1 order, 1 outbox event, 1 outbox delivery, 0 missing/duplicate deliveries. | The winner does not get a second order/payment/outbox effect just because Kafka replays a decision. |

Important boundary: `pending hash=0` is not a complete backlog metric by itself.
It means the current relay has marked all known pending decisions as Kafka-acked.
Backlog/finality must be judged from stream cursor progress, Kafka lag, open
settlement rows, open outbox deliveries, and the payment/finality convergence
gate together.

`07` also hardened the implementation: malformed stream payloads, missing
payloads, auction-id mismatches, or a Kafka `AppendBatch` result count mismatch
now fail the relay pass instead of being silently skipped while the cursor moves.
That is the correct failure mode for an auction ledger: pause/alert and retry,
not "advance past a bad record and lose a bid."

### 3.2 Two clocks: bidder recovery vs back-office convergence

Do not collapse these into one number:

| Clock | Boundary | User/business meaning | Current worst case |
|---|---|---|---:|
| Bidder RTO gate | fault clear / post-load recovery start -> sustained user-visible recovery gate | when new bid attempts are safe again, or when Redis faults stop returning paused/reconciling | 14s (`F-settle`) across current P0/P1 pass set |
| Full convergence | restore start -> Redis pending = 0, Kafka lag = 0, open settlement/outbox = 0, verifier-safe state | when finance/payment/settlement views can be treated as final | 29s (`F-settle`) across current P0/P1 pass set |

For PostgreSQL SIGKILL, `fault_window_decided=1000` proves the foreground bid
decision path is PG-independent. It does **not** mean settlement was final at
that instant. The same run records `restore-start-to-final convergence=19s`:
after PG returned, settlement/outbox/Kafka lag had to drain before payment or
finance views could be considered fully current.

For Kafka SIGKILL, bidder-facing decisions continued (`5000 decided`, `0 paused`,
`0 errors`), but the back-office chain was unavailable until Kafka returned and
the relay/settlement path drained. Current timing:

| Segment | Kafka run value | Meaning |
|---|---:|---|
| `first_decided_after_fault_end_seconds` | 0s | bidders continued receiving Redis decisions immediately after the fault window |
| `kafka_ready_after_restore_start_seconds` | 16s | local single Kafka container became reachable again |
| `restore_end_to_final_convergence_seconds` | 12s | after Kafka was reachable, Redis pending / Kafka lag / settlement drained |
| `restore_start_to_final_convergence_seconds` | 28s | conservative payment/finance confirmation boundary |

Judge-facing implication: during Kafka and PG faults, bidding can continue, but
payment, final winner confirmation, and finance export should remain in a
`settlement_confirming` / `payment_not_open_yet` state until the full-convergence
gate passes.

The implementation now has two payment/finality protections:

| Protection | Boundary |
|---|---|
| Backend payment convergence gate | `PayMock` returns `PROCESSING_RETRY_LATER` for a new payment while the auction has open Redis-engine settlement rows or unpublished outbox deliveries |
| S4 `payment_finality_convergence_gate` | post-fault evidence requires `open_settlements=0`, `open_outbox=0`, `redis_pending=0`, and `kafka_lag=0` before payment/final confirmation is considered safe |

### 3.3 Local environment and non-claims

This evidence is intentionally local and single-node:

| Component | Current S4 environment | Production implication |
|---|---|---|
| Kafka | one `apache/kafka:3.9.1` container, `replication-factor=1`, `min.insync.replicas=1` | functional chaos proof only; not multi-broker HA proof |
| Redis | one `redis:7-alpine` container, AOF enabled, no replica/sentinel/cluster | proves fail-closed and checkpoint rebuild logic; not Redis HA/failover SLA |
| PostgreSQL | one `postgres:16-alpine` container | proves PG-independent hot path and catch-up; not HA primary/standby failover |
| Fault window | 5s bounded SIGKILL/FLUSHALL/Toxiproxy timeout | good graduation chaos evidence; not AZ failure or cross-region DR |

Do not claim "production Kafka leader election recovered in 16s" from this run:
with replication factor 1 there is no multi-broker partition leader failover.
The measured Kafka delay is local broker/container readiness plus relay and
settlement drain.

Do not claim "all cache rebuilds are 2s in production" from Redis FLUSHALL. The
current implementation rebuilds one auction's Redis hot state from
`auction_engine_checkpoints` / PostgreSQL current state via
`resumeRedisEngine -> rebuildRedisFromCheckpoint -> writeRedisStateSnapshot`.
The 2s value is valid for this local single-auction workload; larger auctions,
Redis replicas, disk restore, or cluster recovery need separate evidence.

## 4. The no-double-charge headline test (F-settle, in detail)

This is the single most valuable fault test for the brief's "绝对不允许一笔出价
扣两次钱". It directly exercises Kafka's at-least-once delivery against the
settlement idempotency.

```
1. drive 200 VU steady bids; each accepted decision has (epoch, engine_seq) + client_bid_id
2. SIGKILL the settlement worker MID-BATCH — ideally after the PG write but BEFORE the Kafka
   offset commit (the worst case: the batch is guaranteed to be redelivered)
3. restart worker → Kafka redelivers from last committed offset
4. ASSERT zero duplicates:
     SELECT epoch, engine_seq, COUNT(*) FROM settlement GROUP BY 1,2 HAVING COUNT(*)>1;   -- empty
     AND  count(distinct settled) == count(distinct accepted decisions in the WAL)
5. (depth) start a 2nd worker mid-flight to exercise the consumer-group revoke/reassign duplicate path
```

The mechanism that makes this pass — and what to *say* to judges — is the
**fencing/sequence**: settlement is keyed on `(epoch, engine_seq)` with a unique
constraint, so a redelivered or out-of-order decision is a no-op insert, not a
second charge. This is the Kleppmann fencing-token / transactional-outbox pattern,
and it is the same shape Taobao 秒杀 uses (Redis atomic decision + async DB +
unique constraint). Existing gates already assert this:
`settlement_replay_no_duplicates` + `settlement_replay_complete`.

The focused `08-settlement-idempotency` gate is the sharper unit of proof for
Kafka redelivery. It does not claim Kafka gives exactly-once business semantics.
It proves the application layer converts at-least-once delivery into exactly-one
business effect through settlement uniqueness, order uniqueness, and outbox
delivery uniqueness.

## 5. Measuring RTO and RPO (don't guess — instrument)

### Industry basis for the target

RTO/RPO are business objectives, not universal constants. AWS Well-Architected
defines RTO as the maximum acceptable delay between service interruption and
restoration, and RPO as the maximum acceptable amount of data loss since the
last recovery point:
https://docs.aws.amazon.com/wellarchitected/latest/reliability-pillar/disaster-recovery-dr-objectives.html

Google SRE frames reliability targets as SLOs measured by concrete SLIs, with
error budgets used to decide when reliability work takes priority:
https://sre.google/sre-book/service-level-objectives

For this live auction project, the business implication is stricter than a
generic content feed:

```text
RPO: must be 0 for accepted ENGINE_DURABLE decisions, because a lost accepted bid
     can change the winner or create payment/arbitration risk.

RTO: should recover within seconds to tens of seconds. A short "confirming /
     reconciling" state is acceptable; fabricating accepts or allowing payment
     from incomplete state is not.
```

The current local S4 target is therefore:

| Target | Meaning | Why |
|---|---|---|
| RPO = 0 | no accepted durable bid is lost, duplicated, or settled twice | auction/payment correctness |
| RTO <= 30s | acceptable local recovery target for a 5s bounded fault | judge-facing "fast recovery" line |
| RTO <= 45s | hard local ceiling for single-node/container restart noise | avoids over-penalizing local Kafka/PG cold starts |

AWS Fault Injection Service also emphasizes stop conditions around steady-state
alarms, so S4 treats recovery as a trailing-window SLI, not one successful
request after a restart:
https://docs.aws.amazon.com/fis/latest/userguide/stop-conditions.html

**RTO** — the harness emits machine timestamps; recovery is declared only when the
SLI holds over a window, never on one good sample:
```
fault_injected → first_error → first_success_after → slo_recovered
RTO = slo_recovered − fault_injected     (user-visible: fault_clear → first sustained DECIDED)
```
Already captured in `recovery-breakdown.json` / `recovery-start.json` /
`recovery-end.json` (restore duration, component-ready deltas, first-decided-after
markers). Targets: ≤ 10 s excellent · ≤ 30 s acceptable · ≤ 45 s hard local
ceiling (single-container Kafka cold start).

**RPO = 0** — proven by reconciliation, not uptime:
```
count(distinct accepted, client-confirmed)
  == count(Redis Stream engine decisions)
  == count(Kafka WAL entries)
  == count(distinct settled PG rows)
AND zero phantom accepts in the fault window
```
The response boundary is **ENGINE_DURABLE**. RPO=0 is claimed only after the
Redis Stream -> Kafka WAL -> PostgreSQL settlement chain reconciles with zero
phantom accepts and zero duplicate settlement.

## 6. Toxiproxy patterns (for F-partial and network realism)

Current P2 partial-fault harness:

```text
backend REDIS_ADDR=localhost:16379
Toxiproxy proxy name=redis, listen=0.0.0.0:16379, upstream=redis:6379
optional toxic: upstream + downstream latency when TOXIPROXY_LATENCY_MS is set
default toxic: upstream + downstream reset_peer, toxicity=1.0
default toxic: downstream timeout, toxicity=1.0, timeout=250ms
fault window: 5s
```

This is different from Redis SIGKILL. Redis stays alive, but the backend's Redis
network path stalls through a proxy. User-visible behavior should still be
fail-closed (`ENGINE_PAUSED`/`RECONCILING`/HTTP error), with zero admission
contamination and full settlement convergence after the toxic is removed.

Current 2026-06-04 result is counted as P2 pass for Redis partial network:
`pts-1c-partial-20260604T224626` exercised the Redis proxy with reset-peer plus
timeout. Clients observed `ENGINE_PAUSED=200`, admission contamination stayed
0, recovery RTO was 8s, restore-start-to-final convergence was 25s, and all
payment/finality, Redis/Kafka, and verifier gates passed.

Two earlier diagnostic attempts are intentionally not counted as pass:
`pts-1c-partial-20260604T220112` used a latency-heavy toxic and raised HTTP tail
latency to about 4.95s p99 / 5.04s max, but did not create a client-visible
fail-closed signature. A timeout-only rerun also did not reliably trip existing
go-redis pooled connections. Those failures explain why the default P2 toxic is
now reset-peer plus timeout; latency remains an optional enhancement, not the
pass/fail baseline.

| Toxic | Simulates | Use |
|---|---|---|
| `latency` (+jitter) | slow dependency | Redis/Kafka latency spike; verify deadline fail-closed |
| `bandwidth` (rate 0) | congestion / cut | backpressure on the relay |
| `timeout` | hung/half-open connection | stalled Kafka append |
| `reset_peer` | abrupt peer crash, mid-request RST | partial connection loss |
| proxy `enabled:false` | hard outage / partition | PG-down without killing the container |

Toxiproxy's global `toxicity` can be reduced for future probabilistic weak
network tests (for example 30% reset/latency). The current S4 evidence uses
`toxicity=1.0` intentionally because the gate is fail-closed behavior under a
bounded Redis network stall, not a pass-rate sampling exercise. Always remove
toxics in teardown; `tests/chaos/s4-toxiproxy-fault.mjs clear` recreates a clean
Redis proxy.

## 7. Run sequence (existing harness)

```bash
# P0 core
FAULT_TYPE=redis      bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=settlement SERVER_START_CMD="ALLOW_MOCK_AUTH=true BID_ENGINE_MODE=redis_ledger ADMISSION_ENABLED=false ./live-auction-server" \
                      bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=pg         bash tests/pts/run-pts-1c-concurrent-fault.sh
# P1
FAULT_TYPE=kafka      bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=redis-flush bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=both       bash tests/pts/run-pts-1c-concurrent-fault.sh
# P2 weak-network depth
FAULT_TYPE=partial    bash tests/pts/run-pts-1c-concurrent-fault.sh

# Focused P0 depth gates; run sequentially because they share local Redis/PG test dependencies.
bash tests/chaos/07-relay-backpressure.sh
bash tests/chaos/08-settlement-idempotency.sh
```
Default profile (judge-facing RTO): `L1F_PROFILE=rto K6_VUS=200 K6_DURATION=25s
SLEEP_MS=1000 FAULT_WINDOW_SECONDS=5 L1F_RTO_TARGET_SECONDS=45`.

## 8. What to show judges (per fault, one panel)

1. **RTO timeline** — a strip with `fault_injected` and the SLI-recovery crossing,
   RTO labeled in seconds (from `recovery-breakdown.json`), not narrated.
2. **Full-convergence timeline** — `restore_start -> component_ready ->
   pending/lag/outbox/settlement drained`. For PG/Kafka faults, this is the
   payment/finance confirmation boundary.
3. **RPO=0 reconciliation table** — accepted == Redis Stream == Kafka WAL == settled, zero orphans.
4. **Fail-closed evidence** — "during 5 s Redis-down, 0 of N bids returned accepted
   without an `ENGINE_DURABLE` decision record; all rejects carry a decision-time basis."
5. **Payment/finality gate** — show `payment_finality_convergence_gate PASS`
   with `open_settlements=0 open_outbox=0 redis_pending=0 kafka_lag=0`.
6. **Recovery curve** — success-rate / p99 over normal → degraded → recovered.

## 9. Pitfalls

- **Guessed RTO** ("about a minute") — disqualifying; emit timestamps.
- **Recovery declared on one good sample** — require a trailing window.
- **HTTP 200 == outcome** — inspect `ENGINE_*`/durability/settlement, not status.
- **Claiming RPO=0 from HTTP 200 alone** — claim it only from Redis Stream,
  Kafka WAL, PostgreSQL settlement, and verifier agreement.
- **Citing the backlog-drain time as RTO** — `L1F_PROFILE=backlog` manufactures a
  queue; it answers "can a huge backlog drain?", not "how fast does a user
  recover?".
- **Only SIGKILL faults** — include at least one network-path fault through
  Toxiproxy. Current F-partial is Redis reset-peer plus downstream timeout;
  latency-only is optional diagnostic coverage, not the current pass gate.
- **Parallel-running 07/08 and treating a shared-test Redis cleanup as a fault** —
  these focused integration gates share local test Redis/PG dependencies and
  should be run sequentially. Parallel pollution can produce `RECONCILING` that
  is a test isolation artifact, not a relay-backpressure failure.

## 10. Industrial reference points for 07/08

- Apache Kafka durability settings are a production expansion path, not proof
  from this local single-broker run. A real HA profile should use RF=3,
  `min.insync.replicas=2`, producer `acks=all`, idempotence where applicable,
  and `unclean.leader.election.enable=false`.
- Redis Streams are append-only logs, so they are a reasonable local WAL-like
  structure for the hot decision relay. The safety gate must prove cursor
  progress and payload integrity, not just "some messages exist".
- AWS Builders Library warns that queues improve durability but can create large
  recovery backlogs; the right metric is backlog age/drain/finality, not only
  producer success. That is why S4 reports full convergence separately from
  bidder RTO.
- AWS transactional outbox guidance explicitly says duplicate messages can occur
  and consumers should be idempotent. That is exactly what S4 08 proves for
  settlement/order/outbox.
- Google SRE overload guidance supports fail-fast/fail-closed behavior under
  resource pressure. For high-value live auction, delayed confirmation is safer
  than fabricating accepts or opening payment from incomplete settlement state.
