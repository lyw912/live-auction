# Evidence Record

Feature/Gate: P2-06 security and abuse diagnostics

Date: 2026-05-24 Asia/Shanghai

Commit: included in this change; final hash recorded after commit

Environment: Windows local development machine; PostgreSQL/Redis local services; Go package tests, Vite builds, Playwright smoke.

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

- Added `AUTH_SESSION_EXPIRED` anomaly producer.
- Added filtered anomaly query for `type`, `room_id`, `user_id`, `auction_id`, and `trace_id`.
- Added PC monitor filter controls.
- Reused existing real producers for ACL, rate-limit, too-hot, payment invalid signature, and payment reconcile mismatch.

## Expected Invariant

- Every P2-06 named anomaly has a real producer.
- PC host can filter anomalies by room/user/auction/trace/type.
- Diagnostic panel remains backed by real rows, not static cards.

## Result

PASS.

## Observed Data

- `cd backend && go test ./internal/gateway` passed.
- `cd backend && go test ./...` passed.
- `pnpm run build` passed; PC bundle still emits the existing Vite chunk-size warning.
- `pnpm test:e2e:h5-live` passed after fixing smoke seed cleanup for `payment_events -> orders` foreign keys.
- `pnpm test:e2e` passed: 19 Playwright route-mocked H5/PC tests.
- `TestExpiredSessionRejects` now proves expired sessions emit `AUTH_SESSION_EXPIRED`.
- `TestMonitorAnomaliesFilterByTypeUserAuctionAndTrace` proves backend filtering.
- PC diagnostics render the anomaly filter control; backend integration proves `type`, `room_id`, `user_id`, `auction_id`, and `trace_id` filtering semantics.

## Known Limits

- Auth anomaly has no user id when session lookup fails before identity is known.
- Alert thresholds and runbook pages are not expanded in this milestone.

## Next Action

Use the P2-07 local baseline and bottleneck harness during Windows development; defer final Linux capacity calibration to P5.
