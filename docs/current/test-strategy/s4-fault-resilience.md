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
| **F-redis** | Redis SIGKILL | fail-closed: `ENGINE_PAUSED`, **zero accepted decisions** during fault | 异常兜底 | **P0** | RTO 4 s ✅ |
| **F-settle** | settlement worker SIGKILL mid-batch | Kafka redelivers; **zero duplicate settlement rows** | 绝不扣两次钱 | **P0** | RTO 26 s, 0 dup ✅ |
| **F-pg** | PostgreSQL SIGKILL / Toxiproxy down | hot path keeps deciding (PG-independent); **zero unsettled** after recovery | 数据一致性 | **P0** | RTO 3 s ✅ |
| **F-kafka** | Kafka SIGKILL | hot path may continue `ENGINE_DURABLE`; Redis pending/relay lag is visible; relay drains after restart | durability | P1 | RTO 26 s ✅ |
| **F-flush** | Redis `FLUSHALL` (state lost, process lives) | detect loss → `RECONCILING` → rebuild from Kafka/PG → resume | 缓存防击穿 | P1 | RTO 4 s ✅ |
| **F-both** | Redis + Kafka SIGKILL | correlated failure: fail-closed, no accepted decisions in window | resilience | P1 | RTO 21 s ✅ |
| **F-partial** | Toxiproxy `latency`/`reset_peer` @ `toxicity≈30%` on Redis & Kafka | deadline fail-closed holds; rejects carry basis; no phantom accept | depth | P2 | new |

P0 alone tells the minimum credible story: *correct under peak contention (S1) +
fail-closed + never double-charges + never loses an accepted bid.*

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

## 5. Measuring RTO and RPO (don't guess — instrument)

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

| Toxic | Simulates | Use |
|---|---|---|
| `latency` (+jitter) | slow dependency | Redis/Kafka latency spike; verify deadline fail-closed |
| `bandwidth` (rate 0) | congestion / cut | backpressure on the relay |
| `timeout` | hung/half-open connection | stalled Kafka append |
| `reset_peer` | abrupt peer crash, mid-request RST | partial connection loss |
| proxy `enabled:false` | hard outage / partition | PG-down without killing the container |

Use the global **`toxicity` (% of connections)** for *partial* failure (e.g. 30%
reset) — that is where phantom-accept/double-charge bugs hide, and judges know
100%-down is the easy case. Always remove toxics in teardown (`defer`).
Scenarios live in `tests/chaos/toxiproxy-scenarios.json`; run via
`tests/chaos/run-toxiproxy-scenario.mjs`.

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
```
Default profile (judge-facing RTO): `L1F_PROFILE=rto K6_VUS=200 K6_DURATION=25s
SLEEP_MS=1000 FAULT_WINDOW_SECONDS=5 L1F_RTO_TARGET_SECONDS=45`.

## 8. What to show judges (per fault, one panel)

1. **RTO timeline** — a strip with `fault_injected` and the SLI-recovery crossing,
   RTO labeled in seconds (from `recovery-breakdown.json`), not narrated.
2. **RPO=0 reconciliation table** — accepted == Redis Stream == Kafka WAL == settled, zero orphans.
3. **Fail-closed evidence** — "during 5 s Redis-down, 0 of N bids returned accepted
   without an `ENGINE_DURABLE` decision record; all rejects carry a decision-time basis."
4. **Recovery curve** — success-rate / p99 over normal → degraded → recovered.

## 9. Pitfalls

- **Guessed RTO** ("about a minute") — disqualifying; emit timestamps.
- **Recovery declared on one good sample** — require a trailing window.
- **HTTP 200 == outcome** — inspect `ENGINE_*`/durability/settlement, not status.
- **Claiming RPO=0 from HTTP 200 alone** — claim it only from Redis Stream,
  Kafka WAL, PostgreSQL settlement, and verifier agreement.
- **Citing the backlog-drain time as RTO** — `L1F_PROFILE=backlog` manufactures a
  queue; it answers "can a huge backlog drain?", not "how fast does a user
  recover?".
- **Only 100%-down faults** — add `toxicity≈30%` partial failure (F-partial).
