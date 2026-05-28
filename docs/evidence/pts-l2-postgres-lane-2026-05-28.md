# PTS L2 PostgreSQL Lane Evidence

Date: 2026-05-28

Scope: implements L2 `postgres_lane` from commit `1d31bf9 docs: add PTS1-Refactoring docs`.

## Implemented

- Added `BID_ENGINE_MODE=postgres_lane` as the default bid engine mode.
- Added per-auction bounded execution lanes around the existing PostgreSQL truth path.
- Default lane settings:
  - `BID_LANE_WORKERS=1`
  - `BID_LANE_QUEUE_SIZE=128`
  - `BID_LANE_QUEUE_TIMEOUT=750ms`
- Completed bid/confirm idempotency replay still happens in admission before lane enqueue.
- Lane full returns `BID_AUCTION_TOO_HOT` with `Retry-After`.
- Lane wait budget expiry returns `BID_RETRY_LATER` with `Retry-After`.
- Once a lane worker starts executing a bid, the HTTP request waits for the real PostgreSQL result instead of returning a false retry while the DB may still commit.
- Added metrics:
  - `auction_bid_queue_depth`
  - `auction_bid_queue_wait_seconds`
  - `auction_bid_queue_rejected_total`
  - `auction_bid_tx_seconds`
  - `auction_bid_lane_config`
- Lane overload writes `system_anomaly_events` with `retry_after_ms`, `retry_after_secs`, `engine_mode`, user, auction, trace, and reason.
- H5 treats `BID_RETRY_LATER` as retryable overload and enters the existing retry-after cooldown path.

## Explicit Non-Goals

- Redis guard and Redis ledger are not implemented in this slice.
- Redis does not decide winner, price, order, auction sequence, or idempotency result.
- No new performance P99/QPS number is claimed from this change alone.

## Validation

Commands run:

```text
go test ./internal/gateway -run "TestBidLane|TestPostgresBidLane|TestBidAdmission" -count=1
go test ./internal/auction -run Test -count=1
go test ./internal/gateway -count=1
pnpm --filter mobile-h5 build
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --grep "H5 rate-limit rejects|H5 processing retry-later|H5 postgres lane retry-later" --workers=1 --reporter=line
MANAGE_SERVER=1 pnpm exec node tests/risk/run-p4-risk-simulator.mjs
```

Result: all passed.

Risk simulator raw output:

```text
docs/perf/raw/p4-risk-simulator-202605281343/
```

## Remaining Evidence Gate

PTS before/after is still required before claiming latency improvement. Use the same JMX and environment as the frozen PTS-1 baseline, then record raw output and metrics snapshots under `docs/perf/pts/evidence/`.
