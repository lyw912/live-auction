# 04 · Data And Storage

## PostgreSQL Principles

PostgreSQL is the only authority for:

- auction state;
- current price/winner;
- bid accept/reject result after execution;
- event sequence;
- order/payment/deposit state;
- idempotency result;
- scheduler job truth.

Redis and WebSocket are projections.

## Schema

### users

```sql
users (
  id text primary key,
  role text not null check (role in ('host','user')),
  display_name text not null,
  city text,
  created_at timestamptz not null default now()
)
```

### rooms

```sql
rooms (
  id text primary key,
  host_id text not null references users(id),
  status text not null check (status in ('OPEN','CLOSED')),
  created_at timestamptz not null default now()
)
```

### items

```sql
items (
  id text primary key,
  title text not null,
  image_url text,
  description text,
  status text not null check (status in ('DRAFT','READY','ARCHIVED')),
  created_at timestamptz not null default now()
)
```

### auctions

```sql
auctions (
  id text primary key,
  room_id text not null references rooms(id),
  item_id text not null references items(id),
  status text not null,
  is_narrating boolean not null default false,
  narrating_started_at timestamptz,
  current_price_cents bigint not null default 0,
  current_winner_id text references users(id),
  start_price_cents bigint not null,
  increment_cents bigint not null,
  cap_price_cents bigint,
  start_at timestamptz,
  end_at timestamptz,
  version bigint not null default 0,
  seq bigint not null default 0,
  accepted_bid_count bigint not null default 0,
  extend_count int not null default 0,
  rule_version int not null default 1,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
)
```

Indexes:

```sql
CREATE UNIQUE INDEX ux_auctions_room_active
  ON auctions(room_id) WHERE status = 'ACTIVE';

CREATE UNIQUE INDEX ux_auctions_room_narrating
  ON auctions(room_id) WHERE is_narrating = true;

CREATE INDEX ix_auctions_room_status ON auctions(room_id, status);
```

### auction_rules

```sql
auction_rules (
  auction_id text not null references auctions(id),
  rule_version int not null,
  duration_seconds int not null,
  extend_window_seconds int not null,
  extend_by_seconds int not null,
  max_extend_count int not null,
  fat_finger_threshold_cents bigint,
  deposit_bps smallint,
  deposit_floor_cents bigint,
  deposit_cap_cents bigint,
  frozen_at timestamptz,
  primary key (auction_id, rule_version)
)
```

### bids

```sql
bids (
  id text primary key,
  auction_id text not null references auctions(id),
  user_id text not null references users(id),
  client_bid_id text not null,
  amount_cents bigint not null,
  seq bigint,
  status text not null,
  reject_reason text,
  request_hash text not null,
  response_json jsonb not null,
  trace_id text not null,
  created_at timestamptz not null default now(),
  unique (auction_id, user_id, client_bid_id)
)
```

### auction_events

```sql
auction_events (
  id bigserial primary key,
  auction_id text not null references auctions(id),
  seq bigint not null,
  event_type text not null,
  payload_json jsonb not null,
  server_time_ms bigint not null,
  trace_id text,
  created_at timestamptz not null default now(),
  unique (auction_id, seq)
)
```

### outbox_events and delivery

```sql
outbox_events (
  id bigserial primary key,
  aggregate_type text not null,
  aggregate_id text not null,
  auction_id text,
  seq bigint,
  event_type text not null,
  payload_json jsonb not null,
  created_at timestamptz not null default now()
)
```

```sql
outbox_delivery (
  outbox_id bigint primary key references outbox_events(id),
  status text not null,
  attempts int not null default 0,
  max_attempts int not null default 5,
  locked_by text,
  locked_until timestamptz,
  next_attempt_at timestamptz not null default now(),
  published_at timestamptz,
  last_error text
)
```

Indexes:

