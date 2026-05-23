# Windows Local k6 Smoke - 2026-05-23

Date: 2026-05-23 Asia/Shanghai

Commit: pending

Environment:

- OS: Windows local development machine
- Go: `go1.26.3 windows/amd64`
- k6: `v2.0.0 windows/amd64`
- Backend: `go run ./cmd/server`
- Infra: local Docker PostgreSQL/Redis/MinIO

This is a Windows local smoke result. It is not the final Linux/native 3-run performance baseline and must not be used for production QPS/P99/fanout/online-user claims.

## Failure Before Fix

Command shape:

```text
go run ./cmd/p0smokeseed
HTTP_ADDR=127.0.0.1:18080 go run ./cmd/server
k6 run --summary-export docs\perf\raw\windows-local-final-second-bid-burst.json tests\load\final-second-bid-burst.js
```

Raw outputs:

- `docs/perf/raw/windows-local-final-second-bid-burst.json`
- `docs/perf/raw/windows-local-final-second-bid-burst-vus4-5s.json`

Observed failures:

- 8 VU / 20s: `bid_http_errors=371`, checks `92.29%`.
- 4 VU / 5s: `bid_http_errors=47`, checks `92.21%`.

Root cause:

- `p0smokeseed` is a demo seed with `auc_live` near cap and a low `cap_price_cents`.
- The k6 workload quickly drove the auction to terminal SOLD or hit missing dynamic bidder user rows.
- That is useful as failure evidence, but not a valid steady local bid-burst smoke.

## Fix

Implemented:

- Added `backend/cmd/p1loadseed`.
- Updated `tests/load/README.md` to use `p1loadseed` for k6 workloads.
- Updated `tests/load/final-second-bid-burst.js` to avoid cap SOLD by default; set `ALLOW_SOLD=true` only for intentional hammer tests.

`p1loadseed` creates:

- ACTIVE `auc_live` with high cap and long end window.
- k6 bidder users for dynamic `k6_bidder_{VU}_{bucket}` IDs.
- k6 WebSocket users for `k6_ws_{VU}` IDs.
- clean Redis auction history/snapshot and ticket keys.

## Passing Windows Local Smoke

### 8 VU / 20s

Command:

```text
go run ./cmd/p1loadseed
HTTP_ADDR=127.0.0.1:18080 go run ./cmd/server
k6 run --summary-export docs\perf\raw\windows-local-final-second-bid-burst-fixed.json tests\load\final-second-bid-burst.js
```

Raw output:

- `docs/perf/raw/windows-local-final-second-bid-burst-fixed.json`

Result:

- checks: `100.00%`
- bid_http_errors: `0`
- http_reqs: `5184`
- request rate: `258.41/s`
- accepted: `1585`
- rejected: `1007`
- accepted rate: `61.14%`
- http_req_duration p95: `13.59 ms`
- http_req_duration p99: `20.44 ms`
- http_req_duration p99.9: `48.8 ms`

### 4 VU / 5s

Command:

```text
go run ./cmd/p1loadseed
HTTP_ADDR=127.0.0.1:18080 go run ./cmd/server
VUS=4 DURATION=5s k6 run --summary-export docs\perf\raw\windows-local-final-second-bid-burst-vus4-5s-fixed.json tests\load\final-second-bid-burst.js
```

Raw output:

- `docs/perf/raw/windows-local-final-second-bid-burst-vus4-5s-fixed.json`

Result:

- checks: `100.00%`
- bid_http_errors: `0`
- http_reqs: `648`
- request rate: `128.44/s`
- accepted: `213`
- rejected: `111`
- accepted rate: `65.74%`
- http_req_duration p95: `13.05 ms`
- http_req_duration p99: `19.16 ms`
- http_req_duration p99.9: `28.66 ms`

## Interpretation

- The k6 workload now exercises the bid endpoint without collapsing into setup errors.
- Rejected bids are expected under hot-row contention and self-leading/increment rules.
- These numbers are Windows local development observations only.
- The final claimable baseline still requires Linux/native or documented equivalent 3-run evidence using `docs/design-v2-industrial/templates/perf-baseline.md`.
