-- Generate role-specific PTS/JMeter session-token CSV data for L2 protocol tests.
--
-- Usage:
--   psql "$DATABASE_URL" \
--     -v user_prefix=k6_ws_ \
--     -v session_count=2000 \
--     -f docs/perf/pts/generate-l2-pts-sessions.sql

\set user_prefix :user_prefix
\set session_count :session_count

CREATE EXTENSION IF NOT EXISTS pgcrypto;

WITH candidates AS (
  SELECT id AS user_id, role
  FROM users
  WHERE role = 'user'
    AND id LIKE :'user_prefix' || '%'
  ORDER BY
    CASE
      WHEN id ~ ('^' || :'user_prefix' || '[0-9]+$') THEN substring(id from ('^' || :'user_prefix' || '([0-9]+)$'))::integer
      WHEN id ~ '^k6_bidder_[0-9]+_[0-9]+$' THEN substring(id from '^k6_bidder_([0-9]+)_')::integer
      ELSE 2147483647
    END,
    CASE
      WHEN id ~ '^k6_bidder_[0-9]+_[0-9]+$' THEN substring(id from '^k6_bidder_[0-9]+_([0-9]+)$')::integer
      ELSE 0
    END,
    id
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
    'pts-jmeter-l2'
  FROM tokens
  RETURNING user_id
)
SELECT tokens.user_id, tokens.token, tokens.role
FROM tokens
JOIN inserted USING (user_id)
ORDER BY tokens.user_id
