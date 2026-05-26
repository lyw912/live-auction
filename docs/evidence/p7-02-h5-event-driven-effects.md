# P7-02 H5 Event-Driven Effects

> Date: 2026-05-26 Asia/Shanghai  
> Classification: AUTHORITATIVE for P7-S2.

## Change

- Replaced the plain atmosphere toast styling with event-specific motion language:
  - leading: price tick and bounded gold ring;
  - outbid: short red edge flash;
  - extended: countdown stretch;
  - sold: hammer/result mark.
- Added a non-interactive atmosphere effect layer inside the live stage. It uses CSS transforms/opacity only and has `pointer-events: none`.
- Added `prefers-reduced-motion: reduce` fallback that disables cue/effect/countdown/price animations while preserving the text cue.
- Bound Bid Dock and Live Stage to the accepted `AtmosphereCue.kind` with data attributes for testability.

## Validation

| Command | Result |
|---|---|
| `pnpm --filter mobile-h5 build` | PASS |
| `pnpm exec playwright test --project=mobile-h5 -g "visual effects\|extension and sold\|longtask\|sticky bid dock"` | PASS, 5 passed |
| `pnpm exec playwright test --project=mobile-h5` | PASS, 30 passed |
| `pnpm exec playwright test --project=visual-mobile-h5` | PASS, 7 passed |
| `pnpm build` | PASS |
| `pnpm test:e2e` | PASS, 42 passed |

## Review

DESIGN VERDICT: COMPETITIVE.

- Effects remain event-driven through P7-S1 atmosphere cue metadata.
- CTA remains outside the cue box and the effect layer cannot intercept pointer events.
- Reduced-motion users receive a static cue without motion loops.
- Flash behavior is bounded to one short edge pulse, below the WCAG three-flashes threshold.
- Longtask gate remains below the existing 100 ms budget.

## Known Limits

- P7-S3/S4 still own action ranking data and RankStrip copy.
- P7-S5 still owns event-specific sound/haptic policy.
- P7-S6 still owns last-10s countdown tenths and full extension explanation copy.
