# 12 · Engineering Rules

## Non-Negotiables

1. PostgreSQL is money truth.
2. Redis is projection/cache, not auction authority.
3. WebSocket is delivery, not truth.
4. Client time never decides close or winner.
5. Every bid attempt after executable section is idempotent.
6. Every auction state mutation writes an event and outbox.
7. Every recovery path must be bounded.
8. Every performance number needs baseline evidence.
9. Every diagnostic panel needs a real producer.
10. No AI in bid/cancel/settlement path.

## Transaction Rules

Inside bid/cancel/end transaction:

- set explicit lock and statement timeouts;
- lock auction row;
- validate using locked row;
- write all related rows in same transaction;
- complete idempotency before commit;
- no WS publish inside transaction;
- no external API call inside transaction.

## Idempotency Rules

- Probe completed idempotency before rate limit.
- Same key + same hash returns first result.
- Same key + different hash rejects.
- PROCESSING has bounded lifetime.
- Unknown result recovery reads truth tables.
- No indefinite retry-later.

## Outbox Rules

- Outbox event payload is immutable.
- Delivery status is mutable separately.
- Same auction seq must publish in order unless lower seq is DEAD.
- DEAD creates anomaly and gap notice.
- Relay at-least-once; clients dedupe.

## WebSocket Rules

- Browser auth must be implementable.
- No long-lived auth token in query string.
- Every connection has bounded queues.
- Social messages cannot block auction messages.
- Gap means snapshot.
- Reconnect storm must not stampede DB.

## Frontend Rules

- No optimistic bid success.
- No client hammer.
- CTA disabled during pending/recovering/disconnected.
- State labels come from server snapshot/events.
- Animation must not hide or block bid controls.
- Mobile layout must be tested for overlap.

## Database Rules

- Money values are integer cents.
- Partial unique index errors mapped by constraint name.
- No hidden DB trigger for domain state changes.
- Every enum-like field has application constants.
- Every unique constraint has a test hitting conflict.

## Performance Rules

- No guessed P99/QPS/connection number.
- No WSL2 final baseline.
- k6 WS uses `k6/websockets`.
- Baseline includes environment and raw output.
- Failed benchmark with diagnosis is acceptable.

## Documentation Rules

- Design claims must say whether they are implemented, tested, or future-work.
- Completeness features are not marketed as deep technical innovation.
- Known industrial alternatives should be listed honestly.
- Do not hide limitations.

## Review Checklist

Before merging:

- Does this change affect auction truth?
- Does it need idempotency?
- Does it need an event/outbox?
- Can it race with cancel/end/bid?
- Can a retry duplicate money state?
- Can a reconnect recover this state?
- Can slow clients cause memory growth?
- Can Redis/DB down produce unsafe behavior?
- Is there a test for the failure mode?
- Is there a diagnostic if it fails in demo?

## Forbidden Shortcuts

- Direct DB commit then direct WS broadcast without outbox.
- Relying on frontend validation for rule correctness.
- Using client timestamp for ordering.
- Storing price/winner only in Redis.
- Adding dashboard cards with fake data.
- Adding performance claims from internet benchmarks.
- Implementing Redis Lua without reconciliation.
- Letting chat/presence share auction correctness path.
