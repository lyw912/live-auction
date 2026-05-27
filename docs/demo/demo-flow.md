# P0 Demo Flow

> P10 judge-facing demo policy lives in `docs/demo/p10-no-mock-auction-demo.md`. P10 should use a backend-created item and auction in the demo session and must not rely on Playwright route mocks or a pre-seeded ACTIVE auction as the main trunk.

Date: 2026-05-22

Commit: pending

## Preconditions

- Local PostgreSQL, Redis, and MinIO are running from `infra/docker-compose.yml`.
- `.env` exists and is based on `.env.example`.
- Database migrations have been applied.
- Demo smoke data has been seeded with `go run ./cmd/p0smokeseed` from `backend`.
- Backend runs on `http://localhost:8080`.
- H5 runs on `http://127.0.0.1:5173`.
- PC console runs on `http://127.0.0.1:5174`.

## Repro Commands

```powershell
docker compose -f infra\docker-compose.yml up -d
$env:DATABASE_URL="postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
goose -dir backend\migrations postgres $env:DATABASE_URL up
cd backend
go run ./cmd/p0smokeseed
go run ./cmd/server
```

In separate terminals:

```powershell
pnpm dev:h5
pnpm dev:pc
```

## Flow

1. Open PC console and confirm active auction/rule/order data for `room_main`.
2. In PC console, create a local demo item and DRAFT auction, save full rule fields on the selected auction, then schedule/start/cancel or narrate as needed for the demo branch.
3. Open H5. The browser enters `room_main`, loads active auctions from `GET /api/rooms/room_main/auctions`, and selects the active auction returned by the API.
4. Confirm H5 obtains a WebSocket ticket and shows connected realtime state.
5. Send a chat message from H5 and confirm it appears in the room chat list.
6. Place a normal bid. H5 enters pending state and only updates after the backend accepts or a live server event arrives.
7. Place a high bid that crosses the fat-finger threshold. H5 shows confirm UI and only submits `POST /api/auctions/{auction_id}/bids/confirm` after confirmation.
8. Trigger or inspect a reject path such as self-leading or invalid amount; verify reason-specific copy and no optimistic success.
9. Drive the active auction to cap/SOLD. H5 refreshes the winner's order from `/api/users/me/orders`, selects the newly generated pending order for the active auction, and calls mock payment once.
10. Open PC diagnostics tabs and show active auction, reject, outbox, scheduler, recovery, anomaly, and single-auction flight recorder data from real backend producers.
11. Run the live backend H5 smoke if a scripted demo proof is needed:

```powershell
pnpm test:e2e:h5-live
```

## Evidence To Capture

- Browser screenshots of H5 connected, pending bid, fat-finger confirm, cap SOLD, generated order/payment, paid state, PC host live flow, and diagnostics tabs.
- Backend logs around `POST /api/auth/ws-ticket`, bid, confirm, generated order payment, PC item/auction/rule/lifecycle APIs, and monitor routes including `/api/monitor/auctions/{auction_id}/flight-recorder`.
- Test output from `pnpm test:e2e:h5-live`.
- P4 risk gate output from `pnpm test:risk:p4` after any performance-path change.
- Optional DB rows from `auctions`, `bids`, `orders`, `auction_events`, `outbox_events`, and `anomalies`.

## Known Limits To State

- H5 still enters deterministic local room `room_main`; this is a demo room, not a full room selector.
- P0 uses mock auth and mock payment.
- P10 narrows the judge-facing trunk: local demo identity/room setup is allowed, but route-mocked API responses and pre-seeded ACTIVE auctions are not allowed as the main demo evidence.
- Payment and real live streaming are outside the P10 no-mock auction trunk; a looping product video can stand in for the live visual layer, and local fake-provider payment must be labeled optional if shown.
- No production performance number is claimed from the demo.
- Redis-backed WebSocket tickets fail closed when Redis is unavailable.

## Fallback Plan

- If browser dev proxies are unstable, run `pnpm test:e2e:h5-live` and show its raw output as the live backend proof.
- If Redis is down, state the known degradation clearly: bidding remains PostgreSQL-authoritative where reachable, but WebSocket ticket/reconnect quality is not claimed during the outage.
- If performance is questioned, show `docs/perf/p0-load-smoke-2026-05-22.md` only as local smoke evidence and defer any QPS/P99/fanout claim until a formal 3-run baseline exists.
