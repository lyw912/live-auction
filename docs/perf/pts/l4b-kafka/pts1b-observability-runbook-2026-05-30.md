# PTS-1B Observability Runbook

Date: 2026-05-30

## Scope

PTS-1B measures the contention/reject final-window hotspot path for:

`POST /api/auctions/{id}/bids`

This runbook separates three questions that must not be mixed:

- PTS RTT p99: client-observed latency, including PTS agent, network, TLS/LB, server queueing, and response transfer.
- Backend stage p99: Prometheus histograms from finite server stages.
- Single request chain: OpenTelemetry spans in Tempo, correlated by `X-Trace-Id` / `X-Request-Id`.

Prometheus/Grafana answers where the population is slow. Tempo answers why a sampled request was slow. PTS 100% sampling is still required to prove the true PTS-side p99 and to pick exact tail requests for trace/log lookup.

## Start Stack

```bash
docker compose -f infra/docker-compose.yml up -d prometheus grafana tempo otel-collector pyroscope
```

For the standard PTS-1B cloud workload, reset the pressure data and start the
backend through the PTS reset script so the JMX, CSV, database, Redis, Kafka
topics, and backend environment stay in one audited path:

```bash
OTEL_TRACES_ENABLED=true \
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318 \
OTEL_TRACES_SAMPLER_RATIO=1 \
OTEL_SERVICE_NAME=live-auction-backend \
L4B_PROFILE=pts-1b \
SESSION_COUNT=1000 \
SESSION_CSV=pts-1ab-1000vu-sessions.csv \
bash tests/pts/reset-l4b-final-second-pressure.sh
```

The PTS workload should keep the existing `pts-1b-contention-burst-1000vu-1m.jmx`
load model: 1000 VU, 1 second ramp, 60 second duration, one bid per VU,
`ADMISSION_ENABLED=false`, and `BID_ENGINE_MODE=redis_ledger`. During p99
diagnosis, change only the PTS pressure-log sampling rate to `100%`; changing
the load shape at the same time invalidates comparison with prior PTS-1B runs.

For Alibaba Cloud multi-agent PTS, enable CSV splitting for
`pts-1ab-1000vu-sessions.csv`. This is a hard gate, not an optimization. Without
split/disjoint data, each pressure agent starts from row 1 and duplicates
`user_id`, `client_bid_id`, and `Idempotency-Key`, which turns the run into an
idempotency replay test instead of a 1000-user hotspot test. Reject the report if
PostgreSQL has fewer than 1000 distinct `client_bid_id` rows for `auc_live`.

If running the backend manually instead of using the reset script, start it with
tracing enabled during the diagnostic run:

```bash
OTEL_TRACES_ENABLED=true \
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318 \
OTEL_TRACES_SAMPLER_RATIO=1 \
OTEL_SERVICE_NAME=live-auction-backend \
BID_ENGINE_MODE=redis_ledger \
go run ./cmd/server
```

Use 100% trace sampling only for short PTS-1B investigations. For longer runs, lower `OTEL_TRACES_SAMPLER_RATIO` after the bottleneck is identified.

## Dashboards

- Grafana: `http://localhost:3000`
- Dashboard: `Live Auction PTS1-B Bottlenecks`
- Tempo Explore datasource: `Live Auction Tempo`
- Prometheus: `http://localhost:9090`
- Pyroscope endpoint is provisioned for function-level profiling at `http://localhost:4040`; use it when Prometheus shows runtime CPU/GC pressure but spans only show broad application time.

## Required Evidence Per Run

- PTS raw sampling logs with 100% sampling and request/response headers.
- PTS summary p50/p95/p99 and error classification.
- PostgreSQL uniqueness evidence:
  `count(distinct user_id)=1000` and `count(distinct client_bid_id)=1000` for
  `bids where auction_id='auc_live'`.
- Grafana screenshot/export of `Live Auction PTS1-B Bottlenecks`.
- Prometheus snapshot or query export for the PromQL below.
- At least 3 Tempo traces for p99-ish requests, using the response `X-Trace-Id`.
- `/api/monitor/redis-engine` output after the run.
- `tests/pts/verify-l4b-pts-correctness.sh <run-id>` output.

