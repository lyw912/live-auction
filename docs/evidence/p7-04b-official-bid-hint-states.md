# P7-04b Official Bid Hint States

> Date: 2026-05-26 Asia/Shanghai  
> Classification: AUTHORITATIVE for P7-S4b.

## Change

- Added bid-adjacent hint copy inside the Bid Dock feedback row, next to amount and CTA.
- Covered official semantics:
  - self-leading guardrail: user sees that they are already highest and cannot repeat bid;
  - multi-step bid hint: user sees how much above current price and above the minimum next bid;
  - prepared price change: when an authoritative event changes the current price, the prepared amount is clamped to the new valid minimum and the hint updates.
- Split prepared bid amount from `minimumNextBidCents`, so UI hints do not infer legality from display strings.
- Kept the hint within existing Bid Dock height budget; state-matrix longtask remains under the 100 ms gate.

## Validation

| Command | Result |
|---|---|
| `pnpm --filter mobile-h5 build` | PASS |
| `pnpm exec playwright test --project=mobile-h5 -g "longtask\|opens browser WebSocket\|official bid hints\|prepared bid hint"` | PASS, 4 passed |
| `pnpm exec playwright test --project=mobile-h5` | PASS, 33 passed |
| `pnpm exec playwright test --project=visual-mobile-h5 --update-snapshots` | PASS, baselines updated |
| `pnpm exec playwright test --project=visual-mobile-h5` | PASS, 7 passed |
| `pnpm build` | PASS |
| `pnpm test:e2e` | PASS, 45 passed |

## Review

DESIGN VERDICT: COMPETITIVE.

- Hints are amount/CTA adjacent, not hidden in toast.
- No local success, winner, or legality decision was introduced; bid submission still uses server authority.
- Prepared amount is clamped upward on authoritative price changes, avoiding stale underbids.
- The hint did not add another CTA and did not break result sheet or bottom sheet clickability.

## Known Limits

- P7-S5 still owns opt-in sound/haptic capability and event-specific patterns.
- P7-S6 still owns last-10-second countdown display and extension explanation detail.
