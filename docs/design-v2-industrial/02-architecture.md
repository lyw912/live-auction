# 02 · Architecture

## Technical Route

| Layer | Choice | Reason | Not Chosen |
|---|---|---|---|
| Frontend | React + TypeScript + Vite | official full-stack fit, fast PC/H5 delivery | Flutter/native |
| PC UI | Semi Design or Arco Design | mature admin components | fully custom admin |
| H5 UI | custom mobile auction UI | auction CTA/status/recovery need domain UX | admin components on mobile |
| Backend | Go modular monolith | concurrency, pprof, simple deployment | microservices |
| HTTP | Go standard net/http + router or Hertz | either OK; choose by team familiarity | framework-driven decision |
| WebSocket | self hub with bounded queues and recovery gates | direct control of recovery/backpressure | Socket.IO mainline |
| DB | PostgreSQL | transactions, row locks, constraints, audit | Redis-only truth |
| Cache | Redis | snapshot/history/rate-limit/tickets/presence | Redis as money truth |
| Object Storage | MinIO | S3-compatible local object storage | mixed local/multipart paths |
| Scheduler | DB lease worker | durable and testable | memory timers |
| Observability | P0 diagnostics, P1 Prometheus/Grafana | real evidence before dashboards | static panels |
| Load/Chaos | k6, Toxiproxy | repeatable tests | hand refresh only |

## Runtime Architecture

```text
React PC Console
  -> REST APIs
  -> WS host room view

React Mobile H5
  -> REST snapshot/detail/bid/order/chat
  -> WS room event stream

Go API Gateway
  -> auth/session
  -> schema validation
  -> ACL
  -> idempotency probe
  -> rate limits

Auction Service
  -> rule validation
  -> bid transaction
  -> state machine
  -> auction_events/outbox

Order Service
  -> order creation
  -> mock payment
  -> deposit status

Outbox Relay
  -> sharded ordered delivery
  -> Redis history/snapshot
  -> WS hub publish

Realtime Hub
  -> browser ticket auth
  -> room subscriptions
  -> per-client queues
  -> recovery and backpressure

Scheduler Worker
  -> START_AUCTION
  -> END_AUCTION
  -> ORDER_EXPIRE
  -> ANOMALY_SCAN

PostgreSQL
  -> source of truth

Redis
  -> projection/cache/recovery helpers

MinIO
  -> item images
```

## Module Boundaries

| Module | Owns | Does Not Own |
|---|---|---|
| gateway | auth, ACL, schema, idempotency probe, rate limit | business mutation |
| auction | rules, bid, state transitions, auction events | WS transport |
| order | order/payment/deposit lifecycle | bid validation |
| realtime | WS auth, room hub, recovery, backpressure | deciding winner |
| relay | outbox delivery, Redis projection, WS publish | changing auction truth |
| scheduler | durable jobs and retries | direct client interaction |
| observability | anomaly scanners, diagnostics, metrics | static fake data |
| frontend | rendering, input states, recovery UX | final auction decisions |

## Trust Boundaries

Never trust client-provided:

- `user_id`
- `room_id` membership
- `auction_id` ownership
- timestamps
- `current_price`
- `winner`
- order/payment status
- `client_seen_seq` as authority

Client-provided values used only as hints:

- `client_bid_id`
- `client_seen_seq`
- desired bid amount
- chat body

## Data Flow: Bid

```text
H5 POST /api/auctions/{id}/bids
  -> gateway auth/schema/idempotency/rate-limit
  -> auction tx locks auction row
  -> validate rules/state/time
  -> write bid + event + outbox + idempotency
  -> commit
  -> HTTP response returns committed truth
  -> relay publishes Redis + WS
  -> all clients converge by event or snapshot
```

## Data Flow: Reconnect

```text
H5 gets ws ticket
  -> connects with last_seq
  -> server checks Redis history
  -> if complete: replay deltas
  -> else: snapshot cache/singleflight/semaphore
  -> client applies snapshot and resumes
```

## Data Flow: End Auction

```text
END_AUCTION job fires
  -> locks auction row
  -> if now < end_at: reschedule to current end_at
  -> if terminal: noop
  -> if no bids: ENDED
  -> if winner: SOLD + order
  -> event + outbox + commit
```

## Deployment Assumption

P0 is a single backend process. This is deliberate:

- smaller failure surface;
- easier demo;
- correct write path still survives restart via DB/outbox/jobs;
- horizontal scaling is not claimed.

Known multi-instance changes:

- WS fanout must be tested explicitly before any horizontal realtime claim.
- outbox relay must remain single-owner per shard.
- local in-memory limits need Redis/shared counters.
