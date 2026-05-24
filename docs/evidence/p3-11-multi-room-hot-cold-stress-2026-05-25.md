# P3-11 Multi-Room Hot/Cold Stress

Date: 2026-05-25 Asia/Shanghai

Status: `BOTTLENECK_FOUND`

## Target

P3-R2 asks whether a hot auction room can degrade an unrelated cold room under real backend pressure.

This round uses downstream-pressure mode:

- `ADMISSION_ENABLED=false`;
- real backend, PostgreSQL, Redis, outbox relay, and WebSocket paths;
- hot room: `room_main` / `auc_live`;
- cold room: `room_side` / `auc_side`;
- port cleanup verified after runs.

## Harness Changes

`tests/load/multi-room-isolation.js` now records per-room signals:

- hot bid response count, business response rate, and latency trend;
- cold WebSocket open, error, message, and first-message timing;
- cold room low-rate heartbeat bids and latency trend;
- cross-room leak rate.

`tests/load/run-p3-local-stress.mjs` now passes `COLD_SESSION_SECONDS` and `HOT_SLEEP_SECONDS` through to the multi-room workload and preserves custom trend p95/p99/max values in compact reports.

## Rounds

| Round | Raw path | Profile | Load | Result |
|---|---|---|---|---|
| Smoke | `docs/perf/raw/p3-r2-multi-room-smoke-20260525-01/` | downstream-pressure | 4 hot VU, 2 cold WS, 8s | PASS, harness check. |
| Hot/cold 01 | `docs/perf/raw/p3-r2-multi-room-hotcold-20260525-01/` | downstream-pressure | 80 hot VU, 40 cold WS, 45s | PASS; hot bid p99 about 947ms; no cold WS errors or cross-room leak. |
| Hot/cold 02 | `docs/perf/raw/p3-r2-multi-room-hotcold-20260525-02/` | downstream-pressure | 160 hot VU, 80 cold WS, 60s | ENV_LIMIT signal: Windows/local accept refused on bid POST; not used as system bottleneck proof. |
| Hot/cold 03 | `docs/perf/raw/p3-r2-multi-room-hotcold-20260525-03/` | downstream-pressure | 100 hot VU, 50 cold WS, 2 cold bid rps, 60s | PASS; clean attribution run. |
| Cold baseline | `docs/perf/raw/p3-r2-multi-room-cold-baseline-20260525-01/` | downstream-pressure | 1 hot VU sleeping 1s, 50 cold WS, 2 cold bid rps, 60s | PASS; baseline for cold room latency. |

## Evidence

Clean hot/cold run `p3-r2-multi-room-hotcold-20260525-03`:

- admission proof: `auction_admission_enabled 0/0`, all admission reject deltas zero;
- environment signals: none;
- dropped iterations: `0`;
- checks: `11144/11144` passed;
- HTTP failures: `0`;
- hot bid responses: `10923`;
- hot bid latency: p95 about `845ms`, p99 about `1114ms`;
- cold bid responses: `121`;
- cold bid latency under hot load: p95 about `506ms`, p99 about `518ms`;
- cold WS opened: `50`;
- cold WS errors: `0`;
- cross-room leak rate: `0`;
- DB lock metric: `lock_auction_for_bid` count `11044`, sum about `1037s`.

Cold baseline `p3-r2-multi-room-cold-baseline-20260525-01`:

- admission proof: `auction_admission_enabled 0/0`, all admission reject deltas zero;
- environment signals: none;
- dropped iterations: `0`;
- checks: `281/281` passed;
- HTTP failures: `0`;
- cold bid responses: `121`;
- cold bid latency: p95 about `25ms`, p99 about `28ms`;
- cold WS opened: `50`;
- cold WS errors: `0`;
- cross-room leak rate: `0`;
- DB lock metric: `lock_auction_for_bid` count `181`, sum about `0.125s`.

Post-run port checks showed only `TIME_WAIT` entries on checked local ports, no active owner process.

## Attribution

Verdict: `BOTTLENECK_FOUND`.

The hot room does not leak events to the cold room and does not break cold WebSocket connectivity in this Windows-local run. The bottleneck is shared bid-path database serialization: heavy hot-room bid traffic increases lock/query time and degrades unrelated cold-room bid latency from about `25ms` p95 baseline to about `506ms` p95.

The higher 160 hot VU / 80 cold WS round hit Windows/local `connectex actively refused` on bid POST. That is an environment ceiling signal and is not used as the primary bottleneck proof.

## Design Decision

Do not implement a shortcut that drops low bids before the auction lock in this round.

Reason: current rejected bids are audited as `auction_events`, increment auction `seq`, and are delivered through the outbox/realtime path. Silently rejecting outside the serialized path would change audit/recovery semantics and needs an ADR plus invariant proof. This belongs with P3-R4/P3-R6/P4, not as an unsafe P3-R2 hot patch.

## Next

P3-R3 should run clean realtime fanout and slow-consumer drilldown.

P3-R4 should investigate PG hot-row/shared pool pressure with lock, transaction, and pool evidence, then decide whether to:

- preserve current semantics and accept the shared-resource cost;
- add safe per-room or per-auction concurrency isolation;
- introduce an ADR-backed early-reject/audit policy;
- or defer Redis Lua reservation until reconciliation and invariant proof exist.
