# P0-25 H5 History UI

Gate:
- H5 bid history
- H5 order history

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
- H5 room now includes a compact `我的历史` panel.
- Refresh calls:
  - `GET /api/users/me/bids`
  - `GET /api/users/me/orders`
- Bid rows render API-provided auction/result/amount.
- Order rows render API-provided order/status/amount.
- Empty and error states are explicit; history is not hard-coded as fake result data.

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
12 passed (8.8s)
```

```text
go test ./...
passed
```

Known limits:
- This evidence file uses mocked history APIs; the live H5 smoke separately reads backend bid/order history for the deterministic demo auction.
- History rows are compact summaries and do not yet open detail views.
- Pagination is not implemented.

Next action:
- Wire H5 to real WebSocket ticket/connect flow or add PC full rule fields for duration/extension/deposit.
