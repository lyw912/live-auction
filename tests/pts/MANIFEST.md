# PTS Workload Manifest

> Last updated: 2026-06-01. Read `docs/current/performance-correctness-contract.md`
> before running or interpreting any workload here.

This manifest is the authoritative index of all performance test workloads.
A file existing in `tests/pts/` does not mean it is current evidence.
Historical scripts are indexed in `tests/pts/HISTORICAL.md`.

---

## Framework Overview

The PTS suite is organized into layers. Tests must pass lower layers before
higher layers are meaningful. When a higher-layer regression appears, isolate
the cause by re-running the corresponding lower-layer workload.

```
L0   SMOKE          Connectivity + business chain gate (CI, fast)
L1   COMPONENT      Hot-path isolation: SLA + correctness (no faults, no background noise)
L1-F FAULT          Concurrent fault injection under L1-level bid load (correctness under failure)
L2   PROTOCOL       Bid + one background protocol at a time (attribution layer)
L3   SCENARIO       Realistic bid distribution + full auction lifecycle
L4   COMBINED       All traffic types simultaneously (production readiness gate)
```

### Why L1 and L1-F are separate, and why that is sufficient

Performance tests (L1-C1, and future L2–L4) prove latency SLA under increasing
traffic complexity. Fault tests (L1-F) prove correctness under failure. These two
dimensions are intentionally separated because:

- **Latency degrades during faults by design.** Mixing fault injection into a
  latency SLA test would conflate a correctness result with a performance result.
  The correct answer to "what is p99 when Redis crashes?" is "undefined — the
  system fail-closes; latency is not the metric that matters."

- **Fault correctness is structural, not statistical.** The Redis engine uses a
  single-writer Lua script: all bid decisions are serialised atomically in Redis,
  regardless of VU count. This means the fail-closed race conditions that matter
  are identical at 10 VU and 1000 VU. L1-F at 200 VU exposes these races; higher
  VU count would add queuing delay but would not surface new correctness failure modes.

- **L2–L4 fault tests are not needed.** Adding WebSocket fanout or read traffic
  (L2) does not change how Redis or Kafka failure propagates to the bid decision
  path. The bid path is structurally isolated from the read/WS paths; a fault on
  the bid engine affects L2 scenarios identically to L1. Running L2+fault would
  add noise (which path caused the failure?) without adding proof value.

**The complete correctness argument is: L1-C1 (correctness under peak load, no
faults) + L1-F (correctness under faults, concurrent load) + chaos scripts
(protocol correctness for each fault mode individually).**

---

## L0 — Smoke (CI Gate)

**Purpose**: verify that the deployment is reachable and the core API chain works.
Not capacity evidence. Run before every PTS session to confirm the harness is healthy.

| ID | JMX | Status | Note |
|---|---|---|---|
| `L0-S1` | `L0-smoke/live-auction-pts-smoke.jmx` | CURRENT | 4-step connectivity: readyz, login, rooms, auctions |
| `L0-S2` | `L0-smoke/live-auction-pts-business-smoke.jmx` | CURRENT | Full business chain: login → snapshot → WS ticket → bid → monitor → metrics |

Thread settings: 1 VU, 1 loop, 1s ramp. Not performance evidence.

---

## L1 — Component Isolation

**Purpose**: measure the hot bid path with zero background noise. These tests answer
"can the engine itself meet the SLA?" They do not answer "will it hold under real traffic?"

| ID | JMX | Status | SLA | Note |
|---|---|---|---|---|
| `L1-C1` | `L1-component/pts-1b-contention-burst-1000vu-1m.jmx` | **VALIDATED** ✅ | bid p99 ≤ 50ms | 1000 VUs, final-second contention burst, one hot auction. PRIMARY SLA gate. |
| `L1-C0` | `L1-component/pts-1a-accepted-ladder-1000vu-1m.jmx` | CURRENT (control) | — | Ordered accepted ladder — all bids succeed. Sanity/baseline only. Do not cite as contention evidence. |

