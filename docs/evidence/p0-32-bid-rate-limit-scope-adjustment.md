# Bid Rate Limit Scope Adjustment

Feature/Gate: Redis down bid limit scope adjustment

Date: 2026-05-22

Commit: pending

Environment: Windows 11, Go 1.26.3, local PostgreSQL/Redis from Docker Compose

Command: `rg -n "RATE_LIMITED|BID_AUCTION_TOO_HOT|rate limit|limiter" backend frontend tests docs/evidence`

Raw Output Path: terminal output in development session

## Setup

The P0 failure/degradation matrix includes `Redis down bid limit`. That gate only applies when the bid path has a Redis-backed rate limiter. The current implementation does not include a bid rate limiter in the executable bid path.

## Expected Invariant

- The bid truth path remains PostgreSQL-authoritative.
- Redis is not used to accept, reject, price, winner, order, or idempotency-complete a bid.
- No documentation should claim bid rate limiting or Redis-down rate-limit fail-open behavior until a limiter exists.
- The UI may display copy for rate-limit-like business codes, but that is not evidence of a backend limiter.

## Result

PASS as a documented P0 scope adjustment.

## Observed Data

- Backend bid code does not implement a Redis-backed limiter.
- Existing backend degradation evidence covers DB lock timeout and idempotency timeout, not rate limiting.
- Existing Redis-down evidence covers WS ticket fail-closed and snapshot rebuild saturation, not bid limiting.
- The P0 coverage ledger keeps this explicit instead of marking a nonexistent limiter green.

## Failure Interpretation

If a future Redis-backed bid limiter is added, this scope adjustment becomes stale. The project must then add an automated Redis-down bid-limit test that proves the intended fail-open/fail-closed behavior and anomaly output.

## Known Limits

- This is not a rate-limit implementation.
- This does not provide abuse protection for bid floods.
- No performance or QPS claim is made.

## Next Action

Keep the bid path correctness gates focused on PostgreSQL truth/idempotency. Add a Redis-backed limiter only with a dedicated design note, degradation test, and anomaly producer.
