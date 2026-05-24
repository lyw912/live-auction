# P3-13 PG Hot-Row Drilldown

Date: 2026-05-25 Asia/Shanghai

Status: `BOTTLENECK_FOUND_AND_OPTIMIZED`

## Target

P3-R4 investigates the shared bid-path bottleneck found by P3-R2 and P3-R3. The target is the PostgreSQL hot auction row and DB pool behavior under clean downstream pressure.

All accepted rounds used:

- `P3_PROFILE=downstream-pressure`;
- `ADMISSION_ENABLED=false`;
- real backend, PostgreSQL, Redis, outbox relay, and bid HTTP paths;
- `P3_ARTIFACT_MODE=full`;
- managed isolated backend per workload;
- post-run local port cleanup.

## Harness Changes

`tests/load/run-p3-local-stress.mjs` now includes the focused `p3-bid-pressure` workload and full-mode samples for:

- Prometheus metrics during the workload;
- `pg_stat_activity`;
- `pg_locks`;
- `outbox_delivery` status and `auction_events` counts.

`backend/internal/observability/metrics.go` now exports pgxpool snapshot metrics:

- `db_pool_conns{state=...}`;
- `db_pool_max_conns`;
- `db_pool_acquire_total`;
- `db_pool_empty_acquire_total`;
- `db_pool_canceled_acquire_total`;
- `db_pool_acquire_duration_seconds_total`;
- `db_pool_empty_acquire_wait_seconds_total`.

## Rounds

| Round | Raw path | Load | Result |
|---|---|---|---|
| Baseline smoke | `docs/perf/raw/p3-r4-bid-pressure-baseline-20260525-01/` | 20 rps, 20s | PASS; proves workload and metrics. |
| Before 01 | `docs/perf/raw/p3-r4-bid-pressure-hotrow-20260525-01/` | 160 rps, 35s | PASS; hot-row lock bottleneck visible. |
| Before 02 | `docs/perf/raw/p3-r4-bid-pressure-hotrow-20260525-02/` | 160 rps, 35s | PASS; repeated hot-row lock bottleneck, outbox pending sampled. |
| After 01 | `docs/perf/raw/p3-r4-bid-pressure-after-seq-elision-20260525-01/` | 160 rps, 35s | PASS; conservative transaction-work reduction improved tail. |
| After 02 | `docs/perf/raw/p3-r4-bid-pressure-after-seq-elision-20260525-02/` | 160 rps, 35s | PASS; improvement repeated. |

## Evidence

### Baseline Smoke

`p3-r4-bid-pressure-baseline-20260525-01`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- iterations: `401`;
- dropped iterations: `0`;
- p99 HTTP duration: about `20ms`;
- `auction_bid_lock_wait_seconds`: count `401`, sum about `0.241s`;
- `db_pool_empty_acquire_wait_seconds_total`: about `0.105s`.

### Before Optimization

`p3-r4-bid-pressure-hotrow-20260525-01`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- iterations: `5601`;
- dropped iterations: `0`;
- p99 HTTP duration: about `492ms`;
- accepted/rejected: `2646` / `2955`;
- `auction_bid_lock_wait_seconds`: count `5601`, sum about `269.006s`;
- `db_pool_empty_acquire_wait_seconds_total`: about `50.039s`;
- DB samples showed active `SELECT ... FOR UPDATE OF a` sessions waiting on tuple and transaction locks.

`p3-r4-bid-pressure-hotrow-20260525-02`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- iterations: `5601`;
- dropped iterations: `0`;
- p99 HTTP duration: about `439ms`;
- accepted/rejected: `2874` / `2727`;
- `auction_bid_lock_wait_seconds`: count `5601`, sum about `275.705s`;
- `db_pool_empty_acquire_wait_seconds_total`: about `39.285s`;
- outbox pending at final sample: about `4187`.

### Optimization

The accepted code change is deliberately narrow:

- keep PostgreSQL as auction truth;
- keep auction row lock, idempotency, bid row, `auction_events`, and `outbox_delivery`;
- keep rejected bids audited and sequenced;
- reuse the `seq` already returned by `appendAuctionEventWithSeq` instead of issuing a second `SELECT seq FROM auctions WHERE id = $1` inside the bid transaction.

This removes one redundant transaction-internal query from both accepted and rejected bid paths without changing event semantics.

### After Optimization

`p3-r4-bid-pressure-after-seq-elision-20260525-01`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- iterations: `5601`;
- dropped iterations: `0`;
- p99 HTTP duration: about `323ms`;
- accepted/rejected: `3534` / `2067`;
- `auction_bid_lock_wait_seconds`: count `5601`, sum about `107.180s`;
- `db_pool_empty_acquire_wait_seconds_total`: about `19.851s`;
- outbox pending at final sample: about `4168`.

`p3-r4-bid-pressure-after-seq-elision-20260525-02`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- iterations: `5601`;
- dropped iterations: `0`;
- p99 HTTP duration: about `358ms`;
- accepted/rejected: `3899` / `1702`;
- `auction_bid_lock_wait_seconds`: count `5601`, sum about `152.756s`;
- `db_pool_empty_acquire_wait_seconds_total`: about `22.435s`;
- outbox pending at final sample: about `4173`.

## Quantitative Delta

Same local Windows setup, same 160 rps / 35s workload:

| Metric | Before avg | After avg | Delta |
|---|---:|---:|---:|
| HTTP p99 | about `465.5ms` | about `340.6ms` | about `26.8%` lower |
| bid lock wait sum | about `272.4s` | about `130.0s` | about `52.3%` lower |
| DB pool empty-acquire wait sum | about `44.7s` | about `21.1s` | about `52.7%` lower |
| dropped iterations | `0` | `0` | unchanged |
| admission reject delta | `0` | `0` | unchanged |

These are Windows-local regression/direction numbers only. They are not final capacity claims.

## Attribution

Verdict: `BOTTLENECK_FOUND_AND_OPTIMIZED`.

The primary bottleneck was the hot auction row on the bid path. This was proven by:

- high `auction_bid_lock_wait_seconds` and `db_query_latency_seconds{query="lock_auction_for_bid"}`;
- repeated `pg_locks` tuple/transaction wait samples;
- `pg_stat_activity` showing `SELECT ... FOR UPDATE OF a`;
- clean k6 execution with zero dropped iterations and no HTTP errors;
- clean admission proof.

DB pool wait was a secondary amplifier, not the primary cause. It moved with the hot-row bottleneck and dropped after reducing transaction work.

Outbox is now the next pressure target. Even after the bid-path improvement, `outbox_delivery` pending rose to about `4.17k` at the end of each 160 rps round. This does not invalidate the P3-R4 optimization, but it moves P3-R5 to outbox second-order pressure.

## Design Decision

Keep the current PostgreSQL truth path for this release track.

Do not introduce Redis Lua reservation or early-reject outside the auction lock from P3-R4 alone. The safe optimization budget is transaction-work reduction that preserves audit, seq, idempotency, outbox, and recovery semantics.

Redis Lua reservation remains evidence-gated behind an ADR and invariant verifier because it would change pre-settlement semantics and reconciliation obligations.

## Next

P3-R5 should attack outbox second-order pressure under the same admission-off rules:

- longer/multi-room outbox burst;
- backlog growth and drain rate;
- delivery lag;
- `outbox_delivery` status distribution;
- table/update/dead-tuple evidence;
- claim/update query plan if backlog grows.