`L1-C1` is the current primary performance/correctness workload.

### L1-C1 Runtime Profile

```text
BID_ENGINE_MODE=redis_ledger
ADMISSION_ENABLED=false
REDIS_ADDR=localhost:6380
KAFKA_BROKERS=localhost:9092
```

### L1-C1 Run Sequence

```bash
L4B_PROFILE=pts-1b SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
BASE_URL=http://127.0.0.1:18080 bash tests/pts/preflight-l4b-pts-guards.sh before-<run-label>
# Run JMeter with L1-component/pts-1b-contention-burst-1000vu-1m.jmx
# Session CSV: docs/perf/pts/pts-1ab-1000vu-sessions.csv
BASE_URL=http://127.0.0.1:18080 bash tests/pts/collect-server-evidence.sh <report-id-or-label>
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh <report-id-or-label>
```

Evidence output goes to `docs/perf/pts/evidence/incoming/<label>`. Move reviewed runs
to `current/` or `archive/*/` after classification.

---

## L1-F — Concurrent Fault Injection

**Purpose**: inject Redis or Kafka failure while 200 VUs are bidding concurrently.
Proves that the engine fail-closes correctly (ENGINE\_PAUSED during fault) and that
the final auction state satisfies all correctness invariants after recovery.

This is **not** a latency test. p99 degrades during the fault window by design.

| ID | Script | Status | Fault modes | Note |
|---|---|---|---|---|
| `L1-F1` | `run-pts-1c-concurrent-fault.sh` | CURRENT | `FAULT_TYPE=redis` | Redis SIGKILL → ENGINE\_PAUSED fail-closed under 200 VU load |
| `L1-F2` | `run-pts-1c-concurrent-fault.sh` | CURRENT | `FAULT_TYPE=kafka` | Kafka SIGKILL → hot path continues (DECIDED); relay drains after restart |
| `L1-F3` | `run-pts-1c-concurrent-fault.sh` | CURRENT | `FAULT_TYPE=both` | Redis + Kafka simultaneous SIGKILL (correlated failure) |
| `L1-F4` | `run-pts-1c-concurrent-fault.sh` | CURRENT | `FAULT_TYPE=redis-flush` | Redis FLUSHALL (container lives, state evaporates → RECONCILING → rebuild) |
| `L1-F5` | `run-pts-1c-concurrent-fault.sh` | CURRENT | `FAULT_TYPE=pg` | PostgreSQL SIGKILL → hot path must keep accepting bids without PG |
| `L1-F6` | `run-pts-1c-concurrent-fault.sh` | CURRENT | `FAULT_TYPE=settlement` | Backend process SIGKILL → Kafka replay on restart must be idempotent |

### L1-F Runtime profile

```text
ALLOW_MOCK_AUTH=true        ← k6 uses X-Mock headers; no JWT session pool needed
BID_ENGINE_MODE=redis_ledger
ADMISSION_ENABLED=false
```

### L1-F Run sequence

```bash
# F1: Redis SIGKILL — fail-closed under concurrent bids
FAULT_TYPE=redis bash tests/pts/run-pts-1c-concurrent-fault.sh

# F2: Kafka SIGKILL — relay falls behind; hot path continues
FAULT_TYPE=kafka bash tests/pts/run-pts-1c-concurrent-fault.sh

# F3: Both simultaneously — correlated infrastructure failure
FAULT_TYPE=both bash tests/pts/run-pts-1c-concurrent-fault.sh

# F4: Redis FLUSHALL — OOM eviction simulation; state lost, process lives
FAULT_TYPE=redis-flush bash tests/pts/run-pts-1c-concurrent-fault.sh

# F5: PostgreSQL SIGKILL — proves hot path does not depend on PG
FAULT_TYPE=pg bash tests/pts/run-pts-1c-concurrent-fault.sh

# F6: Backend process SIGKILL — Kafka replay idempotency on restart
SERVER_START_CMD="ALLOW_MOCK_AUTH=true BID_ENGINE_MODE=redis_ledger ADMISSION_ENABLED=false ./live-auction-server" \
  FAULT_TYPE=settlement bash tests/pts/run-pts-1c-concurrent-fault.sh
```

