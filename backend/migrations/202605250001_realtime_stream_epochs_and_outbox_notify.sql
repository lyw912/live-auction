-- +goose Up
CREATE TABLE realtime_stream_epochs (
  auction_id text PRIMARY KEY REFERENCES auctions(id) ON DELETE CASCADE,
  value text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_realtime_stream_epochs_expires
  ON realtime_stream_epochs(expires_at);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_outbox_delivery_ready()
RETURNS trigger AS $$
BEGIN
  IF NEW.status IN ('PENDING','FAILED')
     AND NEW.next_attempt_at <= now()
     AND (TG_OP = 'INSERT'
       OR OLD.status IS DISTINCT FROM NEW.status
       OR OLD.next_attempt_at IS DISTINCT FROM NEW.next_attempt_at) THEN
    PERFORM pg_notify('outbox_delivery_ready', COALESCE(NEW.shard_id::text, ''));
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS tr_outbox_delivery_ready_notify ON outbox_delivery;
CREATE TRIGGER tr_outbox_delivery_ready_notify
AFTER INSERT OR UPDATE OF status, next_attempt_at ON outbox_delivery
FOR EACH ROW
EXECUTE FUNCTION notify_outbox_delivery_ready();

-- +goose Down
DROP TRIGGER IF EXISTS tr_outbox_delivery_ready_notify ON outbox_delivery;
DROP FUNCTION IF EXISTS notify_outbox_delivery_ready();
DROP TABLE IF EXISTS realtime_stream_epochs;
