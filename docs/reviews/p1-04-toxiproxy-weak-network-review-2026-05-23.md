# P1-04 Toxiproxy Weak-Network Review - 2026-05-23

## Scope

Review target: P1-04 Toxiproxy weak-network harness, Docker Compose wiring, scenario runner, and evidence.

Design basis:

- `docs/design-v2-industrial/08-observability-and-ops.md`
- `docs/design-v2-industrial/09-performance-and-benchmark.md`
- `docs/design-v2-industrial/10-test-gates.md`
- `docs/design-v2-industrial/12-engineering-rules.md`

## Findings

No remaining P0/P1 findings after fixes.

Fixed during review:

- [P1] Scenario runner initially only configured toxics and printed desired state. Added active Toxiproxy API readback to prove the injected toxic exists.
- [P1] Runner initially lacked a cleanup mode. Added `--clear` so fault injection cannot leak into later local gates.
- [P1] `--clear` initially printed successful cleanup but exited non-zero on Windows due to explicit `process.exit(0)` triggering a Node assertion. Reworked the command to exit naturally.
- [P2] Parallel scenario commands race on shared Toxiproxy state. Evidence and docs now instruct sequential execution.

## Missing Tests

No blocker for P1-04 harness readiness. Current evidence proves:

- Docker Compose renders a Toxiproxy service.
- Toxiproxy image starts locally with PostgreSQL and Redis dependencies.
- Scenario commands create real active toxics through the Toxiproxy API.
- Cleanup removes active toxics and returns exit code 0.
- Backend configuration can reach PostgreSQL and Redis through proxy ports.

Still required before claiming weak-network resilience:

- Run H5 reconnect smoke with backend `REDIS_ADDR=localhost:16379` under `redis_latency_reconnect`.
- Run k6 reconnect storm through the proxy ports and store raw output.
- Run a bid-path degradation smoke through `postgres_bid_latency` and verify no duplicate money state.
- Capture metrics/anomalies during the run once alert rules are in place.

## V2 Drift

No drift in committed harness.

No production chaos-resilience claim, QPS claim, P99 claim, or online-user claim was added.

## Residual Risk

- The backend readiness smoke returned PostgreSQL and Redis healthy through proxies but MinIO timed out in the local environment. That blocks calling the entire service ready, but it does not invalidate PG/Redis proxy wiring.
- The harness currently targets PostgreSQL and Redis. Browser-level packet loss or bandwidth shaping remains future expansion if the demo needs client-network evidence.
- Scenario names encode expected behavior, but the runner does not assert business invariants by itself. It must be paired with Playwright/k6/integration tests for final chaos evidence.
