#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"

export SESSION_COUNT="${SESSION_COUNT:-5000}"
export SESSION_CSV="${SESSION_CSV:-pts-l3l4-all-sessions.csv}"
export P1_LOAD_USER_COUNT="${P1_LOAD_USER_COUNT:-5000}"
export P1_LOAD_WS_COUNT="${P1_LOAD_WS_COUNT:-2000}"
export P1_LOAD_BIDDER_VUS="${P1_LOAD_BIDDER_VUS:-512}"
export P1_LOAD_AUCTION_END_MINUTES="${P1_LOAD_AUCTION_END_MINUTES:-120}"
export L4B_PROFILE="${L4B_PROFILE:-pts-1b}"

cd "$ROOT_DIR"

bash tests/pts/prepare-l2-protocol-pressure.sh

docker exec "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -c "
INSERT INTO rooms (id, host_id, status)
VALUES ('room_inv_001', 'host_1', 'OPEN')
ON CONFLICT (id) DO UPDATE SET host_id = EXCLUDED.host_id, status = EXCLUDED.status;

INSERT INTO items (id, title, image_url, description, status)
VALUES ('item_inv_001', 'Isolation Auction Item', NULL, 'L3/L4 isolation pressure item.', 'READY')
ON CONFLICT (id) DO UPDATE
SET title = EXCLUDED.title,
    image_url = EXCLUDED.image_url,
    description = EXCLUDED.description,
    status = EXCLUDED.status;

DELETE FROM bids WHERE auction_id = 'auc_inv_001';
DELETE FROM outbox_delivery
WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id = 'auc_inv_001');
DELETE FROM outbox_events WHERE auction_id = 'auc_inv_001';
DELETE FROM auction_events WHERE auction_id = 'auc_inv_001';
DELETE FROM orders WHERE auction_id = 'auc_inv_001';
DELETE FROM auction_rules WHERE auction_id = 'auc_inv_001';
DELETE FROM auctions WHERE id = 'auc_inv_001';

INSERT INTO auctions (
  id, room_id, item_id, status, is_narrating,
  current_price_cents, current_winner_id,
  start_price_cents, increment_cents, cap_price_cents,
  start_at, end_at, version, seq, accepted_bid_count,
  extend_count, rule_version, updated_at
) VALUES (
  'auc_inv_001', 'room_inv_001', 'item_inv_001', 'ACTIVE', true,
  30000, NULL,
  30000, 10000, 100000000000000,
  now() - interval '1 minute', now() + interval '120 minutes',
  1, 0, 0,
  0, 1, now()
);

INSERT INTO auction_rules (
  auction_id, rule_version, duration_seconds,
  extend_window_seconds, extend_by_seconds, max_extend_count,
  fat_finger_threshold_cents, deposit_bps,
  deposit_floor_cents, deposit_cap_cents, frozen_at
) VALUES (
  'auc_inv_001', 1, 1800,
  10, 10, 3,
  NULL, 1000,
  5000, 50000, now()
);

INSERT INTO room_memberships (room_id, user_id, role, status)
SELECT 'room_inv_001', id, 'viewer', 'ACTIVE'
FROM users
WHERE id LIKE 'k6_user_%' OR id LIKE 'k6_bidder_%' OR id LIKE 'k6_ws_%'
ON CONFLICT (room_id, user_id)
DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL;
"

docker exec "$REDIS_CONTAINER" sh -c "redis-cli --scan --pattern 'bid:{auc_inv_001}:*' | xargs -r redis-cli del" >/dev/null
docker exec "$REDIS_CONTAINER" redis-cli del auction:auc_inv_001:events auction:auc_inv_001:snapshot >/dev/null

echo "L3/L4 pressure data ready:"
echo "- Base CSVs from prepare-l2-protocol-pressure.sh"
echo "- Additional active auction: auc_inv_001 in room_inv_001"
