# P6-S3 H5 Bottom Sheet Navigation Evidence

> Date: 2026-05-26 Asia/Shanghai  
> Slice: P6-S3 Add Bottom Sheet System  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

- Added a reusable H5 bottom sheet with backdrop close, close button, and tabs for product list, rules, leaderboard, bid history, and orders.
- Moved product/rules/leaderboard/history/orders out of normal demo-page sections for the default H5 entry; the state-matrix route still keeps old panels for coverage inspection.
- Added room auction list state so the sheet can show current and upcoming products without hardcoding a single lot.
- Kept the BidDock as the only primary bid action surface. The sheet never renders a second bid CTA, and its top edge is constrained above the fixed dock.
- Preserved existing user API paths for history/orders refresh; the sheet wraps the existing `/api/users/me/bids` and `/api/users/me/orders` data instead of adding mock-only state.
- Updated H5 visual regression to capture the viewport first screen, which is the correct gate for fixed mobile controls.

## Validation

```text
pnpm --filter mobile-h5 build
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line --workers=1
```

Result: PASS, 22 passed.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --update-snapshots --reporter=line --workers=1
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --reporter=line --workers=1
```

Result: PASS, 7 updated and 7 passed.

```text
pnpm build
pnpm test:e2e
```

Result: PASS, build passed and e2e passed 34 tests.

## Review

DESIGN VERDICT: COMPETITIVE for P6-S3.

- Product list, rules, leaderboard, history, and orders are now secondary drawer surfaces instead of page sections that push the auction action path down.
- The fixed BidDock remains visible and singular while sheets are open; Playwright checks the sheet does not overlap the dock and that only one `bid-cta` exists.
- History and orders still use the existing user APIs and are not silently downgraded to local/demo-only data.
- Direct Playwright port isolation remains intact; the full wrapper run starts H5/PC on fixed 5173/5174 and passes after direct-run dynamic port support from P5-S4.

## Known Limits

- P6-S4 still owns the richer product trust detail sheet with media evidence, proof language, and user-facing deposit/shipping/cap/extension explanations.
- P6-S5 still owns winner, loser, and unsold result sheets.
