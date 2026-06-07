# Performance And Correctness Contract

> Status: governing PTS-1B contract, 2026-06-07.

## Target

PTS-1B means the final-second contention burst: 1000 users bid against one hot auction in the last second.

Passing target:

- user-visible bid decision p99 <= 60ms in the current `kafka_ack` response
  profile, with `KAFKA_ACKED` ratio >= 99% and bounded `ENGINE_DURABLE`
  responses that later converge to Kafka/PostgreSQL;
- no wrong winner;
- no unjustified low-price reject;
- no duplicate accepted decision for one idempotency key/request hash;
- no hidden data loss across Redis decision log, Kafka relay, PostgreSQL settlement, and outbox;
- fault-injection proof for Redis, Kafka, PostgreSQL, worker crash, reconnect, and replay.

## Synchronous Decision Boundary

### Current boundary — `KAFKA_ACKED` with bounded Redis-AOF response result

```text
HTTP bid request
  -> Redis Lua atomic decision
  -> Redis Stream decision log + idempotency record persisted as ENGINE_DURABLE
  -> triggerRelayForAuction [non-blocking]
  -> relayAuctionLogBatch: XREAD -> AppendBatch(acks=all) -> idem KAFKA_ACKED
  -> group-commit latch unblocks handler
  -> HTTP returns ENGINE_ACCEPTED / ENGINE_REJECTED / ENGINE_SOLD
     normally with durability_status=KAFKA_ACKED
     or with durability_status=ENGINE_DURABLE on timeout/fail-fast/circuit-open
```

This boundary is pinned in code; `BID_ENGINE_RESPONSE_DURABILITY` is no longer a
runtime switch. Kafka fault protection uses fail-fast wakeup and
`kafkaRelayUnhealthy` circuit breaker so Kafka faults degrade to `ENGINE_DURABLE`
without adding seconds of tail latency. An `ENGINE_DURABLE` response is still a
final `ENGINE_*` decision, not an undecided request. The run can only be called
correct after relay, Kafka lag, settlement, and outbox gates prove convergence.

### Evidence requirements

Fault evidence must prove: Redis AOF/no-eviction, Redis Stream retention, relay
drain, Kafka lag/DLQ, settlement replay idempotency, checkpoint/reconcile, and
outbox drain before a run can be classified as current pass.

## Correctness Invariants

| Invariant | Required proof |
|---|---|
| Highest valid amount wins | verifier compares all valid engine decisions and final settled winner |
| Low reject is justified | each low reject records current required price/current price basis at decision time |
| Monotonic decisions | engine_seq is gap-free or gaps are explicitly reconciled and fenced |
| Idempotency | same key + same hash replays same result; same key + different hash conflicts |
| Terminal uniqueness | SOLD/CANCELLED/ENDED terminal transition happens once |
| Settlement coverage | every durable engine decision is settled, pending with bounded reason, or escalated to DLQ/pause |
| No fake client truth | client never decides winner, terminal state, or accepted success locally |
| Recovery honesty | uncertain engine state disables dangerous bid actions |

## Response Contract

The bid response must separate business decision from settlement status.

Recommended fields:

```json
{
  "result": "ENGINE_ACCEPTED",
  "engine_seq": 123,
  "decision_status": "DECIDED",
  "durability_status": "ENGINE_DURABLE",
  "settlement_status": "PENDING",
  "current_price_cents": 65000,
  "current_winner_id": "user_42",
  "decision_basis": {
    "previous_price_cents": 64000,
    "required_min_price_cents": 65000
  }
}
```

Rules:

- `ENGINE_*` is the user-visible business decision.
- `settlement_status` is not the decision.
- `durability_status` values in ascending order: `ENGINE_DURABLE` (Redis-AOF-local) -> `KAFKA_ACKED` (Kafka append acknowledged). Both indicate a final `DECIDED` response.
- `ENGINE_DURABLE` does not mean Kafka quorum or PostgreSQL settlement has completed. The response must expose `settlement_status`, and post-run evidence must prove relay/settlement/outbox convergence.
- `KAFKA_ACKED` means AppendBatch `acks=all` confirmed. In local single-broker PTS it means the local broker acknowledged the batch; in production it only deserves replicated quorum semantics when Kafka is configured with RF=3 and `min.insync.replicas=2`. It does not mean PostgreSQL settlement has completed.
- If the Redis decision log/idempotency record cannot be written, the system must fail closed as `RECONCILING` / `ENGINE_PAUSED`. It must not fabricate an `ENGINE_*` result.
- Kafka relay failure after `ENGINE_DURABLE` must surface through lag, pending decisions, DLQ, pause/reconcile, and verifier gates. It is not part of M1 latency, but it is part of M3/M5 correctness.
- In `kafka_ack` mode, `ENGINE_DURABLE` fallback (timeout or fault-triggered) is NOT an error or data loss. The decision was already written to Redis AOF + Stream + idem record before the response was sent; Kafka relay confirms asynchronously. Post-run correctness evidence must show these decisions appear in the settled count with `kafka_lag=0`.
- In `kafka_ack` mode, Kafka fault degrades to `ENGINE_DURABLE` (fail-fast or circuit-breaker); decision correctness is preserved.
- `202` is acceptable only for explicit pending engine durability or pending settlement semantics. It must not hide a normal success before the Redis decision-log boundary.
- Normal PTS-1B must not degrade into bulk `PROCESSING_RETRY_LATER`, vague `409`, or second-level waiting.

