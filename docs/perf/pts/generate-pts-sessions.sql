-- Generate a PTS/JMeter session-token CSV for cloud pressure tests.
--
-- Run on the same PostgreSQL database used by the deployed backend, after the
-- load seed users and room memberships exist. This creates real auth_sessions;
-- the generated bearer tokens work with ALLOW_MOCK_AUTH=false.
--
-- Example:
--   psql "$DATABASE_URL" -v session_count=4096 -f docs/perf/pts/generate-pts-sessions.sql

\set session_count :session_count

CREATE EXTENSION IF NOT EXISTS pgcrypto;

WITH candidates AS (
  SELECT id AS user_id, role
  FROM users
  WHERE role = 'user'
    AND (
      id LIKE 'k6_bidder_%'
      OR id LIKE 'k6_ws_%'
      OR id LIKE 'k6_user_%'
    )
  ORDER BY id
  LIMIT :session_count
),
tokens AS (
  SELECT
    user_id,
    role,
    'pts_' || encode(gen_random_bytes(32), 'hex') AS token
  FROM candidates
),
inserted AS (
  INSERT INTO auth_sessions (
    id,
    user_id,
    role,
    token_hash,
    expires_at,
    created_ip,
    user_agent
  )
  SELECT
    'sess_pts_' || replace(gen_random_uuid()::text, '-', ''),
    user_id,
    role,
    encode(digest(convert_to(token, 'UTF8'), 'sha256'), 'hex'),
    now() + interval '12 hours',
    'pts',
    'pts-jmeter'
  FROM tokens
  RETURNING user_id
)
SELECT tokens.user_id, tokens.token, tokens.role
FROM tokens
JOIN inserted USING (user_id)
ORDER BY tokens.user_id
