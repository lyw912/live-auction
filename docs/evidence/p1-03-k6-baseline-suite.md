# P1-03 k6 Baseline Suite Evidence

Gate: P1-03 k6 baseline suite
Date: 2026-05-23 Asia/Shanghai
Base commit: 9fb9c9b

## Design Mapping

- `docs/design-v2-industrial/01-scope-and-roadmap.md`: P1-03 requires a k6 baseline suite after P0 flow is stable.
- `docs/design-v2-industrial/09-performance-and-benchmark.md`: workload scripts must cover final-second bid burst, watcher fanout, reconnect storm, slow consumer, and outbox burst; WebSocket scripts must use `k6/websockets`.
- `docs/design-v2-industrial/10-test-gates.md`: load gates block only final materials with performance numbers.
- `docs/design-v2-industrial/12-engineering-rules.md`: no guessed QPS/P99/fanout number.

## Implemented

Added k6 workload scripts:

- `tests/load/final-second-bid-burst.js`
- `tests/load/watcher-fanout.js`
- `tests/load/reconnect-storm.js`
- `tests/load/slow-consumer.js`
- `tests/load/outbox-burst.js`

Added shared helper:

- `tests/load/lib/live-auction.js`

Added validation and docs:

- `tests/load/validate-k6-suite.mjs`
- `tests/load/README.md`
- `package.json` script `test:load:p1:validate`

Design constraints implemented:

- WebSocket workloads import `WebSocket` from `k6/websockets`.
- Deprecated `k6/experimental/websockets` is forbidden by validator.
- No remote JS imports are used.
- Scripts include p99 and p99.9 summary trend stats.
- Scripts define thresholds so smoke failures are not silent.
- Scripts use browser-compatible WS ticket issuance before connecting.

## Review Result

`live-auction-v2-code-review` was applied manually against:

- `12-engineering-rules.md`
- `10-test-gates.md`
- `09-performance-and-benchmark.md`
- touched load-suite diff

Findings addressed before evidence:

- Removed a remote `jslib.k6.io` helper import to keep baselines reproducible/offline.
- Added explicit threshold validation for all scripts.
- Kept raw smoke output labeled as smoke, not baseline.
- Did not publish any QPS/P99/fanout/online-user capacity claim.

Current review status: no remaining P0/P1 findings for the P1-03 suite slice.

## Verification

Commands:

```text
pnpm test:load:p1:validate
k6 inspect tests/load/final-second-bid-burst.js
k6 inspect tests/load/watcher-fanout.js
k6 inspect tests/load/reconnect-storm.js
k6 inspect tests/load/slow-consumer.js
k6 inspect tests/load/outbox-burst.js
```

Result: PASS.

Execution smoke:

```text
cd backend
HTTP_ADDR=127.0.0.1:18080 go run ./cmd/p0smokeseed
HTTP_ADDR=127.0.0.1:18080 go run ./cmd/server
k6 run --vus 1 --iterations 1 --summary-export docs/perf/raw/p1-k6-final-second-smoke.json tests/load/final-second-bid-burst.js
```

Result: PASS.

Raw smoke output:

- `docs/perf/raw/p1-k6-final-second-smoke.json`

Known limits:

- This evidence proves the k6 suite is committed, syntactically valid, and one HTTP workload executes against a real local backend.
- It is not a formal 3-run Linux/native baseline.
- The one-iteration smoke used CLI `--vus/--iterations`, so k6 warned that CLI configuration overrode script scenarios. Formal baseline runs must use the script scenarios as committed and record 3 raw runs per workload.
- No performance number is claimed from this smoke.

Next action:

- Run full 3-run raw baselines per workload on Linux native or a documented equivalent before any capacity claim.
- P1-04 Toxiproxy weak-network tests can now build on stable WS/recovery scripts.
