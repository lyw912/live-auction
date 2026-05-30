#!/usr/bin/env bash
set -euo pipefail

LABEL="${1:-manual}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVIDENCE_ROOT="${EVIDENCE_ROOT:-$ROOT_DIR/docs/perf/pts/evidence/incoming}"
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

curl -fsS "$BASE_URL/readyz" > "$OUT_DIR/readyz.json"
curl -fsS "$BASE_URL/metrics" > "$OUT_DIR/metrics.prom"

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
