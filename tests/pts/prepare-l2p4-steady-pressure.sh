#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/docs/perf/pts"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"

export SESSION_COUNT="${SESSION_COUNT:-12000}"
export SESSION_CSV="${SESSION_CSV:-pts-l2-all-sessions.csv}"
export P1_LOAD_USER_COUNT="${P1_LOAD_USER_COUNT:-12000}"
export P1_LOAD_WS_COUNT="${P1_LOAD_WS_COUNT:-10000}"
export P1_LOAD_BIDDER_VUS="${P1_LOAD_BIDDER_VUS:-512}"
export L4B_PROFILE="${L4B_PROFILE:-pts-1b}"

cd "$ROOT_DIR"

SKIP_PTS_CACHE_PRESEED=1 \
JMX_PATH="$ROOT_DIR/tests/pts/L2-protocol/pts-2p4-steady-interactive-auction.jmx" \
  bash tests/pts/reset-l4b-final-second-pressure.sh

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
      -f - < "$OUT_DIR/generate-l2-pts-sessions-with-hash.sql"
  } > "$OUT_DIR/$file"
}

strip_hash_csv() {
  local hashed_file="$1"
  local public_file="$2"
  awk -F',' 'BEGIN{OFS=","} {print $1,$2,$3}' "$OUT_DIR/$hashed_file" > "$OUT_DIR/$public_file"
}

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
  done < "$OUT_DIR/$file"
  docker exec -i "$REDIS_CONTAINER" redis-cli --pipe < "$pipe_file" >/dev/null
  rm -f "$pipe_file"
  trap - RETURN
  echo "preseeded $total sessions from $file"
}

generate_csv_with_hash "k6_bidder_" 360 "pts-l2p4-bidder-360-sessions.with-hash.csv"
generate_csv_with_hash "k6_ws_" 2400 "pts-l2p4-viewer-2400-sessions.with-hash.csv"
generate_csv_with_hash "k6_user_" 240 "pts-l2p4-reader-240-sessions.with-hash.csv"

strip_hash_csv "pts-l2p4-bidder-360-sessions.with-hash.csv" "pts-l2p4-bidder-360-sessions.csv"
strip_hash_csv "pts-l2p4-viewer-2400-sessions.with-hash.csv" "pts-l2p4-viewer-2400-sessions.csv"
strip_hash_csv "pts-l2p4-reader-240-sessions.with-hash.csv" "pts-l2p4-reader-240-sessions.csv"

acl_room_id="$(docker exec "$DB_CONTAINER" psql -q -A -t -U "$DB_USER" \
  -d "$DB_NAME" -c "SELECT room_id FROM auctions WHERE id = 'auc_live'")"

preseed_csv "pts-l2p4-bidder-360-sessions.with-hash.csv"
preseed_csv "pts-l2p4-viewer-2400-sessions.with-hash.csv"
preseed_csv "pts-l2p4-reader-240-sessions.with-hash.csv"

echo "L2-P4 steady pressure data ready:"
echo "- tests/pts/L2-protocol/pts-2p4-steady-interactive-auction.jmx"
echo "- docs/perf/pts/pts-l2p4-bidder-360-sessions.csv"
echo "- docs/perf/pts/pts-l2p4-viewer-2400-sessions.csv"
echo "- docs/perf/pts/pts-l2p4-reader-240-sessions.csv"
