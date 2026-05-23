# P1 k6 Baseline Suite

These scripts implement the P1-03 workload plan from `docs/design-v2-industrial/09-performance-and-benchmark.md`.

Prerequisites:

```powershell
docker compose -f infra\docker-compose.yml up -d postgres redis minio prometheus grafana
cd backend
go run ./cmd/p1loadseed
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

`p1loadseed` keeps `auc_live` ACTIVE with a high cap so burst scripts measure bid contention instead of immediately turning the auction SOLD. It also creates active `room_main` memberships for seeded demo users and k6 users so P2 room ACL is exercised instead of bypassed. Set `ALLOW_SOLD=true` only when intentionally testing cap hammer behavior.

Room and auction defaults:

```powershell
$env:ROOM_ID='room_main'
$env:AUCTION_ID='auc_live'
```

Future P2/P7 multi-room runs should seed additional rooms and pass their IDs explicitly rather than assuming a single fixed room.

Formal baseline rules:

- Run on Linux native or a clearly documented equivalent.
- Record 3 raw runs per workload before publishing any QPS/P99/fanout/online-user claim.
- Use `docs/design-v2-industrial/templates/perf-baseline.md`.
- Do not use local Windows smoke outputs as final capacity evidence.
