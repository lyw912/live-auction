# Evidence Record

Feature/Gate: P2-03 remove fixed room path

Date: 2026-05-24 Asia/Shanghai

Commit: included in this change; final hash recorded after commit

Environment: Windows local development machine; PostgreSQL/Redis local services; Go package tests, Vite builds, Playwright mock and live smoke.

Command:

```text
cd backend && go test ./internal/gateway
cd backend && go test ./...
pnpm run build
pnpm test:e2e:h5-live
pnpm test:e2e
```

Raw Output Path: this evidence file records command output summary; no separate raw log captured for this gate.

## Setup

- Added `GET /api/rooms`.
- PC console loads accessible rooms and exposes a room selector.
- H5 derives room from `/rooms/{room_id}`.
- Demo seed creates `room_main` and `room_side`, with independent auction/chat data.
- P1 load seed now creates memberships for demo and k6 users.

## Expected Invariant

- PC can switch rooms and create/manage auctions in the selected room.
- H5 reload into each room restores that room's auction/chat state.
- Room routes do not leak `room_main` auction/chat into `room_side`.
- Existing UI state matrix tests remain backend-independent.

## Result

PASS

## Observed Data

- `go test ./internal/gateway` passed.
- `go test ./...` passed.
- `pnpm run build` passed with existing PC chunk-size warning.
- `pnpm test:e2e:h5-live` passed with three tests, including H5 `/rooms/room_side` isolation and PC room selector flow.
- `pnpm test:e2e` passed: 19 mock UI tests.

## Failure Interpretation

Mock E2E initially failed because H5 now requires session readiness and mock tests did not stub `/api/auth/me|login`. The tests were updated to mock auth/rooms explicitly; live smoke remained the real backend verification.

## Known Limits

- `room_main` remains the default root fallback and sample room for local demos.
- PC room selector is functional but minimal; richer room metadata and room management are outside P2-03.
- Formal multi-room load evidence is still P2-07.

## Next Action

Implement P2-04 bid admission control and abuse behavior.
