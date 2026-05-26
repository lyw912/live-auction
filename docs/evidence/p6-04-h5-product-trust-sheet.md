# P6-S4 H5 Product Trust Sheet Evidence

> Date: 2026-05-26 Asia/Shanghai  
> Slice: P6-S4 Product Trust Detail Sheet  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`, `docs/design-v2-industrial/19-extreme-bidding-atmosphere.md`

## Changed

- Upgraded the H5 `商品与规则` sheet into a product trust detail surface.
- Added item media preview, description, proof grid, dimensions, material, flaw, shipping, and return policy fields.
- Replaced raw engineering rule labels with user-facing explanations for:
  - current price and increment;
  - cap price;
  - deposit/payment handling;
  - last-window extension;
  - fat-finger confirmation;
  - after-sale policy.
- Kept the BidDock as the only primary bid CTA; the trust sheet does not render a second bid action or mutate bidding state.

## Validation

```text
pnpm --filter mobile-h5 build
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line --workers=1
```

Result: PASS, 23 passed.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --update-snapshots --reporter=line --workers=1
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --reporter=line --workers=1
```

Result: PASS, 7 updated and 7 passed.

```text
pnpm build
pnpm test:e2e
```

Result: PASS, build passed and e2e passed 35 tests.

## Review

DESIGN VERDICT: COMPETITIVE for P6-S4.

- The detail sheet now answers why a high-value bidder should trust the item before bidding: certificate, condition, dimensions, material, flaw disclosure, shipping, deposit, extension, cap, high-bid confirmation, and after-sale language are visible.
- The wording avoids raw field names such as `extend_window_seconds` and explains the consequence in bidder language.
- Money truth and bid lifecycle remain server-authoritative; this slice changes presentation and copy only.

## Known Limits

- P6-S5 still owns winner, loser, and unsold result sheets with payment/next-auction handoff.
- Verification/deposit enforcement beyond displayed auction rule data remains later P9 scope.
