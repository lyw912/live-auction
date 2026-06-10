#!/usr/bin/env bash
set -euo pipefail

LABEL="${1:-manual}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVIDENCE_ROOT="${EVIDENCE_ROOT:-$ROOT_DIR/artifacts/pts/evidence/incoming}"
OUT_DIR="$EVIDENCE_ROOT/$LABEL"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"
BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"

mkdir -p "$OUT_DIR"

echo "[collect] writing $OUT_DIR"
date -Is > "$OUT_DIR/collected-at.txt"
uptime > "$OUT_DIR/uptime.txt" || true
free -m > "$OUT_DIR/free-m.txt" || true
df -h > "$OUT_DIR/df-h.txt" || true
ss -s > "$OUT_DIR/ss-s.txt" || true
ps -eo pid,ppid,pcpu,pmem,rss,vsz,cmd --sort=-pcpu | head -n 40 > "$OUT_DIR/top-processes.txt" || true

curl --retry 5 --retry-delay 1 --retry-all-errors -fsS "$BASE_URL/readyz" > "$OUT_DIR/readyz.json"
curl --retry 5 --retry-delay 1 --retry-all-errors -fsS "$BASE_URL/metrics" > "$OUT_DIR/metrics.prom"

docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -f - > "$OUT_DIR/postgres-summary.txt" <<'SQL'
\timing on
select now() as ts;
select id, status, current_price_cents, accepted_bid_count, seq, end_at from auctions where id in ('auc_live','auc_side');
select count(*) as bids,
       count(*) filter (where status='ACCEPTED') as accepted,
       count(*) filter (where status='REJECTED') as rejected
from bids
where auction_id in ('auc_live','auc_side');
select status, count(*) from outbox_delivery group by status order by status;
select count(*) as pending,
       max(now() - event_created_at) as max_pending_age,
       avg(extract(epoch from now() - event_created_at)) as avg_pending_age_seconds
from outbox_delivery
where status='PENDING';
select shard_id, status, count(*),
       max(now() - event_created_at) as max_age
from outbox_delivery
where status in ('PENDING','FAILED','PUBLISHING')
group by shard_id, status
order by count(*) desc, shard_id
limit 20;
select wait_event_type, wait_event, state, count(*)
from pg_stat_activity
where datname = current_database()
group by wait_event_type, wait_event, state
order by count(*) desc;
select locktype, mode, granted, count(*)
from pg_locks
group by locktype, mode, granted
order by count(*) desc;
SQL

docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -f - > "$OUT_DIR/postgres-read-attribution.txt" <<'SQL' || true
\timing on
select now() as ts;
select extname
from pg_extension
where extname = 'pg_stat_statements';
select case
  when to_regclass('pg_stat_statements') is null then
    $$select 'pg_stat_statements is not installed; skipping query attribution' as note;$$
  else
    $$select calls,
             round(total_exec_time::numeric, 2) as total_exec_ms,
             round(mean_exec_time::numeric, 2) as mean_exec_ms,
             round(max_exec_time::numeric, 2) as max_exec_ms,
             rows,
             left(regexp_replace(query, '\s+', ' ', 'g'), 240) as query
      from pg_stat_statements
      where dbid = (select oid from pg_database where datname = current_database())
        and query ilike any(array[
          '%FROM auctions a JOIN items i%',
          '%FROM bids WHERE user_id = $1%',
          '%FROM bids WHERE auction_id = $1 AND status = ''ACCEPTED''%',
          '%FROM max_bid_intents WHERE auction_id = $1 AND user_id = $2%'
        ])
      order by total_exec_time desc
      limit 20;$$
  end
\gexec
SQL

docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -f - > "$OUT_DIR/postgres-s2-read-explain.txt" <<'SQL' || true
\timing on
EXPLAIN (ANALYZE, BUFFERS)
SELECT a.id, a.room_id, a.item_id, a.status, a.is_narrating,
       a.current_price_cents, a.current_winner_id,
       a.start_price_cents, a.increment_cents, a.cap_price_cents,
       a.start_at, a.end_at, floor(extract(epoch from clock_timestamp()) * 1000)::bigint,
       a.version, a.seq, a.accepted_bid_count,
       a.extend_count, a.rule_version, a.created_at, a.updated_at,
       i.id, i.title, i.image_url, i.description, i.status, i.created_at,
       ar.duration_seconds, ar.extend_window_seconds, ar.extend_by_seconds,
       ar.max_extend_count, ar.fat_finger_threshold_cents,
       COALESCE(ar.deposit_bps, 1000),
       COALESCE(ar.deposit_floor_cents, 10000),
       COALESCE(ar.deposit_cap_cents, 100000000),
       ar.frozen_at
FROM auctions a
JOIN items i ON i.id = a.item_id
JOIN auction_rules ar ON ar.auction_id = a.id AND ar.rule_version = a.rule_version
WHERE a.id = 'auc_live';

EXPLAIN (ANALYZE, BUFFERS)
WITH best AS (
  SELECT user_id, max(amount_cents) AS amount_cents, count(*) AS bid_count, max(created_at) AS last_bid_at
  FROM bids
  WHERE auction_id = 'auc_live' AND status = 'ACCEPTED'
  GROUP BY user_id
),
ranked AS (
  SELECT user_id, amount_cents, bid_count, last_bid_at,
         row_number() OVER (ORDER BY amount_cents DESC, last_bid_at ASC, user_id ASC) AS rank
  FROM best
),
selected AS (
  SELECT * FROM ranked WHERE rank <= 5
  UNION
  SELECT * FROM ranked WHERE user_id = 'k6_user_1'
)
SELECT rank, user_id, amount_cents, bid_count, last_bid_at
FROM selected
ORDER BY rank;

EXPLAIN (ANALYZE, BUFFERS)
SELECT id, auction_id, amount_cents, COALESCE(response_json->>'result', status), created_at
FROM bids
WHERE user_id = 'k6_user_1'
ORDER BY created_at DESC
LIMIT 50;

EXPLAIN (ANALYZE, BUFFERS)
SELECT id, auction_id, user_id, max_amount_cents, status, source,
       created_at, updated_at, cancelled_at, exhausted_at, last_applied_seq, version
FROM max_bid_intents
WHERE auction_id = 'auc_live' AND user_id = 'k6_user_1';
SQL

docker exec "$REDIS_CONTAINER" redis-cli INFO all > "$OUT_DIR/redis-info.txt"

if command -v iostat >/dev/null 2>&1; then
  iostat -xz 1 5 > "$OUT_DIR/iostat-xz.txt" || true
fi
if command -v mpstat >/dev/null 2>&1; then
  mpstat 1 5 > "$OUT_DIR/mpstat.txt" || true
fi
if command -v vmstat >/dev/null 2>&1; then
  vmstat 1 5 > "$OUT_DIR/vmstat.txt" || true
fi

echo "[collect] done: $OUT_DIR"
