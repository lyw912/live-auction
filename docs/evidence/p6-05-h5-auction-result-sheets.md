# P6-S5 H5 Auction Result Sheets Evidence

> Date: 2026-05-26 Asia/Shanghai  
> Slice: P6-S5 Winner And Loser Result Sheets  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`, `docs/design-v2-industrial/19-extreme-bidding-atmosphere.md`

## Changed

- Added an H5 result sheet above the fixed BidDock for terminal states.
- Winner result sheet shows sold price, locked order id, payment status, deposit/payment handling, and a payment CTA.
- Winner payment CTA shares the existing `payOrder` path and in-flight guard with the BidDock CTA, preserving one payment request under double click.
- Loser result sheet shows final sold price, masked winner, final gap language when available, and next-auction handoff.
- Unsold result sheet explains that no valid order is generated and offers next-item/list continuation.
- Result sheets are hidden when a normal bottom sheet is open, so product/history/order sheets remain explicit user actions.

## Validation

```text
pnpm --filter mobile-h5 build
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line --workers=1
```

Result: PASS, 25 passed.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --update-snapshots --reporter=line --workers=1
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --reporter=line --workers=1
```

Result: PASS, 7 updated and 7 passed.

```text
pnpm build
pnpm test:e2e
```

Result: PASS, build passed and e2e passed 37 tests.

## Review

DESIGN VERDICT: COMPETITIVE for P6-S5.

- Winner flow now has a terminal result surface instead of relying only on BidDock copy, while preserving payment idempotency.
- Loser and unsold states no longer dead-end; they explain the result and point to the next scheduled/draft item when present.
- Dangerous actions remain disabled in loser and unsold states.
- The result sheet is presentation-only: PostgreSQL winner/order truth and payment mock idempotency remain unchanged.

## Known Limits

- Result sheet winner masking currently uses the winner user id available in H5 state (`us**`) when the event does not persist `leader_user_masked`; richer masked-name carryover is a future event-state refinement.
- Similar-auction recommendation is a deterministic next scheduled/draft item from the room list, not an algorithmic recommendation engine.
