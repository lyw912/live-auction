# P0-35 UI Countdown, Control Surface, And Drilldown

Gate: Frontend P0 UI pressure states
Date: 2026-05-26
Commit: pending
Command: `pnpm --filter mobile-h5 build`; `pnpm --filter pc-console build`; `pnpm test:e2e`; `git diff --check`
Environment: Windows local frontend Vite + Playwright

## Result

Implemented and tested the UI gaps from the v2 UI review:

- H5 shows server-authoritative countdown in the same auction panel as price, status, connection and CTA.
- H5 countdown is derived from `end_at` plus `server_time_ms`, with HTTP `Date` as a compatibility fallback for older detail responses; local zero disables bid CTA and triggers snapshot recovery instead of deciding terminal state.
- H5 extension copy is tied to server response/event `end_at` movement, not local animation.
- PC selected-auction control surface now shows current price, leader, countdown, bid count, extension count, connection/recovery health, and recent events.
- PC diagnostics rows now link auction-scoped rows to the host-only flight recorder API.

## Raw Output

- `pnpm --filter mobile-h5 build`: passed.
- `pnpm --filter pc-console build`: passed with the existing Arco/Vite large chunk warning.
- `pnpm test:e2e`: 20 passed.
- `git diff --check`: passed; Windows line-ending warnings only.

## Known Limits

- PC participant count remains approximate until a dedicated backend participant counter is exposed. The UI labels this as `approx`.
- H5 payment countdown still depends on order APIs; this change only fixes auction countdown truth.

## Next Action

Use this gate as the regression target for future H5 countdown, PC control surface, and diagnostic drilldown changes.
