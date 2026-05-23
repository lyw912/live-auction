# Live Auction Alert Runbooks

These runbooks cover local P1 Prometheus alert rules. They are operational guardrails for diagnosis and demo evidence, not a production paging policy.

## LiveAuctionCriticalAnomaly

Meaning:

- `auction_anomaly_total{severity=~"HIGH|CRITICAL"}` is non-zero.
- PostgreSQL `system_anomaly_events` is the source behind this metric.

First checks:

1. Open PC diagnostics Anomalies panel.
2. Query recent rows from `system_anomaly_events`, ordered by `created_at desc`.
3. Match anomaly `type` to the specific domain runbook below when available.
4. Do not mutate Redis or auction state directly.

Escalate when:

- anomaly type is `OUTBOX_DEAD_LETTER`, `ORDER_CREATE_FAILED`, `CLOCK_STEP_BACKWARD`, or `REDIS_DB_RECONCILIATION_DRIFT`.

## LiveAuctionOutboxDeadLetter

Meaning:

- `auction_outbox_dead_total` increased.
- A delivery exhausted retry attempts and the relay should have emitted `OUTBOX_DEAD_LETTER` plus direct `outbox_gap_notice`.

First checks:

1. Open PC diagnostics Outbox panel.
2. Check the lowest dead or failed outbox row for `auction_id`, `seq`, `attempts`, and `last_error`.
3. Confirm an `OUTBOX_DEAD_LETTER` anomaly exists.
4. Confirm affected clients can recover through snapshot after `outbox_gap_notice`.

Do:

- keep PostgreSQL event rows as truth;
- investigate Redis availability and relay logs;
- capture raw evidence before any manual retry.

Do not:

- hand-edit Redis history or snapshot as a hidden fix.

## LiveAuctionOutboxLagHigh

Meaning:

- P95 `auction_outbox_lag_seconds` is above the local P1 guard threshold.

First checks:

1. Open Grafana Live Auction Overview.
2. Check outbox lag, outbox dead count, Redis command latency, and backend logs.
3. Inspect `outbox_delivery` for pending or publishing rows with old `next_attempt_at`.
4. Check whether a lower sequence row is blocking head-of-line delivery.

Interpretation:

- This is not a QPS/P99 capacity claim.
- It means current local relay delivery is slower than the guard threshold and needs diagnosis.

## LiveAuctionSchedulerDriftHigh

Meaning:

- P95 `auction_scheduler_drift_seconds` is above the local P1 guard threshold for a scheduler job type.

First checks:

1. Open PC diagnostics Scheduler panel.
2. Inspect `scheduler_jobs` rows with `PENDING` or `RUNNING` status.
3. Check backend logs for `scheduler_process_failed`.
4. Check DB availability and clock rollback anomalies.

Do:

- allow idempotent scheduler retry;
- preserve auction truth in PostgreSQL.

## LiveAuctionReconnectSpike

Meaning:

- `auction_ws_recover_total` increased sharply.
- Clients are reconnecting or recovering state more often than expected.

First checks:

1. Open PC diagnostics Recovery panel.
2. Check snapshot source mix and slow consumer disconnects.
3. Check Redis availability and Toxiproxy scenarios if chaos tests are running.
4. Inspect browser/network failures before blaming auction state.

Do:

- keep H5 CTA disabled during recovering/disconnected states.
- avoid claiming realtime quality during Redis outage.

## LiveAuctionSnapshotRebuildPressure

Meaning:

- `auction_snapshot_source_total` shows pressure on DB/stale/unavailable snapshot recovery paths.

First checks:

1. Check Redis history/snapshot availability.
2. Check `SNAPSHOT_REBUILD_SATURATED` anomalies.
3. Check DB latency and snapshot rebuild semaphore behavior.
4. Verify clients are receiving snapshot or retry-after, not stale active UI.

Do not:

- bypass recovery by trusting client-side state.

## LiveAuctionSlowConsumerDisconnects

Meaning:

- `auction_ws_slow_consumer_disconnect_total` increased.
- The WebSocket hub is closing clients that cannot keep up.

First checks:

1. Check whether slow clients are isolated while healthy clients still receive events.
2. Inspect backend logs for close phase: recovery, queue_closed, write, or backpressure.
3. Check fanout latency and runtime goroutines.
4. Use slow-consumer k6/Playwright evidence before making capacity claims.

Expected behavior:

- slow consumers are closed;
- healthy clients continue;
- auction truth remains PostgreSQL-authoritative.
