-- +goose Up
CREATE TABLE auction_ai_settings (
  auction_id text PRIMARY KEY REFERENCES auctions(id) ON DELETE CASCADE,
  auto_commentary_enabled boolean NOT NULL DEFAULT true,
  updated_by text REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS auction_ai_settings;
