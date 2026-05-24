# P3-00 Stress Attacker Round 1

Date: 2026-05-24 Asia/Shanghai

Status: `BOTTLENECK_FOUND`

Raw output root: `docs/perf/raw/p3-attack-20260524-035952/`

This is the first real P3-00 stress-attacker result. It supersedes the earlier `p3-00` smoke bundle as bottleneck evidence. The earlier bundle remains useful only as harness validation: seed/auth/ACL/WebSocket lifecycle could run, but the run was dominated by admission `429` and did not prove downstream pressure.

This run is Windows local evidence. It proves bottleneck direction and concrete failure mode under the same local setup. It is not a final capacity claim.

## Target

- Subsystem: PostgreSQL hot auction row and outbox relay claim path.
- Hypothesis: once admission ceilings are explicitly raised, the current single-auction write path and outbox relay query will become the first P3 bottleneck before realtime transport does.
- Why this matters: P3 architecture work must start from measured bottlenecks. This result gives a concrete fix target before making relay shard ownership, Redis Lua, CDC, or other data-path decisions.

## Pressure Profile

Backend was run with downstream-pressure admission ceilings:

```text
HTTP_ADDR=127.0.0.1:18080
ALLOW_MOCK_AUTH=true
BID_USER_LIMIT_PER_SECOND=100000
BID_IP_LIMIT_PER_SECOND=100000
BID_AUCTION_LIMIT_PER_SECOND=100000
BID_AUCTION_MAX_IN_FLIGHT=512
```

Seed and dependency setup:

```text
docker compose -f infra\docker-compose.yml up -d postgres redis minio
goose -dir backend\migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
cd backend
go run ./cmd/p1loadseed
```

Temporary attack scripts:

- `tests/load/tmp-p3-bid-pressure.js`
- `tests/load/tmp-p3-ws-fanout-pressure.js`

These scripts were intentionally marked temporary because they are pressure probes, not the committed long-term runner yet.

## Rounds

| Round | Script | Load model | Scale/duration | Result | Next hypothesis |
|---|---|---|---|---|---|
| R1 | `tmp-p3-bid-pressure.js` | `constant-arrival-rate` | `50 rps / 20s` | 1001 iterations, no HTTP errors, but only 5 accepted because the first script version did not keep amounts ahead of current price | Fix amount generation so accepted writes continue |
| R2 | `tmp-p3-bid-pressure.js` | `constant-arrival-rate` | `100 rps / 30s` | 3000 iterations, 2980 accepted, 20 rejected, no `429`, p99 `23.50ms` | Raise pressure until hot-row/outbox saturation appears |
| R3 | `tmp-p3-bid-pressure.js` | `constant-arrival-rate` | target `300 rps / 45s` | 8393 iterations, 5108 dropped iterations, effective `~169 rps`, p95 `4.97s`, p99 `5.25s`, no `429` | Attribute PG lock vs outbox relay |
| R4 | `tmp-p3-ws-fanout-pressure.js` | session/VU model | 200 watchers, 4 trigger VUs, `45s` | 200 WS sessions, 138600 messages, 0 WS errors, about `2519 msg/s` | WS hub is not the first bottleneck at this scale |

## K6 Results

| Round | Offered load | Iterations | Dropped | Accepted | Rejected | Limited/429 | p95 | p99 | p99.9 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| R1 bid sanity | 50 rps / 20s | 1001 | 0 | 5 | 996 | 0 | `13.27ms` | `19.76ms` | `42.46ms` |
| R2 bid pressure | 100 rps / 30s | 3000 | 0 | 2980 | 20 | 0 | `17.34ms` | `23.50ms` | `48.66ms` |
| R3 bid bottleneck | 300 rps / 45s target | 8393 | 5108 | 3244 | 5149 | 0 | `4.97s` | `5.25s` | `5.59s` |
| R4 WS fanout | 200 watchers | 2845 trigger iterations | 0 | n/a | n/a | 0 | HTTP `36.72ms` | HTTP `137.08ms` | HTTP `190.45ms` |

R4 WebSocket-specific result:

```text
ws_sessions: 200
auction_k6_ws_pressure_opened_total: 200
auction_k6_ws_pressure_messages_total: 138600
auction_k6_ws_pressure_errors_total: 0
ws_msgs_received rate: 2519.0359237314/s
```

## Evidence

Raw outputs:

- `docs/perf/raw/p3-attack-20260524-035952/round2-bid-pressure-100rps.json`
- `docs/perf/raw/p3-attack-20260524-035952/round3-bid-pressure-300rps.json`
- `docs/perf/raw/p3-attack-20260524-035952/round4-ws-fanout-200.json`
- `docs/perf/raw/p3-attack-20260524-035952/round3-outbox-claim-explain-after-30s.txt`
- `docs/perf/raw/p3-attack-20260524-035952/round3-db-activity-during.txt`
- `docs/perf/raw/p3-attack-20260524-035952/round3-outbox-status-during.txt`
- `docs/perf/raw/p3-attack-20260524-035952/round3-outbox-status-after.txt`
- `docs/perf/raw/p3-attack-20260524-035952/round3-outbox-status-after-30s.txt`

