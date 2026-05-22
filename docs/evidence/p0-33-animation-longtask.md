# Animation Longtask Evidence

Feature/Gate: H5 animation longtask

Date: 2026-05-22

Commit: pending

Environment: Windows 11, Node 24.9.0, pnpm 11.2.2, Playwright Chromium

Command: `pnpm test:e2e`

Raw Output Path: terminal output in development session

## Setup

The H5 app currently uses lightweight CSS and state transitions only; there is no heavy canvas/WebGL/particle animation. A Playwright test installs a browser `PerformanceObserver` for `longtask` entries before page load, exercises active bidding controls and recovery tabs, waits briefly, then asserts the largest observed longtask is below 100 ms.

## Expected Invariant

- H5 bid controls remain responsive during normal interaction.
- Any future visual effect that creates unacceptable main-thread longtasks should fail the test.
- This evidence does not create or claim a production performance baseline.

## Result

PASS for current H5 interaction surface.

## Observed Data

- `pnpm test:e2e` ran 15 tests.
- `H5 interaction surface has no unacceptable animation longtask` passed.
- Existing H5 state matrix, pending/rejected/confirm/recovery/payment/history tests still passed.

## Failure Interpretation

If this test fails after adding atmosphere effects, the effect must be reduced, deferred, or disabled for test/low-end paths before claiming the H5 UI is safe under auction pressure.

## Known Limits

- This is a local Chromium smoke measurement, not a device lab result.
- No P99/QPS/fanout/performance number is claimed.
- Load gates remain unclaimed.

## Next Action

Leave load gates unclaimed until raw benchmark scripts and environment baselines are produced.
