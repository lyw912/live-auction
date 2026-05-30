# Route B+ Implementation Plan

Date: 2026-05-30

Status: ready-to-implement plan, split into phases.

Design source:

- `docs/perf/pts/l4b-kafka/single-hotspot-redesign-from-first-principles-2026-05-30.md`

Goal:

Move Redis hot-engine HTTP success behind a synchronous Kafka decision append,
then harden idempotency, failure handling, settlement, observability, and PTS
evidence until the system can defend high-value live auction correctness.

## Current Implementation Facts

Current hot path:

```text
Engine.PlaceBid
  -> completedReplay from PostgreSQL idempotency_records
  -> load Redis/DB snapshot
  -> Redis Lua decision writes hot state + idempotency + pending hash
  -> return result.response()

Worker.ProcessPendingAppends
  -> later reads Redis pending hash
  -> KafkaLedger.Append
  -> HDEL pending only after append succeeds

Worker.ProcessKafka
  -> later settles Kafka messages into PostgreSQL
```

Reusable pieces:

- `engine_epoch`, `engine_seq`, `engine_paused`;
- Redis Lua hot state and pending hash;
- Kafka writer already uses `RequiredAcks: kafka.RequireAll`, `Async: false`,
  `BatchSize: 1`;
- `redis_engine_settlements` table;
- settlement worker;
- reconciliation and pause concepts;
- PTS correctness scripts.

Primary gaps:

- HTTP success currently returns before Kafka append.
- DB idempotency replay only works after PostgreSQL settlement completed.
- Kafka append unknown outcome is not modeled in the API.
- Kafka decision offsets are not persisted as recovery checkpoint.
- H5/PC must treat `ENGINE_ACCEPTED` as pending settlement, not final audit.
- Failure tests do not yet prove Redis-decision/Kafka-append crash windows under
  the new contract.

## Phase 0: Contract Freeze

No code behavior change yet.

### P0 Decisions

| Decision | Value |
|---|---|
| Final success boundary | `Redis Lua decision && Kafka append ack`. |
| Redis-only success | forbidden for high-value auction. |
| PostgreSQL settlement | async, visible as `settlement_status=PENDING`. |
| Unknown append result | retry/reconciling, never success. |
| Rejected bids | also decision records; otherwise audit and idempotency are incomplete. |
| Same-auction order | Redis `engine_seq` is source; Kafka records must preserve that sequence. |
| Uncertainty | pause/reconcile affected auction. |

### Kafka Partition And Consumer Contract

Kafka is a per-auction ordered WAL, not a global serial worker.

| Area | Contract | Failure if violated |
|---|---|---|
| topic partitions | `auction.bid-events` has a fixed partition count before PTS; current preflight requires `16` for course pressure runs. | Auto-created or single-partition topics turn settlement into a global bottleneck. |
| producer key | every decision uses `key = auction_id`; kafka-go `Hash` balancer maps that key to one partition. | Same auction can be split across partitions and settle out of order. |
| consumer group | all settlement workers use `GroupID = settlement-workers`. | Independent consumers can process the same partition concurrently and race settlement. |
| partition ownership | Kafka assigns each partition to at most one consumer in the group; one worker may own many partitions if worker count is lower than partition count. | More consumers than partitions gives idle consumers; fewer consumers increases per-worker backlog but preserves order. |
| auction cardinality | `auction_count > partition_count` is legal. Multiple auctions share a partition, but each auction's messages remain ordered because their key is stable. | A hot auction can cause head-of-line lag for other auctions on the same partition; monitor group lag by partition. |
| offset commit | commit only after PostgreSQL settlement transaction commits. | Committing before DB commit loses a Kafka decision on crash. |
| crash recovery | consumer group resumes from the last committed offset; replay is expected. | Settlement must dedupe by `(auction_id, engine_epoch, engine_seq, payload_sha256)` and Kafka `(topic,partition,offset)`. |

Consumer shape:

