# P1 k6 Baseline Suite

These scripts implement the P1-03 workload plan from `docs/design-v2-industrial/09-performance-and-benchmark.md`.

Prerequisites:

```powershell
docker compose -f infra\docker-compose.yml up -d postgres redis minio prometheus grafana
cd backend
go run ./cmd/p0smokeseed
$env:HTTP_ADDR='127.0.0.1:18080'
go run ./cmd/server
```

Run smoke-sized checks:

```powershell
k6 run --summary-export docs\perf\raw\final-second-bid-burst-smoke.json tests\load\final-second-bid-burst.js
k6 run --summary-export docs\perf\raw\watcher-fanout-smoke.json tests\load\watcher-fanout.js
k6 run --summary-export docs\perf\raw\reconnect-storm-smoke.json tests\load\reconnect-storm.js
k6 run --summary-export docs\perf\raw\slow-consumer-smoke.json tests\load\slow-consumer.js
k6 run --summary-export docs\perf\raw\outbox-burst-smoke.json tests\load\outbox-burst.js
```

Formal baseline rules:

- Run on Linux native or a clearly documented equivalent.
- Record 3 raw runs per workload before publishing any QPS/P99/fanout/online-user claim.
- Use `docs/design-v2-industrial/templates/perf-baseline.md`.
- Do not use local Windows smoke outputs as final capacity evidence.
