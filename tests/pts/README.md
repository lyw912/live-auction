# PTS Performance Test Suite

> Entry point: read `docs/s1-s5/00-overview.md` first for the only
> current S1-S5 test plan. Then read `tests/pts/MANIFEST.md` for script/data
> asset locations.
> Read `docs/design/02-performance-correctness-contract.md` before interpreting any result.

## Asset Directory Structure

```
scenarios/        Current S1-S5 PTS/k6 scenario assets
  s1-final-second-contention/
  s3-room-fanout/
  s4-fault-resilience/
```

Do not create new plans or reports named L2/L3/L4; current work should use
S1-S5 names.

---

## Why `192.168.1.104` Is Not Enough

`192.168.1.104` is a private LAN address. Alibaba Cloud PTS cannot reach it directly. Use one of:

- a Cloudflare/ngrok tunnel public host, for local smoke;
- an ECS public IP/domain, for a more realistic cloud deployment smoke;
- an Alibaba Cloud internal endpoint only when PTS and the SUT are in the same reachable VPC setup.

For temporary local smoke, start:

```powershell
cloudflared tunnel --url http://127.0.0.1:18080
```

Use the generated host, for example `abc.trycloudflare.com`, as the PTS domain.

## Legacy Smoke Scripts

The old `live-auction-pts-smoke.jmx` and
`live-auction-pts-business-smoke.jmx` assets were removed from the current
S1-S5 pressure tree. Recreate a tiny local JMeter smoke only when connectivity
debugging needs it; do not cite it as current capacity evidence.

The historical tiny connectivity shape was:

1. `GET /readyz`
2. `POST /api/auth/login` with `{"account":"user"}`
3. `GET /api/rooms`
4. `GET /api/rooms/${ROOM_ID}/auctions`

The historical business-chain smoke shape was:

1. readiness;
2. bidder login/session;
3. room and active auction discovery;
4. auction snapshot;
5. leaderboard and next bid extraction;
6. WS ticket issuance;
7. one idempotent bid;
8. post-bid leaderboard, bid history, and orders;
9. host login;
10. host diagnostics: monitor auctions, flight recorder, rejects, outbox, outbox watermarks;
11. Prometheus `/metrics`.

Thread settings in the JMX are intentionally tiny:

- threads: `1`
- loop: `1`
- ramp: `1s`

## PTS Parameters

Set these JMeter variables in PTS:

| Name | Example | Note |
|---|---|---|
| `PROTOCOL` | `https` | Cloudflare/ngrok usually uses HTTPS |
| `HOST` | `abc.trycloudflare.com` | Host only, no `https://` prefix |
| `PORT` | empty | Leave empty for HTTPS default |
| `ROOM_ID` | `room_main` | Must exist in the local seed data |
| `DEFAULT_AUCTION_ID` | `auc_live` | Fallback when JSON extraction cannot find an active auction |

Do not use the LAN IP `192.168.1.104` as `HOST`.

## Local Prerequisites

Make sure the app is ready before starting the tunnel:

```powershell
curl http://127.0.0.1:18080/readyz
```

Expected:

```json
{"status":"ready"}
```

Then verify the tunnel:

```powershell
curl https://abc.trycloudflare.com/readyz
```

## Expected PTS Result

For `live-auction-pts-smoke.jmx`, all four samplers should pass with HTTP 200.

For `live-auction-pts-business-smoke.jmx`, all numbered samplers should pass. If sampler `10 place bid` returns a business `REJECTED` result, the script can still be useful for connectivity, but do not call it a clean business smoke until the seeded auction is `ACTIVE` and executable. Re-run `p0smokeseed` before a clean local or ECS smoke if the auction has already been sold, cancelled, or expired.

If a sampler shows `200/failed`, the network and HTTP route were reachable but a JMeter assertion failed. Check the assertion name and response body. For example, a snapshot response with `"status":"SOLD"` means the demo auction has already reached the cap/hammer state; reseed or create a fresh active auction before expecting the bid step to succeed.

Known limits:

- Cloudflare/ngrok tunnel latency is not representative.
- This smoke does not test WebSocket fanout or bid pressure.
- This smoke does not produce valid performance numbers.

## Where To See Results In PTS

After the PTS run finishes, open the pressure-test report and check these views:

- request details by sampler name: look for `01 public readiness`, `10 place bid`, `16 flight recorder`, etc.;
- response time percentiles by sampler: p50/p90/p95/p99 show which API is slow from PTS;
- success/error count by sampler: identifies whether failure is login, room discovery, bid, monitor, or metrics;
- sampled response bodies or error samples, if enabled: use this to distinguish HTTP failures from business rejects such as `AUCTION_NOT_ACTIVE`, `RATE_LIMITED`, or `BID_AUCTION_TOO_HOT`.

