# P0-18 Frontend State Surfaces

Gate:
- H5 state matrix
- pending bid has no optimistic success
- rejected bid reason-specific copy
- recovering/disconnected CTA disabled
- cap sold winner/loser views
- PC diagnostics show real API rows in tests

Date:
- 2026-05-22 Asia/Shanghai

Commit:
- pending

Command:
- `pnpm install`
- `pnpm build`
- `pnpm test:e2e`
- `go test ./...`

Environment:
- Windows workspace
- Node v24.9.0
- pnpm 11.2.2
- Playwright Chromium installed through `pnpm exec playwright install chromium`
- PostgreSQL/Redis/MinIO local Docker infra remains the backend test environment

Result:
- Frontend workspace added at repo root with Vite React apps:
  - `frontend/mobile-h5`
  - `frontend/pc-console`
- H5 renders the P0 state matrix as an operational auction room surface. Unsafe states keep the bid CTA disabled:
  - self-leading
  - pending
  - recovering
  - disconnected
  - sold loser
  - ended
  - cancelled
- H5 recovery states show stale status and keep bid controls disabled until authoritative recovery.
- H5 winner SOLD state exposes mock payment CTA; loser SOLD state is disabled.
- PC console renders auction rows, rule/order surfaces, and diagnostic tabs backed by `/api/monitor/*` calls.
- E2E tests mock monitor endpoints to prove diagnostic rows come from API responses, not hard-coded dashboard cards.

Raw output:

```text
pnpm build
mobile-h5: tsc -b && vite build passed
pc-console: tsc -b && vite build passed
warning: PC chunk exceeds 500 kB after minification due Arco bundle; no performance claim made.
```

```text
pnpm test:e2e
Running 3 tests using 1 worker
3 passed (4.0s)
```

```text
go test ./...
passed
```

Known limits:
- Frontend is a P0 scaffold/state surface, not full backend-integrated bidding yet.
- H5 does not yet execute real bid POST, fat-finger confirm, history recovery, or payment double-click flows.
- PC rule form is presentational; illegal cap suggestions and backend error surfacing remain future slices.
- E2E covers state gating and diagnostic API rendering, not every P0 frontend gate from `10-test-gates.md`.
- No frontend performance number is claimed.

Next action:
- Add real H5 bid/recovery protocol integration and PC rule validation in later P0 frontend slices.
