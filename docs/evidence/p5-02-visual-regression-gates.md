# P5-S2 Visual Regression Gates Evidence

> Date: 2026-05-26 Asia/Shanghai  
> Slice: P5-S2 Add Visual Regression Harness  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

- Added `tests/e2e/visual-regression.spec.ts`.
- Added Playwright visual projects in `playwright.config.ts`:
  - `visual-mobile-h5`
  - `visual-pc-console`
- Added committed screenshot baselines under `tests/e2e/visual-regression.spec.ts-snapshots/`.

## Visual Gates

H5 route-mocked UI contract states:

- active
- self-leading
- outbid/rejected
- recovering
- sold winner
- sold loser
- unsold ended

PC route-mocked UI contract state:

- command and diagnostics initial state

The tests freeze visual time and disable animations/transitions before screenshots to avoid nondeterministic countdown and motion drift.

## Validation

Baseline generation:

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --project=visual-pc-console --update-snapshots --reporter=line
```

Result: PASS, 8 passed.

Regression run:

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --project=visual-pc-console --reporter=line
```

Result: PASS, 8 passed.

Build:

```text
pnpm build
```

Result: PASS.

## Review

- The harness uses mocked routes only for UI contract coverage and is labeled as such.
- No fake bids, fake viewers, or fake heat were added to product code.
- H5 dangerous action states remain covered by existing behavior tests; this slice adds screenshot regression coverage around those states.

## Known Limits

- P5-S2 does not redesign H5/PC surfaces.
- Bottom sheet and official image alignment states become fuller once P6 adds the sheet system.
- Live backend and money correctness evidence remain in existing backend/live e2e gates, not in this visual harness.
