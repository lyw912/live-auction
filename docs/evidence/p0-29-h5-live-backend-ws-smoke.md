# H5 Live Backend WebSocket Smoke

Feature/Gate: H5 live backend WebSocket smoke

Date: 2026-05-22

Commit: pending

Environment: Windows 11, Go 1.26.3, Node 24.9.0, pnpm 11.2.2, local PostgreSQL/Redis/MinIO from Docker Compose

Command: `pnpm test:e2e:h5-live`

Raw Output Path: terminal output in development session

## Setup

The smoke runner seeds deterministic local data for `room_main` and `auc_live`, starts the Go backend on `127.0.0.1:18080`, starts the H5 Vite app on `127.0.0.1:5175`, and proxies `/api` plus `/ws` from Vite to the backend.

The browser opens the real H5 app. H5 loads the active auction from `GET /api/rooms/room_main/auctions`, requests `/api/auth/ws-ticket` from the Go backend for that selected auction, and opens `/ws?room_id=room_main&auction_id=auc_live&last_seq=41` using browser subprotocols `auction.v1` and `ticket.<token>`.

The test then sends a real HTTP bid through the backend. The backend writes bid/event/outbox rows, the server outbox relay publishes the event, and the already-connected H5 WebSocket receives and applies the authoritative event.

## Expected Invariant

- H5 can obtain a backend-issued one-time WebSocket ticket.
- A real browser WebSocket can connect through the Vite dev proxy to Go `/ws`.
- The backend accepts only the browser subprotocol contract.
- A backend bid that commits to outbox is delivered to H5 through the live WS path.
- H5 updates price only after the authoritative event, not from client-side prediction.

## Result

PASS.

## Observed Data

- Backend log observed `POST /api/auth/ws-ticket`.
- Backend log observed `POST /api/auctions/auc_live/bids`.
- Browser initially displayed `WebSocket 已连接 · 状态来自服务端事件`.
- Backend bid response was `ACCEPTED` for `auc_live` at `seq: 42` and `current_price_cents: 40000`.
- H5 displayed `event seq 42` and `¥400.00` after the live backend event arrived.

## Failure Interpretation

If this test fails before the bid request, the browser ticket or WebSocket connect path is broken. If it fails after the bid response, the outbox relay, WS fanout, or H5 event apply path is broken. Either failure means the UI contract tests are insufficient for live backend readiness.

## Known Limits

- This smoke uses deterministic local seed data and one browser client; it is not a reconnect storm, slow consumer, or multi-client jitter test.
- The H5 app still enters deterministic local room `room_main`; the active auction ID is selected from backend API responses. A full room selector remains outside this P0 demo slice.
- The Vite proxy is only a local development path. Production deployment should serve API/WS routing explicitly.

## Next Action

Continue with live backend REST smoke for H5 paths not covered by this WS slice.
