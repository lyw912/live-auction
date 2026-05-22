# P1-02 Grafana Dashboards Evidence

Gate: P1-02 Grafana dashboards
Date: 2026-05-23 Asia/Shanghai
Base commit: 66f6a23

## Design Mapping

- `docs/design-v2-industrial/01-scope-and-roadmap.md`: P1-02 requires Grafana dashboards after real metrics exist.
- `docs/design-v2-industrial/02-architecture.md`: observability must be Prometheus/Grafana with real evidence, not static panels.
- `docs/design-v2-industrial/08-observability-and-ops.md`: metrics and dashboards must be backed by real producers.
- `docs/design-v2-industrial/12-engineering-rules.md`: fake dashboard data is forbidden.

## Implemented

- Added Prometheus service to `infra/docker-compose.yml`.
- Added Grafana service to `infra/docker-compose.yml`.
- Added Prometheus scrape config at `infra/prometheus/prometheus.yml` for backend `/metrics`.
- Added Grafana datasource provisioning for Prometheus.
- Added Grafana dashboard provisioning.
- Added `infra/grafana/dashboards/live-auction-overview.json`.
- Added static validation script `tests/infra/validate-observability-config.mjs`.
- Updated `infra/README.md` with Prometheus/Grafana URLs and backend scrape prerequisite.

Dashboard panels reference only P1-01 metric families emitted by backend `/metrics`:

- `http_request_total`
- `auction_bid_request_total`
- `auction_bid_latency_seconds_bucket`
- `auction_bid_lock_wait_seconds_bucket`
- `auction_outbox_lag_seconds_bucket`
- `auction_fanout_latency_seconds_bucket`
- `auction_ws_connections`
- `auction_ws_recover_total`
- `auction_ws_slow_consumer_disconnect_total`
- `auction_anomaly_total`
- `auction_outbox_dead_total`

## Review Result

`live-auction-v2-code-review` was applied manually against:

- `12-engineering-rules.md`
- `10-test-gates.md`
- `08-observability-and-ops.md`
- touched infra/dashboard diff

Findings addressed before evidence:

- Avoided static/fake dashboard data; every panel query references real metric names implemented in P1-01.
- Avoided unreliable image healthchecks that could block Grafana startup due to missing shell tools.
- Verified dashboard is provisioned through Grafana API, not only JSON-valid.
- Documented that backend must listen on `:8080` for the default Prometheus target.

Current review status: no remaining P0/P1 findings for P1-02.

## Verification

Commands:

```text
node tests/infra/validate-observability-config.mjs
docker compose -f infra/docker-compose.yml config
```

Result: PASS.

Real smoke:

```text
HTTP_ADDR=:8080 go run ./cmd/server
docker compose -f infra/docker-compose.yml up -d prometheus grafana
curl.exe -fsS http://127.0.0.1:9090/-/ready
curl.exe -fsS http://127.0.0.1:9090/api/v1/targets?state=active
curl.exe -fsS http://admin:admin@127.0.0.1:3000/api/health
curl.exe -fsS "http://admin:admin@127.0.0.1:3000/api/search?query=Live%20Auction%20Overview"
```

Result: PASS.

Observed:

- Prometheus target `live-auction-backend` health was `up`.
- Grafana API health returned database `ok`, version `11.2.0`.
- Grafana dashboard search returned `Live Auction Overview` with UID `live-auction-overview` in folder `Live Auction`.

Known limits:

- This is a local Docker provisioning/dashboard slice, not alert rules.
- No performance number is claimed from these panels.
- Prometheus/Grafana images were pulled during the first real smoke, causing that initial command to time out before verification; rerun after image pull passed.

Next action:

- P1-03 k6 baseline suite can now use the observability stack for correlated metrics.
- P1-07 alert rules should build on these metric families and include runbooks.
