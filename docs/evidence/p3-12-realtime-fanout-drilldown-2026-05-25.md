# P3-12 Realtime Fanout Drilldown

Date: 2026-05-25 Asia/Shanghai

Status: `NO_REGRESSION_WITH_CEILING`

## Target

P3-R3 validates the app-owned self-hub under fanout, slow-consumer, and reconnect pressure without confusing the result with admission or Windows connect-storm noise.

All accepted evidence below used:

- `P3_PROFILE=downstream-pressure`;
- `ADMISSION_ENABLED=false`;
- real backend, PostgreSQL, Redis, outbox relay, and WebSocket paths;
- `P3_ARTIFACT_MODE=full`;
- managed isolated backend per workload;
- post-run local port cleanup.

## Rounds

| Round | Raw path | Load | Verdict |
|---|---|---|---|
| Staggered fanout | `docs/perf/raw/p3-r3-fanout-clean-20260525-01/` | 300 watchers, 40 trigger rps, 60s, 10ms connect stagger | Clean PASS. |
| Healthy vs slow high trigger | `docs/perf/raw/p3-r3-healthy-vs-slow-20260525-01/` | 150 healthy + 150 slow, 40 trigger rps | ENV_LIMIT/PG pollution signal: dropped iterations and bid lock tail; healthy WS errors stayed 0. |
| Healthy vs slow clean | `docs/perf/raw/p3-r3-healthy-vs-slow-clean-20260525-03/` | 150 healthy + 150 slow, 5 trigger rps | Clean PASS. |
| Reconnect high | `docs/perf/raw/p3-r3-reconnect-clean-20260525-01/` | 200 reconnect VU, 45s | Ceiling signal: small ticket/reconnect error rate under snapshot/recovery pressure. |
| Reconnect clean | `docs/perf/raw/p3-r3-reconnect-clean-20260525-02/` | 100 reconnect VU, 45s | Clean PASS. |

## Evidence

### Staggered Fanout

`p3-r3-fanout-clean-20260525-01`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- environment signals: none;
- dropped iterations: `0`;
- watchers opened: `300`;
- WS errors: `0`;
- trigger business response rate: `1.0`;
- fanout messages observed by k6: `720600`;
- backend `auction_ws_publish_subscribers_count`: `2401`;
- backend `auction_ws_publish_subscribers_sum`: `720300`;
- backend send queue depth sum: `0`;
- runtime goroutines after run: `12`.

### Healthy vs Slow

Clean run `p3-r3-healthy-vs-slow-clean-20260525-03`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- environment signals: none;
- dropped iterations: `0`;
- healthy opened: `150`;
- slow opened: `150`;
- healthy WS errors: `0`;
- healthy messages: `45300`;
- slow messages: `4551`;
- backend send queue depth sum: `0`;
- runtime goroutines after run: `12`.

High-trigger run `p3-r3-healthy-vs-slow-20260525-01`:

- healthy WS errors still `0`;
- dropped iterations `124` and bid lock tail appeared;
- use this as a PG/load-generator pressure ceiling signal, not self-hub failure evidence.

### Reconnect

Clean run `p3-r3-reconnect-clean-20260525-02`:

- admission proof: enabled `0/0`, all admission reject deltas zero;
- environment signals: none;
- dropped iterations: `0`;
- checks: `7598/7598`;
- HTTP failures: `0`;
- recovered reconnects: `3799`;
- recovery sources: `3599` history, `100` DB snapshot, `100` Redis snapshot;
- runtime goroutines after run: `12`.

High reconnect run `p3-r3-reconnect-clean-20260525-01`:

- recovered reconnects: `6997`;
- reconnect errors: `6`;
- ticket/check failures: `10`;
- HTTP failed rate about `0.14%`;
- keep this as reconnect/snapshot ceiling evidence, not a clean release claim.

## Interpretation

Verdict: `NO_REGRESSION_WITH_CEILING`.

The self-hub remains acceptable as the only runtime realtime implementation for the current release path under the tested Windows-local clean rounds:

- 300 watcher fanout delivered without WS errors, queue growth, or goroutine growth;
- mixed healthy/slow clients did not produce healthy-client WS errors at the clean trigger profile;
- 100 VU reconnect recovery completed without errors or dropped iterations.

The current ceilings are outside the self-hub core fanout loop:

- high trigger profiles are polluted by bid-path/PG pressure already found in P3-R2;
- 200 VU reconnect pressure exposes small ticket/recovery error rate and should feed reconnect/snapshot drilldown or admission calibration later.

This is Windows-local direction/regression evidence only. It is not a final capacity claim.

## Next

P3-R4 should use P3-R2 and P3-R3 as input for PG hot-row/shared bid-path drilldown.

P3-R6 should keep self-hub as the runtime default unless later Linux/final evidence contradicts these clean local rounds.