```go
consumerGroup := "settlement-workers"
partitionStrategy := "hash(auction_id) % partition_count"
// Kafka group assignment gives one active consumer per partition.
// Per-auction order is additionally fenced in settlement by engine_seq == db_seq+1.
```

Preflight must capture:

- topic partition count;
- replication factor and ISR health;
- `settlement-workers` group lag by partition;
- `kafka_offset_matches_engine_order` from the correctness verifier.

### API Semantics

| Case | HTTP status | Business code/result | User message |
|---|---:|---|---|
| Redis decision + Kafka ack | `200` | `ENGINE_ACCEPTED/ENGINE_REJECTED/ENGINE_SOLD`, `settlement_status=PENDING` | engine result, settlement pending. |
| Redis Lua rejects before decision with business error | `409/400` | existing business code | no success. |
| Kafka append fails definitely | `503` or `409` | `ENGINE_RECONCILING` / `ENGINE_PAUSED` | bid not confirmed; auction recovering. |
| Kafka append unknown/timeout | `202` or `409` | `BID_CONFIRMATION_PENDING` or `PROCESSING_RETRY_LATER` | retry with same idempotency key. |
| Replay after Kafka ack before PG settlement | `200` | same engine response from Redis/Kafka pending idempotency | same as original response. |
| Replay after PG settlement | `200` | same response from DB idempotency | existing behavior. |

Recommended first implementation: use existing `CodeProcessingRetryLater`,
`CodeEngineReconciling`, and `CodeEnginePaused` where possible; add a new code
only if UI needs to distinguish Kafka-unknown from normal processing.

### Config Flags

Add or document:

```text
BID_ENGINE_MODE=redis_ledger
BID_ENGINE_KAFKA_ACK_BEFORE_RETURN=true
BID_ENGINE_KAFKA_APPEND_TIMEOUT=750ms
BID_ENGINE_PAUSE_ON_APPEND_FAILURE=true
BID_ENGINE_MAX_PENDING_SETTLEMENT_LAG=5s
```

Keep the old async mode only as `BID_ENGINE_KAFKA_ACK_BEFORE_RETURN=false` for
diagnostic comparison, not final evidence.

### Kafka Acks, Timeout, And Degradation

Current writer policy is:

```go
RequiredAcks: kafka.RequireAll
Async:        false
BatchSize:    1
MaxAttempts:  10
```

This deliberately pays append latency for durability. It also means ISR trouble
is a correctness signal, not a reason to silently downgrade:

| Condition | Expected behavior |
|---|---|
| all ISR healthy | append returns ACK, Redis idempotency status becomes `ACKED`, HTTP can return engine result. |
| append returns definite error | Redis status becomes `FAILED`, HTTP returns reconciling, auction pauses. |
| append context times out / outcome unknown | Redis status remains or becomes `UNKNOWN`, HTTP returns `PROCESSING_RETRY_LATER`, auction pauses. |
| broker loses required ISR | writer eventually returns error or times out; do not switch to async or `acks=1` for high-value evidence. |

`BID_ENGINE_KAFKA_APPEND_TIMEOUT=750ms` bounds the HTTP success boundary. The
writer's own `WriteTimeout` remains longer than the request boundary; the engine
context is the effective product contract. A timeout is not a failed bid and not
a successful bid: the same idempotency key must replay `PROCESSING_RETRY_LATER`
until reconciliation proves ACKED or FAILED.

## Phase 1: Move HTTP Success Behind Kafka Ack

### Code Changes

Target: `backend/internal/redisengine/engine.go`.

Current `PlaceBid` ends with:

```go
ledgerRunner.Record("ok", time.Since(start))
recordDecision(result.Result, time.Since(start))
return result.response(), nil
```

Change to:

```text
1. Run Redis Lua.
2. Decode `engineResult`.
3. Append `engineResult` to Kafka synchronously in the same HTTP request.
4. Only after append success:
   - record success metrics;
   - return result.response().
5. If append fails:
   - do not return engine success;
   - mark auction reconciling/paused;
   - keep Redis pending decision;
   - return retry/reconciling error.
```

Implementation sketch:

```go
appendCtx, cancel := context.WithTimeout(ctx, e.kafkaAppendTimeout())
msg, appendErr := e.ledger.Append(appendCtx, result)
cancel()
if appendErr != nil {
    observability.Inc("auction_bid_kafka_append_total", labels("status", "error"))
    _ = e.pause(ctx, auctionID, "KAFKA_APPEND_FAILED_BEFORE_RETURN", appendErr.Error(), traceID)
    return auction.BidResponse{}, apierrors.New(
        apierrors.CodeEngineReconciling,
        "bid engine decision is recovering; retry with the same idempotency key",
        http.StatusConflict,
    )
}
observability.Inc("auction_bid_kafka_append_total", labels("status", "ok"))
observability.Observe("auction_bid_kafka_append_seconds", ...)
return result.response(), nil
```

Important: `Append` must be idempotency-aware enough for retries. Kafka itself
may store duplicates under some failure modes unless transactional/idempotent
producer semantics are configured. Therefore downstream settlement must dedupe
by business key, not by Kafka offset.

### Pending Deletion

Do not delete Redis pending in `PlaceBid` immediately after Kafka append in
Phase 1. Keep current worker deletion after append/recovery until replay safety
is proven. The pending hash remains a recovery buffer.

Later optimization can record an `appended` marker or delete only if:

- Kafka append returned ack;
- the exact ledger id / decision id is stored in Redis idempotency;
- duplicate append handling is fully tested.

### Worker Changes

`ProcessPendingAppends` must become idempotent with already-appended decisions.
Options:

1. Append duplicate and rely on settlement unique constraints.
2. Store an append marker in Redis idempotency and skip duplicate append.
3. Add compacted decision-index topic/table by `auction_id + engine_epoch + engine_seq`.

Phase 1 recommendation: allow duplicate Kafka append but make settlement and
reconcile robust. This is simpler and safer than losing a decision by deleting
pending too early.

Settlement must treat duplicate Kafka messages with identical payload as
idempotent `SKIPPED/SETTLED`, not fatal.

### Kafka Message Identity

Extend Kafka headers/value if missing:

```text
decision_id = auction_id + ":" + engine_epoch + ":" + engine_seq
auction_id
engine_epoch
engine_seq
client_bid_id
request_hash
trace_id
result
server_time_ms
```

The current `ledgerID()` likely covers `auction_id/epoch/seq`; confirm and make
it explicit in tests.

### Metrics

Add:

```text
auction_bid_kafka_append_seconds{result,status}
auction_bid_kafka_append_total{result,status}
auction_bid_kafka_append_fail_total{reason}
auction_bid_http_stage_seconds{stage="redis_lua|kafka_append|total"}
auction_bid_engine_pause_total{reason}
```

`auction_bid_redis_ledger_seconds` should remain Redis Lua time only, not total
HTTP time.

### Phase 1 Tests

Unit/integration:

- Redis Lua success + Kafka append success returns `ENGINE_ACCEPTED`.
- Kafka append failure after Redis success returns no engine success.
- Kafka append failure leaves pending decision.
- Kafka append failure pauses/reconciling auction.
- Duplicate retry after append success but before PG settlement returns same
  engine result from Redis idempotency, not `PROCESSING_RETRY_LATER`.
- Worker appending the same pending decision twice does not create two accepted
  bids/orders after settlement.

Focused failure injection:

- fake ledger that fails before writing;
- fake ledger that writes then returns timeout/unknown;
- fake ledger duplicate append.

## Phase 2: Idempotency Before PostgreSQL Settlement

Current replay checks PostgreSQL `idempotency_records`, which only completes
after settlement. Under Route B, there is a legal window:

```text
Kafka ack succeeded
HTTP response lost
PG settlement not done yet
client retries same idempotency key
```

The retry must return the same engine result.

### Required Redis Idempotency Contract

Redis idempotency key must store:

```text
request_hash
result_json
engine_epoch
engine_seq
kafka_append_status = ACKED | UNKNOWN | FAILED
kafka_topic
kafka_partition optional
kafka_offset optional if client exposes it
expires_at
```