R3 DB wait snapshot:

```text
Lock / tuple / active: 8
LWLock / LockManager / active: 6
Lock / transactionid / active: 3
```

R3 outbox backlog:

```text
during:       PENDING=3401, PUBLISHED=5557
after:        PENDING=8326, PUBLISHED=5605
after 30 sec: PENDING=8142, PUBLISHED=5789
```

The relay drained only 184 rows in 30 seconds after pressure, while 8142 rows remained pending.

Outbox claim query plan after R3 backlog:

```text
Execution Time: 1584.153 ms
Rows Removed by Join Filter: 31305491
Seq Scan on outbox_events e: rows=13931
Seq Scan on outbox_delivery d: rows=8142 after filter
```

Metric deltas from R2 to R3:

```text
auction_bid_lock_wait_seconds:
  R2 cumulative: count=4001, sum=2.9384741s
  R3 cumulative: count=12394, sum=902.6219312s
  R3 delta:      count=8393, sum=899.6834571s, avg=107.19ms

auction_bid_latency_seconds:
  R2 cumulative: count=4001, sum=35.6663105s
  R3 cumulative: count=12394, sum=14611.8402431s
  R3 delta:      count=8393, sum=14576.1739326s, avg=1.737s
```

## Bottleneck Attribution

Primary bottleneck: outbox relay claim query and backlog behavior.

Evidence:

- R3 created a large pending backlog even though admission was raised and no `429` dominated the run.
- `outbox_delivery` had 8326 pending rows immediately after R3 and 8142 rows still pending 30 seconds later.
- `EXPLAIN (ANALYZE, BUFFERS)` for the current claim query took `1584.153ms`.
- The current anti-join removed `31305491` rows by join filter under only about 8k pending rows.

Secondary bottleneck: PostgreSQL hot auction row lock under open-model bid pressure.

Evidence:

- R3 hit k6 VU saturation: 800 active VUs and 5108 dropped iterations.
- R3 p95/p99 jumped from R2's `17.34ms/23.50ms` to `4.97s/5.25s`.
- DB wait snapshot captured tuple and transaction lock waits.
- `auction_bid_lock_wait_seconds` R3 delta averaged `107.19ms` per bid observation.

Not primary at this scale: WebSocket hub fanout.

Evidence:

- R4 held 200 watcher sessions with 0 WS errors.
- R4 received 138600 WS messages, about `2519 msg/s`.
- Runtime goroutines returned to 11 and `auction_ws_connections{room="room_main"}` returned to 0 after the run.

Alternative explanations ruled out:

- Admission ceiling: explicitly raised; R2 and R3 had no `429`/limited-dominated signal.
- ACL/auth harness: seed and mock-auth users were valid and used real HTTP/WS backend paths.
- Pure frontend/UI issue: no route-mocked browser path was used.

Remaining uncertainty:

- Windows local environment may amplify absolute p95/p99 and k6 VU behavior. Do not publish final capacity from this run.
- After fixing outbox claim, PG hot-row contention must be retested because R3 contains both row-lock pressure and relay backlog.

## Required Action

- [P0] Fix outbox claim complexity. The current `NOT EXISTS` older-unpublished anti-join is not viable under backlog. Replace it with an indexed per-auction next-publish strategy, shard/lease cursor, bounded batch claim, or another plan that avoids O(pending squared) behavior.
- [P0] Promote the temporary downstream-pressure probes into a durable P3 runner with a run ID, profile label, before/during/after metrics, DB snapshots, Redis snapshots, and invariant checks.
- [P1] Retest R3 after the outbox claim fix with the same local setup and the same `300 rps / 45s` profile.
- [P1] Only after outbox relay is no longer first bottleneck, run higher WS fanout profiles: 500 watchers, then 1000 watchers if local environment allows.

## Next Attack

Falsification run after outbox fix:

```text
RATE=300
DURATION=45s
PRE_ALLOCATED_VUS=320
MAX_VUS=800
k6 run --summary-export docs/perf/raw/<next-run>/bid-pressure-300rps.json tests/load/tmp-p3-bid-pressure.js
```

Pass conditions for the fix:

- no admission-dominated result;
- no monotonic outbox backlog growth during and after the run;
- outbox claim query remains sub-10ms with thousands of delivery rows;
- k6 does not hit 800 VUs with thousands of dropped iterations under the same local setup;
- DB wait snapshot no longer shows relay claim query as the dominant stalled path.