### What L1-F verifies (gates per fault type)

| Gate | Applies to | Severity | What it proves |
|---|---|---|---|
| `fault_observed_by_clients` | all | P0 | Fault signature reached k6: ENGINE\_PAUSED (redis/flush/both), DECIDED (kafka/pg), HTTP errors (settlement) |
| `no_admission_contamination` | all | P0 | Zero RATE\_LIMITED — ADMISSION\_ENABLED=false respected throughout |
| `no_accepted_settlement_during_redis_fault` | redis, redis-flush, both | P0 | Zero accepted bids settled during Redis unavailability — no phantom accepts |
| `kafka_relay_drained_after_recovery` | kafka, both | P0 | Redis pending hash empty — relay fully drained after Kafka restart |
| `pg_recovery_settlement_complete` | pg | P0 | Zero unsettled accepted bids after PG recovery — queued decisions all settled |
| `settlement_replay_no_duplicates` | settlement | P0 | Zero duplicate (epoch, seq) rows — Kafka at-least-once replay was idempotent |
| `settlement_replay_complete` | settlement | P0 | All pre-crash decisions settled — consumer group resumed from committed offset |
| All `verify-l4b-pts-correctness.sh` P0 gates | all | P0 | Final state: winner = highest bid, engine\_seq gap-free, outbox drained |

### Why L2–L4 fault tests are not needed

Fault correctness is structural: the Redis single-writer Lua script serialises all
bid decisions atomically regardless of how many concurrent protocols (WS, reads) are
also running. Adding L2 background traffic to a fault test would not surface new
correctness failure modes — it would only add noise that makes root-cause harder.
**The complete fault argument is L1-F + the individual chaos scripts.**

### Known difference from L1-C1

L1-F uses `ALLOW_MOCK_AUTH=true` (X-Mock headers). L1-C1 uses the full JWT session
pool (`ALLOW_MOCK_AUTH=false`). This is intentional: L1-F tests fault correctness,
not auth pipeline throughput under load.

---

**Purpose**: add one background protocol at a time to the L1-C1 bid load. Attribute
any new latency regression to the specific protocol interaction.

| ID | File | Status | SLA | Note |
|---|---|---|---|---|
| `L2-P1` | `L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx` | CURRENT | bid p99 ≤ 55ms | 1000 bid VU + 500–2000 WS viewers |
| `L2-P2` | `L2-protocol/pts-2p2-bid-plus-reads.jmx` | CURRENT | bid p99 ≤ 55ms | 1000 bid VU + 2000–5000 read VU |
| `L2-P3` | `L2-protocol/pts-2p3-bid-ws-reads.jmx` | CURRENT | bid p99 ≤ 60ms | bid + WS + reads combined |

See `docs/perf/pts/l2-l4-upload-and-pressure-config.md` for upload files and Alibaba PTS/JMeter configuration.
See `docs/current/chaos-test-runbook.md` for L1-F and Toxiproxy fault execution.

**Prerequisite**: L1-C1 must pass before any L2 run.

---

## L3 — Scenario Realism

**Purpose**: test realistic bid arrival curves (not just final-second bursts) and full
30-minute auction lifecycle. Validates outbox delivery, settlement, and memory stability.

| ID | File | Status | SLA | Note |
|---|---|---|---|---|
| `L3-S1` | `L3-scenario/pts-3s1-full-lifecycle-30min.jmx` | CURRENT | bid p99 ≤ 60ms (final 60s); outbox lag ≤ 2s | Full 30-min lifecycle approximation |
| `L3-S2` | `L3-scenario/pts-3s2-multi-room-isolation.jmx` | CURRENT | per-auction p99 ≤ 60ms | 3 concurrent auctions, room isolation |

