#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_DIR="${PTS_RUNTIME_DIR:-/tmp/live-auction-pts}"
GEN_DIR="$RUNTIME_DIR/s3-room-fanout-generated"
OUT_DIR="$ROOT_DIR/docs/perf/pts/inputs/s1-s5"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"

cd "$ROOT_DIR"
mkdir -p "$GEN_DIR" "$OUT_DIR"

SKIP_PTS_CACHE_PRESEED=1 \
JMX_PATH="$ROOT_DIR/tests/pts/scenarios/s3-room-fanout/s3-mixed-final-burst-4500vu.jmx" \
SESSION_COUNT=7000 \
SESSION_CSV="s3-mixed-final-burst-4500-sessions.csv" \
L4B_PROFILE=pts-1b \
  bash tests/pts/reset-l4b-final-second-pressure.sh

docker exec "$DB_CONTAINER" psql -q -v ON_ERROR_STOP=1 \
  -U "$DB_USER" -d "$DB_NAME" \
  -c "DELETE FROM auth_sessions WHERE created_ip = 'pts' OR user_agent LIKE 'pts-jmeter%';"
docker exec "$DB_CONTAINER" psql -q -v ON_ERROR_STOP=1 \
  -U "$DB_USER" -d "$DB_NAME" \
  -c "DELETE FROM user_activity_events WHERE auction_id = 'auc_live';"
docker exec "$REDIS_CONTAINER" sh -c "redis-cli --scan --pattern 'auth:session:*' | xargs -r redis-cli del" >/dev/null
docker exec "$REDIS_CONTAINER" sh -c "redis-cli --scan --pattern 'acl:membership:{auc_live}:*' | xargs -r redis-cli del" >/dev/null

generate_csv_with_hash() {
  local prefix="$1"
  local count="$2"
  local file="$3"
  {
    echo "user_id,token,role,token_hash"
    docker exec -i "$DB_CONTAINER" psql -q -A -F ',' -t -v ON_ERROR_STOP=1 \
      -U "$DB_USER" -d "$DB_NAME" \
      -v user_prefix="$prefix" \
      -v session_count="$count" \
      -f - < "$ROOT_DIR/docs/perf/pts/generate-l2-pts-sessions-with-hash.sql"
  } > "$GEN_DIR/$file"
}

strip_hash_csv() {
  local hashed_file="$1"
  local public_file="$2"
  awk -F',' 'BEGIN{OFS=","} {print $1,$2,$3}' "$GEN_DIR/$hashed_file" > "$GEN_DIR/$public_file"
}

generate_csv_with_hash "k6_bidder_" 1008 "s3-bidder-1008-sessions.with-hash.csv"
generate_csv_with_hash "k6_ws_" 4998 "s3-viewer-4998-sessions.with-hash.csv"
generate_csv_with_hash "k6_user_" 994 "s3-reader-994-sessions.with-hash.csv"

strip_hash_csv "s3-bidder-1008-sessions.with-hash.csv" "s3-bidder-1008-sessions.csv"
strip_hash_csv "s3-viewer-4998-sessions.with-hash.csv" "s3-viewer-4998-sessions.csv"
strip_hash_csv "s3-reader-994-sessions.with-hash.csv" "s3-reader-994-sessions.csv"

head -n 6 "$GEN_DIR/s3-bidder-1008-sessions.csv" > "$GEN_DIR/s3-smoke-bidder-5-sessions.csv"
head -n 21 "$GEN_DIR/s3-viewer-4998-sessions.csv" > "$GEN_DIR/s3-smoke-viewer-20-sessions.csv"
head -n 6 "$GEN_DIR/s3-reader-994-sessions.csv" > "$GEN_DIR/s3-smoke-reader-5-sessions.csv"

acl_room_id="$(docker exec "$DB_CONTAINER" psql -q -A -t -U "$DB_USER" \
  -d "$DB_NAME" -c "SELECT room_id FROM auctions WHERE id = 'auc_live'")"

append_redis_set() {
  local out_file="$1"
  local key="$2"
  local value="$3"
  local ttl="$4"
  {
    printf '*5\r\n'
    printf '$3\r\nSET\r\n'
    printf '$%s\r\n%s\r\n' "${#key}" "$key"
    printf '$%s\r\n%s\r\n' "${#value}" "$value"
    printf '$2\r\nEX\r\n'
    printf '$%s\r\n%s\r\n' "${#ttl}" "$ttl"
  } >> "$out_file"
}

