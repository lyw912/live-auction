# P0-24 H5 Fat-Finger Confirm

Gate:
- fat-finger confirm
- no optimistic bid success

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
- H5 bid response `FAT_FINGER_CONFIRM_REQUIRED` no longer updates price or winner.
- UI shows a second-step `确认高额出价` CTA with the server returned amount.
- Confirm CTA calls `POST /api/auctions/auc_live/bids/confirm`.
- Confirm request includes:
  - `confirm_token`
  - `idempotency_key`
  - matching `Idempotency-Key` header from the original bid attempt.
- Price and self-leading state update only after confirm API returns accepted bid response.

Raw output:

```text
pnpm build
mobile-h5: tsc -b && vite build passed
pc-console: tsc -b && vite build passed
warning: PC chunk exceeds 500 kB after minification due Arco bundle; no performance claim made.
```

```text
pnpm test:e2e
Running 11 tests using 1 worker
11 passed (7.7s)
```

```text
go test ./...
passed
```

Known limits:
- E2E uses mocked bid and confirm endpoints; this slice proves client confirm protocol behavior.
- Confirm token expiry UI is not implemented.
- Real backend/session identity integration remains future work.

Next action:
- Wire H5 to real WebSocket ticket/connect flow or add bid/order history UI.
