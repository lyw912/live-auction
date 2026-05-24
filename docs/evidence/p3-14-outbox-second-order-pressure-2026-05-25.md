# P3-14 Outbox Second-Order Pressure

Date: 2026-05-25 Asia/Shanghai

Status: `BOTTLENECK_FOUND_AND_OPTIMIZED`

## Target

P3-R5 investigates the outbox backlog left by P3-R4. The target is relay drain rate, backlog growth, and whether the second-order pressure comes from claim/update/publish/watermark work.

All accepted rounds used:

- `P3_PROFILE=downstream-pressure`;
- `ADMISSION_ENABLED=false`;
- real backend, PostgreSQL, Redis, embedded outbox relay, and bid HTTP paths;
- `P3_ARTIFACT_MODE=full`;
- managed isolated backend per workload;
- post-workload observation;
- post-run local port cleanup.

## Harness Change

`tests/load/run-p3-local-stress.mjs` now supports `POST_WORKLOAD_OBSERVE_SECONDS`. In full artifact mode, the runner keeps the managed backend alive after k6 exits and continues sampling metrics, `pg_stat_activity`, `pg_locks`, and outbox status.

This closes a P3 evidence gap: outbox pressure must show both backlog growth under load and drain behavior after input stops.

## Rounds

| Round | Raw path | Load | Result |
|---|---|---|---|
| Before | `docs/perf/raw/p3-r5-outbox-burst-drain-20260525-01/` | 160 VU, no sleep, 45s, 30s post-observe | PASS; backlog persisted and relay watermark refresh dominated samples. |
| After | `docs/perf/raw/p3-r5-outbox-burst-batch-watermark-20260525-01/` | same load and observe window | PASS; relay drain improved after batching watermark refresh. |

## Evidence

### Before

`p3-r5-outbox-burst-drain-20260525-01`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- iterations: `9242`;
- dropped iterations: `0`;
- HTTP failures: `0`;
- business responses: `9242`;
- bid outcomes: `9233` `BID_TOO_LOW`, `8` `REJECTED_SELF_LEADING`, `1` accepted;
- k6 p99 HTTP duration: about `1299ms`;
- `auction_bid_lock_wait_seconds`: count `9242`, sum about `781.8s`;
- outbox published by relay during run plus post-observe: `auction_outbox_lag_seconds_count` `1053`;
- final sample at k6 end: `9171` pending;
- after 30s post-observe: `8190` pending;
- post-observe drain: about `981` events in 30s, roughly `33/s`;
- DB activity samples after pressure stopped repeatedly showed `INSERT INTO outbox_relay_watermarks ... SELECT ... FROM outbox_delivery`.

### Optimization

The accepted code change is conservative:

- keep the same `claimOne`, publish, Redis history/snapshot, outbox delivery update, ordering, DEAD, and gap semantics;
- keep `ProcessOne` behavior refreshing watermark immediately for existing diagnostic tests and one-shot flows;
- change `ProcessBatch` to process events with per-event delivery state updates but refresh `outbox_relay_watermarks` once per touched shard at the end of the batch.

This removes one full-shard watermark aggregation per published event during relay batch drain.

### After

`p3-r5-outbox-burst-batch-watermark-20260525-01`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- iterations: `9178`;
- dropped iterations: `0`;
- HTTP failures: `0`;
- business responses: `9178`;
- bid outcomes: `9169` `BID_TOO_LOW`, `8` `REJECTED_SELF_LEADING`, `1` accepted;
- k6 p99 HTTP duration: about `1343ms`;
- `auction_bid_lock_wait_seconds`: count `9178`, sum about `783.4s`;
- outbox published by relay during run plus post-observe: `auction_outbox_lag_seconds_count` `4785`;
- final sample at k6 end: `8993` pending;
- after 30s post-observe: `4427` pending;
- post-observe drain: about `4566` events in 30s, roughly `152/s`.

## Quantitative Delta

Same local Windows setup, same 160 VU / 45s / 30s post-observe workload:

| Metric | Before | After | Delta |
|---|---:|---:|---:|
| k6 iterations | `9242` | `9178` | comparable |
| admission reject delta | `0` | `0` | unchanged |
| dropped iterations | `0` | `0` | unchanged |
| published count observed by outbox lag metric | `1053` | `4785` | about `4.5x` higher |
| pending at k6 end | `9171` | `8993` | similar generation pressure |
| pending after 30s post-observe | `8190` | `4427` | about `46%` lower |
| post-observe drain rate | about `33/s` | about `152/s` | about `4.6x` higher |

These are Windows-local regression/direction numbers only. They are not final capacity claims.

## Attribution

Verdict: `BOTTLENECK_FOUND_AND_OPTIMIZED`.

P3-R5 found that the relay was spending too much work refreshing shard watermarks once per published event under backlog. Batching watermark refresh by touched shard improved drain rate materially without weakening delivery state semantics.

Remaining pressure is still real:

- input generated roughly `200` bid responses per second;
- optimized single embedded relay drained roughly `152/s` during the post-observe window;
- backlog remained after 30 seconds.

The next bottleneck candidate is single-relay sequential claim/publish/update throughput under one hot auction. P3-R6 should decide whether to keep current single relay for release, tune admission below this drain cliff, or introduce an ADR-backed parallel/batch relay plan.

## Design Decision

Keep the DB-backed relay mainline. Do not introduce Debezium/CDC from this evidence alone.

Reason: the current bottleneck improved with project-owned relay batching. CDC would add runtime complexity and still need auction seq, delivery status, gap notice, Redis history/snapshot, diagnostics, and rollback semantics.

## Next

P3-R6 architecture go/no-go should use P3-R4 and P3-R5:

- PostgreSQL bid truth remains current mainline;
- self-hub remains current realtime mainline;
- outbox relay batching is improved but not fully above the tested input rate;
- any parallel relay or CDC decision needs an ADR and invariant evidence;
- admission calibration must stay below the measured downstream cliff.