preseed_csv() {
  local file="$1"
  local total=0
  local pipe_file
  pipe_file="$(mktemp)"
  trap 'rm -f "$pipe_file"' RETURN
  while IFS=',' read -r user_id token role token_hash; do
    [ "$user_id" = "user_id" ] && continue
    [ -z "$user_id" ] && continue
    append_redis_set "$pipe_file" "auth:session:${token_hash}" "{\"ID\":\"${user_id}\",\"Role\":\"${role}\"}" 43200
    append_redis_set "$pipe_file" "acl:membership:{auc_live}:${user_id}" "${acl_room_id}" 43200
    total=$((total+1))
  done < "$GEN_DIR/$file"
  docker exec -i "$REDIS_CONTAINER" redis-cli --pipe < "$pipe_file" >/dev/null
  rm -f "$pipe_file"
  trap - RETURN
  echo "preseeded $total sessions from $file"
}

preseed_csv "s3-bidder-1008-sessions.with-hash.csv"
preseed_csv "s3-viewer-4998-sessions.with-hash.csv"
preseed_csv "s3-reader-994-sessions.with-hash.csv"

snapshot_payload="$(docker exec "$DB_CONTAINER" psql -q -A -t -U "$DB_USER" -d "$DB_NAME" -c "
SELECT jsonb_build_object(
  'event_type', 'snapshot',
  'auction_id', a.id,
  'seq', a.seq,
  'stream_epoch', COALESCE(epoch.value, ''),
  'snapshot_version', a.version,
  'source', 'redis',
  'stale', false,
  'payload', jsonb_build_object(
    'status', a.status,
    'current_price_cents', a.current_price_cents,
    'current_winner_id', a.current_winner_id,
    'end_at', a.end_at,
    'accepted_bid_count', a.accepted_bid_count,
    'reason', NULL
  )
)::text
FROM auctions a
LEFT JOIN LATERAL (
  SELECT value
  FROM realtime_stream_epochs
  WHERE auction_id = a.id
) epoch ON true
WHERE a.id = 'auc_live';
")"
if [ -z "$snapshot_payload" ]; then
  echo "failed to build auc_live realtime snapshot" >&2
  exit 1
fi
snapshot_pipe="$(mktemp)"
append_redis_set "$snapshot_pipe" "auction:auc_live:snapshot" "$snapshot_payload" 1800
docker exec -i "$REDIS_CONTAINER" redis-cli --pipe < "$snapshot_pipe" >/dev/null
rm -f "$snapshot_pipe"
echo "preseeded realtime snapshot auction:auc_live:snapshot source=redis ttl=1800s"

echo "S3 room fanout pressure data ready:"
echo "- tests/pts/scenarios/s3-room-fanout/s3-mixed-final-burst-4500vu.jmx"
echo "- tests/pts/scenarios/s3-room-fanout/s3-mixed-smoke-30vu.jmx"

write_mixed_csv() {
  local out_file="$1"
  shift
  echo "load_role,user_id,token,role" > "$OUT_DIR/$out_file"
  while [ "$#" -gt 0 ]; do
    local role="$1"
    local file="$2"
    local count="$3"
    shift 3
    awk -v role="$role" -v count="$count" 'NR > 1 && seen < count { print role "," $0; seen++ }' "$GEN_DIR/$file" >> "$OUT_DIR/$out_file"
  done
}

write_mixed_csv "s3-mixed-final-burst-4500-sessions.csv" \
  viewer "s3-viewer-4998-sessions.csv" 3000 \
  reader "s3-reader-994-sessions.csv" 500 \
  bidder "s3-bidder-1008-sessions.csv" 1000

write_mixed_csv "s3-mixed-smoke-30-sessions.csv" \
  viewer "s3-smoke-viewer-20-sessions.csv" 20 \
  reader "s3-smoke-reader-5-sessions.csv" 5 \
  bidder "s3-smoke-bidder-5-sessions.csv" 5

echo "- docs/perf/pts/inputs/s1-s5/s3-mixed-final-burst-4500-sessions.csv"
echo "- docs/perf/pts/inputs/s1-s5/s3-mixed-smoke-30-sessions.csv"
