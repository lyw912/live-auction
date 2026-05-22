# P0-21 H5 Payment Double Click

Gate:
- payment double click

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
- H5 SOLD winner CTA calls `POST /api/orders/{order_id}/pay-mock`; this route-mocked E2E uses fixture id `ord_pending`.
- Request includes `Idempotency-Key` and `{ "confirm": true }`.
- Payment pending disables the CTA and keeps the UI waiting for server confirmation.
- Double-clicking the payment CTA sends one mocked payment request.
- Server `PAID` response moves the UI to paid state and keeps the CTA disabled.

Raw output:

```text
pnpm build
mobile-h5: tsc -b && vite build passed
pc-console: tsc -b && vite build passed
warning: PC chunk exceeds 500 kB after minification due Arco bundle; no performance claim made.
```

```text
pnpm test:e2e
Running 7 tests using 1 worker
7 passed (5.6s)
```

```text
go test ./...
passed
```

Known limits:
- Payment API is mocked in E2E; this slice proves client double-click behavior, not live backend integration.
- Live backend order generation/payment is covered separately by `p0-30-h5-live-backend-rest-smoke.md`.
- Payment failure copy is minimal and should be expanded in P1.

Next action:
- Preserve one-click payment idempotency when real payment provider integration is added.
