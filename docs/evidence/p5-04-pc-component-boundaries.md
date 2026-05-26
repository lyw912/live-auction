# P5-S4 PC Component Boundary Evidence

> Date: 2026-05-26 Asia/Shanghai  
> Slice: P5-S4 PC Component Boundary Refactor  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

Refactored `frontend/pc-console/src/main.tsx` to extract PC rendering boundaries without changing host console behavior:

- `AuctionCommandPanel`
- `AuctionQueue`
- `RuleEditor`
- `OrdersPanel`
- `DiagnosticsPanel`
- `EventTimeline`

The slice also extracted `ConsoleNav`, `ConsoleToolbar`, `ItemCreatePanel`, and `AuctionControlSummary` so the top-level `App` keeps ownership of session, room, auction selection, rule save, item creation, lifecycle actions, diagnostics loading, and flight-recorder event loading.

## Validation

```text
pnpm --filter pc-console exec tsc --noEmit
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console --reporter=line --workers=1
```

Result: PASS, 4 passed.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-pc-console --reporter=line --workers=1
```

Result: PASS, 1 passed.

```text
pnpm build
pnpm test:e2e
```

Result: PASS, build passed and e2e passed 29 tests.

```text
Start-Job { pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console --reporter=line --workers=1 }
Start-Job { pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-pc-console --reporter=line --workers=1 }
```

Result: PASS, concurrent direct Playwright runs completed with PC E2E 4 passed and PC visual 1 passed.

## Review

- No backend, auction truth, bidding semantics, outbox, payment, or diagnostic data producer changed.
- Existing PC E2E coverage still proves live API auction rendering, order panel rendering, diagnostics tabs, item upload/create, rule save payload, and lifecycle controls.
- Existing PC visual baseline still passes, so this slice did not intentionally redesign the console.
- Direct Playwright runs now allocate per-process Vite ports by default. The `pnpm test:e2e` wrapper still uses fixed local ports `5173` and `5174` through environment variables.

## Known Limits

- P8 still owns the future host/seller studio redesign and richer PC command layout.
- Component code remains in `main.tsx` for this slice to minimize import churn; later slices can move stable components into separate files.
