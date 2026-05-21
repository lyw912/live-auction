# 06 · Realtime And Recovery

## Guarantees

The system promises:

- server-authoritative sequence per auction;
- recoverable client state via history or snapshot;
- at-least-once server delivery attempts;
- client dedupe by `auction_id + seq`;
- bounded memory for slow clients.

The system does not promise:

- WebSocket exactly-once delivery;
- every social/chat message recovered;
- client time deciding close/hammer;
- Redis as audit truth.

## Outbox Pattern

Bid/end/cancel transactions write:

```text
auction row mutation
bid/order row if applicable
auction_events
outbox_events
outbox_delivery
idempotency_records completion
```

All in one DB transaction. If process dies after commit but before WS publish, relay can resume from outbox.

## Relay Claim

Workers are partitioned by aggregate hash:

```text
hash(aggregate_type + ':' + aggregate_id) % OUTBOX_RELAY_SHARDS
```

For each shard:

1. claim due delivery rows.
2. join immutable event row.
3. enforce same-auction head-of-line.
4. publish serially within shard.
5. mark PUBLISHED or FAILED/DEAD.

Head-of-line rule:

```sql
NOT EXISTS (
  SELECT 1
  FROM outbox_events e2
  JOIN outbox_delivery d2 ON d2.outbox_id = e2.id
  WHERE e2.auction_id = e.auction_id
    AND e2.seq < e.seq
    AND d2.status NOT IN ('PUBLISHED','DEAD')
)
```

## Poison Event

After max attempts:

1. mark delivery DEAD.
2. write `OUTBOX_DEAD_LETTER` anomaly.
3. publish `outbox_gap_notice` directly to WS room.
4. clients fetch snapshot.
5. later events can continue.

Do not recursively write gap notice into outbox.

## Redis Projection

Relay writes:

1. `auction:{id}:events` append event.
2. `auction:{id}:snapshot` update snapshot.
3. optional leaderboard/hot fields.

History defaults:

```text
AUCTION_HISTORY_EVENTS=4096
AUCTION_HISTORY_TTL=active + 30m
```

These are configuration defaults, not performance claims.

## Client Recovery State Machine

```text
connected
  -> degraded
  -> recovering
  -> snapshot_applied
  -> connected
  -> disconnected
```

Client behavior:

- `seq == last_seq + 1`: apply.
- `seq <= last_seq`: discard.
- `seq > last_seq + 1`: pause and fetch snapshot.
- `outbox_gap_notice`: pause and fetch snapshot.
- disconnected/recovering: disable bid CTA.
- local countdown reaches zero: disable CTA and fetch snapshot; do not self-hammer.

## Snapshot Path

```text
try Redis snapshot
  if fresh enough -> return
else per-room singleflight
  -> global DB rebuild semaphore
     -> DB read and cache update
```

Semaphore full:

- return stale Redis snapshot if available with `stale=true`;
- else return `503 Retry-After: 1`;
- client remains recovering.

## WebSocket Auth

Browser-compatible:

1. HTTP session/mock auth obtains one-time `ws_ticket`.
2. browser connects with subprotocols `["auction.v1", "ticket.<token>"]`.
3. server validates and consumes ticket.
4. server accepts only `auction.v1`.

Ticket:

- TTL <= 60s.
- bound to user, room, auction.
- one-time.
- stored in Redis.

## Room Isolation

Server checks:

- user has access to room.
- auction belongs to room.
- host owns room for PC/host channels.
- `room_id` from query is not trusted.

Forged room test is P0.

## Backpressure

Per connection:

| Queue | Default | Overflow |
|---|---:|---|
| auction events | 256 | close `SLOW_CONSUMER` |
| social/chat | 32 | drop oldest social |

All socket writes use deadlines. Slow client must not pin goroutine or memory.

## Reconnect Backoff

Client:

```text
base = 2s
jitter = random(base/2, base*2)
max = 30s
server retry_after wins
```

Server can reject excessive reconnect with `retry_after_ms`.

## Background Browser

Browser timers can throttle/freeze in background. Therefore:

- heartbeat is advisory.
- visibilitychange triggers normal resync through snapshot path.
- no `fresh=1` bypassing cache.
- server must not classify all hidden clients as slow consumers solely from JS heartbeat delay.

## Centrifugo Contingency

Self hub must pass go/no-go. If not:

- relay publishes to Centrifugo API.
- Centrifugo history/recovery handles channel history.
- app still keeps DB snapshot fallback.
- client protocol adapter is isolated.

Centrifugo does not remove the need for outbox or snapshot truth.
