# P0-26 PC Full Rule Fields

Gate:
- PC rule form
- backend rule contract coverage

Date:
- 2026-05-22 Asia/Shanghai

Commit:
- pending

Command:
- `pnpm build`
- `pnpm test:e2e`
- `go test ./...`

Environment:
- Windows workspace
- Node v24.9.0
- pnpm 11.2.2
- Playwright Chromium
- Backend tests use local PostgreSQL/Redis test setup

Result:
- PC rule form now includes P0 rule fields beyond start/increment/cap:
  - duration seconds
  - extension window seconds
  - extension by seconds
  - max extension count
  - fat-finger threshold cents
  - deposit bps
  - deposit floor cents
  - deposit cap cents
- `PATCH /api/auctions/auc_next/rules` body now includes those backend field names.
- Backend `PATCH /api/auctions/{id}/rules` now accepts and persists `start_price_cents`, `increment_cents`, `cap_price_cents`, and the full embedded rule in one DRAFT-only update.
- Repository tests verify the money fields update the selected auction and reset `current_price_cents` before schedule/start.
- E2E validates the submitted backend contract body.
- E2E also blocks invalid cross-field UI combinations for extension seconds, fat-finger threshold vs increment, and deposit floor vs cap before save.
- Backend still remains final authority for rule acceptance.

Raw output:

```text
pnpm build
mobile-h5: tsc -b && vite build passed
pc-console: tsc -b && vite build passed
warning: PC chunk exceeds 500 kB after minification due Arco bundle; no performance claim made.
```

```text
pnpm test:e2e
Running 14 tests using 1 worker
14 passed (8.3s)
```

```text
go test ./...
passed
```

Known limits:
- E2E uses mocked save responses; backend persistence is proven by Go tests, not browser-to-live-backend PC smoke.
- Frontend cross-field validation is still a guardrail, not final authority; backend remains authoritative.
- Rule inputs are compact operational controls, not a final polished console layout.

Next action:
- Add a dedicated live PC browser smoke if P1 wants end-to-end host workflow evidence beyond API and mocked browser contract coverage.
