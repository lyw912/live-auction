# P1 Toxiproxy Weak-Network Scenarios

These assets implement P1-04 from `docs/design-v2-industrial/01-scope-and-roadmap.md`.

Start Toxiproxy:

```powershell
docker compose -f infra\docker-compose.yml up -d toxiproxy
```

Configure a scenario:

```powershell
node tests/chaos/run-toxiproxy-scenario.mjs redis_latency_reconnect
node tests/chaos/run-toxiproxy-scenario.mjs redis_timeout_reconnect
node tests/chaos/run-toxiproxy-scenario.mjs postgres_bid_latency
```

Inspect or clear configured toxics:

```powershell
node tests/chaos/run-toxiproxy-scenario.mjs --status
node tests/chaos/run-toxiproxy-scenario.mjs --clear
```

Run the backend through proxies when testing faults:

```powershell
$env:DATABASE_URL='postgres://live_auction:live_auction@localhost:15432/live_auction?sslmode=disable'
$env:REDIS_ADDR='localhost:16379'
$env:HTTP_ADDR='127.0.0.1:18080'
cd backend
go run ./cmd/server
```

Use the existing Playwright live smoke or k6 reconnect scripts against that backend. Record raw command output under `docs/evidence/` or `docs/perf/raw/` and do not claim production chaos resilience from local smoke alone.
