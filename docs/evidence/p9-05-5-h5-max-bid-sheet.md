# P9-S5-5 H5 Max Bid Sheet Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S5-5 H5 Max Bid controls and disclosure<br>
> ADR: `docs/adr/p9-04-max-bid-pre-bid-decision.md`

## Changed

Added an H5 secondary Max Bid sheet:

- entry lives in the Bid Dock shortcut row, not the primary bid CTA;
- shows the authenticated user's private `max_bid_intent` from the auction snapshot/read API;
- lets the user raise/lower a max amount on the auction increment grid;
- `PUT /api/auctions/{id}/max-bid-intent` and `DELETE /api/auctions/{id}/max-bid-intent` wait for committed API responses before changing success state;
- disables Max Bid set/cancel while pending, recovering, disconnected, stale, or terminal;
- keeps the primary fixed-increment bid CTA singular and visible.

The sheet copy states the max amount is private and that the server bids by increments without publicly exposing the ceiling.

## Validation

```text
pnpm --filter mobile-h5 exec tsc --noEmit
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5
pnpm build
pnpm exec playwright test tests/e2e/visual-regression.spec.ts -g "H5" --project=visual-mobile-h5
```

Result: PASS.

Updated H5 visual baselines after verifying the new six-shortcut Bid Dock row at mobile size.

Covered:

- Max Bid sheet opens from Bid Dock while keeping exactly one primary bid CTA;
- existing product/rules/leaderboard/history/orders workflows remain reachable;
- Max Bid does not show optimistic success before API response;
- submit/cancel use real max-bid intent APIs with idempotency headers;
- recovery disables Max Bid controls;
- H5 visual states remain stable after adding the Max shortcut.

## Review

- No client-side proxy bidding or winner logic was added.
- No public Max Bid amount is rendered outside the current user's sheet.
- The H5 sheet consumes backend current-user private state added in P9-S5-4.

## Known Limits

- PC aggregate/audit surfaces are pending P9-S5-6.
- Fat-finger/churn abuse behavior remains pending P9-S5-7.
