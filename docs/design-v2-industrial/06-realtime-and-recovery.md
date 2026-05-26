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

## P3 Debezium-Borrowed Relay Governance

The app-owned relay borrows Debezium-style event and offset discipline without adding Debezium runtime:

- `outbox_events.event_schema_version` gates event contract evolution.
- `outbox_events.event_key` is the routing/ordering key, currently `auction_id`.
- `outbox_events.payload_sha256` is generated from PostgreSQL JSONB text and validated before publish.
- `outbox_relay_watermarks` exposes per-shard progress and lag using project offsets, not WAL LSN.
- publish/failure refreshes only the affected shard watermark.

## P3 NATS/JetStream-Borrowed Delivery Semantics

The app-owned relay also borrows NATS/JetStream consumer vocabulary without adding a NATS runtime:

- `outbox_id` is the delivery message id. Future broker adapters may map it to a broker message-id header, but broker sequence must never replace `auction_id + seq`.
- `outbox_delivery.status` maps to consumer delivery state in diagnostics: `PENDING/FAILED -> READY or NAK_RETRY_WAIT`, `PUBLISHING -> ACK_PENDING`, `PUBLISHED -> ACKED`, `DEAD -> TERM`.
- `attempts`, `max_attempts`, `redelivery_count`, `next_attempt_at`, `locked_until`, `last_error_class`, and `last_error_retriable` are exposed in monitor APIs so retries and poison handling can be audited.
- `outbox_relay_watermarks` plus monitor lateral counts expose ack-pending, retrying, redelivered, dead, oldest ready, and oldest retry age by shard.
- Non-retriable payload failures are treated like a JetStream `+TERM`: immediate `DEAD`, anomaly, and `outbox_gap_notice`.
- WebSocket slow-consumer events include pending-message or pending-byte reason and queue pressure fields.

Failure classes:

| Class | Retriable | Behavior |
|---|---:|---|
| `PAYLOAD_INVALID` | no | delivery goes `DEAD` immediately, anomaly and gap notice emitted |
| `REDIS_UNAVAILABLE` | yes | normal retry budget/backoff |
| `PUBLISH_TIMEOUT` | yes | normal retry budget/backoff |
| `UNKNOWN` | yes | normal retry budget/backoff |

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

## Heartbeat

Server WebSocket connections send explicit protocol ping frames on a fixed interval and require pong completion before the heartbeat timeout. Heartbeat failure closes the connection and records a `ws_heartbeat_timeout` user activity event plus `auction_ws_heartbeat_timeout_total`.

Defaults:

```text
interval = 20s
timeout = 5s
```

The heartbeat is a transport liveness signal only. It does not replace `auction_id + seq` recovery, snapshot fallback, or slow-consumer queue limits.

## Reconnect Backoff

Client:

```text
base = 2s
jitter = random(base/2, base*2)
max = 30s
server retry_after wins
```

Server can reject excessive reconnect with `retry_after_ms`.

## Operator Signals

Host-only monitor APIs may write `system_control_signals`; relay processes them outside bid/cancel/end transactions.

Supported signals:

| Signal | Target | Effect |
|---|---|---|
| `force_snapshot_rebuild` | `auction` | rebuilds Redis snapshot from PostgreSQL and records snapshot lifecycle rows |
| `retry_dead_outbox` | `outbox` | moves a `DEAD` delivery back to `PENDING` and clears failure fields |
| `pause_relay_shard` | `relay_shard` | marks a relay shard lease as paused for short operator investigation |
| `resume_relay_shard` | `relay_shard` | removes a paused shard lease |

Signals do not mutate auction truth, price, winner, order, or idempotency result.

## Background Browser

Browser timers can throttle/freeze in background. Therefore:

- heartbeat is advisory.
- visibilitychange triggers normal resync through snapshot path.
- no `fresh=1` bypassing cache.
- server must not classify all hidden clients as slow consumers solely from JS heartbeat delay.

## Self-Hub Release Gate

The runtime realtime implementation is the app-owned self hub. It must keep:

- browser ticket auth through `auction.v1` and one-time tickets;
- app-owned `auction_id + seq` recovery semantics;
- Redis history and DB snapshot fallback;
- bounded per-connection queues and slow-consumer closure;
- diagnostics for reconnect, recovery source, slow close, and snapshot saturation.

Do not add a second transport path without a new ADR and a clean self-hub failure bundle.
