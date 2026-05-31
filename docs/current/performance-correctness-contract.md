# Performance And Correctness Contract

> Status: governing PTS-1B contract, 2026-05-31.

## Target

PTS-1B means the final-second contention burst: 1000 users bid against one hot auction in the last second.

Passing target:

- user-visible bid decision p99 <= 50ms;
- no wrong winner;
- no unjustified low-price reject;
- no duplicate accepted decision for one idempotency key/request hash;
- no hidden data loss across Redis/Kafka/PostgreSQL/outbox;
- fault-injection proof for Redis, Kafka, PostgreSQL, worker crash, reconnect, and replay.

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
  "durability_status": "KAFKA_ACKED",
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
- `ENGINE_ACCEPTED`, `ENGINE_REJECTED`, and `ENGINE_SOLD` may be returned only after `durability_status = KAFKA_ACKED`.
- `durability_status = KAFKA_UNKNOWN` must not be paired with an `ENGINE_*` result. It must return an explicit pending/reconciling response such as `PROCESSING_RETRY_LATER` with `decision_status = PENDING_DURABILITY`, and the UI must not show winner/leading truth from it.
- `durability_status = KAFKA_FAILED` must fail closed as `RECONCILING` / `ENGINE_PAUSED`; dangerous bid actions remain disabled until reconciliation proves safety.
- `202` is acceptable only for explicit pending durability or pending settlement semantics. It must not hide a normal success before the Kafka fence.
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
- correctness verifier output;
- failure-injection result if claiming resilience;
- explicit statement whether the result is current evidence or historical evidence.

Old PG-lane, Redis-guard, unsafe fast Redis, or failed Kafka-fence runs cannot prove the current system. They may only be used as historical bottleneck evidence.

### Latency Semantics

For PTS-1B, the primary p99 is the user-visible final business decision latency:

```text
bid click/request start -> final ENGINE_ACCEPTED / ENGINE_REJECTED / ENGINE_SOLD response visible to the user
```

HTTP `202` latency is not final bid-decision latency. It is only acceptance or
enqueue latency:

```text
bid click/request start -> PROCESSING_RETRY_LATER / PENDING_DURABILITY acknowledgement
```

A run dominated by `202` may prove that the ingress path accepted load, but it
does not prove the bid engine delivered final user-visible decisions at that
p99. Do not cite `202` RTT as PTS-1B decision p99, capacity, or user experience
success.

Every PTS-1B report must separate:

- `accept_latency_ms`: HTTP request to `202` pending acknowledgement;
- `final_decision_latency_ms`: request start to final `ENGINE_*`;
- `settlement_latency_ms`: Kafka durable decision to PostgreSQL settlement;
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
| Kafka append timeout/unknown | do not claim durable success; expose pending/paused according to contract |
| Kafka unavailable | fail closed or bounded pending with worker proof; no unbounded silent queue |
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
