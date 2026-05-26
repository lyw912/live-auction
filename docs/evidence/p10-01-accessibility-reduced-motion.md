# P10-S1 Accessibility And Reduced Motion Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P10-S1 Accessibility And Reduced Motion Gate<br>
> Design: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `19-extreme-bidding-atmosphere.md`, `20-ui-ux-redesign.md`

## Changed

Added an executable accessibility/reduced-motion gate for the auction UI:

- H5 price and feedback state expose explicit live-region semantics;
- H5 bottom sheet is a labelled modal dialog, has a named close control, supports Escape dismissal, and keeps the single Bid CTA stable;
- H5 reduced-motion CSS now disables the specific movement effect selectors for leading/outbid/sold layers, not just low-specificity base classes;
- H5 compact viewport touch targets for the bid stepper and dock shortcuts are held to practical 40px+ minimums;
- PC diagnostic metrics and risk queue expose polite status regions and named diagnostic/filter controls without announcing the per-second clock.

## Validation

```text
pnpm --filter mobile-h5 exec tsc --noEmit
pnpm --filter pc-console exec tsc --noEmit
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 -g "accessibility gate|bottom sheet is dialog|visual effects stay nonblocking"
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console -g "accessibility gate"
```

Result: PASS.

Planned full-slice validation before commit:

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console
pnpm build
go test -p 1 ./...
git diff --check
```

## Covered

- Reduced motion removes H5 price/cue/effect animations while leaving server-state text visible.
- Event cues use `role="status"` and `aria-live="polite"` when present.
- Price, rejected/stale feedback, status chips, rank text, and connection copy provide non-color state indicators.
- Bid CTA, increase/decrease, and dock shortcut controls meet practical touch target minimums at 360px.
- Bottom sheet is keyboard dismissible and does not displace the fixed Bid CTA.
- PC route-mocked diagnostics expose bounded status regions and named controls as UI contract coverage.

## Review

- No auction truth, bidding, payment, outbox, or WebSocket behavior changed.
- The PC route-mocked accessibility gate is UI contract coverage only; live/no-route-mock evidence is handled in P10-S3/S4.
- Touch target sizing was corrected in CSS instead of weakening the gate.

## Known Limits

- This gate does not run a full automated WCAG scanner; it uses targeted Playwright checks for the auction-critical surfaces named in P10-S1.
- Route-mocked Playwright coverage proves UI semantics and layout contracts, not backend live data flow.
