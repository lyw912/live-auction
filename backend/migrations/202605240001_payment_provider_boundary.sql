-- +goose Up
ALTER TABLE orders
  DROP CONSTRAINT orders_status_check,
  ADD CONSTRAINT orders_status_check CHECK (status IN ('ORDER_PENDING','PAYMENT_INITIATED','PAYMENT_SUCCEEDED','PAID','ORDER_EXPIRED'));

ALTER TABLE orders
  ADD COLUMN provider_payment_id text,
  ADD COLUMN payment_initiated_at timestamptz,
  ADD COLUMN payment_succeeded_at timestamptz;

CREATE UNIQUE INDEX ux_orders_provider_payment_id
  ON orders(provider_payment_id)
  WHERE provider_payment_id IS NOT NULL;

CREATE TABLE payment_events (
  id bigserial PRIMARY KEY,
  provider text NOT NULL,
  provider_event_id text NOT NULL,
  provider_payment_id text NOT NULL,
  order_id text NOT NULL REFERENCES orders(id),
  event_type text NOT NULL CHECK (event_type IN ('payment_initiated','payment_succeeded','payment_failed')),
  signature_valid boolean NOT NULL,
  processed_at timestamptz,
  payload_json jsonb NOT NULL DEFAULT '{}',
  trace_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(provider, provider_event_id)
);

CREATE INDEX ix_payment_events_order ON payment_events(order_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS payment_events;

DROP INDEX IF EXISTS ux_orders_provider_payment_id;

ALTER TABLE orders
  DROP COLUMN IF EXISTS payment_succeeded_at,
  DROP COLUMN IF EXISTS payment_initiated_at,
  DROP COLUMN IF EXISTS provider_payment_id;

ALTER TABLE orders
  DROP CONSTRAINT orders_status_check,
  ADD CONSTRAINT orders_status_check CHECK (status IN ('ORDER_PENDING','PAID','ORDER_EXPIRED'));
