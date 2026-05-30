-- +goose Up
CREATE TABLE auction_engine_checkpoints (
  auction_id text PRIMARY KEY REFERENCES auctions(id) ON DELETE CASCADE,
  engine_epoch bigint NOT NULL,
  engine_seq bigint NOT NULL,
  decision_topic text NOT NULL,
  decision_partition int NOT NULL,
  next_decision_offset bigint NOT NULL,
  state_hash text NOT NULL,
  snapshot_json jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_auction_engine_checkpoints_seq
  ON auction_engine_checkpoints(engine_epoch, engine_seq, updated_at);

-- +goose Down
DROP INDEX IF EXISTS ix_auction_engine_checkpoints_seq;
DROP TABLE IF EXISTS auction_engine_checkpoints;
