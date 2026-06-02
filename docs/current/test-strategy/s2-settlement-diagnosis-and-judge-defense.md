# S2 Settlement Diagnosis And Judge Defense

> Status: current diagnostic note, 2026-06-02.
> Scope: local S2 steady-auction stair runs, PostgreSQL settlement convergence,
> and judge-facing answers. This document is evidence interpretation, not a new
> workload definition.

## 1. Executive Verdict

S2 currently proves two different things with different confidence levels:

| Claim | Current verdict | Evidence |
|---|---|---|
| M1 steady bid decision p99 <= 100ms | PASS | `s2-stair-1000-setbased-logsuppressed-100s-20260602T211311`: HTTP p99 5.44ms, p99.9 32.21ms, 0 dropped iterations |
| Redis/Lua not the source of tail regression | PASS | Redis SLOWLOG LEN = 0; Redis LATENCY LATEST empty during diagnostic runs |
| Correct eventual settlement after verifier wait | PASS | Same run: `l4b-invariant-gates.tsv` all P0/P1 PASS after verifier wait |
| Async settlement convergence <= 100s | FAIL / near miss | Same run timed out at 102s with Kafka lag 1371, settlement_total 69774/70999 |
| Async settlement convergence <= 110s | TARGET, not yet proven by a 110s run | Requires rerun with `S2_CONVERGENCE_TIMEOUT_SECONDS=110` before judge report can mark PASS |
| Direct-SETTLED fast rejected SQL | REJECTED and reverted | `s2-stair-1000-directsettled-100s-20260602T212330`: worse lag 32033 at 101s; verifier failed due to incomplete convergence |

The honest judge-facing statement:

> "The foreground decision engine is comfortably inside the steady 100ms p99
> target under a 200/s -> 600/s -> 1000/s open-arrival local stair. The remaining
> bottleneck is asynchronous PostgreSQL settlement drain for rejected decisions,
> not Redis Lua or bid decision latency. Set-based rejected settlement improved
> convergence from 158s to roughly the 100-122s band depending on local noise.
> The current internal acceptance target is 110s, but a 110s rerun is needed
> before marking S2 settlement convergence PASS."

## 2. What This Test Actually Is

The local S2 diagnostic run is not a constant 1000/s run. It is an open-arrival
stair:

```text
Tool       : local k6 via Docker, ramping-arrival-rate
Duration   : 40s @ 200/s, 40s ramp toward 600/s, 40s ramp toward 1000/s,
             plus 30s ramp-down
Total      : 70,999 bid decisions in ~150s
Average    : ~473 decisions/s
Peak       : 1000 offered decisions/s near the top of the stair
VU ceiling : preAllocatedVUs=800, maxVUs=1600; actual active VUs stayed <= 9
Admission  : ADMISSION_ENABLED=false pressure profile
Boundary   : HTTP request -> final ENGINE_* with durability_status=ENGINE_DURABLE
```

This is an open model. `dropped_iterations=0` means the offered load was actually
delivered; the script did not hide overload by slowing down like a closed VU loop.

## 3. Why There Is No M2 Fanout p99 In This S2 Result

No M2 fanout p99 is claimed from these local S2 runs.

Reason:

- The local S2 k6 script only posts bids and measures M1 decision latency plus
  async convergence.
- It does not hold WebSocket observers and does not compute client
  `publish -> receive` latency.
- The server metric `auction_fanout_latency_seconds` may appear because a few
  accepted updates produce outbox/WS work, but this is not a valid M2 proof:
  the run accepted only about 5 updates and did not hold thousands of clients.

M2 belongs primarily to S3:

```text
S2 proves: steady bid decision path + stability + settlement convergence.
S3 proves: accepted updates x subscribers fanout path, e.g. 10k WS observers.
```

If a judge asks why S2 does not include fanout p99:

> "We deliberately separate attribution. S2 is an open-arrival decision and soak
> workload. Fanout pressure is accepted updates times subscribers, so we measure
> it in S3 with many held WebSocket clients. Mixing them in this diagnostic run
> would obscure whether a tail came from Redis decision, PG settlement, or WS
> fanout."

## 4. Evidence Timeline