Replay order:

```text
1. Check PostgreSQL completed idempotency.
2. Check Redis engine idempotency.
   - same hash + ACKED: return same engine response.
   - same hash + UNKNOWN: return retry/reconciling, do not create new decision.
   - same hash + FAILED/PAUSED: return engine recovering.
   - different hash: reject key reuse.
3. If neither exists, execute new decision.
```

This may require adding a Redis-side replay method separate from current Lua
script replay, or extending the Lua script result to expose append status.

### Why This Matters

Without pre-settlement replay, moving HTTP success behind Kafka ack can still
produce bad UX:

- user receives network timeout after Kafka ack;
- retry sees no PG row yet;
- system returns "processing" or creates a duplicate attempt.

For a high-value auction, retry must be deterministic.

## Phase 3: Settlement Idempotency And Offset Safety

### Settlement Rules

`settlePayload` must be safe for duplicates:

| Duplicate scenario | Required behavior |
|---|---|
| same Kafka decision same offset | no-op after first settlement. |
| same decision different offset | no-op if payload hash matches existing settlement. |
| same `auction_id + epoch + seq` different payload | mark FAILED, pause auction, anomaly. |
| same `client_bid_id` same request hash | idempotent response. |
| same `client_bid_id` different request hash | idempotency conflict/anomaly. |

Existing `redis_engine_settlements UNIQUE (auction_id, engine_epoch, engine_seq)`
is the right base, but error handling must distinguish identical duplicate from
corrupt conflict.

### Kafka Offset Commit

Current `ProcessKafka` commits after `settleLedgerMessage` succeeds. Keep that.

But document/ensure:

- commit after DB transaction commit only;
- on DB failure, do not commit offset;
- reprocessing is expected;
- settlement must be idempotent;
- DLQ/dead only after bounded attempts and anomaly emission.

### Checkpoint Table

Add in Phase 3 or Phase 4:

```sql
CREATE TABLE auction_engine_checkpoints (
  auction_id text PRIMARY KEY REFERENCES auctions(id) ON DELETE CASCADE,
  engine_epoch bigint NOT NULL,
  engine_seq bigint NOT NULL,
  decision_topic text NOT NULL,
  decision_partition int NOT NULL,
  next_decision_offset bigint NOT NULL,
  state_hash text NOT NULL,
  snapshot_json jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
```

Update in the same transaction as settlement of the corresponding decision.

This table is required before claiming fast Redis rebuild from Kafka+PG
checkpoint. Before it exists, recovery claims must be limited to pending drain
and invariant verification.

## Phase 4: Reconciliation And Pause Policy

### Reconciler Inputs

- Redis engine state;
- Redis pending hash;
- Kafka decision log / settlement table;
- PostgreSQL `auctions`;
- `bids`;
- `redis_engine_settlements`;
- `auction_events`;
- `outbox_events/outbox_delivery`;
- `orders`;
- idempotency records.

### Reconciler Checks

P0 checks:

- no HTTP success without Kafka decision marker;
- Redis `engine_seq` >= DB `engine_seq`;
- Redis pending decisions are either ACKED to Kafka or require recovery;
- every Kafka accepted/sold decision reaches terminal settlement;
- `engine_seq` gapless for decisions that reached Kafka;
- accepted public seq contiguous;
- final winner/current price equals highest valid accepted decision;
- one sold auction creates at most one order;
- idempotency response matches decision response.

### Pause Policy

Pause affected auction when:

- Kafka append failure after Redis decision;
- Redis pending cannot be recovered;
- settlement poison after retry budget;
- duplicate `engine_seq` with different payload;
- settlement lag exceeds hard threshold;
- Redis state hash mismatch after rebuild;
- stale `engine_epoch` tries to settle.

Pause must:

- reject new bids with `ENGINE_RECONCILING` or `ENGINE_PAUSED`;
- allow snapshot/read-only APIs;
- show PC diagnostics reason;
- trigger WS gap/recovery or room notice;
- provide operator resume only after invariant check passes.

