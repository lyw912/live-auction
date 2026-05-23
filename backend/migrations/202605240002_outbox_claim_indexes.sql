-- +goose Up
ALTER TABLE outbox_delivery
  ADD COLUMN auction_id text,
  ADD COLUMN auction_seq bigint,
  ADD COLUMN event_created_at timestamptz;

UPDATE outbox_delivery d
SET auction_id = e.auction_id,
    auction_seq = e.seq,
    event_created_at = e.created_at
FROM outbox_events e
WHERE e.id = d.outbox_id;

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
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_sync_outbox_delivery_event_fields
BEFORE INSERT ON outbox_delivery
FOR EACH ROW
EXECUTE FUNCTION sync_outbox_delivery_event_fields();

CREATE INDEX IF NOT EXISTS ix_outbox_delivery_ready
  ON outbox_delivery(status, next_attempt_at, locked_until, outbox_id)
  WHERE status IN ('PENDING','FAILED','PUBLISHING');

CREATE INDEX IF NOT EXISTS ix_outbox_delivery_unfinished_auction_head
  ON outbox_delivery(auction_id, auction_seq, outbox_id)
  WHERE status NOT IN ('PUBLISHED','DEAD') AND auction_id IS NOT NULL AND auction_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_outbox_delivery_unfinished_created
  ON outbox_delivery(event_created_at, outbox_id)
  WHERE status NOT IN ('PUBLISHED','DEAD');

CREATE INDEX IF NOT EXISTS ix_outbox_delivery_ready_created
  ON outbox_delivery(event_created_at, outbox_id, auction_id, auction_seq)
  WHERE status IN ('PENDING','FAILED','PUBLISHING');

-- +goose Down
DROP INDEX IF EXISTS ix_outbox_delivery_ready_created;
DROP INDEX IF EXISTS ix_outbox_delivery_unfinished_created;
DROP INDEX IF EXISTS ix_outbox_delivery_unfinished_auction_head;
DROP INDEX IF EXISTS ix_outbox_delivery_ready;
DROP TRIGGER IF EXISTS trg_sync_outbox_delivery_event_fields ON outbox_delivery;
DROP FUNCTION IF EXISTS sync_outbox_delivery_event_fields();
ALTER TABLE outbox_delivery
  DROP COLUMN IF EXISTS event_created_at,
  DROP COLUMN IF EXISTS auction_seq,
  DROP COLUMN IF EXISTS auction_id;
