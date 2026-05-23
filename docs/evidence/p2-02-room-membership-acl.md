# Evidence Record

Feature/Gate: P2-02 room membership and host ownership ACL

Date: 2026-05-23 Asia/Shanghai

Commit: pending

Environment: Windows local development machine; PostgreSQL/Redis local services; Go package tests and Vite/live Playwright smoke.

Command:

```text
goose -dir backend\migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
cd backend && go test ./internal/gateway
cd backend && go test ./...
pnpm run build
pnpm test:e2e:h5-live
```

Raw Output Path: this evidence file records command output summary; no separate raw log captured for this gate.

## Setup

- Added `room_memberships` migration.
- Seeded demo host/viewer memberships.
- Added gateway `roomACL` for host ownership and viewer membership.
- Protected host auction mutations, room auction listing, auction detail, bid, confirm bid, chat, and WS ticket issuance.
- Added `ACL_FORBIDDEN` anomaly producer.

## Expected Invariant

- Host APIs reject foreign host sessions.
- Viewer bid/chat/ws-ticket reject foreign or banned room membership.
- Auction id alone is not enough to bypass room access.
- ACL diagnostics are backed by real reject producers.

## Result

PASS

## Observed Data

- `goose` migrated database to `202605230002`.
- `go test ./internal/gateway` passed with forged-room, banned-viewer, WS-ticket, foreign-host ACL, and no-room-id auction-list leak tests.
- `go test ./...` passed.
- `pnpm run build` passed with existing PC chunk-size warning.
- `pnpm test:e2e:h5-live` passed after demo seed created explicit memberships.

## Failure Interpretation

No correctness failure observed in this gate. One live smoke attempt timed out waiting for `/readyz` before the server process printed startup; after warming `cmd/server` compilation and clearing ports, the same command passed.

## Known Limits

- Current implementation blocks banned users from issuing new WS tickets. Mid-ticket revocation and disconnecting already-connected banned users remains a follow-up realtime revocation-window problem.
- PC monitor can show ACL anomalies in the existing anomaly table, but P2-06 still needs richer filter/drilldown UX.

## Next Action

Implement P2-03 remove fixed room path: PC room selector, H5 room route, two-room E2E, and multi-room seed/load env.
