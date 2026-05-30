# PTS-1 Hotspot Optimization Plan

> 2026-05-31 supersession notice: this is a historical PG-lane/Redis-guard
> optimization plan. It explains why PostgreSQL row-lock contention failed the
> millisecond goal, but it is not the current Redis/Kafka PTS-1B plan.

Status: proposed next task

Baseline report: `9VY7W7BF`

Baseline evidence:

- `docs/perf/pts/evidence/archive/historical/after-9VY7W7BF-pts1-hotspot-review/analysis-summary.md`
- `docs/perf/pts/evidence/archive/historical/before-pts1-hotspot-20260528-1647/`
- `docs/perf/pts/evidence/archive/historical/after-9VY7W7BF-pts1-hotspot-review/`

## Problem

PTS-1 single-auction hotspot correctness passed, but latency failed the intended
millisecond-level realtime story.

Observed under 1000 VU offered pressure:

- bid P99: about `2265ms`;
- snapshot P99: about `1688ms`;
- WS ticket P99: about `1119ms`;
- `auction_events` seq remained continuous;
- all outbox deliveries were published;
- Redis and outbox were not the first bottleneck;
- DB pool wait and auction row-lock wait dominated.

This contradicts any claim that the single-auction hotspot path already provides
millisecond-level realtime behavior.

## Goal

Reduce PTS-1 single-auction hotspot P99 materially while preserving auction
truth guarantees:

- PostgreSQL remains final money truth.
- `auction_id + seq` remains authoritative.
- all state mutations still write `auction_events` and outbox in one DB
  transaction.
- clients never see optimistic bid success.

The next PTS-1 run must compare directly against `9VY7W7BF` using the same JMX:

```text
tests/pts/archive/historical/live-auction-hotspot-pressure.jmx
```

## Non-Goals

- Do not claim platform accepted-bid throughput from this workload.
- Do not make Redis authoritative for price, winner, order, or seq.
- Do not bypass idempotency or outbox.
- Do not hide overload by dropping requests without metrics and an explicit
  business response.

## Proposed Direction

### 1. Per-Auction Bounded Sequencer

Introduce an in-process execution lane keyed by `auction_id`.

For a hot auction, requests should enter a bounded queue before trying to
acquire DB connections and auction row locks. A small worker count per auction
executes the existing PostgreSQL truth transaction.

Expected effect:

- fewer goroutines simultaneously wait on DB pool/row lock;
- lower tail latency for admitted requests;
- explicit queue wait metrics instead of opaque DB wait;
- safe fast rejection when the queue is overloaded.

### 2. Hotspot Admission

When per-auction queue depth or predicted queue wait exceeds threshold, return a
stable retryable response instead of letting requests wait seconds.

Candidate response:

```text
HTTP 429 or 409 with code BID_AUCTION_TOO_HOT / BID_RETRY_LATER
Retry-After or retry_after_ms
```

The exact status code must match existing API semantics before implementation.

### 3. Transaction Slimming

Audit the bid transaction and remove non-essential work from the locked section.

Keep inside the transaction:

- lock auction row;
- validate rules against locked truth;
- write bid truth;
- write public event if applicable;
- write outbox;
- complete idempotency;
- commit.

Keep outside the transaction:

- network publish;
- expensive diagnostics reads;
- optional projection work.

### 4. Idempotency Retry-Later Reduction

Investigate `PROCESSING_RETRY_LATER` under hotspot pressure. Prefer returning a
completed replay result when possible, or waiting briefly for the in-flight
record before returning retry-later.

This must remain bounded. No indefinite wait.

### 5. Metrics

Add metrics before re-running PTS-1:

| Metric | Purpose |
|---|---|
| `auction_bid_queue_depth{auction_id}` | current hotspot pressure |
| `auction_bid_queue_wait_seconds` | time before execution starts |
| `auction_bid_queue_rejected_total{reason}` | fast overload decisions |
| `auction_bid_tx_seconds` | DB truth transaction duration |
| existing `auction_bid_lock_wait_seconds` | verify lock pressure reduction |
| existing `db_pool_empty_acquire_wait_seconds_total` | verify pool wait reduction |
| idempotency retry-later counter | verify conflict-window improvement |

Use low-cardinality labels. Avoid unbounded `auction_id` metrics in production;
the pressure profile can use a dedicated `profile="pts1"` label or aggregate
hotspot gauges if needed.

## Acceptance Criteria

The optimized run should satisfy:

| Area | Criteria |
|---|---|
| Correctness | seq continuous, no duplicate terminal truth, outbox fully published |
| Bid latency | P99 materially below `2265ms`; define exact target before running |
| Snapshot latency | P99 materially below `1688ms` |
| DB pressure | pool wait and row-lock wait materially reduced |
| Overload | queue rejects or retry-later are bounded and explicitly measured |
| Evidence | before/after snapshots plus PTS details and sampling logs saved |

## Decision Boundary

If a Redis candidate path is considered later, it needs a separate ADR and
failure proof:

- Redis success + DB failure reconciliation;
- process crash between Redis and DB;
- duplicate/replay handling;
- Redis restart or data loss;
- seq and winner recovery;
- client snapshot correctness;
- rollback plan.

Until that proof exists, Redis may filter or project, but it must not decide
auction truth.