PTS "success" only means the JMeter assertions passed. It does not explain whether the backend was close to saturation, whether PostgreSQL lock wait grew, whether outbox lag accumulated, or whether WebSocket fanout was healthy.

## How Bottleneck Diagnosis Works

Use PTS and the system machine together:

| Source | What it tells you | What it cannot tell you alone |
|---|---|---|
| PTS report | Which external request is slow or failing, by sampler name | Whether the cause is DB lock, Redis, outbox, Go runtime, or network |
| Backend logs | path, status, trace id, rough request timeline | percentile latency and client-side network latency |
| `/metrics` | HTTP latency, bid latency, lock wait, admission counters, runtime/app counters | exact SQL plan or OS resource cause |
| PC diagnostics / monitor APIs | auction state, rejects, outbox delivery, recovery, flight-recorder timeline | load-generator health |
| PostgreSQL | locks, active queries, slow statements, pool pressure | browser or PTS network latency |
| Redis | command latency, memory, evictions, ticket/limiter symptoms | DB lock contention |
| ECS/Docker host | CPU, memory, disk, network, file descriptors | business-level reject reason |

The workflow is:

1. Use PTS sampler names to find the first slow or failing step.
2. On the system machine, collect `/metrics`, backend logs, DB, Redis, and Docker/ECS stats during the same time window.
3. Attribute only when both sides agree. For example, `10 place bid` p99 high plus `auction_bid_lock_wait_seconds` high points to the hot auction row; `08 ws ticket` high plus Redis latency points to ticket/Redis; `18 monitor outbox` high plus outbox backlog points to relay/outbox pressure.

## Quick System-Machine Diagnostics

Run these on the machine hosting the system while PTS is running. Replace the host when testing ECS instead of the local tunnel.

```powershell
curl http://127.0.0.1:18080/readyz
curl http://127.0.0.1:18080/metrics
curl http://127.0.0.1:18080/api/monitor/auctions -H "X-Mock-Role: host" -H "X-Mock-User-Id: host_1"
curl http://127.0.0.1:18080/api/monitor/rejects -H "X-Mock-Role: host" -H "X-Mock-User-Id: host_1"
curl http://127.0.0.1:18080/api/monitor/outbox -H "X-Mock-Role: host" -H "X-Mock-User-Id: host_1"
curl http://127.0.0.1:18080/api/monitor/outbox/watermarks -H "X-Mock-Role: host" -H "X-Mock-User-Id: host_1"
docker stats
docker compose -f infra/docker-compose.yml logs --tail=200 backend
```

For PostgreSQL lock/activity drilldown:

```powershell
docker compose -f infra/docker-compose.yml exec postgres psql -U live_auction -d live_auction -c "select pid, state, wait_event_type, wait_event, now() - query_start as age, left(query, 160) as query from pg_stat_activity where datname = 'live_auction' order by age desc limit 20;"
docker compose -f infra/docker-compose.yml exec postgres psql -U live_auction -d live_auction -c "select locktype, mode, granted, count(*) from pg_locks group by locktype, mode, granted order by count(*) desc;"
```

For Redis:

```powershell
docker compose -f infra/docker-compose.yml exec redis redis-cli INFO memory
docker compose -f infra/docker-compose.yml exec redis redis-cli LATENCY LATEST
```

## What Common Failures Mean

| Slow/failing sampler | First diagnosis |
|---|---|
| `01 public readiness` | tunnel/ECS security group/backend process/readiness dependency |
| `02 bidder login` or `14 host login` | seed data, auth session table, cookie handling |
| `05 list room auctions` | room seed, membership ACL, wrong `ROOM_ID` |
| `06 auction snapshot` | wrong auction id or missing membership |
| `08 ws ticket` | Redis/ticket path, ACL, realtime admission |
| `10 place bid` | auction not active, business rule reject, admission, PG hot row lock, idempotency |
| `16 flight recorder` | host auth, monitor SQL, DB pressure |
| `18/19 monitor outbox` | outbox table/relay backlog |
| `20 metrics` | metrics endpoint exposure or backend process |

If `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, HTTP `429`, or WS admission rejection dominates, the run primarily proves admission protection. It is not evidence of a PostgreSQL/outbox/WebSocket bottleneck unless admission ceilings are deliberately changed and documented.

## Formal Cloud Run Discipline

When the system is deployed on ECS, use the ECS public domain/IP for smoke. For formal performance evidence, run the system on Linux/ECS, collect server metrics, record repeat runs per workload, and follow `docs/design/02-performance-correctness-contract.md`. Do not cite local raw artifacts as judge-facing proof unless they include `ENGINE_*`, durability, settlement, verifier, and fault-injection evidence.

Do not use the Cloudflare quick tunnel for capacity conclusions. It is only a temporary reachability and script-debugging bridge.
