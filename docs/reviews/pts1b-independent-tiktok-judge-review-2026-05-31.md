# PTS-1B Independent TikTok Judge Review

> Date: 2026-05-31
> Scope: independent hostile review against `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md`, `单热点调研.md`, current source code, and current PTS-1B contract.
> Classification: `CURRENT_ADJACENT` review. This is not `CURRENT_PASS` evidence.

## JUDGE VERDICT: FAIL

The implementation has the right industrial direction, but it cannot currently claim "PTS-1B p99 <= 50ms with absolute correctness and fault-injection readiness".

The core issue is not that the Redis/Kafka/PostgreSQL architecture is wrong. The issue is that the current code does not draw a hard enough boundary between Redis live decision, Kafka durable fence, PostgreSQL settlement, and user-visible truth.

## Disqualifiers

- [P0] `backend/internal/redisengine/engine.go` - Redis Lua mutates live state before Kafka append. When the append lock is busy, the code returns `result.response()` to the user while the idempotency record is still `kafka_append_status=UNKNOWN` and `kafka_append_attempted=0`. For high-value auction, this is not a durable user-visible decision boundary.
- [P0] `backend/internal/redisengine/engine.go` - idempotency replay returns the business decision even when `kafka_append_status=UNKNOWN`. A retry can therefore continue to show an `ENGINE_ACCEPTED` style result before durable WAL proof exists.
- [P0] `infra/docker-compose.yml` - Kafka is single broker and bid/DLQ topics are created with replication factor 1. This is acceptable for local demo, but not industrial durable WAL evidence.
- [P0] Redis state loss handling fails closed in some cases, but the complete automatic restore path from checkpoint/Kafka/PG with measured RTO is not implemented as a proven flow.
- [P1] `resume_redis_engine` increments `engine_epoch` and deletes Redis state, but the reviewed code does not prove "replay Kafka offset to checkpoint, verify current price/winner/seq, then resume" as one controlled recovery procedure.
- [P1] H5 can display `ENGINE_* + settlement_status=PENDING`, but the backend response has no explicit `durability_status`, so users and tests cannot distinguish "Kafka acked, PG pending" from "Redis decided, Kafka unknown".

## Claim Audit

| Claim | Code proof | Verdict | Attack |
|---|---|---|---|
| PostgreSQL is no longer the PTS-1B synchronous decision point | Router constructs `redisengine.Engine` for non-`postgres_lane`/`redis_guard` mode and `PlaceBid` delegates to `Engine.PlaceBid`. | Proven | Correct direction, but not enough by itself. |
| Redis Lua atomically decides one hot auction | Lua script validates status, amount, increment, self-leading, cap, soft-close, and mutates price/winner/engine_seq. | Proven | Redis atomicity is live-state atomicity, not durable auction truth. |
| Kafka is the durable decision WAL/fence | Kafka writer uses sync write and `RequiredAcks=RequireAll`. | Partially proven | Single broker RF=1 and responses can be returned before append. |
| Low-price rejects are decision-time based | Rejects include current price/winner and reason. | Partially proven | Missing explicit `required_min_price_cents` in the public response/audit shape weakens verifier and interview defense. |
| Settlement is fenced and idempotent | Settlement uses `engine_seq = previous + 1`, unique settlement keys, and idempotency records. | Strong partial | Depends on complete ordered Kafka append; current early-return gap undermines it. |
| WebSocket reconnect converges to server truth | Server uses `last_seq` recovery and frontend pulls snapshot on gaps/stale state. | Partially proven | Needs fault-injection proof under Redis/Kafka/PG/worker failures. |
| Current PTS-1B success exists | Current evidence policy marks old UF5DX7GG, 0Z57X76G, R652X74G-era evidence as current-adjacent/partial. | False | Cannot combine "fast from one run" and "correct from another". |

## PG Source-Of-Truth Position

It is not a contradiction to remove PostgreSQL from the hot decision path. It is a contradiction to keep calling PostgreSQL the only real-time decision truth.

The defensible wording is:

- Redis is the live hot-state decision state machine.
- Kafka is the immutable durable decision WAL/fence.
- PostgreSQL is the settlement, audit, order, recovery-checkpoint, and long-term query truth.

This can preserve correctness only if a user-visible `ENGINE_ACCEPTED` or `ENGINE_SOLD` either:

