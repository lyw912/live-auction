-- +goose Up
ALTER TABLE idempotency_records
  DROP CONSTRAINT idempotency_records_scope_type_check,
  ADD CONSTRAINT idempotency_records_scope_type_check CHECK (scope_type IN ('bid','payment','max_bid_intent'));

ALTER TABLE bids
  ADD COLUMN source text NOT NULL DEFAULT 'MANUAL',
  ADD CONSTRAINT bids_source_check CHECK (source IN ('MANUAL','AUTO_MAX_BID'));

CREATE TABLE max_bid_intents (
  id text PRIMARY KEY,
  auction_id text NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
  user_id text NOT NULL REFERENCES users(id),
  max_amount_cents bigint NOT NULL CHECK (max_amount_cents > 0),
  status text NOT NULL CHECK (status IN ('ACTIVE','CANCELLED','EXHAUSTED','TERMINAL')),
  source text NOT NULL CHECK (source IN ('PRE_BID','MAX_BID')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  cancelled_at timestamptz,
  exhausted_at timestamptz,
  last_applied_seq bigint,
  version bigint NOT NULL DEFAULT 0,
  UNIQUE (auction_id, user_id),
  CONSTRAINT ck_max_bid_intents_cancelled_at CHECK (
    (status = 'CANCELLED' AND cancelled_at IS NOT NULL)
    OR (status <> 'CANCELLED' AND cancelled_at IS NULL)
  ),
  CONSTRAINT ck_max_bid_intents_exhausted_at CHECK (
    (status = 'EXHAUSTED' AND exhausted_at IS NOT NULL)
    OR (status <> 'EXHAUSTED' AND exhausted_at IS NULL)
  )
);

CREATE INDEX ix_max_bid_intents_auction_active
  ON max_bid_intents(auction_id, max_amount_cents DESC, created_at ASC, id ASC)
  WHERE status = 'ACTIVE';

CREATE INDEX ix_max_bid_intents_user_updated
  ON max_bid_intents(user_id, updated_at DESC);

-- +goose Down
DROP INDEX IF EXISTS ix_max_bid_intents_user_updated;
DROP INDEX IF EXISTS ix_max_bid_intents_auction_active;
DROP TABLE IF EXISTS max_bid_intents;

ALTER TABLE bids
  DROP CONSTRAINT IF EXISTS bids_source_check,
  DROP COLUMN IF EXISTS source;

ALTER TABLE idempotency_records
  DROP CONSTRAINT idempotency_records_scope_type_check,
  ADD CONSTRAINT idempotency_records_scope_type_check CHECK (scope_type IN ('bid','payment'));
