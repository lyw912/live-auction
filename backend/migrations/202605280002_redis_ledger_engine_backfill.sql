-- +goose Up
UPDATE auctions
SET engine_seq = seq
WHERE engine_seq < seq;

UPDATE auction_events
SET engine_epoch = 1,
    engine_seq = seq
WHERE engine_seq IS NULL
  AND seq > 0;

-- +goose Down
UPDATE auction_events
SET engine_epoch = NULL,
    engine_seq = NULL
WHERE engine_epoch = 1
  AND engine_seq = seq;
