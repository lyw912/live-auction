-- +goose Up
CREATE TABLE ai_generation_jobs (
  id text PRIMARY KEY,
  room_id text REFERENCES rooms(id),
  auction_id text REFERENCES auctions(id),
  kind text NOT NULL CHECK (kind IN ('listing_draft','auction_commentary','sentinel_explanation','auction_recap','product_qa')),
  status text NOT NULL CHECK (status IN ('PENDING','SUCCEEDED','FAILED','DISABLED')),
  input_hash text NOT NULL,
  prompt_version text NOT NULL,
  provider text NOT NULL,
  model text NOT NULL,
  input_json jsonb NOT NULL DEFAULT '{}',
  output_json jsonb NOT NULL DEFAULT '{}',
  safety_json jsonb NOT NULL DEFAULT '{}',
  error_message text,
  reviewed_by text REFERENCES users(id),
  applied_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_ai_generation_jobs_room_kind_created
  ON ai_generation_jobs(room_id, kind, created_at DESC);

CREATE INDEX ix_ai_generation_jobs_auction_kind_created
  ON ai_generation_jobs(auction_id, kind, created_at DESC)
  WHERE auction_id IS NOT NULL;

CREATE INDEX ix_ai_generation_jobs_kind_hash
  ON ai_generation_jobs(kind, input_hash);

CREATE TABLE auction_system_messages (
  id bigserial PRIMARY KEY,
  room_id text NOT NULL REFERENCES rooms(id),
  auction_id text REFERENCES auctions(id),
  source text NOT NULL CHECK (source IN ('SYSTEM_TEMPLATE','SYSTEM_AI','HOST')),
  source_seq bigint,
  style text NOT NULL DEFAULT 'steady',
  body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 80),
  facts_json jsonb NOT NULL DEFAULT '{}',
  safety_json jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(auction_id, source, source_seq)
);

CREATE INDEX ix_auction_system_messages_room_created
  ON auction_system_messages(room_id, created_at DESC, id DESC);

CREATE TABLE auction_risk_alerts (
  id bigserial PRIMARY KEY,
  room_id text NOT NULL REFERENCES rooms(id),
  auction_id text NOT NULL REFERENCES auctions(id),
  severity text NOT NULL CHECK (severity IN ('LOW','MED','HIGH','CRITICAL')),
  risk_type text NOT NULL,
  score int NOT NULL CHECK (score BETWEEN 0 AND 100),
  explanation text NOT NULL,
  recommended_action text NOT NULL,
  features_json jsonb NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','ACKED','DISMISSED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_auction_risk_alerts_auction_status
  ON auction_risk_alerts(auction_id, status, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS auction_risk_alerts;
DROP TABLE IF EXISTS auction_system_messages;
DROP TABLE IF EXISTS ai_generation_jobs;