```sql
CREATE UNIQUE INDEX ux_outbox_event_seq
  ON outbox_events(aggregate_type, aggregate_id, event_type, seq)
  WHERE seq IS NOT NULL;

CREATE INDEX ix_outbox_delivery_claim
  ON outbox_delivery(status, next_attempt_at, outbox_id);

CREATE INDEX ix_outbox_events_auction_seq
  ON outbox_events(auction_id, seq);
```

### idempotency_records

```sql
idempotency_records (
  scope_type text not null check (scope_type in ('bid','payment')),
  scope_id text not null,
  user_id text not null,
  idempotency_key text not null,
  request_hash text not null,
  status text not null,
  attempts int not null default 0,
  max_attempts int not null default 5,
  http_status int,
  result_code text,
  response_json jsonb,
  locked_until timestamptz,
  first_attempt_at timestamptz not null default now(),
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  primary key (scope_type, scope_id, user_id, idempotency_key)
)
```

### orders

```sql
orders (
  id text primary key,
  auction_id text not null unique references auctions(id),
  winner_id text not null references users(id),
  amount_cents bigint not null,
  status text not null,
  deposit_cents bigint not null default 0,
  deposit_status text not null,
  expire_at timestamptz not null,
  paid_at timestamptz,
  created_at timestamptz not null default now()
)
```

### scheduler_jobs

```sql
scheduler_jobs (
  id text primary key,
  job_type text not null,
  target_type text not null,
  target_id text not null,
  idempotency_key text not null,
  run_at timestamptz not null,
  status text not null,
  attempts int not null default 0,
  max_attempts int not null default 5,
  locked_by text,
  locked_until timestamptz,
  next_attempt_at timestamptz not null default now(),
  last_error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(job_type, target_type, target_id, idempotency_key)
)
```

### social and diagnostics

```sql
chat_messages (
  id bigserial primary key,
  room_id text not null references rooms(id),
  user_id text not null references users(id),
  client_msg_id text not null,
  body text not null,
  created_at timestamptz not null default now(),
  unique(room_id, user_id, client_msg_id)
)
```

```sql
user_activity_events (
  id bigserial primary key,
  room_id text,
  auction_id text,
  user_id text,
  event_type text not null,
  source text not null,
  trace_id text,
  payload_json jsonb not null default '{}',
  created_at timestamptz not null default now()
)
```

```sql
system_anomaly_events (
  id bigserial primary key,
  severity text not null,
  type text not null,
  auction_id text,
  message text not null,
  payload_json jsonb not null default '{}',
  created_at timestamptz not null default now(),
  resolved_at timestamptz
)
```

## Redis Keys

| Key | Type | TTL | Purpose |
|---|---|---|---|
| `auction:{id}:snapshot` | JSON/string | active + 30m | hot snapshot |
| `auction:{id}:events` | stream/list | active + 30m | ordered recovery history |
| `ws_ticket:{ticket}` | string | <=60s | one-time WS ticket |
| `rate:bid:user:{auction}:{user}` | counter/bucket | short | user bid limit |
| `rate:bid:ip:{auction}:{ip}` | counter/bucket | short | IP bid limit |
| `rate:bid:auction:{auction}` | counter/bucket | short | auction global limit |
| `room:{id}:presence` | set/hash | short | approximate presence |
| `user_joined:{room}:{user}` | string | 30s | join dedupe |
| `snapshot:singleflight:{room}` | lock | short | optional cross-process guard |

## MinIO

Use only presigned PUT:

```text
POST /api/items/upload-url
client PUT upload_url
POST /api/items
```

Do not implement parallel multipart upload endpoint unless the presigned path is removed.

## Archival

Outbox:

- keep active delivery rows until PUBLISHED/DEAD.
- archive immutable `outbox_events` only after retention boundary and after clients no longer dedupe by outbox id.
- client dedupe window equals Redis history window, not 24h archive window.

Events:

- keep auction_events at least through demo and evidence capture.
- replay design can be P2.

## Migration Rules

- Every enum-like text field must have application constants and tests.
- Every partial unique index must have named constraint/index and error mapping.
- Every money amount uses integer cents.
- No float arithmetic for money.
- No DB trigger with hidden domain side effects in P0.
