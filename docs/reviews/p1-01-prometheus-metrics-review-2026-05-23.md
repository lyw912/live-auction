# P1-01 Prometheus Metrics Review - 2026-05-23

## Scope

Review target: first P1 evidence slice, Prometheus-style metrics endpoint and real metric producers.

Design basis:

- `docs/design-v2-industrial/08-observability-and-ops.md`
- `docs/design-v2-industrial/10-test-gates.md`
- `docs/design-v2-industrial/12-engineering-rules.md`

## Findings

No remaining P0/P1 findings after fixes.

Fixed during review:

- [P1] HTTP request metric used raw URL paths. This would create high-cardinality labels for IDs and violate operational quality. Fixed by using chi route patterns.
- [P1] Runtime RSS/open-FD gauges were initially approximated on Windows. Fixed by emitting those gauges only from Linux `/proc` where values are real.
- [P1] Scraping `/metrics` initially mutated `auction_outbox_lag_seconds`. Fixed by recording outbox lag only at actual relay publish time.
- [P1] `auction_ws_connections{room}` initially used auction id. Fixed by counting connections in `ServeWS` with the actual room id.
- [P2] Slow-consumer disconnect count could double-count hub queue closure plus connection close. Fixed by removing hub-side counting.

## Missing Tests

No blocker for P1-01. Current tests prove:

- Prometheus text rendering for counters, gauges, histograms, and runtime metric presence.
- `/metrics` route exports runtime and DB-backed anomaly metrics from a real test database.
- HTTP middleware records route-pattern labels and status.
- Existing backend integration suite remains green with metrics instrumentation enabled.

Future P1 tests belong to later slices:

- Grafana panel load with real Prometheus data.
- Alert-rule evaluation fixtures.
- k6 baseline scrape sidecar evidence.

## V2 Drift

No current drift for P1-01.

The implementation does not introduce performance claims. It adds raw metric producers only.

## Residual Risk

- Metrics are in-process and reset on restart. This is acceptable for P1-01 exporter setup; long-term retention belongs to Prometheus/Grafana deployment.
- Runtime RSS/open-FD gauges are unavailable on Windows local dev. Final benchmark evidence must run on Linux native or a documented equivalent per `09-performance-and-benchmark.md`.
