#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"
BID_MAX_VUS="${BID_MAX_VUS:-400}"
READ_MAX_VUS="${READ_MAX_VUS:-5000}"

cd "$ROOT_DIR"

ALLOW_MOCK_AUTH="${ALLOW_MOCK_AUTH:-true}" bash tests/pts/prepare-l2p4-steady-pressure.sh

acl_room_id="$(docker exec "$DB_CONTAINER" psql -q -A -t -U "$DB_USER" \
  -d "$DB_NAME" -c "SELECT room_id FROM auctions WHERE id = 'auc_live'")"

if [ -z "$acl_room_id" ]; then
  echo "auc_live room_id not found" >&2
  exit 1
fi

docker exec -i "$DB_CONTAINER" psql -q -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" \
  -v room_id="$acl_room_id" \
  -v bid_max_vus="$BID_MAX_VUS" \
  -v read_max_vus="$READ_MAX_VUS" \
  -f - <<'SQL'
INSERT INTO users (id, role, display_name, city)
SELECT 'k6_bidder_' || generate_series(1, :'bid_max_vus'::int)::text,
       'user',
       'k6_bidder_' || generate_series(1, :'bid_max_vus'::int)::text,
       's2-read'
ON CONFLICT (id) DO NOTHING;

INSERT INTO users (id, role, display_name, city)
SELECT 'k6_user_' || generate_series(1, :'read_max_vus'::int)::text,
       'user',
       'k6_user_' || generate_series(1, :'read_max_vus'::int)::text,
       's2-read'
ON CONFLICT (id) DO NOTHING;

INSERT INTO room_memberships (room_id, user_id, role, status)
SELECT :'room_id', 'k6_bidder_' || generate_series(1, :'bid_max_vus'::int)::text, 'viewer', 'ACTIVE'
ON CONFLICT (room_id, user_id) DO UPDATE
SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL;

INSERT INTO room_memberships (room_id, user_id, role, status)
SELECT :'room_id', 'k6_user_' || generate_series(1, :'read_max_vus'::int)::text, 'viewer', 'ACTIVE'
ON CONFLICT (room_id, user_id) DO UPDATE
SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL;
SQL

{
  for i in $(seq 1 "$BID_MAX_VUS"); do
    key="acl:membership:{auc_live}:k6_bidder_${i}"
    printf '*5\r\n'
    printf '$3\r\nSET\r\n'
    printf '$%s\r\n%s\r\n' "${#key}" "$key"
    printf '$%s\r\n%s\r\n' "${#acl_room_id}" "$acl_room_id"
    printf '$2\r\nEX\r\n'
    printf '$5\r\n43200\r\n'
  done
  for i in $(seq 1 "$READ_MAX_VUS"); do
    key="acl:membership:{auc_live}:k6_user_${i}"
    printf '*5\r\n'
    printf '$3\r\nSET\r\n'
    printf '$%s\r\n%s\r\n' "${#key}" "$key"
    printf '$%s\r\n%s\r\n' "${#acl_room_id}" "$acl_room_id"
    printf '$2\r\nEX\r\n'
    printf '$5\r\n43200\r\n'
  done
} | docker exec -i "$REDIS_CONTAINER" redis-cli --pipe >/dev/null

echo "S2 read-interference pressure data ready:"
echo "- Backend: http://127.0.0.1:18080"
echo "- Auction: auc_live room=$acl_room_id"
echo "- Mock auth: ALLOW_MOCK_AUTH=true required"
echo "- Plain bidders: k6_bidder_1..${BID_MAX_VUS}"
echo "- Readers: k6_user_1..${READ_MAX_VUS}"
