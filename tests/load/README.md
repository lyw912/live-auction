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

P3 downstream-pressure bid run, only for local bottleneck attribution with admission fully disabled:

```powershell
$env:ADMISSION_ENABLED='false'
$env:RATE='300'
$env:DURATION='45s'
$env:PRE_ALLOCATED_VUS='320'
$env:MAX_VUS='800'
k6 run --summary-export docs\perf\raw\p3-bid-pressure.json tests\load\p3-bid-pressure.js
```

Label this as a downstream-pressure profile. It is not a production capacity profile and it is not comparable to admission-on abuse tests. Do not use guessed high admission limits for performance exploration; use `ADMISSION_ENABLED=false` and verify there are no admission rejection counter deltas.

Preferred P3 runner for new pressure work:

```powershell
$env:P3_PROFILE='downstream-pressure'
$env:WORKLOADS='p3-bid-pressure'
$env:RATE='300'
$env:DURATION='45s'
$env:PRE_ALLOCATED_VUS='320'
$env:MAX_VUS='800'
node tests\load\run-p3-local-stress.mjs
```

`P3_PROFILE=downstream-pressure` is the default. The runner starts managed workloads with `ADMISSION_ENABLED=false`, records the effective admission environment, verifies `auction_admission_enabled 0` before and after each workload, and fails downstream-pressure evidence if admission rejection counters increase.

Artifact retention is intentionally minimal by default. `P3_ARTIFACT_MODE=minimal` keeps compact analysis files, k6 summary JSON, before/after metrics, and environment metadata. Full logs, during-sample metrics, DB snapshots, and readyz dumps are kept for failed workloads, or for all workloads only when `P3_ARTIFACT_MODE=full`. Generated backend binaries are built under `backend/tmp`, not under `docs/perf/raw`.

For attribution or comparison, read the compact index first:

```powershell
pnpm exec node tests/load/analyze-p3-artifacts.mjs
```

This writes `docs/perf/raw/p3-artifact-index.json` and prints the latest workload verdict hints. Open raw JSON, Prometheus snapshots, DB snapshots, or logs only after the compact report points to a specific workload and candidate bottleneck.

Default `P3_ARTIFACT_MODE=minimal` is enough for normal P3 development: smoke, regression checks, admission pollution checks, environment-limit detection, and first-pass bottleneck attribution. Use `P3_ARTIFACT_MODE=full` only for a single focused drilldown when the compact report says the run reached the backend, admission stayed off, environment signals are clean, and the remaining question needs full logs, during metrics, DB snapshots, or runtime profiles.

When adding a P3 pressure script, design it so the first result can distinguish three cases:

- admission/protection interference: HTTP `429`, `RATE_LIMITED`, `BID_AUCTION_TOO_HOT`, WebSocket ticket/connect admission, or admission counter deltas;
- environment or load-generator limit: k6 `dropped_iterations`, max-VU exhaustion, local connect refusal, socket/ephemeral-port exhaustion, timeout before backend saturation, or Docker/Windows resource ceiling;
- real system bottleneck: PG lock/pool/transaction growth, outbox backlog/lag, WebSocket queue/fanout/slow-close growth, Redis latency, Go CPU/heap/goroutine growth, or invariant failure.

Do not write a script that reports only pass/fail or HTTP status. Add k6 counters/trends for business outcomes and preserve raw logs so the runner can classify admission pollution and environment limits before anyone claims a PG, outbox, or realtime bottleneck.

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
- P3 local stress cadence and bottleneck drilldown rules are defined in `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md`; use `live-auction-v2-stress-attacker` for adversarial pressure rounds.
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