See `docs/perf/pts/l2-l4-upload-and-pressure-config.md` for upload files and Alibaba PTS/JMeter configuration.

**Prerequisite**: L2-P3 must pass before any L3 run.

---

## L4 — Combined Production

**Purpose**: all traffic types simultaneously. Production-readiness gate before going live.
A regression here that does not appear in L1–L3 points to emergent full-system interaction.

| ID | File | Status | SLA | Note |
|---|---|---|---|---|
| `L4-M1` | `L4-combined/pts-4m1-full-mixed.jmx` | CURRENT | hot bid p99 ≤ 65ms; zero DLQ | ~5200 VU: bid + WS + reads + side auction |

See `docs/perf/pts/l2-l4-upload-and-pressure-config.md` for upload files and Alibaba PTS/JMeter configuration.

**Prerequisite**: L3-S1 and L3-S2 must pass. Run on ECS/production-spec hardware only.

---

## Required Data

| File | Purpose |
|---|---|
| `docs/perf/pts/pts-1ab-1000vu-sessions.csv` | Current 1000-user PTS session pool (L1 + L2) |
| `docs/perf/pts/pts-l2-bidder-1000-sessions.csv` | Current L2-L4 bidder session pool generated by `prepare-l2-protocol-pressure.sh` |
| `docs/perf/pts/pts-l2-viewer-2000-sessions.csv` | Current L2-L4 WebSocket viewer session pool generated by `prepare-l2-protocol-pressure.sh` |
| `docs/perf/pts/pts-l2-reader-5000-sessions.csv` | Current L2-L4 HTTP reader session pool generated by `prepare-l2-protocol-pressure.sh` |
| `tests/pts/pts_sessions.csv.example` | Example CSV shape only |

---

## Utility Scripts

| Script | Role |
|---|---|
| `tests/pts/reset-l4b-final-second-pressure.sh` | Reset/seed for L1-C0/L1-C1 (supports `L4B_PROFILE=pts-1a\|pts-1b`) |
| `tests/pts/prepare-l2-protocol-pressure.sh` | Reset/seed and generate L2 bidder/viewer/reader CSVs |
| `tests/pts/prepare-l3-l4-pressure.sh` | Extend L2 seed with `auc_inv_001` for L3-S2/L4 |
| `tests/pts/preflight-l4b-pts-guards.sh` | Preflight gate: Redis/Kafka/settlement protections |
| `tests/pts/collect-server-evidence.sh` | Post-run server evidence collector |
| `tests/pts/verify-l4b-pts-correctness.sh` | Correctness verifier (ENGINE_* distribution, seq gaps, DLQ) |
| `tests/pts/fetch-pts-sampling-logs.sh` | Optional sampling-log helper |
| `tests/pts/summarize-pts-sampling-logs.sh` | Optional sampling-log summarizer |
| `tests/pts/prepare-cloud-pressure.sh` | Shared seed/session helper used by reset scripts |

---

## Historical / Archived Workloads

See `tests/pts/HISTORICAL.md`. Archived JMX files live under `tests/pts/archive/`.
Do not cite them as current evidence without explicit current-doc promotion.

---

## Interpretation Rules

- HTTP `200` count is not accepted-bid count.
- Business result fields: `ENGINE_ACCEPTED`, `ENGINE_REJECTED`, `ENGINE_SOLD`, `ENGINE_PAUSED`, `RECONCILING`, `PROCESSING_RETRY_LATER`.
- Dominant `PROCESSING_RETRY_LATER`, vague `409`, or second-level pending states fail L1-C1 UX even if settlement later converges.
- A latency number without verifier output is not success evidence.
- A report review that omits runtime profile, `ENGINE_*` distribution, settlement status, verifier output, or fault-injection scope is incomplete.
- Do not use HTTP status alone as auction outcome; inspect `ENGINE_*`, durability, and settlement fields.
