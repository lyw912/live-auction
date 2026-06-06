-- +goose Up
CREATE TABLE auction_highlight_assets (
  id text PRIMARY KEY,
  auction_id text NOT NULL REFERENCES auctions(id),
  room_id text NOT NULL REFERENCES rooms(id),
  job_id text NOT NULL REFERENCES ai_generation_jobs(id),
  status text NOT NULL CHECK (status IN ('PENDING','RENDERED','FAILED')),
  media_type text NOT NULL CHECK (media_type IN ('text/html','video/webm','video/mp4')),
  title text NOT NULL,
  asset_url text NOT NULL,
  render_profile text NOT NULL,
  duration_ms int NOT NULL CHECK (duration_ms > 0),
  facts_json jsonb NOT NULL DEFAULT '{}',
  risk_json jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_auction_highlight_assets_auction_created
  ON auction_highlight_assets(auction_id, created_at DESC);

CREATE INDEX ix_auction_highlight_assets_room_created
  ON auction_highlight_assets(room_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS auction_highlight_assets;
