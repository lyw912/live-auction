# P3-01 Realtime Fanout Attack

Date: 2026-05-24 Asia/Shanghai

Status: `EVIDENCE_IN_PROGRESS`

This is Windows local evidence. It is useful for bottleneck direction, harness quality, and regression checks only. It is not final capacity evidence.

Policy update after this round: future P3/P4/P5 downstream-pressure evidence must use `ADMISSION_ENABLED=false`, not raised ceilings. Each run must prove `auction_admission_enabled 0` before and after the workload and must fail as bottleneck evidence if admission rejection counters increase.

## Target

Subsystem: realtime self hub, WebSocket fanout, connection startup, and slow consumers.

Hypothesis: if the self hub fails stable fanout or slow-consumer isolation under focused pressure, P3-01 should introduce a realtime adapter. If failures are limited to local connection storm or load-generator behavior, adapter adoption remains evidence-gated.

## Harness Updates

Updated `tests/load/run-p3-local-stress.mjs`:

- supports `START_DELAY_SECONDS` after `/readyz` and before k6 to avoid measuring immediate startup jitter;
- captures `*-metrics-samples.prom` during each k6 run so live WebSocket connection and goroutine peaks are visible;
- keeps before/after metrics and failure DB diagnostics from the repaired runner.

Added focused pressure workloads:

- `tests/load/p3-ws-fanout-pressure.js`
- `tests/load/p3-slow-consumer-pressure.js`
- `tests/load/p3-ws-connection-storm.js`
- `tests/load/p3-healthy-vs-slow-consumer.js`

The focused fanout, slow-consumer, and healthy-vs-slow workloads use real WebSocket tickets, real `/ws`, real bid triggers, and downstream-pressure admission ceilings. They also support `CONNECT_STAGGER_MS` and `TRIGGER_START_DELAY` so connection-storm pressure can be separated from steady fanout pressure.

Added backend WebSocket admission/backoff controls behind the global admission switch:

- `ADMISSION_ENABLED`
- `WS_TICKET_MAX_IN_FLIGHT`
- `WS_CONNECT_MAX_IN_FLIGHT`
- `WS_RETRY_AFTER`

When `ADMISSION_ENABLED=true`, the ticket endpoint and `/ws` endpoint return HTTP `429` with `Retry-After` when the configured admission slots are saturated. When `ADMISSION_ENABLED=false`, these admission paths are bypassed. This is required for performance exploration so pressure reaches the downstream bottleneck instead of stopping at a protection guard.

## Rounds

| Round | Run record | Load model | Result | Interpretation |
|---|---|---|---|---|
| Clean fanout baseline | `docs/perf/raw/p3-ws-attack-20260524-1433/` | 100 watchers, 20 trigger rps, 30s | PASS | No local regression at low focused pressure: 100 sockets, 600 bid trigger iterations, about 60,100 WS messages, 0 WS errors. |
| Fanout middle pressure | `docs/perf/raw/p3-local-stress-202605240651/` | 200 watchers, 30 trigger rps, 45s | PASS | 200 sockets, 1,350 trigger iterations, 270,200 WS messages, 0 dropped iterations, 0 WS errors. Runtime goroutines returned to 10 after close. |
| Slow-consumer pressure | `docs/perf/raw/p3-local-stress-202605240653/` | 200 slow consumers, `BLOCK_MS=150`, 30 trigger rps, 45s | PASS with pressure signal | 0 WS errors, but trigger side dropped 63 iterations, HTTP p95 rose to 2.24s, p99 to 4.45s, and outbox lag tail grew. This is mixed slow-client plus local load-generator pressure, not a clean adapter proof. |
| Synchronous fanout high pressure | `docs/perf/raw/p3-local-stress-202605240655/` | 300 watchers, 50 trigger rps, 45s, simultaneous connection start | FAIL | 3 WS errors and first-second `ws-ticket` connection refused events. No k6 dropped iterations, no DB lock pileup, backend survived and cleaned up. Primary signal is connection-storm/local accept boundary. |
| Staggered fanout high pressure | `docs/perf/raw/p3-local-stress-202605240658/` | 300 watchers, 50 trigger rps, 45s, `CONNECT_STAGGER_MS=20`, `TRIGGER_START_DELAY=8s` | PASS | Same steady fanout scale passed: 300 sockets, 2,250 trigger iterations, 675,300 WS messages, 0 dropped iterations, 0 HTTP failures, 0 WS errors. |
| Synchronous connection storm | `p3-local-stress-202605240719` summary retained here; raw directory cleaned by P3-09 | 300 connection VUs, `CONNECT_STAGGER_MS=0`, app admission configured but not yet wired into `cmd/server` | FAIL | 300 sockets eventually opened, but 58 checks failed, 2 WS errors, and `ws-ticket` had local `connectex` refusals. No app `429` or `auction_ws_admission_rejected_total` appeared, proving this failure happened before app admission could help. |
| Staggered connection storm | `p3-local-stress-202605240720` summary retained here; raw directory cleaned by P3-09 | 300 connection VUs, `CONNECT_STAGGER_MS=15` | PASS | 300 tickets, 300 opened sockets, 600/600 checks passed, 0 HTTP failures, 0 WS errors. `/metrics` showed 300 active room connections during the run and 0 after cleanup. |
| Healthy-vs-slow isolation | `p3-local-stress-202605240723` summary retained here; raw directory cleaned by P3-09 | 100 healthy watchers, 100 slow consumers with `BLOCK_MS=150`, 30 trigger rps, 45s | PASS with upstream pressure signal | Healthy group opened 100 sockets, had 0 healthy WS errors, and received 96,055 messages. Slow group opened 100 sockets and received only 6,629 messages. Trigger side still showed 41 non-business responses and 35 dropped iterations, so the remaining pressure is bid/upstream or local generator contention rather than healthy-client fanout collapse. |
| Connect admission proof | `p3-local-stress-202605240728` summary retained here; raw directory cleaned by P3-09 | `WS_CONNECT_MAX_IN_FLIGHT=1`, one held socket, 20 `/ws` probe VUs | PASS | Real `cmd/server` path returned controlled retry-later: 100 `connect` rejections, 0 uncontrolled failures, 1 opened socket, and `auction_ws_admission_rejected_total{stage="connect"} 100`. This fixed the earlier wiring gap where `cmd/server` bypassed router-created admission. |

