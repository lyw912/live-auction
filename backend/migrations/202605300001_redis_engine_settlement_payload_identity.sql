-- +goose Up
ALTER TABLE redis_engine_settlements
  ADD COLUMN payload_sha256 text NOT NULL DEFAULT 'legacy-unhashed',
  ADD COLUMN conflict_stream_id text,
  ADD COLUMN conflict_payload_sha256 text;

CREATE INDEX ix_redis_engine_settlements_payload_identity
  ON redis_engine_settlements(auction_id, engine_epoch, engine_seq, payload_sha256);

-- +goose Down
DROP INDEX IF EXISTS ix_redis_engine_settlements_payload_identity;

ALTER TABLE redis_engine_settlements
  DROP COLUMN IF EXISTS conflict_payload_sha256,
  DROP COLUMN IF EXISTS conflict_stream_id,
  DROP COLUMN IF EXISTS payload_sha256;
