-- +goose Up
CREATE TABLE liveops_lucky_draw_entries (
  campaign_id text NOT NULL REFERENCES liveops_campaigns(id) ON DELETE CASCADE,
  room_id text NOT NULL REFERENCES rooms(id),
  user_id text NOT NULL REFERENCES users(id),
  entry_status text NOT NULL CHECK (entry_status IN ('ENTERED','OPENED')),
  reward_key text,
  reward_label text,
  entered_at timestamptz NOT NULL DEFAULT now(),
  opened_at timestamptz,
  PRIMARY KEY (campaign_id, user_id)
);

CREATE INDEX ix_liveops_lucky_draw_room_status
  ON liveops_lucky_draw_entries(room_id, entry_status, entered_at DESC);

-- +goose Down
DROP TABLE IF EXISTS liveops_lucky_draw_entries;
