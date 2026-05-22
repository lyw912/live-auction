# H5 WebSocket Ticket Connect

Feature/Gate: H5 real WebSocket ticket/connect flow

Date: 2026-05-22

Commit: pending

Environment: Windows 11, Go 1.26.3, Node 24.9.0, pnpm 11.2.2

Command: `pnpm build`; `pnpm test:e2e`; `go test ./...` from `backend`

Raw Output Path: terminal output in development session

## Setup

The mobile H5 app now requests `/api/auth/ws-ticket` with `{room_id, auction_id}` and opens `/ws?room_id=...&auction_id=...&last_seq=...` using browser-compatible subprotocols `["auction.v1", "ticket.<token>"]`.

Playwright installs a browser WebSocket mock for `/ws` so the test can assert the exact URL/subprotocol contract and inject authoritative server events without depending on a live backend process.

## Expected Invariant

- H5 obtains a one-time WS ticket before connecting.
- The browser WebSocket uses the v2 subprotocol contract, not an Authorization header.
- Authoritative WS events and existing test-injected events use the same client recovery path.
- Gap or `outbox_gap_notice` still disables the CTA and fetches `/api/auctions/{id}` snapshot.

## Result

PASS.

## Observed Data

- Playwright coverage asserts the ticket body uses the current room and the active auction selected from `GET /api/rooms/{room_id}/auctions`.
- Playwright coverage asserts the socket URL uses the selected room, selected auction, and last known sequence.
- New Playwright coverage asserts subprotocols are `auction.v1` and `ticket.ticket_live`.
- Injected WS `bid_accepted` seq 42 updates price, leader, and event feedback from the authoritative message.
- Existing snapshot gap tests continue to prove CTA disable and snapshot recovery behavior.

## Failure Interpretation

If ticket issuance fails or the socket closes, H5 marks the connection as disconnected, shows stale state, disables the dangerous CTA for the active auction surface, and schedules reconnect. This avoids pretending a disconnected room is safe to bid in.

## Known Limits

- This is browser contract coverage with a WebSocket mock. It does not run the Vite H5 app against the Go backend in one process.
- The current room is still the deterministic local demo room `room_main`; the auction ID is selected from API responses.
- Reconnect backoff is a fixed 2 seconds in the scaffold, not the full jittered production policy from `06-realtime-and-recovery.md`.
- Live backend smoke coverage for H5 REST and WS remains a separate slice.

## Next Action

Add live backend smoke tests for H5 REST paths, then replace scaffold IDs with room/auction selection from `/api/rooms/{room_id}/auctions`.
