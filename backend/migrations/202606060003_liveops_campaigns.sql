-- +goose Up
CREATE TABLE liveops_campaigns (
  id text PRIMARY KEY,
  room_id text NOT NULL REFERENCES rooms(id),
  status text NOT NULL CHECK (status IN ('ACTIVE','PAUSED','ENDED')),
  title text NOT NULL,
  description text NOT NULL,
  rules_json jsonb NOT NULL DEFAULT '{}',
  starts_at timestamptz NOT NULL DEFAULT now(),
  ends_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_liveops_campaigns_room_active
  ON liveops_campaigns(room_id)
  WHERE status = 'ACTIVE';

CREATE TABLE liveops_task_progress (
  campaign_id text NOT NULL REFERENCES liveops_campaigns(id) ON DELETE CASCADE,
  room_id text NOT NULL REFERENCES rooms(id),
  user_id text NOT NULL REFERENCES users(id),
  task_key text NOT NULL CHECK (task_key IN ('watch','follow','ask','leaderboard')),
  completed_at timestamptz NOT NULL DEFAULT now(),
  payload_json jsonb NOT NULL DEFAULT '{}',
  PRIMARY KEY (campaign_id, user_id, task_key)
);

CREATE INDEX ix_liveops_task_progress_room_user
  ON liveops_task_progress(room_id, user_id, completed_at DESC);

-- +goose Down
DROP TABLE IF EXISTS liveops_task_progress;
DROP TABLE IF EXISTS liveops_campaigns;
