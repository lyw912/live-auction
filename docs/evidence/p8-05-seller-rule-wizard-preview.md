# P8-S5 Seller Rule Wizard And Preview Evidence

> Date: 2026-05-27 Asia/Shanghai  
> Slice: P8-S5 Seller Rule Wizard And Preview  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`, `docs/design-v2-industrial/07-frontend-ux.md`

## Changed

Converted the PC seller setup surface from a flat rule form into grouped operational steps:

- product step on the item creation panel;
- price step for start price, increment, cap, cap reachability, and nearest legal cap suggestions;
- time/extension step for duration, extension window, extension amount, and max extension count;
- trust/deposit step for fat-finger threshold and deposit ratio/floor/cap;
- preview step showing the H5 bidding chips and rule explanation before save.

The save/create API payloads remain unchanged:

- `POST /api/items`
- `POST /api/auctions`
- `PATCH /api/auctions/{id}/rules`

## Validation

```text
pnpm --filter pc-console exec tsc --noEmit
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console --reporter=line --workers=1
```

Result: PASS, 7 passed. Added coverage for wizard steps, H5 preview chips, unchanged rule-save payload, product creation, and frozen non-DRAFT rule explanation.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-pc-console --update-snapshots --reporter=line --workers=1
```

Result: PASS, 1 passed. PC visual baseline was regenerated for the wizard and preview layout.

```text
LIVE_AUCTION_PC_URL=http://127.0.0.1:5177 pnpm exec playwright test -c tests/e2e/playwright-live.config.ts --project=pc-console-live --reporter=line
```

Result: PASS, 1 passed against live backend + PC Vite after `p0smokeseed`. The smoke covered create item/auction, rule save, schedule, start, diagnostics, outbox, and cancel.

```text
pnpm build
go test ./...
pnpm test:e2e
```

Result: PASS. `pnpm test:e2e` passed 52 tests. `pnpm build` kept the existing PC bundle-size warning.

Cleanup: backend, PC Vite, Postgres, Redis, and MinIO were stopped after live smoke; 18080/5177 had no remaining LISTENING process.

## Review

- Backend remains final authority for rule validation and DRAFT-only rule updates.
- The PC preview is display-only; it does not decide hammer/sold state or compute auction truth.
- The rule save button remains disabled outside DRAFT and now explains the frozen state.
- No backend, bid, outbox, WebSocket, order, or payment path changed.

## Known Limits

- Preview uses the draft rule values on the PC setup surface. Full mobile H5 product trust sheet is owned by later H5 slices.
