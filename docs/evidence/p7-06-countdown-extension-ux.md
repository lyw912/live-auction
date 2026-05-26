# P7-06 Countdown And Extension UX

> Date: 2026-05-26 Asia/Shanghai
> Classification: AUTHORITATIVE for P7-S6.

## Change

- H5 countdown uses stable tabular digits and switches to tenths only in the final 10 seconds.
- Extension events now show the server-authoritative old/new end time and extend count when the event payload provides them.
- Local countdown expiry keeps H5 in syncing/recovering behavior with the bid CTA disabled; it does not render SOLD, ENDED, or winner locally.
- Countdown row width was increased and visual baselines were updated to cover the wider stable countdown capsule.

## Validation

| Command | Result |
|---|---|
| `pnpm --filter mobile-h5 build` | PASS |
| `pnpm exec playwright test --project=mobile-h5 -g "countdown shows stable tenths\|local zero enters syncing"` | PASS, 2 passed |
| `pnpm exec playwright test --project=mobile-h5 -g "countdown shows stable tenths\|local zero enters syncing\|live panel keeps\|sticky bid dock\|longtask"` | PASS, 6 passed |
| `pnpm exec playwright test --project=mobile-h5` | PASS, 37 passed |
| `pnpm exec playwright test --project=visual-mobile-h5 --update-snapshots` | PASS, baselines updated |
| `pnpm exec playwright test --project=visual-mobile-h5` | PASS, 7 passed |
| `pnpm build` | PASS |
| `pnpm test:e2e` | PASS, 49 passed |
| `H5_PORT=21200 PC_PORT=21201 node node_modules/@playwright/test/cli.js test --project=pc-console --reporter=line` | PASS, 4 passed |
| `H5_PORT=21202 PC_PORT=21203 node node_modules/@playwright/test/cli.js test --project=visual-pc-console --reporter=line` | PASS, 1 passed |

## Review

DESIGN VERDICT: COMPETITIVE.

- Countdown display still derives from `end_at` plus `server_time_ms`; browser time only ages a server timestamp.
- The client does not infer hammer, winner, SOLD, or ENDED from local zero.
- Extension copy is bound to an event with a later `end_at` and displays server-provided count metadata.
- The countdown row remains visible with price, rank, connection state, and CTA at 390px and 360px widths.
- Sound, haptic, and visual effects from earlier P7 slices remain event-driven and non-blocking.

## PC E2E And PC Visual Port Note

- The wrapper command `pnpm test:e2e` still runs all projects through one Playwright invocation with fixed `H5_PORT=5173` and `PC_PORT=5174`; it passed 49 tests.
- Direct single-process PC commands also pass with explicit dynamic ports.
- Directly starting separate PC E2E and PC visual Playwright processes with different port pairs avoids Vite port sharing, but on this Windows run both Chromium workers exited with code `3221226505`. That failure is not a Vite `strictPort` conflict and should not replace the wrapper as the recommended full gate.

## Known Limits

- The extension response path still shows the generic "服务端已延时" copy; detailed old/new end-time copy is implemented for realtime events where both previous and new server end times are available.
