# H5 Live Backend REST Smoke

Feature/Gate: H5 live backend REST smoke

Date: 2026-05-22

Commit: pending

Environment: Windows 11, Go 1.26.3, Node 24.9.0, pnpm 11.2.2, local PostgreSQL/Redis/MinIO from Docker Compose

Command: `pnpm test:e2e:h5-live`

Raw Output Path: terminal output in development session

## Setup

The existing live H5 smoke runner seeds deterministic `room_main`, `auc_live`, and `ord_pending`, starts the Go backend on `127.0.0.1:18080`, starts H5 Vite on `127.0.0.1:5175`, and proxies `/api` plus `/ws` to the backend.

This slice extends the live smoke beyond WebSocket connect/fanout. The browser and Playwright API client call real backend REST endpoints for room auctions, snapshot, bid, fat-finger confirm, bid history, order history, and mock payment.

## Expected Invariant

- `GET /api/rooms/room_main/auctions` returns the seeded active auction.
- `GET /api/auctions/auc_live` returns authoritative snapshot/detail at seq 41 before the smoke bid.
- `POST /api/auctions/auc_live/bids` accepts a user bid with matching `Idempotency-Key`.
- A large bid returns `FAT_FINGER_CONFIRM_REQUIRED` without changing the displayed price/winner.
- `POST /api/auctions/auc_live/bids/confirm` accepts the original idempotency key and server token before H5 enters accepted UI.
- `GET /api/users/me/bids` returns the accepted bid for the current user.
- `GET /api/users/me/orders` returns `ord_pending` before payment and `PAID` after payment.
- `POST /api/orders/ord_pending/pay-mock` lets the winner pay once and H5 moves to paid UI only after the backend response.

## Result

PASS for the implemented live REST paths.

## Observed Data

- Backend log observed `GET /api/rooms/room_main/auctions`.
- Backend log observed `GET /api/auctions/auc_live`.
- Backend log observed `POST /api/auctions/auc_live/bids`.
- Backend log observed `POST /api/auctions/auc_live/bids/confirm`.
- Backend log observed `GET /api/users/me/bids`.
- Backend log observed `GET /api/users/me/orders`.
- Backend log observed `POST /api/orders/ord_pending/pay-mock`.
- Browser rendered live confirm flow `确认高额出价` and only reached accepted UI after confirm response.
- Browser rendered bid history row `¥500.00 · ACCEPTED`.
- Browser rendered order history row `ord_pending` and `¥600.00 · ORDER_PENDING`.
- Browser payment UI reached `已支付` and order history returned `order_status: "PAID"`.

## Failure Interpretation

If this smoke fails before payment, the H5 live REST integration is not demo-ready and the mocked UI tests are insufficient. If payment succeeds but history does not update, the backend response shape or H5 current-user headers have drifted.

## Known Limits

- This smoke uses one deterministic local auction and one browser client, not multi-client jitter or pagination.
- H5 still uses scaffold constants for `room_main`, `auc_live`, and `ord_pending`.

## Next Action

Replace scaffold H5 IDs with room auction selection from `/api/rooms/{room_id}/auctions`, then add Redis-down reconnect evidence.
