-- +goose Up
ALTER TABLE auctions
  ADD CONSTRAINT ck_auctions_bid_projection_consistent
  CHECK (
    (accepted_bid_count = 0 AND current_winner_id IS NULL)
    OR
    (accepted_bid_count > 0 AND current_winner_id IS NOT NULL)
  )
  NOT VALID;

-- +goose Down
ALTER TABLE auctions
  DROP CONSTRAINT IF EXISTS ck_auctions_bid_projection_consistent;
