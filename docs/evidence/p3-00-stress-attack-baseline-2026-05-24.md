# P3-00 Stress Attack Smoke Baseline

Date: 2026-05-24 Asia/Shanghai

Commit: `c5aee15adff08fda3da3d510e9dc66b4c549a7ef`

Environment: Windows local development machine. Docker PostgreSQL, Redis, MinIO, Prometheus, and Grafana were running. Backend was started on `127.0.0.1:18080` with `ALLOW_MOCK_AUTH=true` for the k6 load-generation harness.

Status: `P3_SMOKE_BASELINE_CAPTURED`

This document replaces the earlier failed `HARNESS_GAP` note. The earlier result was not valid P3 evidence because load entrypoints were blocked by P2 ACL/admission and WS lifecycle gaps before the intended stress paths were exercised. Those were treated as P2 harness/runtime defects and fixed before this baseline was recorded.

This is still a Windows local smoke baseline, not a capacity claim.

## Fixes Made Before Baseline

- `backend/cmd/p1loadseed/main.go`: seed `k6_user_*` and align room memberships for `k6_user_*`, `k6_bidder_*`, and `k6_ws_*`.
- `backend/internal/realtime/server.go`: revalidate ticket room access after ticket consume and before WS accept, accept explicit `X-Auction-WS-Ticket` for k6, and add a read/cancel pump so client close terminates WS handlers and subscriptions.
- `tests/load/*.js`: use seeded k6 identities, record P2 admission responses separately, and treat `200`/business `429` as valid business responses where appropriate.
- `tests/load/run-p3-local-stress.mjs`: add a P3 runner with per-workload raw logs, environment capture, WS smoke timing, and a per-workload timeout watchdog.
- `tests/load/preflight.js`: add a seed/auth/ACL preflight before stress workloads.

## Commands

```text
cd backend
go run ./cmd/p1loadseed
go test ./...
cd ..
pnpm exec node tests/load/validate-k6-suite.mjs
$env:HTTP_ADDR='127.0.0.1:18080'; $env:ALLOW_MOCK_AUTH='true'; go run ./cmd/server
$env:DURATION='5s'; $env:VUS='2'; $env:COLD_WS_VUS='1'; $env:WORKLOAD_TIMEOUT_MS='45000'; pnpm exec node tests/load/run-p3-local-stress.mjs
curl http://127.0.0.1:18080/metrics > docs/perf/raw/p3-00/metrics-after.prom
docker exec live-auction-postgres psql -U live_auction -d live_auction -c "<diagnostics>"
```

## Workload Results

Raw output root: `docs/perf/raw/p3-00/`

| Workload | Status | Checks | Iterations | HTTP reqs | WS sessions |
|---|---:|---:|---:|---:|---:|
| `preflight` | PASS | 5/5 | 1 | 3 | 0 |
| `final-second-bid-burst` | PASS | 332/332 | 166 | 332 | 0 |
| `outbox-burst` | PASS | 393/393 | 393 | 393 | 0 |
| `watcher-fanout` | PASS | 12/12 | 6 | 9 | 6 |
| `slow-consumer` | PASS | 12/12 | 6 | 15 | 6 |
| `reconnect-storm` | PASS | 20/20 | 10 | 15 | 10 |
| `multi-room-isolation` | PASS | 176/176 | 174 | 175 | 1 |
| `bid-abuse` | PASS | 1194/1194 | 597 | 1194 | 0 |

Runner summary: `docs/perf/raw/p3-00/summary.json`

## Evidence

Metrics snapshot: `docs/perf/raw/p3-00/metrics-after.prom`

- Bid requests: 54 accepted, 137 `BID_TOO_LOW`, 18 `REJECTED_SELF_LEADING`, 1139 `RATE_LIMITED`.
- HTTP totals: 764 auction snapshot reads, 209 bid `200`, 1139 bid `429`, 24 WS ticket `200`, 23 WS upgrade `101`.
- WS recovery: 22 history recoveries, 1 DB snapshot recovery.
- WS connection gauges after run: `room_main=0`, `room_side=0`.
- Runtime goroutines after run: 10.
- Bid lock wait observations: 209 samples, sum 0.178547 seconds.
- Outbox lag observations: 209 samples.
- Fanout latency observations: 209 samples.

DB diagnostics:

- `docs/perf/raw/p3-00/db-outbox-status.txt`: `outbox_delivery` has 5740 `PUBLISHED`, no pending/failed state in the status aggregate.
- `docs/perf/raw/p3-00/db-recent-anomalies.txt`: last 10 minutes show `RATE_LIMITED` only, count 5106.
- `docs/perf/raw/p3-00/db-activity.txt`: no captured lock wait group; one active diagnostic query plus idle/background sessions.

## Stress Attack Verdict

`NO_REGRESSION_WITH_CEILING`

The corrected harness can now drive the required P3 smoke workloads through real backend paths: PostgreSQL, Redis, outbox relay, WebSocket ticketing, recovery, fanout, slow-consumer behavior, multi-room isolation, and admission/rate-limit abuse.

This run does not prove throughput capacity. It proves that the P3 attack loop now has a valid entrypoint and raw evidence bundle. The dominant signal at this small scale is P2 admission behavior: many bid attempts correctly return business `429` and are recorded as such instead of being mislabeled as harness failure.

## Remaining Limits

- Windows local results must not be used for final capacity, p99 SLA, or horizontal scale claims.
- The default admission profile limits PG hot-row pressure. A separate P3 pressure profile with explicit high admission ceilings is still required before making PG lock or data-path evolution decisions.
- Future P3 attribution must first classify the run as `admission-on` or `downstream-pressure`. If `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, or HTTP `429` dominate, the primary conclusion is admission ceiling unless independent PG/outbox/WS saturation evidence exists.
- Raising admission ceilings is acceptable only as a documented pressure profile. It must not be reported as production-capacity evidence, and it must not be hidden behind a passing k6 threshold.
- Workload user IDs should represent legitimate business actors. `outbox-burst` uses seeded bidders because bids are the legitimate way to produce outbox events; a subsystem-named user family such as `k6_outbox_*` would only be correct if it were also seeded with room membership and auction access.
- Slow-consumer smoke uses `BLOCK_MS=5` to produce complete iterations in a 5s smoke window. A later slow-consumer attack should deliberately raise `BLOCK_MS`, connection count, and duration to find the queue/backpressure limit.
- Multi-instance relay ownership, transport replacement, and Redis Lua/CDC decisions remain evidence-gated and are not justified by this smoke baseline.

## Next Attack

1. Run a 2-5 minute local P3 round with higher `VUS`, longer WS sessions, and saved before/after metrics.
2. Run a high-admission PG hot-row profile to separate admission ceiling from lock contention.
3. Run a dedicated slow-consumer/reconnect storm with increased `BLOCK_MS`, connection count, and fanout event rate.
4. Move any capacity claim to Linux with the P2/P3 runner, OS limits verified, and raw artifacts saved.
