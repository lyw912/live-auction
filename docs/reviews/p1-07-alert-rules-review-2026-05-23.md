# P1-07 Alert Rules Review - 2026-05-23

## Scope

Review target: Prometheus alert rules, Prometheus/Compose wiring, runbooks, validation script, and evidence.

Design basis:

- `docs/design-v2-industrial/08-observability-and-ops.md`
- `docs/design-v2-industrial/10-test-gates.md`
- `docs/design-v2-industrial/12-engineering-rules.md`

## Findings

No remaining P0/P1 findings after verification.

Fixed during review:

- [P1] Alert rules initially risked becoming documentation-only. Added Prometheus `rule_files` wiring, Docker Compose mount, and promtool validation.
- [P1] Alert rules must not reference fake metrics. Added validation that every `auction_*` metric in an alert expression is in the implemented producer allowlist.
- [P1] Alerts without runbooks violate v2 observability rules. Added `docs/runbooks/alerts.md` and validation for every alert runbook section/link.

## Alert Coverage

| Alert | Metric source | Runbook | Gap |
|---|---|---|---|
| LiveAuctionCriticalAnomaly | `auction_anomaly_total` from PostgreSQL anomalies | yes | none for local P1 |
| LiveAuctionOutboxDeadLetter | `auction_outbox_dead_total` relay producer | yes | none for local P1 |
| LiveAuctionOutboxLagHigh | `auction_outbox_lag_seconds_bucket` relay producer | yes | threshold is guardrail, not SLO |
| LiveAuctionSchedulerDriftHigh | `auction_scheduler_drift_seconds_bucket` scheduler producer | yes | threshold is guardrail, not SLO |
| LiveAuctionReconnectSpike | `auction_ws_recover_total` realtime producer | yes | threshold is guardrail, not SLO |
| LiveAuctionSnapshotRebuildPressure | `auction_snapshot_source_total` realtime producer | yes | threshold is guardrail, not SLO |
| LiveAuctionSlowConsumerDisconnects | `auction_ws_slow_consumer_disconnect_total` realtime producer | yes | none for local P1 |

## Missing Tests

No blocker for P1-07 readiness.

Current verification proves:

- Prometheus config loads alert rule files.
- Docker Compose mounts alert rules.
- Prometheus promtool parses config and all 7 rules.
- Static validation catches missing runbooks and unimplemented metrics.

Still useful before final release:

- Capture `/api/v1/rules` and `/api/v1/alerts` output from a live Prometheus instance during load/chaos runs.
- Add Alertmanager only when there is a real receiver/escalation policy.

## V2 Drift

No current drift.

The alert rules use real metric producers and have runbooks. They do not claim production SLOs or capacity.

## Residual Risk

- Local guard thresholds are intentionally conservative and may need tuning after formal baseline runs.
- No Alertmanager receiver is configured.
- Rules were syntax-validated and structurally validated; forced firing fixtures are deferred to future operational hardening.
