-- +goose Up
-- +goose NO TRANSACTION
-- P1a: HOT-update enablement on high-churn settlement tables.
-- fillfactor < 100 leaves free space on each page so UPDATE creates a HOT
-- (Heap Only Tuple) chain instead of a new index entry, halving WAL volume
-- and eliminating index bloat on these rows.
--
-- auctions: updated on every accepted bid (price/winner/seq) and every batch
--   (engine_seq advance). Very hot on single-auction stair tests.
-- redis_engine_settlements: now inserted SETTLED directly (P1b), but still
--   updated by the per-message fallback retry path. fillfactor 75 keeps
--   headroom for those retries.
-- bids: accepted rows are inserted once; rejected rows are inserted once too
--   (no further updates after the batch path). fillfactor 85 is mild — the
--   table already has ON CONFLICT UPDATE clauses for replay, so some headroom
--   is useful without over-allocating.
--
-- VACUUM ANALYZE reclaims dead tuples from prior double-writes immediately.

ALTER TABLE auctions SET (fillfactor = 75);
ALTER TABLE redis_engine_settlements SET (fillfactor = 75);
ALTER TABLE bids SET (fillfactor = 85);

VACUUM (ANALYZE) auctions;
VACUUM (ANALYZE) redis_engine_settlements;
VACUUM (ANALYZE) bids;

-- +goose Down
ALTER TABLE auctions RESET (fillfactor);
ALTER TABLE redis_engine_settlements RESET (fillfactor);
ALTER TABLE bids RESET (fillfactor);