## Performance Evidence Rules

Every current PTS-1B claim must include:

- report ID and time;
- git SHA and dirty-tree note;
- JMX path and CSV path;
- full PTS parameters;
- reset/preflight command;
- server metrics snapshot;
- Redis/Kafka/PostgreSQL diagnostics;
- business distribution by `ENGINE_*`, HTTP status, and settlement status;
- durability distribution: current `kafka_ack` mode — `KAFKA_ACKED` ≥ 99% and bounded `ENGINE_DURABLE` ≤ 1% (0.2% observed in S1 UIPAX7JG; this is correct graceful degradation, not data loss — see runtime-profiles.md for full analysis); `auction_kafka_ack_wait_timeout_total{reason=timeout|circuit_open|fail_fast}` distribution recorded and interpreted;
- Redis pending-decision count, Kafka lag/DLQ, settlement gap, and outbox backlog after convergence;
- correctness verifier output;
- failure-injection result if claiming resilience;
- explicit statement whether the result is current evidence or historical evidence.

Old PG-lane, Redis-guard, unsafe fast Redis, or failed Kafka-fence runs cannot prove the current system. They may only be used as historical bottleneck evidence.

### Latency Semantics

For PTS-1B, the primary p99 is the user-visible final business decision latency:

```text
bid click/request start -> final ENGINE_ACCEPTED / ENGINE_REJECTED / ENGINE_SOLD response visible to the user
```

HTTP `202` latency is not final bid-decision latency. It is only pending
acknowledgement latency:

```text
bid click/request start -> PROCESSING_RETRY_LATER / PENDING_DURABILITY acknowledgement
```

A run dominated by `202` may prove that the ingress path accepted load, but it
does not prove the bid engine delivered final user-visible decisions at that
p99. Do not cite `202` RTT as PTS-1B decision p99, capacity, or user experience
success.

Every PTS-1B report must separate:

- `accept_latency_ms`: HTTP request to `202` pending acknowledgement;
- `final_decision_latency_ms`: request start to final `ENGINE_*` with the current response boundary (`KAFKA_ACKED` normally, bounded `ENGINE_DURABLE` response on timeout/fault);
- `relay_latency_ms`: Redis Stream decision to Kafka append acknowledgement;
- `settlement_latency_ms`: Redis/Kafka durable decision to PostgreSQL settlement;
- `pending_ratio`: share of requests returning pending durability;
- `timeout_ratio`: share of requests that do not reach final `ENGINE_*` inside
  the measurement window.

If the first response is `202`, the load script or post-run evidence must keep
using the same `client_bid_id` / idempotency key until it observes final
`ENGINE_*` or timeout. A PTS sampler that stops at `202` can only be used for
ingress/queueing evidence.

## Failure-Injection Gates

| Fault | Required behavior |
|---|---|
| Redis unavailable before decision | fail closed or safe fallback; no fabricated accept |
| Redis state lost | pause affected auction, rebuild from Kafka/PG, verify, then resume |
| Kafka relay timeout/unknown after ENGINE_DURABLE | decision remains replayable from Redis Stream; relay lag is visible; auction may pause/reconcile if lag or uncertainty exceeds bound |
| Kafka unavailable | no loss of `ENGINE_DURABLE` decisions; bounded Redis pending backlog; relay drains after recovery or auction remains paused/reconciling; in `kafka_ack` mode: fail-fast wakeup + circuit breaker ensure zero extra handler latency — responses degrade gracefully to `ENGINE_DURABLE` |
| Settlement worker crash | decisions remain replayable from Kafka; settlement resumes idempotently |
| PostgreSQL unavailable | live engine may not overclaim final settlement; orders/audit wait safely |
| Reconciler mismatch | pause, surface anomaly, and block dangerous actions |
| Client reconnect/gap | history/snapshot recovery or stale/recovering UI; no local success |

## Why One `200` Is Not Automatically A Failure

HTTP `200` count is not accepted-bid count. A run can have one `200` and still have many valid engine decisions if the contract returns decisions through `202`. Conversely, a run with many `200`s can still fail if the winner is wrong, rejects are unjustified, or settlement is unsafe.

The scoring evidence must report business results, not just HTTP statuses:

- `ENGINE_ACCEPTED`
- `ENGINE_REJECTED`
- `ENGINE_SOLD`
- `ENGINE_PAUSED`
- `RECONCILING`
- `PROCESSING_RETRY_LATER`

For PTS-1B, `PROCESSING_RETRY_LATER` should be exceptional, not the dominant normal user experience.
