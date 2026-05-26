# P6-S2 H5 Sticky Bid Dock Evidence

> Date: 2026-05-26 Asia/Shanghai  
> Slice: P6-S2 Implement Sticky Bid Dock  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

- Converted the H5 auction state panel into a sticky bottom `BidDock`.
- Moved current price, server countdown, status, leader/rank summary, stepper, CTA, and quick-entry row into the dock.
- Added dock state variants through `data-dock-state`: `ACTIVE`, `SELF_LEADING`, `OUTBID`, `PENDING`, `RECOVERING`, `SOLD_WINNER`, `SOLD_LOSER`.
- Restored visible feedback text inside the dock for pending, rejected, recovery, payment, and realtime event states so error/action context is not lost in the compact layout.
- Preserved no-optimistic-success behavior: pending keeps old authoritative price and disables CTA until server confirmation.

## Validation

```text
pnpm --filter mobile-h5 build
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line --workers=1
```

Result: PASS, 20 passed.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --grep "sticky bid dock" --reporter=line --workers=1
```

Result: PASS, 2 passed. Confirms price/countdown/rank/CTA are visible at 390x844 and 360px and CTA stays inside the dock.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --update-snapshots --reporter=line --workers=1
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --reporter=line --workers=1
```

Result: PASS, 7 updated and 7 passed.

```text
pnpm build
pnpm test:e2e
```

Result: PASS, build passed and e2e passed 32 tests.

## Review

DESIGN VERDICT: COMPETITIVE for P6-S2.

- Price/countdown/rank/CTA are now persistent in the bottom dock under active bidding.
- Unsafe states remain disabled and still show user-facing reasons near the CTA.
- Rejected/outbid, pending, recovering, and sold winner/loser states are visually distinct by dock state and text, not color alone.
- Full leaderboard remains below the stage until P6-S3 moves secondary surfaces into sheets; the dock carries only the rank/action summary.

## Known Limits

- P6-S3 still owns the reusable bottom sheet and moving product/rules/leaderboard/history/orders into sheets.
- P6-S4 still owns rich product trust details.
- P6-S5 still owns winner/loser/unsold result sheets.
