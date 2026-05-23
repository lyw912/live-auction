# P1-07 Alert Rules Evidence

Gate: P1-07 alert rules
Date: 2026-05-23 Asia/Shanghai
Base commit: 87165a7

## Design Mapping

- `docs/design-v2-industrial/01-scope-and-roadmap.md`: P1-07 requires alert rules after anomaly semantics are stable.
- `docs/design-v2-industrial/08-observability-and-ops.md`: alerts must have real producers and runbooks.
- `docs/design-v2-industrial/12-engineering-rules.md`: no fake diagnostic or unmeasured performance claims.

## Implemented

Prometheus rules:

- `infra/prometheus/rules/live-auction-alerts.yml`

Prometheus wiring:

- `infra/prometheus/prometheus.yml` loads `/etc/prometheus/rules/*.yml`.
- `infra/docker-compose.yml` mounts `infra/prometheus/rules` read-only into Prometheus.

Runbooks:

- `docs/runbooks/alerts.md`

Validation:

- `tests/infra/validate-alert-rules.mjs`
- `package.json` script `test:alerts:p1:validate`

Rules added:

- `LiveAuctionCriticalAnomaly`
- `LiveAuctionOutboxDeadLetter`
- `LiveAuctionOutboxLagHigh`
- `LiveAuctionSchedulerDriftHigh`
- `LiveAuctionReconnectSpike`
- `LiveAuctionSnapshotRebuildPressure`
- `LiveAuctionSlowConsumerDisconnects`

Metric families referenced:

- `auction_anomaly_total`
- `auction_outbox_dead_total`
- `auction_outbox_lag_seconds_bucket`
- `auction_scheduler_drift_seconds_bucket`
- `auction_ws_recover_total`
- `auction_snapshot_source_total`
- `auction_ws_slow_consumer_disconnect_total`

Each metric family is emitted by P1-01 backend producers. No alert references dashboard-only, fake, or unimplemented metrics.

## Verification

Static rule validation:

```text
pnpm test:alerts:p1:validate
```

Result:

```text
alert rules ok (7 rules)
```

Existing observability validation:

```text
node tests/infra/validate-observability-config.mjs
```

Result:

```text
observability config ok
```

Compose rendering:

```text
docker compose -f infra/docker-compose.yml config
```

Result: PASS.

Prometheus syntax validation:

```text
docker run --rm --entrypoint promtool -v ${PWD}/infra/prometheus:/etc/prometheus:ro prom/prometheus:v2.54.1 check config /etc/prometheus/prometheus.yml
```

Result:

```text
SUCCESS: 1 rule files found
SUCCESS: /etc/prometheus/prometheus.yml is valid prometheus config file syntax
SUCCESS: 7 rules found
```

## Review Result

`live-auction-v2-code-review` was applied manually against:

- `08-observability-and-ops.md`
- `10-test-gates.md`
- `12-engineering-rules.md`
- touched Prometheus, runbook, and validation diff

Current review status: no remaining P0/P1 findings for P1-07.

## Known Limits

- No Alertmanager receiver is configured in the local stack.
- Thresholds are local P1 guardrails, not production SLOs.
- Alert firing was not forced with synthetic Prometheus time series; this slice validates rule loading, metric references, and runbook coverage.
- No QPS, P99, fanout, or online-user performance claim is made.

## Next Action

- Use these alerts during formal load/chaos runs and capture Prometheus rule state as raw evidence.
- If production paging is added later, create an Alertmanager config and escalation policy as a separate slice.
