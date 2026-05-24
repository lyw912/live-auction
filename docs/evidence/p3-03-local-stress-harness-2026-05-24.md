# P3 Local Stress Harness Repair

Date: 2026-05-24 Asia/Shanghai

Status: `HARNESS_GAP_FIXED`

This is Windows local harness evidence, not capacity evidence.

## Problem

The P3 stress loop had two Windows-local reliability gaps:

- Node/PowerShell wrappers could leave the real backend or relay binary alive after the parent process exited.
- k6 can exit successfully even when a workload produces zero completed iterations and zero checks. The old runner treated that as `PASS`.

This created false confidence and also explained why later workloads could appear to hang after a previous workload.

## Fix

Updated `tests/load/run-p3-local-stress.mjs`:

- can build and manage a backend binary directly with `MANAGE_SERVER=1`;
- starts processes without shell wrappers;
- supports explicit `P3_PROFILE=admission-on` and `P3_PROFILE=downstream-pressure`;
- supports `WORKLOADS=...` for focused probes;
- defaults to isolated workload execution, starting a fresh backend process per workload;
- validates k6 summaries and fails any workload with zero checks;
- captures `/readyz`, `/metrics`, PostgreSQL activity, and lock snapshots on failure.

Updated load scripts:

- `final-second-bid-burst.js`
- `outbox-burst.js`
- `bid-abuse.js`

These now accept `GRACEFUL_STOP` so failed local probes do not spend 30 seconds in k6 graceful-stop limbo.

## Evidence

Admission-on full smoke:

```text
MANAGE_SERVER=1 P3_PROFILE=admission-on DURATION=5s VUS=2 COLD_WS_VUS=1 WORKLOAD_TIMEOUT_MS=90000 pnpm exec node tests/load/run-p3-local-stress.mjs
```

Raw output:

- `docs/perf/raw/p3-local-stress-202605240620/`
- `summary.json`: preflight, final-second-bid-burst, outbox-burst, watcher-fanout, slow-consumer, reconnect-storm, multi-room-isolation, and bid-abuse all passed.
- `bid-abuse.log`: 143 limited responses, threshold `auction_k6_bid_limited_total count>0` passed.

Downstream-pressure realtime and isolation smoke:

```text
MANAGE_SERVER=1 P3_PROFILE=downstream-pressure WORKLOADS=watcher-fanout,slow-consumer,reconnect-storm,multi-room-isolation DURATION=10s VUS=8 COLD_WS_VUS=4 WORKLOAD_TIMEOUT_MS=120000 pnpm exec node tests/load/run-p3-local-stress.mjs
```

Raw output:

- `docs/perf/raw/p3-local-stress-202605240623/`
- `summary.json`: watcher-fanout, slow-consumer, reconnect-storm, and multi-room-isolation all passed.
- `watcher-fanout.log`: 80 checks, 100% succeeded, 128 WebSocket messages observed.
- `multi-room-isolation.log`: 300 checks, 100% succeeded, cross-room leak rate 0, cold WebSocket errors 0.

## Interpretation

This closes the immediate local P3 harness gap. It does not prove final fanout capacity or Linux p99. It makes the Windows P3 loop reliable enough to continue finding bottleneck direction and regressions.

The next P3 attack should increase watcher fanout and slow-consumer pressure in focused downstream-pressure runs, with metrics snapshots and runtime profiles, before deciding whether the self hub is sufficient or a realtime adapter is justified.
