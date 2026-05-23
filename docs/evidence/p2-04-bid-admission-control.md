# Evidence Record

Feature/Gate: P2-04 bid admission control and abuse behavior

Date: 2026-05-24 Asia/Shanghai

Commit: included in this change; final hash recorded after commit

Environment: Windows local development machine; PostgreSQL/Redis local services; Go package tests and k6 script validation.

Command:

```text
cd backend && go test ./internal/gateway
cd backend && go test ./...
pnpm exec node tests/load/validate-k6-suite.mjs
pnpm run build
pnpm test:e2e:h5-live
pnpm test:e2e
```

Raw Output Path: this evidence file records command output summary; no separate raw k6 run captured for this gate.

## Setup

- Added gateway `bidAdmission` before bid transaction entry.
- Added Redis GCRA-style atomic checks for user/IP/auction limits.
- Added local per-auction in-flight semaphore.
- Added `RATE_LIMIT_REDIS_DOWN` anomaly producer with fail-open behavior.
- Added anomaly producers for `RATE_LIMITED` and `BID_AUCTION_TOO_HOT` admission rejects.
- Added `bid-abuse.js` k6 workload for accepted/rejected/limited/too-hot distribution.

## Expected Invariant

- Completed idempotent bid replay bypasses rate limiting.
- Redis unavailable for bid limiting does not block legitimate bids and emits a real anomaly.
- Local auction overload returns `BID_AUCTION_TOO_HOT` and `Retry-After`.
- PostgreSQL remains price/winner/order truth.

## Result

PASS

## Observed Data

- `TestBidAdmissionCompletedReplayBypassesRedisLimiter` proves completed replay returns stored bid even when Redis counters are exhausted.
- `TestBidAdmissionRedisDownFailsOpenAndRecordsAnomaly` proves fail-open and `RATE_LIMIT_REDIS_DOWN` anomaly.
- `TestBidAdmissionUserLimitReturnsRateLimited` proves user abuse returns `RATE_LIMITED` and records an anomaly.
- `TestBidAdmissionLocalAuctionTooHotReturnsRetryAfter` proves local semaphore overload maps to `BID_AUCTION_TOO_HOT` with `Retry-After`.
- `go test ./...` passed.
- `pnpm exec node tests/load/validate-k6-suite.mjs` passed.
- `pnpm run build` passed with existing PC chunk-size warning.
- `pnpm test:e2e:h5-live` passed with three live backend tests.
- `pnpm test:e2e` passed with 19 mock UI tests.

## Known Limits

- Redis limiter is a compact GCRA-style script, not a standalone distributed rate-limit service.
- Local semaphore is per backend process. Multi-instance fairness remains P3.
- k6 abuse script is committed and validated, but formal raw output is deferred to P2-07 baseline.

## Next Action

Implement P2-05 payment provider boundary.