## PromQL

Backend handler p99:

```promql
histogram_quantile(0.99, sum by (le) (rate(http_request_latency_seconds_bucket{path="/api/auctions/{id}/bids",method="POST"}[2m])))
```

Gateway stage p99:

```promql
histogram_quantile(0.99, sum by (le, stage) (rate(auction_bid_gateway_stage_seconds_bucket{mode="redis_ledger"}[2m])))
```

Redis engine hot path:

```promql
histogram_quantile(0.99, sum by (le, stage) (rate(auction_bid_http_stage_seconds_bucket[2m])))
```

Kafka append:

```promql
histogram_quantile(0.99, sum by (le, status) (rate(auction_bid_kafka_append_seconds_bucket[2m])))
```

Correctness failure signals:

```promql
sum by (reason) (increase(auction_bid_engine_pause_total[5m]))
sum by (reason) (increase(auction_bid_kafka_append_fail_total[5m]))
max(auction_bid_redis_pending_decisions)
```

## Trace Stages

A traced bid request should show these spans:

- `http.request`
- `bid.place`
- `bid.auth`
- `bid.decode`
- `bid.acl`
- `bid.admission`
- `bid.redis_engine`
- `bid.idempotency.pg`
- `bid.idempotency.redis`
- `bid.snapshot_load`
- `bid.redis_lua`
- `bid.append_order_wait`
- `bid.kafka_append`
- `bid.redis_marker`

Do not put `trace_id`, `client_bid_id`, `user_id`, or arbitrary `auction_id` into Prometheus hot-path histogram labels. Those identifiers belong in traces, logs, Kafka headers, and sampled PTS logs.

## Triage Matrix

| Symptom | Likely layer | Action |
|---|---|---|
| PTS RTT p99 high, backend `http_request_latency_seconds` p99 low | PTS agent, public network, LB, TLS, connection reuse | Compare PTS agent region, connection settings, response headers, and server access logs by `X-Trace-Id`. |
| Backend handler p99 high, gateway `total` high, all inner stages low | HTTP server queueing, auth middleware, JSON write, kernel/runtime pressure | Check goroutines, FDs, CPU, GC, and Pyroscope/pprof. |
| `bid.admission` high or queue rejects increase | local admission/semaphore/GCRA pressure | Confirm PTS-1B profile intentionally disables admission if measuring engine hot path. |
| `bid.redis_lua` p99 high | Redis CPU, Lua serialization, slow Redis command, hot key contention | Check Redis slowlog/latency, CPU steal, appendfsync, and Lua script work. |
| `bid.append_order_wait` high | earlier engine_seq did not get ACK marker | Inspect pending hash, append marker, Kafka append failures, and engine pause reason. |
| `bid.kafka_append` p99 high | Kafka sync append / RequireAll / ISR / broker I/O | Check broker logs, ISR/min.insync settings, append timeout, and disk/network latency. Do not switch to async as a silent degradation. |
| `bid.redis_marker` high/failing | Redis marker durability after Kafka ACK | Treat as correctness risk; client should receive pending/retry-later, not accepted. |
| Settlement lag high while bid HTTP p99 normal | async worker/DB/outbox bottleneck | Check consumer group lag, settlement transaction duration, DB pool waits, outbox lag. |
| Any engine pause, DLQ, pending decisions after drain | correctness failure | Stop performance claim, run verifier, fix recovery before optimizing latency. |

## Scoring Position

Grafana dashboards count for observability only if they are business-specific and operational:

- auction state: engine epoch/seq, price/winner, pause reason, pending decisions;
- pipeline health: Kafka append p99, consumer/settlement lag, outbox backlog, DLQ;
- anomaly alerts: engine pause, append timeout, pending drain failure, invariant failure;
- runbook: concrete triage and evidence capture.

A custom in-app dashboard is still useful for judges and non-SRE reviewers because it can join business rows (`/api/monitor/redis-engine`, flight recorder, anomalies) that Grafana does not naturally join. The Grafana dashboard should be the SRE view; the in-app monitor should be the product/business forensic view.
