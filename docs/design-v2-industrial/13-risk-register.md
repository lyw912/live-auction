# 13 · Risk Register

## Risk Levels

- P0: can break correctness/demo.
- P1: can weaken scoring or evidence.
- P2: future-work / known limitation.

## Risks

| Risk | Level | Why It Matters | Mitigation |
|---|---|---|---|
| bid transaction too slow under hot row | P0/P1 | final-second contention is core challenge | local semaphore, lock timeout, benchmark, no unmeasured claim |
| outbox relay reorders events | P0 | client can silently miss seq | shard by aggregate + head-of-line |
| outbox poison blocks stream forever | P0 | clients stuck stale | DEAD + gap notice + snapshot |
| idempotency stuck PROCESSING | P0 | user retried forever or duplicates | bounded FSM + unknown recovery |
| END_AUCTION ignores extended end_at | P0 | early hammer | re-read after lock and self-reschedule |
| cap unreachable | P0 | cap feature impossible | rule invariant |
| browser cannot set WS Authorization header | P0 | H5 cannot connect | one-time ticket + subprotocol |
| snapshot fallback DB stampede | P0 | reconnect storm kills DB | Redis cache + singleflight + global semaphore |
| slow clients grow memory | P0 | process dies | bounded queue + close |
| chat overwhelms bid | P0 | social feature harms core | social queue/drop, no outbox |
| hidden DB trigger clears focus | P1 | surprising side effects | app-layer explicit clear |
| performance numbers fake | P1 | credibility loss | baseline discipline |
| self WS hub takes too long | P1 | delivery risk | focused fanout, reconnect, slow-consumer, and backpressure gates |
| Redis down disables rate limit | P1 | abuse risk | bid fail-open but local semaphore, reconnect fail-closed |
| DB/Redis versions differ | P1 | benchmark not reproducible | env recorded in baseline |
| UI animation blocks bid | P1 | user experience and scoring | longtask test and cut rule |

## Traps From Reviews

### Trap: Treating `SKIP LOCKED` As Ordered

It is for non-blocking queue consumption, not fairness or per-aggregate ordering. Do not infer event order from concurrent worker claim order.

### Trap: Chat Through Outbox

Chat has no money correctness. Putting it in outbox increases write load and mixes critical/non-critical streams.

### Trap: Redis Lua As Easy Win

Lua only makes Redis atomic. It does not solve DB/audit/WS consistency. Without reconciliation it creates a split-brain source of truth.

### Trap: Cap Price With Increment Grid

If `(cap-start) % increment != 0`, cap is unreachable. Reject such rules at creation.

### Trap: Rate Limit Before Idempotency

Legitimate retries can be rate limited and never get first result. Probe completed idempotency first.

### Trap: WS Header Auth

Browser WebSocket cannot pass arbitrary `Authorization` header through constructor. Use ticket/subprotocol.

### Trap: Background Timer

Mobile/browser background can throttle timers. UI countdown is display only; resync on visibility.

### Trap: Diagnostic Fake Panels

A fake dashboard hurts credibility. Real small diagnostic is better.

## Known Limitations To State Honestly

- P0 is single backend process, not HA.
- Self WS hub is the only runtime realtime implementation; horizontal realtime scale is a known future limitation until proven.
- Polling outbox is not ultimate large-scale architecture.
- Redis history is bounded; snapshot fallback is normal.
- Chat/presence are best-effort.
- Performance scale is whatever baseline proves.

## Anti-Overengineering Notes

Do not build:

- microservices;
- Temporal;
- NATS;
- OTel before P1 metrics;
- full CDC/WAL outbox in P0;
- Redis Lua in P0;
- real payment/live;
- AI product features.

## If Asked By Reviewer

Use engineering answers, not hype:

- "We chose PG row lock as the serial point for correctness; Redis is projection."
- "Outbox handles commit-after-crash; WS is not exactly once."
- "History is bounded; gap triggers snapshot."
- "We do not claim a performance number until the baseline file proves it."
- "Self hub remains scoped to the tested release envelope; we do not claim horizontal realtime scale until evidence proves it."
