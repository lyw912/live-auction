-- +goose Up
ALTER TABLE ai_generation_jobs
  ADD COLUMN IF NOT EXISTS worker_id text,
  ADD COLUMN IF NOT EXISTS locked_until timestamptz,
  ADD COLUMN IF NOT EXISTS attempts int NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX ux_ai_auto_commentary_input
  ON ai_generation_jobs(kind, input_hash)
  WHERE kind = 'auction_commentary'
    AND (safety_json->>'auto_requested') = 'true';

CREATE INDEX ix_ai_generation_jobs_commentary_pending
  ON ai_generation_jobs(kind, status, locked_until, created_at)
  WHERE kind = 'auction_commentary'
    AND status = 'PENDING'
    AND (safety_json->>'auto_requested') = 'true';

-- +goose Down
DROP INDEX IF EXISTS ix_ai_generation_jobs_commentary_pending;
DROP INDEX IF EXISTS ux_ai_auto_commentary_input;
ALTER TABLE ai_generation_jobs
  DROP COLUMN IF EXISTS attempts,
  DROP COLUMN IF EXISTS locked_until,
  DROP COLUMN IF EXISTS worker_id;
