# P10-S2 UI Performance Gate Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P10-S2 UI Performance Gate<br>
> Design: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `09-performance-and-benchmark.md`, `10-test-gates.md`

## Changed

Added a P10-specific UI performance runner:

- `tests/perf/run-p10-ui-performance-gate.mjs`
- `pnpm test:perf:p10:ui`

The runner starts H5 Vite, launches Playwright Chromium at `360x844`, installs route-mocked UI contract data, and exercises rapid auction events:

- 24 server-style `bid_accepted` events with alternating self/other winners and extension payloads;
- 48 bid stepper click interactions measured by click-to-paint;
- `PerformanceObserver` longtask sampling after page-ready reset;
- `requestAnimationFrame` frame-gap sampling;
- layout-shift sampling;
- Bid CTA y-position sampling to prove the dangerous action surface stays stable.

Raw outputs:

- `docs/perf/raw/p10-ui-performance-gate.json`
- `docs/perf/raw/p10-ui-performance-gate.zip`

## Thresholds

These are local UI safety gates, not production performance or capacity claims:

- max longtask: `< 100 ms`
- max sampled frame gap: `< 250 ms`
- max click-to-paint: `< 100 ms`
- max layout shift: `<= 0.02`
- max Bid CTA y delta: `<= 2 px`
- minimum click samples: `10`
- minimum Bid CTA layout samples: `20`

## Validation

```text
pnpm test:perf:p10:ui
```

Result: PASS.

The JSON summary is the source of truth for observed local values. This evidence intentionally does not quote them as production capacity numbers.

Planned full-slice validation before commit:

```text
pnpm --filter mobile-h5 exec tsc --noEmit
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5
pnpm build
go test -p 1 ./...
git diff --check
```

## Review

- The runner resets the metric buffers after page readiness, so the gate measures rapid event handling rather than initial Vite/page load.
- Browser WebSocket is mocked inside the performance runner to avoid backend proxy noise; this classifies the result as UI contract coverage only.
- No QPS, online-user, P99 backend latency, fanout, device-class, or production capacity claim is made.
- P10-S3/S4 must separately record no-route-mock demo smoke evidence for live backend flow.

## Known Limits

- This is Windows-local Playwright Chromium evidence.
- It does not replace P3/P4 backend stress evidence or final Linux capacity baselines.
- It does not prove live backend bidding throughput; it proves the H5 interaction surface remains stable under rapid UI event rendering.
