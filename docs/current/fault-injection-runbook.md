# Fault Injection And Correctness Runbook

> Status: current fault-gate runbook, 2026-05-31.

This runbook defines what must be proven before claiming Redis/Kafka/PostgreSQL
failure readiness for high-value auction bidding. A prose statement such as
"Redis can rebuild from Kafka" is not evidence.

## Governing Contract

Current hot bid boundary:

```text
Redis live decision state
-> Kafka durable decision WAL/fence
-> PostgreSQL settlement/audit/order truth
-> outbox/realtime projection
-> reconciler verifies and fails closed on uncertainty
```

Fault tests must prove user-visible behavior, not just internal eventual repair.
For jewelry/high-value auctions, silent wrong winner, missing bid, stale winner,
or seconds-long ambiguous confirmation is a failed gate.

## Required Evidence For Every Fault Run

- runtime profile/env source;
- reset/preflight label;
- fault start/end timestamp;
- exact command used to inject and clear the fault;
- user-visible result distribution: `ENGINE_*`, HTTP status, settlement status;
- Redis/Kafka/PostgreSQL health evidence before/during/after;
- verifier output under `docs/perf/pts/evidence/incoming/<label>/`;
- run review using `docs/current/pts-run-review-template.md`;
- final classification: `CURRENT_PASS`, `CURRENT_FAILING`, `CURRENT_ADJACENT`, `HISTORICAL`, `HARNESS_ONLY`, or `RAW_ARTIFACT`.

## Fault Matrix

| Fault | User contract | Required system behavior | Pass evidence |
|---|---|---|---|
| Redis process restart, data retained | no wrong accept; bounded pause/reconcile if needed | reconnect, reload/check hot state, verify against Kafka/PG, resume only when safe | no wrong winner, no missing accepted decision, verifier pass |
| Redis data loss / `FLUSHALL` | fail closed before accepting unsafe bids | pause auction or reject with explicit paused/reconciling state, rebuild from Kafka/PG, verify before resume | RTO measured; no bid accepted against unverified state |
| Kafka append timeout/broker restart | no final success without durable fence | return explicit pending/paused/reconciling state or fail closed; no silent Redis-only success | pending drains or auction pauses; DLQ/lag evidence clean after recovery |
| settlement worker crash/restart | accepted fenced decisions eventually settle once | Kafka offsets/engine_seq remain contiguous; idempotent settlement resumes | no duplicate public seq/order; verifier pass |
| PostgreSQL latency/restart | no wrong settlement/order | hot engine may pause or expose pending; settlement catches up after PG returns | no stale epoch/seq writes; no unresolved settlement gap |
| WebSocket reconnect storm during bids | clients converge to server truth | history/snapshot recovery returns current price/winner/status | no client-side winner/hammer; server timeline authoritative |

## Redis Data Loss Is Not Automatically Safe

Redis loss followed by Kafka rebuild is only acceptable if the system:

1. detects Redis state uncertainty before accepting more live bids;
2. pauses or switches to explicit reconciling/fail-closed semantics;
3. replays Kafka/PG state into Redis;
4. verifies engine epoch/seq/current price/winner/end_at;
5. resumes only after verification passes;
6. records RTO and user-visible impact.

If any bid is accepted while Redis state is unverified, the run fails even if a
later rebuild converges.

## Suggested Local Injection Commands

Use only on dedicated pressure data.

```bash
# Redis process restart, data retained
docker restart live-auction-redis

# Redis full data loss
docker exec live-auction-redis redis-cli FLUSHALL

# Kafka broker restart
docker restart live-auction-kafka

# PostgreSQL restart
docker restart live-auction-postgres
```

For latency/packet-loss injection, use the existing toxiproxy assets or a
dedicated network proxy. Record the exact toxic configuration.

## Required Run Order

1. Reset current profile:

```bash
L4B_PROFILE=pts-1b SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
```

2. Preflight:

```bash
BASE_URL=http://127.0.0.1:18080 bash tests/pts/preflight-l4b-pts-guards.sh fault-<name>-preflight
```

3. Start workload or focused bid script.

4. Inject fault at a recorded timestamp.

5. Collect evidence:

```bash
BASE_URL=http://127.0.0.1:18080 bash tests/pts/collect-server-evidence.sh fault-<name>-after
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh fault-<name>-after
```

6. Write run review from `docs/current/pts-run-review-template.md`.

## Failure Rules

Classify as `CURRENT_FAILING` if any occurs:

- final winner is not highest valid amount;
- any accepted decision is missing from Kafka/PG after recovery window;
- any low reject lacks a decision-time basis;
- Redis loss does not pause/reconcile before new accepts;
- Kafka append uncertainty is hidden behind a normal success response;
- settlement leaves unresolved gaps, DLQ, pending append, or engine pause;
- user-visible state remains vague `409` / `PROCESSING_RETRY_LATER` at scale;
- RTO is unmeasured.

## Minimum Fault Readiness Claim

A final submission may claim fault readiness only after at least:

- Redis restart retained data: pass;
- Redis data loss: pass or explicitly documented fail-closed with measured RTO;
- Kafka restart/timeout: pass or explicitly documented fail-closed with measured RTO;
- settlement worker restart: pass;
- PostgreSQL restart/latency: pass or explicitly documented user-visible pause;
- WebSocket reconnect storm during or after pressure: pass.