## Phase 5: UI And Product State

### H5 Bid State Machine

```text
idle
  -> submitting
  -> engine_accepted_pending_settlement
  -> settled
  -> outbid
  -> rejected
  -> recovering
  -> paused
```

Rules:

- no optimistic success before HTTP response;
- `ENGINE_ACCEPTED + PENDING` copy must not say "final settled";
- CTA disabled during submitting/recovering/paused;
- H5 keeps a synchronous local in-flight lock before `fetch()` so rapid taps or
  React render delay cannot create multiple requests. The lock is released only
  after response/error handling. This is UI-level optimistic locking; it is not
  the source of truth and does not replace server idempotency.
- network timeout or fetch failure shows an uncertain/retry copy and must not
  automatically retry with a new idempotency key. If the user manually retries
  the uncertain bid, H5 must reuse the original `client_bid_id`,
  `Idempotency-Key`, amount, and `client_seen_seq`; otherwise a lost HTTP
  response after server processing can become a second business decision.
- reconnect with last seq; gap triggers snapshot;
- if pending settlement lasts beyond warning threshold, show "settlement delayed"
  only if product wants visible transparency; PC diagnostics must show it.

State table:

| Scenario | H5 behavior |
|---|---|
| tap bid | immediately set `bidPhase=pending`, disable CTA, hold `bidInFlightRef`. |
| `ENGINE_ACCEPTED + PENDING` | show engine accepted / settlement pending, keep CTA disabled. |
| `ENGINE_SOLD + PENDING` | show hammer pending / order syncing, no payment CTA until order settlement. |
| `SETTLED` event/snapshot | show settled leading or sold winner state from server seq. |
| network timeout | show network error/uncertain state; user may retry manually with the same idempotency key and request body. |

### PC Diagnostics

Add:

- engine mode;
- Kafka append success/fail count;
- pending Redis decisions;
- settlement lag p50/p95/p99/max;
- paused auctions and reason;
- latest engine seq vs DB seq;
- Kafka topic/partition/offset if available;
- outbox backlog.

## Phase 6: Performance Evidence

### PTS Requirements

For every formal run:

- PTS sampling logs at `100%`;
- summarize with `tests/pts/summarize-pts-sampling-logs.sh`;
- server stage histograms;
- Redis latency info;
- Kafka broker/topic evidence;
- PostgreSQL pool/wait/lock evidence;
- settlement lag evidence;
- correctness verifier output.

### Expected Tradeoff

Do not claim fixed latency until measured.

Expected directional change:

- HTTP RT increases relative to Redis-only because Kafka append enters the path.
- HTTP RT should remain below PG truth-lane under hot contention if Kafka is
  healthy and Redis Lua is short.
- settlement lag remains async and must be separately optimized.

### Workloads

- PTS-1A accepted ladder;
- PTS-1B contention burst;
- PTS-1C soft-close sniper;
- PTS-2 sustained accepted capacity;
- Kafka failure during bid;
- Redis restart during pending;
- settlement worker kill/restart;
- reconnect storm during settlement lag.

### PTS-1C Soft-Close And Cap Race Assertions

Soft close must be tested as a first-class race, not inferred from final-second
throughput.

| Test item | Assertion |
|---|---|
| last-window accepted bid | `end_at` becomes previous `end_at + extend_by`, never decreases, `extend_count` increments once. |
| many concurrent bids before old `end_at` | all valid accepted bids share the same extended `end_at`; they must not stack N extensions in the same old window. |
| bid after extension | a bid inside the new window is valid; a bid after final `end_at` is rejected as ended/not active. |
| max extension count | after `max_extend_count`, accepted bids do not extend further. |

Cap is a separate terminal race:

| Test item | Assertion |
|---|---|
| two users both bid exactly cap | exactly one `ENGINE_SOLD`; the loser sees terminal/not-active semantics, not `BID_TOO_LOW`. |
| settlement of cap | exactly one order and one settled sold decision. |
| post-cap bid | rejected as terminal, not compared against price grid. |