| Run | Code state / purpose | Decisions | M1 p99 / p99.9 | Dropped | Convergence result | Interpretation |
|---|---:|---:|---:|---:|---:|---|
| `s2-stair-1000-batchdrain-20260602T193000` | batch drain before set-based rejected settlement | 80,999 | 5.79ms / 30.51ms | 0 | PASS in 158s | Correct but too slow for close/payment safety |
| `s2-stair-1000-setbased-logdebug-2min-20260602T202733` | set-based rejected batch, logging not fully suppressed | 70,999 | 5.59ms / 29.29ms | 0 | PASS in 105s | Shows set-based direction works, but logging state was not final |
| `s2-stair-1000-setbased-logsuppressed-2min-20260602T203614` | set-based + settlement success logs suppressed | 70,999 | 5.52ms / 28.80ms | 0 | PASS in 122s | Stable M1; convergence still noisy |
| `s2-stair-1000-setbased-workers4-90s-20260602T204827` | 4 settlement workers, 90s gate | 70,999 | 5.38ms / 27.10ms | 0 | FAIL at 90s; lag 84 | Near miss; extra workers do not fully parallelize one hot auction |
| `s2-stair-1000-setbased-logsuppressed-100s-20260602T211311` | current kept code, 100s gate | 70,999 | 5.44ms / 32.21ms | 0 | FAIL at 102s; lag 1371 | Current best diagnostic evidence; M1 pass, 100s convergence near miss |
| `s2-stair-1000-directsettled-100s-20260602T212330` | rejected fast path direct SETTLED trial | 70,999 | 5.36ms / 31.74ms | 0 | FAIL at 101s; lag 32033; verifier incomplete | Rejected and reverted; SQL became heavier and did not help |

The failed direct-SETTLED run is useful evidence. It proves we did not keep a
performance tweak just because it sounded plausible.

## 5. What "PG Settlement Write Amplification" Means Here

The simplified explanation "one rejected decision writes many PG tables" is
directionally correct, but it needs precision.

Current fast rejected settlement still writes multiple logical records:

```text
Kafka rejected decision
  -> redis_engine_settlements  audit / replay status
  -> auctions                  advance engine_seq checkpoint
  -> bids                      persisted rejected bid row and decision basis
  -> idempotency_records       replay same HTTP result for retry
  -> redis_engine_checkpoints  Kafka position / replay checkpoint
```

The physical cost is larger than the row count:

- each insert/update maintains indexes;
- each update creates PostgreSQL MVCC old row versions that later need vacuum;
- each transaction writes WAL;
- the same hot auction must advance `engine_seq` in order.

PostgreSQL documents that normal `VACUUM` removes dead row versions from tables
and indexes and marks space reusable; our observed `n_dead_tup` on
`redis_engine_settlements` after S2 runs is consistent with update-heavy
settlement churn. See PostgreSQL routine vacuuming:
https://www.postgresql.org/docs/current/routine-vacuuming.html

So the root cause is not merely "5 tables". It is:

```text
high rejected-decision volume
  x per-decision audit/idempotency/bid materialization
  x index/WAL/MVCC cost
  x one hot auction's ordered settlement checkpoint
  x one Kafka partition carrying that auction's ordered stream
```

## 6. Why We Do Not Simply Drop Rejected Bids Or Idempotency

Dropping rejected rows or idempotency would be the easiest way to lower PG write
volume, but it is not a safe default for this project.

### Idempotency

Stripe's API guidance is the relevant industrial analogy: for mutating POST
requests, idempotency stores the first result so a retry can safely get the same
status/body rather than perform a duplicate operation. Source:
https://docs.stripe.com/api/idempotent_requests

In this auction system, a rejected bid is still a user-visible final decision.
If the network drops after the server replies, the client may retry the same
`client_bid_id`. The defensible behavior is same key + same hash -> same
`ENGINE_REJECTED` response, not "maybe recompute against a later price".

### Audit And Arbitration

Jewellery auction has high trust requirements. A low-price reject is not just
noise; it may be challenged:

- "Why did my bid lose?"
- "Was I below the required increment at that exact engine_seq?"
- "Did the system reject me because I was stale or because the auction ended?"

The current verifier depends on persisted decision basis and settlement coverage.
Removing rejected bid materialization would require a replacement audit model,
not a deletion.

### What A Safe P1 Would Look Like

Do not remove correctness evidence. A safer P1 would be:

1. Keep idempotency response for every `ENGINE_*` final decision.
2. Keep an immutable audit record with `engine_seq`, amount, user, reason,
   request hash, and decision basis.
