# 08 · Observability And Ops

## P0 Observability

P0 does not require full Prometheus/Grafana, but it must have:

- structured logs;
- trace_id across HTTP/DB/event/outbox;
- real anomaly events;
- diagnostic pages backed by DB/producers.

## Structured Log Fields

Common:

```text
timestamp
level
trace_id
user_id
room_id
auction_id
component
event
latency_ms
result_code
error
```

Bid:

```text
client_bid_id
amount_cents
seq
lock_wait_ms
tx_duration_ms
idempotency_status
reject_reason
```

WS:

```text
connection_id
last_seq
recover_result
snapshot_source
send_queue_depth
close_reason
```

Outbox:

```text
outbox_id
aggregate_type
aggregate_id
seq
delivery_status
attempts
lag_ms
```

Scheduler:

```text
job_id
job_type
target_id
attempts
drift_ms
lease_expired
```

## Anomalies

| Type | Severity | Trigger |
|---|---|---|
| AUCTION_STUCK_ACTIVE | HIGH | ACTIVE and now > end_at + grace |
| OUTBOX_STUCK | HIGH | pending/publishing beyond SLA |
| OUTBOX_DEAD_LETTER | CRITICAL | delivery attempts exhausted |
| SCHEDULER_JOB_STUCK | HIGH | expired lease or max attempts |
| ORDER_CREATE_FAILED | CRITICAL | SOLD without order |
| HIGH_REJECT_RATE | MED | reject spike in short window |
| RECONNECT_SPIKE | MED | reconnect burst |
| IDEMPOTENCY_STUCK | HIGH | PROCESSING exceeds budget |
| CLOCK_STEP_BACKWARD | HIGH | wall clock rollback > 1s |
| RATE_LIMIT_REDIS_DOWN | MED | Redis unavailable for limit |
| SNAPSHOT_REBUILD_SATURATED | MED | semaphore saturated |
| WS_REVERSE_SEQ_ANOMALY | HIGH | client reports impossible sequence |

## Diagnostic Page

### Active Auctions

Fields:

- auction_id
- room_id
- item
- status
- current_price
- winner
- end_at
- seq
- accepted_bid_count
- extend_count
- last_event_at

### Rejects

Fields:

- time
- auction
- user
- amount
- current price
- reject_reason
- trace_id

### Outbox

Fields:

- outbox_id
- aggregate
- seq
- status
- attempts
- next_attempt_at
- lag
- last_error

### Scheduler

Fields:

- job_id
- job_type
- target
- run_at
- status
- attempts
- locked_until
- last_error

### Recovery Health

Fields:

- room_id
- reconnect_count_recent
- history_recovered
- snapshot_recovered
- snapshot_from_db
- snapshot_stale
- slow_consumer_disconnects

## P1 Metrics

| Metric | Type |
|---|---|
| `auction_bid_request_total{result,reason}` | counter |
| `auction_bid_latency_seconds` | histogram |
| `auction_bid_lock_wait_seconds` | histogram |
| `auction_outbox_lag_seconds` | histogram |
| `auction_outbox_dead_total` | counter |
| `auction_fanout_latency_seconds` | histogram |
| `auction_ws_connections{room}` | gauge |
| `auction_ws_recover_total{result}` | counter |
| `auction_ws_slow_consumer_disconnect_total` | counter |
| `auction_snapshot_source_total{source}` | counter |
| `auction_scheduler_drift_seconds` | histogram |
| `auction_anomaly_total{type,severity}` | counter |
| `db_query_latency_seconds{query}` | histogram |
| `redis_command_latency_seconds{command}` | histogram |
| `runtime_goroutines` | gauge |
| `runtime_open_fds` | gauge |
| `runtime_rss_bytes` | gauge |

## Runbooks

### Outbox Stuck

1. Open diagnostic outbox panel.
2. Check `last_error`.
3. Verify Redis/WS hub health.
4. If transient, wait for backoff.
5. If DEAD, clients should snapshot; do not manually mutate auction truth.
6. Capture evidence for postmortem.

### Auction Stuck Active

1. Check auction end_at and scheduler jobs.
2. Check END_AUCTION job status.
3. If job missed, enqueue idempotent END_AUCTION job.
4. Confirm terminal event emitted.

### Reconnect Spike

1. Check snapshot source mix.
2. If DB rebuild saturated, increase backoff or reduce semaphore only after DB recovers.
3. Verify Redis availability.
4. Watch RSS and connection count.

### Redis Down

1. Bid path remains PG-authoritative but rate-limit fail-open.
2. WS reconnect/snapshot should degrade.
3. Do not claim realtime recovery quality during outage.
4. Restore Redis and watch projection rebuild.

### Clock Step Backward

1. Stop scheduler from starting new auctions.
2. Keep active auctions server-authoritative.
3. After NTP stabilizes, manually acknowledge anomaly.
4. Record incident.

## Evidence Rules

- A diagnostic panel without producer is not allowed.
- An alert without runbook is not allowed.
- A metric without interpretation is low value.
- Logs must not contain auth ticket/token.
