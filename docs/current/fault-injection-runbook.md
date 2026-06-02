# Fault Injection And Correctness Runbook

> Status: current fault-gate runbook, 2026-05-31.

This runbook defines what must be proven before claiming Redis/Kafka/PostgreSQL
failure readiness for high-value auction bidding. A prose statement such as
"Redis can rebuild from Kafka" is not evidence.

## Governing Contract

Current hot bid boundary:

```text
Redis live decision state
-> Redis Stream/idempotency replay record (`ENGINE_DURABLE` response boundary)
-> Kafka relay/WAL/fence
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

This table is the fault readiness checklist. The layered execution plan is in
`docs/current/fault-test-matrix.md`.

| Fault | User contract | Required system behavior | Pass evidence |
|---|---|---|---|
| Redis process restart, data retained | no wrong accept; bounded pause/reconcile if needed | reconnect, reload/check hot state, verify against Kafka/PG, resume only when safe | no wrong winner, no missing accepted decision, verifier pass |
| Redis data loss / `FLUSHALL` | fail closed before accepting unsafe bids | pause auction or reject with explicit paused/reconciling state, rebuild from checkpoint/Kafka/PG, verify before resume | RTO measured; no bid accepted against unverified state |
| Kafka relay timeout/broker restart | no `ENGINE_DURABLE` decision is lost; relay lag is visible | Redis Stream retains decisions; pending count/lag/DLQ visible; relay drains after restart or auction pauses/reconciles | Redis pending drains, Kafka lag/DLQ clean, verifier pass |
| settlement worker crash/restart | accepted engine decisions eventually settle once | Kafka offsets/engine_seq remain contiguous; idempotent settlement resumes | no duplicate public seq/order; verifier pass |
| PostgreSQL latency/restart | no wrong settlement/order | hot engine may pause or expose pending; settlement catches up after PG returns | no stale epoch/seq writes; no unresolved settlement gap |
| WebSocket reconnect storm during bids | clients converge to server truth | history/snapshot recovery returns current price/winner/status | no client-side winner/hammer; server timeline authoritative |

## ENGINE_DURABLE Makes Fault Gates Stricter

Because the HTTP hot path returns at `ENGINE_DURABLE`, Kafka and PostgreSQL are
not allowed to be vague "eventual" promises. Every fault run must prove:

- Redis AOF/no-eviction and Redis Stream retention protect decisions before relay;
- Redis pending decisions are bounded and observable;
- Kafka relay drains after recovery, or the auction stays paused/reconciling;
- PostgreSQL settlement applies every relayed decision exactly once;
- outbox/WebSocket projection catches up or reports a gap/recovery state;
- reconciler detects mismatches and prevents dangerous bidding until safe.

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

The operator resume procedure is not a manual "clear pause" shortcut. A
`resume_redis_engine` signal must:

1. run reconcile preflight;
2. drain Redis pending decisions into Kafka or fail closed;
3. load the latest `auction_engine_checkpoints` snapshot;
4. compare checkpoint hash, engine epoch, engine seq, public seq, current price,
   winner, and terminal status against PostgreSQL settlement state;
5. rebuild the Redis hot-state hash from the verified snapshot;
6. run reconcile postflight;
7. unpause only if postflight is `OK`;
8. record `rto_ms` in the signal result and expose it through Redis Engine diagnostics.

If the checkpoint is missing after any Kafka-backed settlement, resume must fail
and keep the auction paused.

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
- Redis Stream/idempotency uncertainty is hidden behind a normal `ENGINE_*` response;
- Kafka relay lag, DLQ, or pending Redis decisions remain unresolved after the recovery window;
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
