#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_DIR="${PTS_RUNTIME_DIR:-/tmp/live-auction-pts}"
HTTP_ADDR="${HTTP_ADDR:-0.0.0.0:18080}"
HTTP_PORT="${HTTP_ADDR##*:}"

cd "$ROOT_DIR"

docker compose -f infra/docker-compose.yml -f infra/docker-compose.toxiproxy.yml up -d postgres redis kafka kafka-init toxiproxy
node tests/chaos/run-toxiproxy-scenario.mjs --clear >/dev/null

DATABASE_URL="${DATABASE_URL:-postgres://live_auction:live_auction@localhost:15432/live_auction?sslmode=disable}" \
REDIS_ADDR="${REDIS_ADDR:-localhost:16379}" \
ALLOW_MOCK_AUTH=true \
BID_ENGINE_MODE=redis_ledger \
ADMISSION_ENABLED=false \
SESSION_COUNT=0 \
L4B_PROFILE=pts-1b \
bash tests/pts/reset-l4b-final-second-pressure.sh

docker exec live-auction-postgres psql -U live_auction -d live_auction -v ON_ERROR_STOP=1 -c "
INSERT INTO users (id, role, display_name, city)
SELECT id, 'user', id, 'chaos'
FROM (
  SELECT 'chaos_user_1' AS id
  UNION ALL
  SELECT 'chaos_snap_' || generate_series(0, 31)::text
  UNION ALL
  SELECT 'chaos_bidder_' || generate_series(0, 31)::text
) users_to_seed
ON CONFLICT (id) DO NOTHING;

INSERT INTO room_memberships (room_id, user_id, role, status)
SELECT 'room_main', id, 'viewer', 'ACTIVE'
FROM users
WHERE id = 'chaos_user_1'
   OR id LIKE 'chaos_snap_%'
   OR id LIKE 'chaos_bidder_%'
ON CONFLICT (room_id, user_id)
DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL;
"

if [ -f "$RUNTIME_DIR/server.pid" ]; then
  pid="$(cat "$RUNTIME_DIR/server.pid")"
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    sleep 1
  fi
fi

(
  cd "$ROOT_DIR/backend"
  APP_ENV=local \
  HTTP_ADDR="$HTTP_ADDR" \
  DATABASE_URL="${DATABASE_URL:-postgres://live_auction:live_auction@localhost:15432/live_auction?sslmode=disable}" \
  REDIS_ADDR="${REDIS_ADDR:-localhost:16379}" \
  KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}" \
  KAFKA_BID_TOPIC="${KAFKA_BID_TOPIC:-auction.bid-events}" \
  KAFKA_DLQ_TOPIC="${KAFKA_DLQ_TOPIC:-auction.dlq}" \
  ALLOW_MOCK_AUTH=true \
  ADMISSION_ENABLED=false \
  BID_ENGINE_MODE=redis_ledger \
  REDIS_POOL_SIZE="${REDIS_POOL_SIZE:-300}" \
  SESSION_TTL=12h \
  OUTBOX_WORKER_ID=chaos-profile \
  SCHEDULER_WORKER_ID=chaos-profile \
  setsid "$RUNTIME_DIR/live-auction-server" > "$RUNTIME_DIR/server.log" 2> "$RUNTIME_DIR/server.err.log" < /dev/null &
  echo $! > "$RUNTIME_DIR/server.pid"
)

for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${HTTP_PORT}/readyz" >/dev/null 2>&1; then
    echo "chaos profile ready: http://127.0.0.1:${HTTP_PORT}"
    echo "database through toxiproxy: localhost:15432"
    echo "redis through toxiproxy: localhost:16379"
    exit 0
  fi
  sleep 1
done

echo "backend did not become ready; see $RUNTIME_DIR/server.err.log" >&2
exit 1
