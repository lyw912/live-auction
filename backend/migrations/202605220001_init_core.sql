-- +goose Up
CREATE TABLE users (
  id text PRIMARY KEY,
  role text NOT NULL CHECK (role IN ('host','user')),
  display_name text NOT NULL,
  city text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rooms (
  id text PRIMARY KEY,
  host_id text NOT NULL REFERENCES users(id),
  status text NOT NULL CHECK (status IN ('OPEN','CLOSED')),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE items (
  id text PRIMARY KEY,
  title text NOT NULL,
  image_url text,
  description text,
  status text NOT NULL CHECK (status IN ('DRAFT','READY','ARCHIVED')),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auctions (
  id text PRIMARY KEY,
  room_id text NOT NULL REFERENCES rooms(id),
  item_id text NOT NULL REFERENCES items(id),
  status text NOT NULL CHECK (status IN ('DRAFT','SCHEDULED','ACTIVE','SOLD','ENDED','CANCELLED')),
  is_narrating boolean NOT NULL DEFAULT false,
  narrating_started_at timestamptz,
  current_price_cents bigint NOT NULL DEFAULT 0 CHECK (current_price_cents >= 0),
  current_winner_id text REFERENCES users(id),
  start_price_cents bigint NOT NULL CHECK (start_price_cents >= 0),
  increment_cents bigint NOT NULL CHECK (increment_cents > 0),
  cap_price_cents bigint,
  start_at timestamptz,
  end_at timestamptz,
  version bigint NOT NULL DEFAULT 0,
  seq bigint NOT NULL DEFAULT 0,
  accepted_bid_count bigint NOT NULL DEFAULT 0,
  extend_count int NOT NULL DEFAULT 0,
  rule_version int NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ck_auctions_cap_reachable CHECK (
    cap_price_cents IS NULL OR (
      cap_price_cents >= start_price_cents + increment_cents
      AND (cap_price_cents - start_price_cents) % increment_cents = 0
    )
  )
);

CREATE UNIQUE INDEX ux_auctions_room_active
  ON auctions(room_id) WHERE status = 'ACTIVE';

CREATE UNIQUE INDEX ux_auctions_room_narrating
  ON auctions(room_id) WHERE is_narrating = true;

CREATE INDEX ix_auctions_room_status ON auctions(room_id, status);

CREATE TABLE auction_rules (
  auction_id text NOT NULL REFERENCES auctions(id),
  rule_version int NOT NULL,
  duration_seconds int NOT NULL CHECK (duration_seconds BETWEEN 30 AND 86400),
  extend_window_seconds int NOT NULL CHECK (extend_window_seconds BETWEEN 10 AND 30),
  extend_by_seconds int NOT NULL CHECK (extend_by_seconds BETWEEN 10 AND 30),
  max_extend_count int NOT NULL CHECK (max_extend_count BETWEEN 1 AND 10),
  fat_finger_threshold_cents bigint,
  deposit_bps smallint,
  deposit_floor_cents bigint,
  deposit_cap_cents bigint,
  frozen_at timestamptz,
  PRIMARY KEY (auction_id, rule_version),
  CONSTRAINT ck_auction_rules_fat_finger CHECK (
    fat_finger_threshold_cents IS NULL OR fat_finger_threshold_cents > 0
  )
);

CREATE TABLE bids (
  id text PRIMARY KEY,
  auction_id text NOT NULL REFERENCES auctions(id),
  user_id text NOT NULL REFERENCES users(id),
  client_bid_id text NOT NULL,
  amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
  seq bigint,
  status text NOT NULL CHECK (status IN ('ACCEPTED','REJECTED')),
  reject_reason text,
  request_hash text NOT NULL,
  response_json jsonb NOT NULL,
  trace_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (auction_id, user_id, client_bid_id)
);

CREATE TABLE auction_events (
  id bigserial PRIMARY KEY,
  auction_id text NOT NULL REFERENCES auctions(id),
  seq bigint NOT NULL,
  event_type text NOT NULL,
  payload_json jsonb NOT NULL,
  server_time_ms bigint NOT NULL,
  trace_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (auction_id, seq)
);

CREATE TABLE outbox_events (
  id bigserial PRIMARY KEY,
  aggregate_type text NOT NULL,
  aggregate_id text NOT NULL,
  auction_id text,
  seq bigint,
  event_type text NOT NULL,
  payload_json jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE outbox_delivery (
  outbox_id bigint PRIMARY KEY REFERENCES outbox_events(id),
  status text NOT NULL CHECK (status IN ('PENDING','PUBLISHING','PUBLISHED','FAILED','DEAD')),
  attempts int NOT NULL DEFAULT 0,
  max_attempts int NOT NULL DEFAULT 5,
  locked_by text,
  locked_until timestamptz,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz,
  last_error text
);

CREATE UNIQUE INDEX ux_outbox_event_seq
  ON outbox_events(aggregate_type, aggregate_id, event_type, seq)
  WHERE seq IS NOT NULL;

CREATE INDEX ix_outbox_delivery_claim
  ON outbox_delivery(status, next_attempt_at, outbox_id);

CREATE INDEX ix_outbox_events_auction_seq
  ON outbox_events(auction_id, seq);

CREATE TABLE idempotency_records (
  scope_type text NOT NULL CHECK (scope_type IN ('bid','payment')),
  scope_id text NOT NULL,
  user_id text NOT NULL,
  idempotency_key text NOT NULL,
  request_hash text NOT NULL,
  status text NOT NULL CHECK (status IN ('PROCESSING','COMPLETED','FAILED','UNKNOWN')),
  attempts int NOT NULL DEFAULT 0,
  max_attempts int NOT NULL DEFAULT 5,
  http_status int,
  result_code text,
  response_json jsonb,
  locked_until timestamptz,
  first_attempt_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (scope_type, scope_id, user_id, idempotency_key)
);

CREATE TABLE orders (
  id text PRIMARY KEY,
  auction_id text NOT NULL UNIQUE REFERENCES auctions(id),
  winner_id text NOT NULL REFERENCES users(id),
  amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
  status text NOT NULL CHECK (status IN ('ORDER_PENDING','PAID','ORDER_EXPIRED')),
  deposit_cents bigint NOT NULL DEFAULT 0 CHECK (deposit_cents >= 0),
  deposit_status text NOT NULL CHECK (deposit_status IN ('HELD','REFUNDED','FORFEITED')),
  expire_at timestamptz NOT NULL,
  paid_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE scheduler_jobs (
  id text PRIMARY KEY,
  job_type text NOT NULL,
  target_type text NOT NULL,
  target_id text NOT NULL,
  idempotency_key text NOT NULL,
  run_at timestamptz NOT NULL,
  status text NOT NULL CHECK (status IN ('PENDING','RUNNING','SUCCEEDED','FAILED','DEAD')),
  attempts int NOT NULL DEFAULT 0,
  max_attempts int NOT NULL DEFAULT 5,
  locked_by text,
  locked_until timestamptz,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(job_type, target_type, target_id, idempotency_key)
);

CREATE TABLE chat_messages (
  id bigserial PRIMARY KEY,
  room_id text NOT NULL REFERENCES rooms(id),
  user_id text NOT NULL REFERENCES users(id),
  client_msg_id text NOT NULL,
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(room_id, user_id, client_msg_id)
);

CREATE TABLE user_activity_events (
  id bigserial PRIMARY KEY,
  room_id text,
  auction_id text,
  user_id text,
  event_type text NOT NULL,
  source text NOT NULL,
  trace_id text,
  payload_json jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE system_anomaly_events (
  id bigserial PRIMARY KEY,
  severity text NOT NULL CHECK (severity IN ('LOW','MED','HIGH','CRITICAL')),
  type text NOT NULL,
  auction_id text,
  message text NOT NULL,
  payload_json jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz
);

INSERT INTO users (id, role, display_name, city)
VALUES
  ('host_1', 'host', 'Demo Host', 'Hangzhou'),
  ('user_1', 'user', 'Demo User', 'Shanghai'),
  ('user_2', 'user', 'Prior Leader', 'Beijing'),
  ('user_3', 'user', 'Smoke Bidder', 'Shenzhen')
ON CONFLICT (id) DO NOTHING;

INSERT INTO rooms (id, host_id, status)
VALUES
  ('room_1', 'host_1', 'OPEN'),
  ('room_main', 'host_1', 'OPEN'),
  ('room_side', 'host_1', 'OPEN')
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS system_anomaly_events;
DROP TABLE IF EXISTS user_activity_events;
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS scheduler_jobs;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS idempotency_records;
DROP TABLE IF EXISTS outbox_delivery;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS auction_events;
DROP TABLE IF EXISTS bids;
DROP TABLE IF EXISTS auction_rules;
DROP TABLE IF EXISTS auctions;
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS rooms;
DROP TABLE IF EXISTS users;
