#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/docs/perf/pts"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"

export SESSION_COUNT="${SESSION_COUNT:-5000}"
export SESSION_CSV="${SESSION_CSV:-pts-l2-all-sessions.csv}"
export P1_LOAD_USER_COUNT="${P1_LOAD_USER_COUNT:-5000}"
export P1_LOAD_WS_COUNT="${P1_LOAD_WS_COUNT:-2000}"
export P1_LOAD_BIDDER_VUS="${P1_LOAD_BIDDER_VUS:-512}"
export L4B_PROFILE="${L4B_PROFILE:-pts-1b}"

cd "$ROOT_DIR"

JMX_PATH="$ROOT_DIR/tests/pts/L2-protocol/pts-2p3-bid-ws-reads.jmx" \
  bash tests/pts/reset-l4b-final-second-pressure.sh

generate_csv() {
  local prefix="$1"
  local count="$2"
  local file="$3"
  {
    echo "user_id,token,role"
    docker exec -i "$DB_CONTAINER" psql -q -A -F ',' -t -v ON_ERROR_STOP=1 \
      -U "$DB_USER" -d "$DB_NAME" \
      -v user_prefix="$prefix" \
      -v session_count="$count" \
      -f - < "$OUT_DIR/generate-l2-pts-sessions.sql"
  } > "$OUT_DIR/$file"
}

generate_csv "k6_bidder_" 1000 "pts-l2-bidder-1000-sessions.csv"
generate_csv "k6_ws_" 2000 "pts-l2-viewer-2000-sessions.csv"
generate_csv "k6_user_" 5000 "pts-l2-reader-5000-sessions.csv"

acl_room_id="$(docker exec "$DB_CONTAINER" psql -q -A -t -U "$DB_USER" \
  -d "$DB_NAME" -c "SELECT room_id FROM auctions WHERE id = 'auc_live'")"

preseed_csv() {
  local file="$1"
  local total=0
  while IFS=',' read -r user_id token role; do
    [ "$user_id" = "user_id" ] && continue
    [ -z "$user_id" ] && continue
    token_hash="$(printf '%s' "$token" | sha256sum | cut -d' ' -f1)"
    docker exec "$REDIS_CONTAINER" redis-cli \
      SET "auth:session:${token_hash}" "{\"ID\":\"${user_id}\",\"Role\":\"${role}\"}" \
      EX 43200 >/dev/null
    docker exec "$REDIS_CONTAINER" redis-cli \
      SET "acl:membership:{auc_live}:${user_id}" "${acl_room_id}" \
      EX 43200 >/dev/null
    total=$((total+1))
  done < "$OUT_DIR/$file"
  echo "preseeded $total sessions from $file"
}

preseed_csv "pts-l2-bidder-1000-sessions.csv"
preseed_csv "pts-l2-viewer-2000-sessions.csv"
preseed_csv "pts-l2-reader-5000-sessions.csv"

echo "L2 protocol pressure data ready:"
echo "- docs/perf/pts/pts-l2-bidder-1000-sessions.csv"
echo "- docs/perf/pts/pts-l2-viewer-2000-sessions.csv"
echo "- docs/perf/pts/pts-l2-reader-5000-sessions.csv"
