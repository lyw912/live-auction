# P8-S1 PC Command Center Layout Evidence

> Date: 2026-05-27 Asia/Shanghai  
> Slice: P8-S1 PC Command Center Layout  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`, `docs/design-v2-industrial/07-frontend-ux.md`

## Changed

Reworked `frontend/pc-console/src/main.tsx` and `frontend/pc-console/src/styles.css` from a vertical backend-style console into a PC command center:

- top health ribbon with room, server clock, recovery, outbox, scheduler, anomaly counts, room switch, and refresh;
- left auction queue with product thumbnail, status, narrating state, start/increment/cap, current or sold price, bid count, and countdown;
- center active command panel with product media, current price, service countdown, leader, seq, bid count, extension count, status, schedule/start/cancel/narrate controls, and operation guardrails;
- right Live Assist rail with event-backed recovery/outbox/reject/anomaly summaries, recent flight-recorder timeline, and an explicitly disabled prompter pending state;
- secondary workspace for item creation, rule editing, orders, and diagnostics so existing PC workflows remain available.

S1 intentionally does not add fabricated host prompts or heat. The Live Assist rail only uses existing auction state, monitor rows, and flight-recorder timeline. Its prompter block is labeled pending until P8-S3 adds the host-only prompts API; P8-S6 owns heat aggregation.

## Validation

```text
pnpm --filter pc-console exec tsc --noEmit
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console --reporter=line --workers=1
```

Result: PASS, 4 passed.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-pc-console --update-snapshots --reporter=line --workers=1
```

Result: PASS, 1 passed. The PC command center screenshot baseline was intentionally regenerated for the new layout.

```text
pnpm build
```

Result: PASS. Vite retained the existing PC bundle-size warning.

```text
pnpm test:e2e
```

Result: PASS, 49 passed.

```text
go test ./...
```

Result: PASS from `backend`, cached package tests included.

```text
PC live backend smoke:
start PostgreSQL/Redis/MinIO, run `go run ./cmd/p0smokeseed`, start backend on 127.0.0.1:18080, start PC Vite on 127.0.0.1:5177, then:
pnpm exec playwright test -c tests/e2e/playwright-live.config.ts --project=pc-console-live --reporter=line
```

Result: PASS, 1 passed.

```text
pnpm test:e2e:h5-live
```

Result: FAIL outside the PC S1 path. The script started backend, H5, and PC, but the two H5 live tests failed waiting for seeded chat text in `chat-panel`; the PC live test also had an obsolete `.arco-table` selector before this slice updated `tests/e2e/pc-console-live.spec.ts`. The focused PC live backend smoke above passed after updating the PC selector to the new auction queue. S1 does not claim full H5 live smoke success.

## Review

- No backend auction truth, bidding transaction, outbox relay, WebSocket recovery, payment, order, or diagnostic producer changed.
- Existing PC E2E still proves item upload/create, selected-auction rule save payloads, lifecycle API calls, order rendering, diagnostics tabs, and flight-recorder links.
- Existing PC live backend smoke proves the command center can render seeded room auctions, switch rooms, create a real item/auction, save rules, schedule/start/cancel, and inspect diagnostics against a live backend.
- The command panel keeps ACTIVE controls visible without scrolling in the 1440px visual gate.
- A visual issue found during review was fixed: very long future countdowns now render as `>99d`/day-hour labels instead of stretching the command panel.
- UI review corrected the Live Assist rail from a generated "prompter preview" to an explicit `Prompter pending` state so P8-S1 does not masquerade as P8-S3/S4.

## Known Limits

- S1 reshapes the queue but P8-S2 still owns stricter active pinning and narrating/ACTIVE constraint visualization.
- S1 shows the prompter area as pending/disabled; P8-S3/S4 must replace this with host-only prompt API data and action semantics.
- S1 does not implement room/auction heat aggregation; P8-S6 owns that endpoint and honest unavailable states.