3. Consider moving rejected detail from `bids` to a narrower append-only
   `bid_decision_audit` table, or partitioning rejected decisions by auction/time.
4. Keep accepted/sold bids in `bids` because they affect winner, order, payment,
   and public events.
5. Prove with the same verifier that every rejected decision is replayable and
   justified.

That is a schema redesign task, not a small SQL tweak. It is deferred.

## 7. Kafka Worker Scaling Boundary

Adding settlement workers did not linearly improve this S2 run because this test
uses one hot auction. Kafka preserves order per partition, and each partition is
assigned to one consumer in a group. Kafka's own design docs describe topics as
ordered partitions consumed by one consumer within a subscribing group:
https://kafka.apache.org/20/design/design

For one auction keyed to one partition:

```text
more workers != more parallelism for that auction's ordered settlement stream
```

Scaling path:

- many auctions -> partition by `auction_id`, multiple partitions and consumers;
- one auction -> keep one sequencer and optimize per-message settlement cost;
- if one auction must exceed this drain rate, use a dedicated audit schema or a
  business close/payment gate, not unordered parallel settlement.

## 8. Business Interpretation For Jewellery Auction

`ENGINE_DURABLE` means the bidder's decision is safely recorded in Redis hot
state / Redis decision log / idempotency replay record. It does not mean
PostgreSQL accounting is complete.

Business rule:

```text
During live bidding:
  settlement lag is acceptable if M1/M2 stay healthy and replay coverage exists.

At close/payment:
  do not create payment/order truth from incomplete PostgreSQL settlement.
  Show "confirming final result" until Kafka lag = 0, Redis pending = 0,
  PG settlement complete, outbox drained.
```

This is stronger than "stop accepting bids 30s before close", because pre-close
blocking conflicts with soft-close and final-second bidding. The safer product
fallback is post-close payment gating.

## 9. Judge Grill: Likely Questions And Defensible Answers

**Q: Why are there only 5 accepted bids? Is the system only handling 0.03 accepted/s?**

A: Accepted updates are governed by the price ladder and the S2 script's amount
model. The engine handled 70,999 final decisions with p99 5.44ms; rejected
decisions are successful adjudications. Accepted-update fanout is measured in S3,
not inferred from S2.

**Q: Why no M2 fanout p99 here?**

A: This S2 local script does not hold WS observers. M2 is publish-to-receive
latency and belongs to S3. S2 only claims M1 and settlement convergence.

**Q: Can you claim 1000/s?**

A: Only as peak offered rate in a 2-minute open-arrival stair, not as constant
1000/s for the entire run. The average was about 473 decisions/s over 70,999
decisions. The important harness signal is `dropped_iterations=0`.

**Q: Why did the 4-worker change not solve convergence?**

A: The auction's Kafka records are ordered in one partition. Kafka consumer-group
parallelism is partition-limited. Four workers help across many auctions or
partitions, not one hot auction stream.

**Q: Why not skip storing rejected bids?**

A: Because the reject is a final user-visible decision. We need idempotent retry
semantics and auditability of reject reason/decision basis. A future P1 can move
rejected detail to a narrower audit table, but cannot remove replay/audit
coverage.

**Q: Why is a 100s timeout acceptable?**

A: It is not marked PASS at 100s. It is a near miss under a stricter diagnostic
gate. The project now uses 110s as the internal target, but needs a 110s rerun
to publish it as PASS. Product safety is protected by post-close payment gating.

**Q: What would you do next if asked to improve it?**

A: Not direct-SETTLED; that was measured and regressed. The next serious P1 is a
rejected-decision audit schema: append-only/narrow table, partitioned by
auction/time, retaining idempotency and verifier coverage. Then rerun S2 and S1
to prove no M1 regression and no correctness loss.

## 10. Current Action Plan

Keep:

- set-based rejected settlement batch;
- Kafka fetch/commit batching;
- settlement success log suppression for `kafka-settlement`;
- aggregated rejected settlement metric increment;
- 110s internal S2 convergence target, pending one validating rerun.

Do not keep:

- direct-SETTLED fast rejected SQL trial;
- any optimization that drops idempotency or reject audit without a replacement
  verifier and schema.

Next scenarios after S2:

1. S3 fanout: measure M2 publish-to-receive p99 with real WS observers.
2. S4 remaining faults: Kafka, Redis flush, Redis+Kafka correlated fault.
3. Optional S2 110s rerun only when a judge-facing PASS table is being assembled.
