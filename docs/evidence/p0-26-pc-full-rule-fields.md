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
- E2E validates the submitted backend contract body.
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
Running 12 tests using 1 worker
12 passed (8.2s)
```

```text
go test ./...
passed
```

Known limits:
- E2E uses mocked save responses; live backend integration remains future work.
- Cross-field frontend validation for extension/deposit bounds is minimal; backend remains authoritative.
- Rule inputs are compact operational controls, not a final polished console layout.

Next action:
- Wire H5 to real WebSocket ticket/connect flow or add live backend smoke tests for frontend API paths.
