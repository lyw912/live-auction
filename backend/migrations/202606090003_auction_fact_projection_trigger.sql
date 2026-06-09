-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_auction_bid_projection_consistency()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  checked_auction_id text;
  projection record;
  accepted_count bigint;
  top_user_id text;
  top_amount_cents bigint;
BEGIN
  IF TG_TABLE_NAME = 'auctions' THEN
    checked_auction_id := COALESCE(NEW.id, OLD.id);
  ELSE
    checked_auction_id := COALESCE(NEW.auction_id, OLD.auction_id);
  END IF;

  SELECT id, current_price_cents, current_winner_id, accepted_bid_count
  INTO projection
  FROM auctions
  WHERE id = checked_auction_id;

  IF NOT FOUND THEN
    RETURN NULL;
  END IF;

  SELECT count(*)
  INTO accepted_count
  FROM bids
  WHERE auction_id = checked_auction_id
    AND status = 'ACCEPTED';

  SELECT user_id, amount_cents
  INTO top_user_id, top_amount_cents
  FROM bids
  WHERE auction_id = checked_auction_id
    AND status = 'ACCEPTED'
  ORDER BY amount_cents DESC, created_at ASC, user_id ASC
  LIMIT 1;

  IF accepted_count = 0 THEN
    IF projection.accepted_bid_count <> 0 OR projection.current_winner_id IS NOT NULL THEN
      RAISE EXCEPTION 'auction % projection inconsistent: no accepted bid facts but accepted_bid_count=%, current_winner_id=%',
        checked_auction_id, projection.accepted_bid_count, projection.current_winner_id
        USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
  END IF;

  IF projection.accepted_bid_count <> accepted_count
    OR projection.current_winner_id IS DISTINCT FROM top_user_id
    OR projection.current_price_cents IS DISTINCT FROM top_amount_cents THEN
    RAISE EXCEPTION 'auction % projection inconsistent: projection count/winner/price=(%,%,%), fact count/winner/price=(%,%,%)',
      checked_auction_id,
      projection.accepted_bid_count,
      projection.current_winner_id,
      projection.current_price_cents,
      accepted_count,
      top_user_id,
      top_amount_cents
      USING ERRCODE = '23514';
  END IF;

  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER auctions_projection_matches_bid_facts
AFTER INSERT OR UPDATE OF current_price_cents, current_winner_id, accepted_bid_count
ON auctions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_auction_bid_projection_consistency();

CREATE CONSTRAINT TRIGGER bids_projection_matches_auction
AFTER INSERT OR UPDATE OF auction_id, user_id, amount_cents, status, created_at OR DELETE
ON bids
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_auction_bid_projection_consistency();

-- +goose Down
DROP TRIGGER IF EXISTS bids_projection_matches_auction ON bids;
DROP TRIGGER IF EXISTS auctions_projection_matches_bid_facts ON auctions;
DROP FUNCTION IF EXISTS enforce_auction_bid_projection_consistency();
