# ADR P2-04 · Bid Admission Control

Date: 2026-05-24 Asia/Shanghai

Status: Accepted

## Context

P0/P1 protected bid correctness with PostgreSQL row locks and durable idempotency, but every syntactically valid bid could still enter the hot path. P2 requires abuse controls that reduce avoidable row-lock pressure without moving price, winner, or order authority out of PostgreSQL.

## Decision

- Keep PostgreSQL as the only money truth.
- Add gateway bid admission before `Repository.PlaceBid` and `Repository.ConfirmBid`.
- Probe completed bid idempotency before any limiter; completed replay returns the stored result even if Redis counters are already exhausted.
- Enforce Redis-backed GCRA-style admission for:
  - user per auction;
  - IP per auction;
  - global auction rate.
- Keep the limiter small and observable. Redis is a throttle dependency, not correctness authority.
- If Redis is unavailable, fail open for legitimate bidders, keep local admission, and insert a `RATE_LIMIT_REDIS_DOWN` anomaly.
- Add per-auction in-process semaphore before the PostgreSQL bid transaction. Saturation returns `BID_AUCTION_TOO_HOT` with `Retry-After`.
- Record admission rejects as real anomalies so the PC monitor can explain `RATE_LIMITED` and `BID_AUCTION_TOO_HOT` spikes even though they do not create bid rows.
- Expose thresholds through environment-backed config.

## Consequences

- Rate limiting rejects do not create bid rows or auction events because they happen before the executable bid section.
- Redis outages may allow more bid attempts, but the PostgreSQL lock and idempotency core still preserve money correctness.
- In-process semaphore is per backend process only; P3 multi-instance work must add shared or per-room fairness if final scale evidence requires it.
- k6 abuse evidence records distribution of accepted/rejected/limited/too-hot responses, not capacity claims.

## Follow-Up Gates

- P2-06 should surface `RATE_LIMIT_REDIS_DOWN` and `BID_AUCTION_TOO_HOT` clearly in PC diagnostics.
- P2-07 should run bid abuse and final-second workloads with raw output on the baseline machine.
- P3 multi-instance admission must decide whether per-process hot semaphores are sufficient or need a shared room/auction admission layer.
