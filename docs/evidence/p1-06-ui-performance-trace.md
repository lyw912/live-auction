# P1-06 UI Performance Trace Evidence

Gate: P1-06 UI performance trace
Date: 2026-05-23 Asia/Shanghai
Base commit: bcc23a8

## Design Mapping

- `docs/design-v2-industrial/01-scope-and-roadmap.md`: P1-06 requires UI performance trace after H5 event states are complete.
- `docs/design-v2-industrial/07-frontend-ux.md`: H5 animations must not block input; test builds should use `PerformanceObserver` longtask sampling.
- `docs/design-v2-industrial/09-performance-and-benchmark.md`: UI evidence should capture longtask count, input blocking, and frame stability.
- `docs/design-v2-industrial/12-engineering-rules.md`: no performance number may be marketed without baseline evidence.

## Implemented

Added script:

- `tests/perf/run-ui-performance-trace.mjs`

Added package script:

- `pnpm test:perf:p1:ui`

The runner:

- starts or reuses the mobile H5 Vite server on `127.0.0.1:5173`;
- launches Playwright Chromium at a mobile viewport;
- enables Playwright tracing with screenshots and DOM snapshots;
- injects browser-side longtask sampling via `PerformanceObserver`;
- samples `requestAnimationFrame` frame gaps;
- records click-to-paint latency for bid stepper and chat send interactions;
- exercises active, recovering, rejected, extended, self-leading, sold, cancelled, and active-again states;
- writes raw summary JSON and trace zip under `docs/perf/raw/`.

Raw outputs:

- `docs/perf/raw/p1-06-ui-performance-trace.json`
- `docs/perf/raw/p1-06-ui-performance-trace.zip`

## Thresholds

These are local UI safety gates, not production performance claims:

- max longtask: `< 100 ms`
- max sampled frame gap: `< 250 ms`
- max click-to-paint: `< 100 ms`
- minimum click-to-paint samples: `8`

## Verification

Command:

```text
pnpm test:perf:p1:ui
```

Result: PASS.

The generated JSON summary is the source of truth for observed local values. This evidence intentionally does not quote them as production performance numbers.

Additional gates:

```text
pnpm build
pnpm test:e2e
```

Result: PASS, with the existing PC bundle chunk-size warning only.

## Review Result

`live-auction-v2-ui-review` and `live-auction-v2-perf-review` were applied manually against:

- `07-frontend-ux.md`
- `09-performance-and-benchmark.md`
- `10-test-gates.md`
- `12-engineering-rules.md`
- touched performance runner/evidence diff

Current review status: no remaining P0/P1 findings for P1-06.

## Known Limits

- This is local Playwright Chromium evidence, not a device-lab or Linux native production baseline.
- The trace exercises H5 state-matrix surfaces, not live backend bidding under load.
- No QPS, P99, fanout, online-user, or device-class performance claim is made.
- Playwright trace zip can be inspected with `pnpm exec playwright show-trace docs/perf/raw/p1-06-ui-performance-trace.zip`.

## Next Action

- P1-07 alert rules should wire real metric/anomaly semantics and runbooks.
- Any future visual effect must keep this trace gate passing or add a documented low-end/test disable path.
