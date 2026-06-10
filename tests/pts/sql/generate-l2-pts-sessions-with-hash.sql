-- Generate role-specific PTS/JMeter session-token CSV data for L2 protocol tests.
-- Includes token_hash so preparation scripts can preseed Redis without hashing
-- every CSV row in shell.

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
materialized AS (
  SELECT
    user_id,
    role,
    token,
    encode(digest(convert_to(token, 'UTF8'), 'sha256'), 'hex') AS token_hash
  FROM tokens
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
    token_hash,
    now() + interval '12 hours',
    'pts',
    'pts-jmeter-l2'
  FROM materialized
  RETURNING user_id
)
SELECT materialized.user_id, materialized.token, materialized.role, materialized.token_hash
FROM materialized
JOIN inserted USING (user_id)
ORDER BY materialized.user_id