## Key Observations

- The self hub did not show a goroutine leak in these local rounds. The 200-watchers run returned from about 875 runtime goroutines during pressure to 10 after sockets closed. The staggered 300-watchers run returned from about 1,295 runtime goroutines during pressure to 10 after sockets closed.
- Stable fanout was materially stronger than the first failed high-pressure result suggested. With staggered connection setup, 300 watchers at 50 trigger rps completed with 100% checks and 675,300 WebSocket messages.
- The synchronous 300-watchers failure happened at connection setup: first-second `ws-ticket` and one bid request saw `connectex` refused. Server logs show the process was running, returned `/readyz` 200 before the run, and later served `/metrics`; the runner stopped it with SIGTERM after diagnostics.
- PostgreSQL was not the primary root cause of the synchronous fanout failure. Failure diagnostics showed idle DB sessions and only an `AccessShareLock` plus `virtualxid` lock snapshot.
- Slow consumers created a real pressure signal, but not a clean transport failure. The slow-consumer run had 0 WS errors and 200 opened sockets, while HTTP/bid latency and outbox lag increased and the k6 trigger scenario dropped 63 iterations. Because the slow clients busy-wait inside k6 VUs, part of this pressure is local load-generator contention.
- App-level WebSocket admission now works in the real server path and returns controlled `429` with `Retry-After` once the request reaches `/ws`. The failed fully synchronous 300-connection run remains a lower-level local accept/connect-storm limit, not evidence that the self hub cannot fan out once connections are established.
- The healthy-vs-slow probe is a stronger isolation signal than the earlier slow-only workload: healthy clients kept receiving messages with 0 healthy WS errors while slow consumers lagged far behind. The probe still does not prove final capacity because the trigger side showed local/upstream pressure.

## Verdict

`HARNESS_GAP_FIXED` for P3 realtime pressure instrumentation.

`ENV_LIMIT` for synchronous 300-watcher connection storm on Windows local.

`NO_REGRESSION_WITH_CEILING` for steady fanout up to the tested staggered 300 watchers / 50 trigger rps profile on this machine.

`NO_REGRESSION_WITH_CEILING` for healthy-vs-slow isolation at 100 healthy + 100 slow watchers under the tested local profile.

`HARNESS_GAP_FIXED` for `cmd/server` WebSocket admission wiring and durable connection-storm/healthy-vs-slow k6 probes. The follow-up harness policy now requires fully disabled admission for future downstream-pressure evidence.

Do not introduce a realtime adapter based only on these Windows local rounds. The current evidence points to connection storm admission/backoff and slow-consumer isolation as the next work, not an alternate realtime transport. The release path remains the self hub unless scope is explicitly reopened after a clean self-hub failure bundle.

## Required Action

- Keep P3-01 in evidence-gathering status, not done.
- Re-run the P3-01 performance workloads with `ADMISSION_ENABLED=false` and raw before/after proof that no admission path was active.
- Keep client reconnect jitter/backoff as a product requirement for the later admission/protection phase. Do not use it to hide bottlenecks during performance exploration.
- Re-run the 300-connection storm with admission disabled. If Windows-local still fails before the Go handler, record it as an environment/connect boundary and calibrate on Linux before making a transport decision.
- Continue slow-consumer attack on a Linux-native runner before changing transport. The current mixed healthy/slow probe does not justify a realtime adapter by itself.
- Run Linux-native calibration in P5 before publishing any user-count or p99 claim.

## Validation

```text
node --check tests/load/run-p3-local-stress.mjs
node --check tests/load/p3-ws-connection-storm.js
node --check tests/load/p3-healthy-vs-slow-consumer.js
pnpm exec node tests/load/validate-k6-suite.mjs
go test ./internal/realtime
git diff --check
```