1. already has a Kafka durable fence; or
2. is explicitly shown as not durable and not final, with dangerous actions blocked.

The current code sometimes returns a normal engine business result while Kafka status is unknown. That is the unacceptable part.

## Scorecard

| Dimension | Score / 10 | Reason |
|---|---:|---|
| Technical implementation and completeness | 6 | End-to-end pieces exist, but durable boundary and recovery closure are weak. |
| Availability, performance, stability, observability | 4 | Diagnostics exist; current pass and fault gates do not. |
| Technical depth and innovation | 6 | Redis Lua + Kafka + settlement is a strong direction, but currently resembles an unfinished transaction engine. |
| High-concurrency hard optimization | 5 | PostgreSQL hot-row bottleneck is addressed architecturally; p99 <= 50ms is not proven by current evidence. |
| Correctness and money safety | 4 | Settlement is serious, but user-visible early return ahead of WAL proof is a P0. |
| Interview defense | 4 | A reviewer will break the design with: "Redis changed, Kafka unknown, process crashes. What did the buyer see?" |

## Mock / Hardcode / Demo-Only Inventory

- `infra/docker-compose.yml` Kafka RF=1: local-only, not production durability proof.
- `frontend/mobile-h5/src/main.tsx` demo video/image defaults: acceptable for official brief's simulated live video, not a backend correctness issue.
- `frontend/pc-console/src/main.tsx` host demo competing-bid API: acceptable only if labeled demo; not PTS-1B evidence.
- Historical/current-adjacent PTS reports: useful diagnosis, not pass evidence.

## Interview Grill

- Show exactly where a Redis decision becomes durable before the user sees accepted. Expected answer: a code path where Kafka ack happens before accepted is returned, or a non-final pending state is returned. Current code cannot fully defend this.
- If Redis live state is lost after `ENGINE_ACCEPTED` but before Kafka append, what does the user see and how is the bid recovered? Expected answer: fail closed, no final success, deterministic recovery. Current code has an unsafe window.
- Why is PostgreSQL no longer the hot-path truth, and why does that not weaken correctness? Expected answer: Redis state machine + Kafka WAL + PG settlement/checkpoint with invariant verifier. Must stop saying PG is the only real-time truth.
- How do you prove every low reject was fair? Expected answer: decision-time current price, required min, reason, engine_seq, request hash, and verifier output for all 1000 users.
- What is the RTO for Redis `FLUSHALL`, Kafka restart, settlement worker crash, and PostgreSQL latency? Expected answer: measured fault-injection evidence, not prose.

## Required Fixes Before Next Claim

- [P0] Rework the hot-path user boundary. Either write Kafka command first and let a single auction processor decide, or keep Redis Lua but do not return `ENGINE_ACCEPTED/SOLD` as user-visible business result until Kafka append is acked. `UNKNOWN` must not look like success.
- [P0] Add explicit `decision_status`, `durability_status`, and `settlement_status` to the bid response. `KAFKA_UNKNOWN` must map to recovering/uncertain UI, not accepted-leading UX.
- [P0] Implement and prove Redis loss recovery: pause, replay Kafka from checkpoint, compare PG settlement and checkpoint hash, rebuild Redis, bump epoch, resume, record RTO.
- [P0] Define production Kafka posture separately from local compose: RF=3, `min.insync.replicas=2`, `acks=all`, idempotent producer, auction-id keying, and one-partition ordering per auction.
- [P0] Re-run current PTS-1B three times with current JMX, 1000 unique users, `ENGINE_*` distribution, verifier output, Redis/Kafka/PG metrics, and fault-injection gates.
- [P1] Add explicit low-reject basis to engine result and verifier: previous price, required min price, current price, reason, engine_seq.
- [P1] Make PC diagnostics show durability status and recovery RTO, not just append counters.
- [P1] Add tests for process crash / Redis state loss between Lua decision and Kafka append, and assert no accepted user truth survives without WAL proof.

## Final Position

The project can still become a strong differentiator if it stops trying to patch around the old fast-vs-correct loop and instead hardens the decision boundary. The defensible architecture is not "PostgreSQL as real-time truth". It is "Redis decides live, Kafka proves the decision, PostgreSQL settles and audits, and every uncertainty fails closed before the user is allowed to believe they won."

Until that is implemented and proven, the current project is not a pass against the official high-concurrency correctness challenge.
