-- +goose Up
CREATE TABLE liveops_team_choices (
  campaign_id text NOT NULL REFERENCES liveops_campaigns(id) ON DELETE CASCADE,
  room_id text NOT NULL REFERENCES rooms(id),
  user_id text NOT NULL REFERENCES users(id),
  team_key text NOT NULL CHECK (team_key IN ('craft','story')),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (campaign_id, user_id)
);

CREATE INDEX ix_liveops_team_choices_room_team
  ON liveops_team_choices(room_id, team_key, updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS liveops_team_choices;
