# PTS Core Pressure Configuration

Use this bundle to find the first cloud bottleneck with admission disabled.
Do not run it against real customer traffic or shared production data.

## Files

- `tests/pts/live-auction-core-pressure.jmx`
- `pts_sessions.csv`, generated from `docs/perf/pts/generate-pts-sessions.sql`

Upload both files to the same PTS JMeter scene. In the JMX, the CSV path is only
`pts_sessions.csv`, which is the format PTS expects for uploaded data files.

## Backend Preconditions

Start the deployed backend with:

```bash
ALLOW_MOCK_AUTH=false
ADMISSION_ENABLED=false
SESSION_TTL=12h
```

For development/debugging on this Linux host, you can run an Air hot-reload backend:

```bash
cd backend
HTTP_ADDR=0.0.0.0:18080 ADMISSION_ENABLED=false ALLOW_MOCK_AUTH=false air
```

Do not use Air for final PTS evidence, because rebuild/restart events pollute
latency and error metrics. Use the prepared binary started by
`tests/pts/prepare-cloud-pressure.sh` for pressure evidence.

Verify before any pressure round:

```bash
curl -fsS https://YOUR_HOST/readyz
curl -fsS https://YOUR_HOST/metrics | grep 'auction_admission_enabled 0'
```

Seed or reset the dedicated pressure room/auction before each destructive run.
The bundled JMX defaults to:

```text
room_id=room_main
auction_id=auc_live
```

## Token CSV

Generate real session tokens on the cloud database:

```bash
psql "$DATABASE_URL" -v session_count=4096 -f docs/perf/pts/generate-pts-sessions.sql
```

Upload the produced `pts_sessions.csv` with the JMX. Do not upload the example
file.

## PTS JMeter Parameters

Set these user properties in PTS:

```text
protocol=http
host=172.16.179.112
port=18080
room_id=room_main
auction_id=auc_live

base_price_cents=10000
increment_cents=5000
bid_rate_hint_rps=300

bid_threads=600
bid_ramp_sec=120
bid_duration_sec=300

snapshot_threads=100
snapshot_ramp_sec=60
snapshot_duration_sec=300

ticket_threads=200
ticket_ramp_sec=60
ticket_duration_sec=300
```

Escalation rounds:

| Round | bid_rate_hint_rps | bid_threads | Duration | Purpose |
|---|---:|---:|---:|---|
| Smoke | 30 | 60 | 60s | auth/token/ACL path check |
| R1 | 100 | 200 | 180s | prove pressure reaches backend |
| R2 | 300 | 600 | 300s | expose PG/outbox/Redis/runtime trend |
| R3 | 500 | 1000 | 300s | push until bottleneck or PTS/env limit |
| R4 | 800+ | 1600+ | 300s | only if R3 has clean headroom |

Stop increasing when any of these dominate:

- JMeter/PTS connect failures, client timeouts, or generator saturation;
- backend 5xx;
- rising PostgreSQL lock/pool/transaction latency;
- outbox backlog or delivery lag grows monotonically;
- Redis latency/errors;
- Go CPU, heap, goroutines, or file descriptors climb without recovery.

## Interpreting Results

This is a downstream-pressure profile. Valid bottleneck evidence requires:

- `auction_admission_enabled 0` before and after;
- no HTTP 429;
- no `RATE_LIMITED` or `BID_AUCTION_TOO_HOT` in bid responses;
- no obvious PTS generator limit before backend metrics move.

If admission counters move, the run is invalid for architecture decisions.
If PTS saturates first, report `ENV_LIMIT`, not backend capacity.

## WebSocket Fanout

The JMX includes `/api/auth/ws-ticket` pressure, but it does not hold long-lived
WebSocket connections. For fanout bottlenecks, create a separate PTS WebSocket
scene:

```text
WS URL: wss://YOUR_HOST/ws?room_id=room_main&auction_id=auc_live&last_seq=0
Header: X-Auction-WS-Ticket: ${ticket}
Connection model: 500, 1000, 2000 live connections, 3-5 minutes each
Trigger traffic: run bid pressure R1/R2 concurrently
```

Because WebSocket tickets are single-use and expire after 60 seconds, generate
each ticket immediately before opening the socket. If the PTS WebSocket scene
cannot chain HTTP ticket issue into WS connect reliably, use the project k6 WS
scripts for fanout and keep PTS for HTTP bid/outbox pressure.
