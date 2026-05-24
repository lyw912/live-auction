-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE outbox_events
  ADD COLUMN event_schema_version int NOT NULL DEFAULT 1,
  ADD COLUMN event_key text,
  ADD COLUMN payload_sha256 text;

UPDATE outbox_events
SET event_key = COALESCE(auction_id, aggregate_id),
    payload_sha256 = encode(digest(convert_to(payload_json::text, 'UTF8'), 'sha256'), 'hex')
WHERE event_key IS NULL OR payload_sha256 IS NULL;

ALTER TABLE outbox_events
  ALTER COLUMN event_key SET NOT NULL,
  ALTER COLUMN payload_sha256 SET NOT NULL;

ALTER TABLE outbox_delivery
  ADD COLUMN last_error_class text,
  ADD COLUMN last_error_retriable boolean,
  ADD COLUMN last_error_at timestamptz,
  ADD COLUMN last_published_watermark jsonb;

CREATE TABLE outbox_relay_watermarks (
  shard_id int PRIMARY KEY,
  owner_id text,
  last_published_outbox_id bigint,
  last_published_auction_id text,
  last_published_seq bigint,
  last_published_at timestamptz,
  oldest_ready_age_ms bigint NOT NULL DEFAULT 0,
  ready_count bigint NOT NULL DEFAULT 0,
  publishing_count bigint NOT NULL DEFAULT 0,
  dead_count bigint NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE snapshot_rebuild_events (
  id bigserial PRIMARY KEY,
  auction_id text NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
  request_id text NOT NULL,
  source text NOT NULL,
  status text NOT NULL CHECK (status IN ('REQUESTED','STARTED','COMPLETED','SATURATED','STALE','FAILED')),
  stale boolean NOT NULL DEFAULT false,
  duration_ms bigint,
  error_class text,
  error_message text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_snapshot_rebuild_events_auction_created
  ON snapshot_rebuild_events(auction_id, created_at DESC);

CREATE TABLE system_control_signals (
  id bigserial PRIMARY KEY,
  signal_type text NOT NULL CHECK (signal_type IN (
    'force_snapshot_rebuild',
    'retry_dead_outbox',
    'pause_relay_shard',
    'resume_relay_shard'
  )),
  target_type text NOT NULL CHECK (target_type IN ('auction','outbox','relay_shard')),
  target_id text NOT NULL,
  requested_by text NOT NULL,
  reason text NOT NULL,
  payload_json jsonb NOT NULL DEFAULT '{}',
  status text NOT NULL CHECK (status IN ('PENDING','PROCESSING','SUCCEEDED','FAILED','REJECTED')) DEFAULT 'PENDING',
  locked_by text,
  locked_until timestamptz,
  processed_at timestamptz,
  result_json jsonb,
  error_message text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_system_control_signals_claim
  ON system_control_signals(status, created_at, id)
  WHERE status IN ('PENDING','PROCESSING');

CREATE INDEX ix_system_control_signals_target
  ON system_control_signals(target_type, target_id, created_at DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_outbox_delivery_event_fields()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  SELECT e.auction_id, e.seq, e.created_at
  INTO NEW.auction_id, NEW.auction_seq, NEW.event_created_at
  FROM outbox_events e
  WHERE e.id = NEW.outbox_id;

  NEW.shard_id = mod(abs(hashtext(COALESCE(NEW.auction_id, NEW.outbox_id::text))), 16);
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_outbox_delivery_event_fields()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  SELECT e.auction_id, e.seq, e.created_at
  INTO NEW.auction_id, NEW.auction_seq, NEW.event_created_at
  FROM outbox_events e
  WHERE e.id = NEW.outbox_id;

  NEW.shard_id = mod(abs(hashtext(COALESCE(NEW.auction_id, NEW.outbox_id::text))), 16);
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP INDEX IF EXISTS ix_system_control_signals_target;
DROP INDEX IF EXISTS ix_system_control_signals_claim;
DROP TABLE IF EXISTS system_control_signals;
DROP INDEX IF EXISTS ix_snapshot_rebuild_events_auction_created;
DROP TABLE IF EXISTS snapshot_rebuild_events;
DROP TABLE IF EXISTS outbox_relay_watermarks;
ALTER TABLE outbox_delivery
  DROP COLUMN IF EXISTS last_published_watermark,
  DROP COLUMN IF EXISTS last_error_at,
  DROP COLUMN IF EXISTS last_error_retriable,
  DROP COLUMN IF EXISTS last_error_class;
ALTER TABLE outbox_events
  DROP COLUMN IF EXISTS payload_sha256,
  DROP COLUMN IF EXISTS event_key,
  DROP COLUMN IF EXISTS event_schema_version;
