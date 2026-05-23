# Evidence Record

Feature/Gate: P2-01 real session boundary

Date: 2026-05-23 Asia/Shanghai

Commit: pending

Environment: Windows local development machine; PostgreSQL/Redis local services; Go package tests and Vite production builds.

Command:

```text
goose -dir backend\migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
cd backend && go test ./internal/gateway
pnpm --filter mobile-h5 build
pnpm --filter pc-console build
pnpm test:e2e:h5-live
```

Raw Output Path: this evidence file records command output summary; no separate raw log captured for this small gate.

## Setup

- Added `auth_sessions` migration.
- Added local demo login/logout/me APIs.
- Replaced normal `/api` auth middleware with session lookup.
- Kept mock headers gated by `APP_ENV=test` or `ALLOW_MOCK_AUTH=true`.
- Updated H5/PC to establish a real session before protected API calls.

## Expected Invariant

- Host-only APIs reject user sessions.
- Expired or revoked sessions reject.
- Mock headers are unavailable in normal runtime unless explicitly enabled.
- H5/PC do not depend on mock auth headers.

## Result

PASS

## Observed Data

- `goose` migrated database to `202605230001`.
- `go test ./internal/gateway` passed, including arbitrary `user_id` login shortcut rejection.
- `pnpm --filter mobile-h5 build` passed.
- `pnpm --filter pc-console build` passed with the existing Vite large chunk warning.
- `pnpm test:e2e:h5-live` passed: H5 and PC live smoke established real sessions and no longer used mock auth headers.

## Failure Interpretation

No correctness failure observed in this gate. PC bundle-size warning is a frontend packaging concern, not an auth/session failure.

## Known Limits

- Demo login has no password/OAuth/SMS; this is a local session boundary, not a production identity provider.
- Room membership and host ownership ACL remain P2-02.
- Existing mock-auth tests still run under `APP_ENV=test`.

## Next Action

Implement P2-02 room membership and host ownership ACL using session identity as the trusted subject.
