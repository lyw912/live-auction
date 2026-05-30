-- +goose Up
ALTER TABLE auctions
  ADD COLUMN engine_epoch bigint NOT NULL DEFAULT 1,
  ADD COLUMN engine_seq bigint NOT NULL DEFAULT 0,
  ADD COLUMN engine_paused boolean NOT NULL DEFAULT false,
  ADD COLUMN engine_pause_reason text,
  ADD COLUMN engine_paused_at timestamptz;

UPDATE auctions
SET engine_seq = seq
WHERE seq > 0;

ALTER TABLE bids
  ADD COLUMN engine_epoch bigint,
  ADD COLUMN engine_seq bigint,
  ADD COLUMN settlement_status text NOT NULL DEFAULT 'SETTLED'
    CHECK (settlement_status IN ('PENDING','SETTLED','FAILED','SKIPPED'));

ALTER TABLE auction_events
  ADD COLUMN engine_epoch bigint,
  ADD COLUMN engine_seq bigint;

UPDATE auction_events
SET engine_epoch = 1,
    engine_seq = seq
WHERE seq > 0;

CREATE UNIQUE INDEX ux_auction_events_engine_seq
  ON auction_events(auction_id, engine_epoch, engine_seq)
  WHERE engine_seq IS NOT NULL;

CREATE UNIQUE INDEX ux_bids_engine_seq
  ON bids(auction_id, engine_epoch, engine_seq)
  WHERE engine_seq IS NOT NULL AND status = 'ACCEPTED';

CREATE TABLE redis_engine_settlements (
  id bigserial PRIMARY KEY,
  auction_id text NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
  stream_id text NOT NULL,
  engine_epoch bigint NOT NULL,
  engine_seq bigint NOT NULL,
  result text NOT NULL,
  status text NOT NULL CHECK (status IN ('PROCESSING','SETTLED','FAILED','SKIPPED')) DEFAULT 'PROCESSING',
  attempts int NOT NULL DEFAULT 0,
  last_error text,
  payload_json jsonb NOT NULL,
  settled_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (auction_id, stream_id),
  UNIQUE (auction_id, engine_epoch, engine_seq)
);

CREATE INDEX ix_redis_engine_settlements_status
  ON redis_engine_settlements(status, updated_at, id);

CREATE INDEX ix_redis_engine_settlements_auction_seq
  ON redis_engine_settlements(auction_id, engine_epoch, engine_seq);

ALTER TABLE system_control_signals
  DROP CONSTRAINT system_control_signals_signal_type_check,
  ADD CONSTRAINT system_control_signals_signal_type_check CHECK (signal_type IN (
    'force_snapshot_rebuild',
    'retry_dead_outbox',
    'pause_relay_shard',
    'resume_relay_shard',
    'pause_redis_engine',
    'resume_redis_engine',
    'reconcile_redis_engine'
  ));

-- +goose Down
ALTER TABLE system_control_signals
  DROP CONSTRAINT system_control_signals_signal_type_check,
  ADD CONSTRAINT system_control_signals_signal_type_check CHECK (signal_type IN (
    'force_snapshot_rebuild',
    'retry_dead_outbox',
    'pause_relay_shard',
    'resume_relay_shard'
  ));

DROP INDEX IF EXISTS ix_redis_engine_settlements_auction_seq;
DROP INDEX IF EXISTS ix_redis_engine_settlements_status;
DROP TABLE IF EXISTS redis_engine_settlements;

DROP INDEX IF EXISTS ux_bids_engine_seq;
DROP INDEX IF EXISTS ux_auction_events_engine_seq;

ALTER TABLE auction_events
  DROP COLUMN IF EXISTS engine_seq,
  DROP COLUMN IF EXISTS engine_epoch;

ALTER TABLE bids
  DROP COLUMN IF EXISTS settlement_status,
  DROP COLUMN IF EXISTS engine_seq,
  DROP COLUMN IF EXISTS engine_epoch;

ALTER TABLE auctions
  DROP COLUMN IF EXISTS engine_paused_at,
  DROP COLUMN IF EXISTS engine_pause_reason,
  DROP COLUMN IF EXISTS engine_paused,
  DROP COLUMN IF EXISTS engine_seq,
  DROP COLUMN IF EXISTS engine_epoch;
