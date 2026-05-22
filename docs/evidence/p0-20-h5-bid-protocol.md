# P0-20 H5 Bid Protocol

Gate:
- pending bid
- rejected bid
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
- H5 active bid CTA now posts to `/api/auctions/auc_live/bids`.
- Request includes:
  - generated `client_bid_id`
  - matching `Idempotency-Key`
  - `amount_cents`
  - `client_seen_seq`
- During pending, UI keeps the last authoritative price visible and disables the bid CTA.
- Accepted state updates price/seq only after the server response.
- Rejected state maps backend error code to business copy and re-enables CTA.
- Playwright uses mocked API responses to control pending timing and prove there is no optimistic success.

Raw output:

```text
pnpm build
mobile-h5: tsc -b && vite build passed
pc-console: tsc -b && vite build passed
warning: PC chunk exceeds 500 kB after minification due Arco bundle; no performance claim made.
```

```text
pnpm test:e2e
Running 6 tests using 1 worker
6 passed (5.3s)
```

```text
go test ./...
passed
```

Known limits:
- Bid API is mocked in E2E; this slice proves client protocol behavior, not live backend integration.
- Fat-finger confirm is not implemented.
- WS recovery/history is still separate work.
- Accepted bid currently uses `current_winner_id === user_1` to show self-leading in the scaffold; real identity binding remains backend/session integration work.

Next action:
- Add H5 payment double-click UI gate or out-of-order/snapshot recovery client gate.