Automated gates:

- `TestRedisLedgerConcurrentSoftCloseExtendsOnlyOnce`;
- `TestRedisLedgerConcurrentCapOnlyOneSoldAndLoserSeesTerminal`;
- `H5 local bid lock suppresses rapid duplicate clicks`;
- `H5 network retry reuses original bid idempotency key`;
- `verify-l4b-pts-correctness.sh` gates
  `cap_terminal_single_sold_order` and
  `soft_close_no_stacked_subwindow_extension`.

## Phase 7: Rollout And Rollback

### Rollout

1. Keep current async mode behind flag.
2. Add sync Kafka mode disabled by default.
3. Run integration and failure tests with memory ledger/fake ledger.
4. Run local Docker Kafka integration.
5. Run PTS smoke.
6. Run PTS 100% sampling formal workload.
7. Enable sync Kafka mode for final evidence only after invariants pass.

### Rollback

Rollback options:

- switch `BID_ENGINE_KAFKA_ACK_BEFORE_RETURN=false` for performance diagnosis
  only, not final high-value evidence;
- switch `BID_ENGINE_MODE=postgres_lane` for emergency correctness fallback;
- pause affected auctions and settle pending Kafka/Redis decisions before
  changing mode in active pressure tests.

Do not switch modes mid-auction without a reconciliation checkpoint.

## Phase Checklist

### Phase 1 Exit

- HTTP success requires Kafka append ack.
- append failure returns no success and pauses/reconciles.
- duplicate append does not duplicate settlement.
- tests pass.

### Phase 2 Exit

- retry after Kafka ack before PG settlement returns same engine response.
- Redis idempotency has append status.
- idempotency conflict still rejects.

### Phase 3 Exit

- duplicate Kafka messages are harmless.
- offset commit occurs only after DB commit.
- corrupt duplicate pauses.

### Phase 4 Exit

- reconciler catches missing/duplicate/gap/conflict.
- pause/resume operations are tested.

### Phase 5 Exit

- H5 displays pending settlement honestly.
- PC diagnostics expose lag/pause/Kafka append.

### Phase 6 Exit

- PTS 100% sampling logs and stage metrics exist.
- verifier passes after pressure and failure drills.

## Open Technical Questions Before Coding

1. Does `kafka-go` in this version support producer idempotence or transactions
   needed for stronger duplicate guarantees? If not, rely on business
   idempotency and document at-least-once Kafka semantics.
2. Should Kafka append be inside `Engine.PlaceBid`, or should `PlaceBid` return
   an internal decision object and gateway perform append? Prefer engine-level
   append to keep the success boundary centralized.
3. Should append failure pause immediately or first attempt inline retry? Current
   writer already has `MaxAttempts=10`; after that, pause.
4. Should `ENGINE_REJECTED` decisions also be Kafka-acked before return? For
   audit/idempotency under high-value disputes, yes for executable bid attempts.
5. How long should Redis idempotency TTL be? It must exceed the maximum user
   retry and settlement delay window; current default should be verified.
6. Should settlement lag hard-pause threshold be `5s` or lower for final demo?
   Measure first; document threshold as policy, not performance fact.

## Sources

Local:

- `docs/perf/pts/l4b-kafka/single-hotspot-redesign-from-first-principles-2026-05-30.md`
- `backend/internal/redisengine/engine.go`
- `backend/internal/redisengine/kafka_ledger.go`
- `backend/migrations/202605280001_redis_ledger_engine.sql`

External:

- Apache Kafka documentation:
  https://kafka.apache.org/documentation/
- kafka-go writer docs:
  https://pkg.go.dev/github.com/segmentio/kafka-go
- Redis persistence:
  https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/
- Redis Lua:
  https://redis.io/docs/latest/develop/programmability/eval-intro/
- Transactional outbox:
  https://microservices.io/patterns/data/transactional-outbox.html
- Event sourcing:
  https://martinfowler.com/eaaDev/EventSourcing.html
