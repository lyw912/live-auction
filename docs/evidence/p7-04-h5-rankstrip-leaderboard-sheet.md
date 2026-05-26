# P7-04 H5 RankStrip And Leaderboard Sheet

> Date: 2026-05-26 Asia/Shanghai  
> Classification: AUTHORITATIVE for P7-S4.

## Change

- Added an action-oriented RankStrip inside the sticky Bid Dock with:
  - current rank;
  - adjacent-rank gap;
  - next valid bid;
  - sequence/freshness copy.
- Upgraded the leaderboard sheet with Top N plus a next-action card and freshness stats.
- Kept the primary bid CTA singular and stable while sheets are open.
- Increased the Bid Dock reserved safe area and adjusted result/sheet positioning so sheets and result payment CTAs are not covered by the fixed dock.
- Removed the large fixed-dock blur after longtask verification showed it pushed the interaction budget to the edge.

## Validation

| Command | Result |
|---|---|
| `pnpm --filter mobile-h5 build` | PASS |
| `pnpm exec playwright test --project=mobile-h5 -g "realtime leaderboard\|leaderboard sheet\|sticky bid dock"` | PASS, 4 passed |
| `pnpm exec playwright test --project=mobile-h5 -g "winner result sheet\|bottom sheets open close\|leaderboard sheet\|sticky bid dock"` | PASS, 5 passed |
| `pnpm exec playwright test --project=mobile-h5` | PASS, 31 passed |
| `pnpm exec playwright test --project=visual-mobile-h5 --update-snapshots` | PASS, baselines updated |
| `pnpm exec playwright test --project=visual-mobile-h5` | PASS, 7 passed |
| `pnpm build` | PASS |
| `pnpm test:e2e` | PASS, 43 passed |

## Review

DESIGN VERDICT: COMPETITIVE.

- RankStrip is action-oriented, not just a decorative Top N list.
- Sheet opening does not move the Bid Dock CTA.
- Result payment CTA remains clickable above the fixed dock.
- Longtask gate stayed below 100 ms after removing fixed-dock blur.
- No backend truth changes in this slice; it consumes P7-S3 fields.

## Known Limits

- P7-S4b still owns official amount-adjacent hints for multi-step bids, self-leading guardrail, and stale prepared price.
- P7-S5 still owns event-specific sound/haptic policy.
