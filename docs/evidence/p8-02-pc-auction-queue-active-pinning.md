# P8-S2 PC Auction Queue And Active Pinning Evidence

> Date: 2026-05-27 Asia/Shanghai  
> Slice: P8-S2 Auction Queue And Active Pinning  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`, `docs/design-v2-industrial/07-frontend-ux.md`

## Changed

Extended the P8-S1 PC command center queue into an operation-aware auction queue:

- ACTIVE auctions render in an `ACTIVE pinned` group above all other lots.
- SCHEDULED, DRAFT, and finished auctions render in separate groups with counts.
- Each queue card keeps product thumbnail, status, narrating state, start/increment/cap, current or sold price, bid count, and countdown.
- Queue cards show visible constraints:
  - scheduled lots blocked by another ACTIVE auction;
  - non-current lots blocked by another narrating lot;
  - ACTIVE lot marked as the pinned live auction;
  - DRAFT lot marked editable before schedule.
- The command panel disables `开拍` for a scheduled lot when another auction is ACTIVE, and disables `开始讲解` when another lot is narrating. Backend remains final authority.

## Validation

```text
pnpm --filter pc-console exec tsc --noEmit
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console --reporter=line --workers=1
```

Result: PASS, 5 passed. Added coverage for active pinning, scheduled active conflict, and narrating conflict.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-pc-console --update-snapshots --reporter=line --workers=1
```

Result: PASS, 1 passed. The PC command screenshot baseline was intentionally regenerated for queue grouping and constraint chips.

```text
pnpm build
```

Result: PASS. Vite retained the existing PC bundle-size warning.

```text
pnpm test:e2e
```

Result: PASS, 50 passed.

```text
go test ./...
```

Result: PASS from `backend`, cached package tests included.

```text
PC live backend smoke:
start PostgreSQL/Redis/MinIO, run `go run ./cmd/p0smokeseed`, start backend on 127.0.0.1:18080, start PC Vite on 127.0.0.1:5177, then:
pnpm exec playwright test -c tests/e2e/playwright-live.config.ts --project=pc-console-live --reporter=line
```

Result: PASS, 1 passed.

Port cleanup:

```text
netstat -ano | findstr ":18080 :5177" | findstr LISTENING
```

Result: no LISTENING rows after stopping the live smoke processes. A Vite child process briefly remained on 5177 after the first cleanup attempt and was stopped by PID from `netstat`.

## Review

- No backend auction state, bid transaction, outbox, order, payment, or diagnostics producer changed.
- Frontend constraints mirror existing backend invariants; they are not treated as authority.
- The queue still uses real auction rows from `/api/auctions?room_id=...` and does not fabricate state.

## Known Limits

- P8-S3/S4 still own real host prompter API/UI and manual system chat actions.
- P8-S5 still owns wizard/preview refactor for seller setup.
- P8-S6 still owns real heat aggregation and unavailable states.
