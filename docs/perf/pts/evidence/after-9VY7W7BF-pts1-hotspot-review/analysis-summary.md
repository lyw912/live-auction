# PTS-1 Hotspot Review: 9VY7W7BF

Date: 2026-05-28

Report ID: `9VY7W7BF`

Workload: PTS-1 single-auction hotspot pressure

JMX: `tests/pts/live-auction-hotspot-pressure.jmx`

Before evidence: `docs/perf/pts/evidence/before-pts1-hotspot-20260528-1647/`

After evidence: `docs/perf/pts/evidence/after-9VY7W7BF-pts1-hotspot-review/`

## Verdict

```text
CORRECTNESS VERDICT: PASS
PERFORMANCE VERDICT: FAIL FOR MILLISECOND HOTSPOT TARGET
BOTTLENECK: PostgreSQL single-auction row-lock path plus DB pool wait
NEXT TASK: PTS-1 hotspot latency optimization, not another paid repeat run
```

This run is valid evidence for single-auction hotspot correctness under 1000
concurrent virtual users. It is not acceptable evidence for a "millisecond-level"
hotspot latency claim because bid P99 reached about 2.26 seconds.

## PTS Summary

| Metric | Value |
|---|---:|
| Report start | 2026-05-28 16:54:36 |
| Report end | 2026-05-28 17:02:36 |
| Agent count | 2 |
| Scene requests | 477846 |
| Scene avg TPS | 1003.72 |
| Scene avg RT | 788.06 ms |
| Scene P90 | 2120.00 ms |
| Scene P99 | 2275.98 ms |
| Scene failures | 69 |
| Scene success rate | 99.9856% |

Bid sampler:

| Metric | Value |
|---|---:|
| Requests | 414848 |
| Avg TPS | 871.39 |
| Avg RT | 835.01 ms |
| P90 | 2126.90 ms |
| P99 | 2264.99 ms |
| Failures | 69 |
| Success rate | 99.9834% |

Background read paths:

| Sampler | Requests | Avg TPS | P99 | Failures |
|---|---:|---:|---:|---:|
| Snapshot under bid pressure | 22934 | 48.17 | 1688.00 ms | 0 |
| WS ticket issue | 37691 | 79.17 | 1118.99 ms | 0 |
| Readyz/metrics/snapshot preflight | 2373 | ~4.98 total | <= 1723.13 ms | 0 |

## Database Invariants

Focused `auc_live` checks after the run:

| Invariant | Result |
|---|---|
| Auction remained active | PASS |
| `auction_events` seq continuity | PASS, `1..18642`, no gap |
| Outbox delivery state | PASS, all `PUBLISHED` |
| Pending outbox backlog | PASS, `0` |
| Accepted bid count | `18583` |
| Public rejected event count | `59` |
| Duplicate terminal/winner anomaly | Not observed |

Outcome distribution for `auc_live`:

| Result | Count |
|---|---:|
| `ACCEPTED` | 18583 |
| `REJECTED / BID_TOO_LOW` | 400824 |
| `REJECTED / REJECTED_SELF_LEADING` | 59 |

Event and outbox counts:

| Event type | `auction_events` | `outbox_events` |
|---|---:|---:|
| `bid_accepted` | 18583 | 18583 |
| `bid_rejected` | 59 | 59 |

All `18642` outbox deliveries for `auc_live` were `PUBLISHED`.

## Failure Classification

PTS reported 69 failed bid requests. Sampling logs show the sampled failures are
HTTP `409` with:

```json
{"code":"PROCESSING_RETRY_LATER","message":"idempotency completed after replay probe; retry to fetch result"}
```

This is not a process crash, Redis failure, outbox failure, or admission
contamination. It is a bounded idempotency race signal under hotspot pressure.
It still counts against the PTS-1 latency/UX target because users see a retry
response under load.

## Bottleneck Evidence

Admission was disabled throughout:

```text
auction_admission_enabled 0
```

Redis/outbox were not the first bottleneck:

| Metric | Value |
|---|---:|
| Redis outbox publish count | 18642 |
| Redis outbox publish latency | all `<=5ms` |
| Redis blocked clients | 0 |
| Outbox pending | 0 |
| Outbox failed/publishing | 0 |

Host CPU and IO were not saturated in the after snapshot:

| Resource | Observation |
|---|---|
| CPU | mpstat after snapshot avg idle about 97.5% |
| Disk | iostat after snapshot low util, no sustained IO wait signal |

The pressure accumulated in the DB hot path:

| Metric | Value |
|---|---:|
| `db_pool_max_conns` | 90 |
| `db_pool_empty_acquire_total` | 2678204 |
| `db_pool_empty_acquire_wait_seconds_total` | 340187.76 |
| Approx avg empty-acquire wait | ~127 ms |
| `auction_bid_lock_wait_seconds_count` | 419466 |
| `auction_bid_lock_wait_seconds_sum` | 41046.22 |
| Approx avg bid lock wait | ~98 ms |
| `auction_bid_latency_seconds_count` | 419537 |
| `auction_bid_latency_seconds_sum` | 145744.25 |
| Approx avg backend bid latency | ~347 ms |

PTS saw higher end-to-end P99 because the 900 bid VUs pile up behind the same
auction truth path and the request queueing is visible to the load generator.

## Interpretation

This run proves that the current design protects correctness under a severe
single-auction hotspot:

- PostgreSQL remains money truth.
- `seq` stays continuous.
- accepted/rejected public events match outbox.
- relay drains all events.
- Redis remains a fast projection path.

It also proves the current implementation does not satisfy a millisecond-level
hotspot latency target at 1000 VU offered pressure:

- bid P99 is about 2.26 seconds;
- snapshot P99 is about 1.69 seconds;
- WS ticket P99 is about 1.12 seconds;
- retry-later appears in the idempotency path.

The next engineering task should focus on moving the single-auction overload
from unbounded database lock/pool waiting into an explicit, bounded,
observable, server-side auction hotspot control path.

## Optimization Direction

Keep these constraints:

- PostgreSQL remains the final auction truth.
- Redis must not become authoritative for price, winner, order, or seq.
- WebSocket remains delivery, not truth.
- No optimistic client success.

Recommended next implementation target:

1. Add a per-auction in-process sequencer or bounded queue for bid execution.
2. Add fast hotspot admission when the auction queue is full.
3. Expose metrics for queue depth, queue wait, dropped/retry-later count, tx
   time, lock wait, and DB pool wait.
4. Slim the bid transaction so the locked section only performs mandatory
   truth-path writes.
5. Improve idempotency replay handling to reduce `PROCESSING_RETRY_LATER`.
6. Re-run PTS-1 with the same JMX and compare against this report.

Success criteria for the next PTS-1 optimization round:

| Target | Required direction |
|---|---|
| Bid P99 | materially below 2.26s, with a stated millisecond target before run |
| Snapshot P99 | materially below 1.69s |
| DB pool wait | materially reduced |
| Auction lock wait | materially reduced or isolated behind queue wait metrics |
| Retry-later | reduced and explained |
| Correctness | seq continuity, one truth, all outbox published |
