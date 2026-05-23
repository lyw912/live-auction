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
k6 run --summary-export docs\perf\raw\multi-room-isolation-smoke.json tests\load\multi-room-isolation.js
k6 run --summary-export docs\perf\raw\bid-abuse-smoke.json tests\load\bid-abuse.js
```

`p1loadseed` keeps `auc_live` ACTIVE with a high cap so burst scripts measure bid contention instead of immediately turning the auction SOLD. It also creates active `room_main` memberships for seeded demo users and k6 users so P2 room ACL is exercised instead of bypassed. Set `ALLOW_SOLD=true` only when intentionally testing cap hammer behavior.

Room and auction defaults:

```powershell
$env:ROOM_ID='room_main'
$env:AUCTION_ID='auc_live'
```

Future P2/P7 multi-room runs should seed additional rooms and pass their IDs explicitly rather than assuming a single fixed room.

`p1loadseed` now creates `room_main/auc_live` as the hot baseline room and `room_side/auc_side` as the cold isolation room. The multi-room workload defaults to:

```powershell
$env:HOT_ROOM_ID='room_main'
$env:HOT_AUCTION_ID='auc_live'
$env:COLD_ROOM_ID='room_side'
$env:COLD_AUCTION_ID='auc_side'
```

P2 bid abuse smoke:

- Set low limits on the backend, for example `$env:BID_USER_LIMIT_PER_SECOND='1'` and `$env:BID_IP_LIMIT_PER_SECOND='2'`.
- Run the backend with `ALLOW_MOCK_AUTH=true` because the k6 harness uses local mock users for load generation.
- `bid-abuse.js` records accepted, rejected, `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, and `Retry-After` distribution. Treat this as abuse behavior evidence, not capacity evidence.

Formal baseline rules:

- Windows local smoke and relative comparisons are required during development; see `docs/perf/windows-local-strategy.md`.
- P3 local stress cadence and bottleneck drilldown rules are defined in `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`.
- Run final capacity baseline on Linux native or a clearly documented equivalent.
- Record 3 raw Linux runs per workload before publishing any QPS/P99/fanout/online-user claim.
- Use `docs/design-v2-industrial/templates/perf-baseline.md`.
- Do not use local Windows smoke outputs as final capacity evidence.

P2-07 harness:

```bash
node tests/load/run-p2-linux-baseline.mjs --final
```

The final runner refuses non-Linux hosts and low `ulimit -n`. It writes `docs/perf/raw/p2-07/environment.json`, one raw k6 summary per workload/run, one log per workload/run, and `docs/perf/p2-07-linux-baseline-round-1.md`.

For local script validation only:

```powershell
node tests\load\run-p2-linux-baseline.mjs --smoke
```

Smoke mode is not a capacity baseline and must not be used for performance claims.
