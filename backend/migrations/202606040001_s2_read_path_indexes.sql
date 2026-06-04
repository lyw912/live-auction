-- +goose Up
CREATE INDEX IF NOT EXISTS ix_bids_user_created_desc
  ON bids(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS ix_bids_auction_accepted_leaderboard
  ON bids(auction_id, amount_cents DESC, created_at ASC, user_id)
  WHERE status = 'ACCEPTED';

CREATE INDEX IF NOT EXISTS ix_bids_auction_user_accepted_best
  ON bids(auction_id, user_id, amount_cents DESC, created_at DESC)
  WHERE status = 'ACCEPTED';

CREATE INDEX IF NOT EXISTS ix_bids_auction_accepted_recent
  ON bids(auction_id, created_at DESC)
  WHERE status = 'ACCEPTED';

-- +goose Down
DROP INDEX IF EXISTS ix_bids_auction_accepted_recent;
DROP INDEX IF EXISTS ix_bids_auction_user_accepted_best;
DROP INDEX IF EXISTS ix_bids_auction_accepted_leaderboard;
DROP INDEX IF EXISTS ix_bids_user_created_desc;
