-- +goose Up

-- Track the relay cursor per auction so we can reconstruct which stream entries
-- have been durably produced to Kafka. This replaces the old pending-append-lock model.
-- The cursor is stored in Redis (bid:{auction}:engine:relay-cursor) and reset on rebuild;
-- this table provides a durable checkpoint in case Redis is flushed/rebuilt.
-- NOTE: the Redis cursor key is the primary source; this table is for audit/recovery only.
ALTER TABLE auction_engine_checkpoints
  ADD COLUMN relay_cursor_stream_id text;

-- Record that all settlements are now idempotent on (auction_id, engine_seq) rather than
-- (auction_id, stream_id). The unique index already existed; this comment documents intent.
COMMENT ON INDEX ux_bids_engine_seq IS
  'v3: settlement is idempotent by (auction_id, engine_seq). ON CONFLICT DO NOTHING prevents duplicates from relay re-delivery.';

-- +goose Down
ALTER TABLE auction_engine_checkpoints
  DROP COLUMN IF EXISTS relay_cursor_stream_id;
