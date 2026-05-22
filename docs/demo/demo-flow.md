# P0 Demo Flow

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

1. Open PC console and confirm active auction/rule data for `room_main`.
2. Open H5. The browser enters `room_main`, loads active auctions from `GET /api/rooms/room_main/auctions`, and selects the active auction returned by the API.
3. Confirm H5 obtains a WebSocket ticket and shows connected realtime state.
4. Place a normal bid. H5 enters pending state and only updates after the backend accepts or a live server event arrives.
5. Place a high bid that crosses the fat-finger threshold. H5 shows confirm UI and only submits `POST /api/auctions/{auction_id}/bids/confirm` after confirmation.
6. Trigger or inspect a reject path such as self-leading or invalid amount; verify reason-specific copy and no optimistic success.
7. Show order/payment state. H5 selects the pending order for the active auction and calls mock payment once.
8. Open PC diagnostics tabs and show active auction, reject, outbox, scheduler, recovery, and anomaly data from real backend producers.
9. Run the live backend H5 smoke if a scripted demo proof is needed:

```powershell
pnpm test:e2e:h5-live
```

## Evidence To Capture

- Browser screenshots of H5 connected, pending bid, fat-finger confirm, accepted state, paid state, and diagnostics tabs.
- Backend logs around `POST /api/auth/ws-ticket`, bid, confirm, order payment, and monitor routes.
- Test output from `pnpm test:e2e:h5-live`.
- Optional DB rows from `auctions`, `bids`, `orders`, `auction_events`, `outbox_events`, and `anomalies`.

## Known Limits To State

- H5 still enters deterministic local room `room_main`; this is a demo room, not a full room selector.
- P0 uses mock auth and mock payment.
- No production performance number is claimed from the demo.
- Redis-backed WebSocket tickets fail closed when Redis is unavailable.

## Fallback Plan

- If browser dev proxies are unstable, run `pnpm test:e2e:h5-live` and show its raw output as the live backend proof.
- If Redis is down, state the known degradation clearly: bidding remains PostgreSQL-authoritative where reachable, but WebSocket ticket/reconnect quality is not claimed during the outage.
- If performance is questioned, show `docs/perf/p0-load-smoke-2026-05-22.md` only as local smoke evidence and defer any QPS/P99/fanout claim until a formal 3-run baseline exists.
