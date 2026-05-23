-- +goose Up
CREATE TABLE outbox_relay_shard_leases (
  shard_id int PRIMARY KEY,
  owner_id text NOT NULL,
  lease_until timestamptz NOT NULL,
  acquired_at timestamptz NOT NULL DEFAULT now(),
  renewed_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE outbox_delivery
  ADD COLUMN shard_id int;

UPDATE outbox_delivery
SET shard_id = mod(abs(hashtext(COALESCE(auction_id, outbox_id::text))), 16)
WHERE shard_id IS NULL;

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

  NEW.shard_id = mod(abs(hashtext(COALESCE(NEW.auction_id, NEW.outbox_id::text))), 16);
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE outbox_delivery
  ALTER COLUMN shard_id SET NOT NULL;

CREATE INDEX ix_outbox_delivery_shard_ready_created
  ON outbox_delivery(shard_id, event_created_at, outbox_id, auction_id, auction_seq)
  WHERE status IN ('PENDING','FAILED','PUBLISHING');

CREATE INDEX ix_outbox_delivery_shard_unfinished_auction_head
  ON outbox_delivery(shard_id, auction_id, auction_seq, outbox_id)
  WHERE status IN ('PENDING','FAILED','PUBLISHING') AND auction_id IS NOT NULL AND auction_seq IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS ix_outbox_delivery_shard_unfinished_auction_head;
DROP INDEX IF EXISTS ix_outbox_delivery_shard_ready_created;

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

ALTER TABLE outbox_delivery
  DROP COLUMN IF EXISTS shard_id;

DROP TABLE IF EXISTS outbox_relay_shard_leases;
