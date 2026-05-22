# P1-01 Prometheus Metrics Evidence

Gate: P1-01 Prometheus metrics
Date: 2026-05-23 Asia/Shanghai
Base commit: 4157b25

## Design Mapping

- `docs/design-v2-industrial/01-scope-and-roadmap.md`: P1-01 requires Prometheus metrics after P0 diagnostics exist.
- `docs/design-v2-industrial/08-observability-and-ops.md`: metrics must be real, interpretable, and backed by producers.
- `docs/design-v2-industrial/12-engineering-rules.md`: no fake dashboards and no unmeasured performance claim.

## Implemented

- Added `/metrics` Prometheus text endpoint backed by `backend/internal/observability`.
- Added in-process counters, gauges, and histograms for real execution points:
  - HTTP request count and latency by method, route pattern, status.
  - bid request result/reason, bid latency, bid row-lock wait, DB query latency.
  - outbox delivery lag, fanout latency, dead-letter count.
  - Redis command latency for outbox publish and recovery reads.
  - WebSocket connections by room, recovery result, snapshot source, slow-consumer closes.
  - scheduler drift and scheduler claim query latency.
  - anomaly totals by type/severity from `system_anomaly_events`.
  - runtime goroutines; Linux-only real RSS/open-FD gauges when `/proc` is available.

## Review Result

`live-auction-v2-code-review` was applied manually against:

- `12-engineering-rules.md`
- `10-test-gates.md`
- `08-observability-and-ops.md`
- touched backend diff

Findings fixed before evidence:

- HTTP metrics initially used raw URL paths; changed to chi route pattern labels to avoid high-cardinality IDs.
- Runtime RSS/open-FD metrics initially risked being approximations on Windows; changed to Linux `/proc` only and omitted otherwise.
- `/metrics` scrape initially risked adding synthetic outbox lag samples; removed scrape-time outbox histogram mutation.
- WS connection gauge initially used auction id under a `room` label; moved connection accounting to `ServeWS` where real `room_id` is available.
- Slow-consumer disconnects initially risked double counting in hub and connection loop; removed hub-side counter.

Current review status: no remaining P0/P1 findings for P1-01.

## Verification

Commands:

```text
cd backend && go test ./...
```

Result: PASS.

Real endpoint smoke:

```text
HTTP_ADDR=127.0.0.1:18080 go run ./cmd/server
curl.exe -fsS http://127.0.0.1:18080/metrics
```

Result: PASS. Sample families observed:

- `http_request_total`
- `http_request_latency_seconds`
- `auction_anomaly_total`
- `runtime_goroutines`
- `db_query_latency_seconds`
- `auction_scheduler_drift_seconds`
- `redis_command_latency_seconds`
- `auction_outbox_lag_seconds`
- `auction_fanout_latency_seconds`

Known limits:

- This is a metrics endpoint and producer slice, not a Grafana dashboard or alert-rule slice.
- No QPS, P99, fanout, or online-user capacity claim is made from this evidence.
- `runtime_rss_bytes` and `runtime_open_fds` are emitted only on Linux where `/proc` exposes real values.

Next action:

- P1-02 Grafana dashboards can now consume real `/metrics` data.
- P1-07 alert rules can use anomaly/outbox/scheduler/recovery metric families after dashboard semantics are fixed.
