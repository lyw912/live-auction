#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
MIGRATIONS_DIR="$BACKEND_DIR/migrations"
OUT_DIR="$ROOT_DIR/docs/perf/pts"
TMP_SQL="/tmp/live-auction-migrations-up.sql"
BOOTSTRAP_SQL="/tmp/live-auction-bootstrap-before-migrations.sql"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"
HTTP_ADDR="${HTTP_ADDR:-0.0.0.0:18080}"
HTTP_PORT="${HTTP_ADDR##*:}"
DATABASE_URL="${DATABASE_URL:-postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable}"
REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
SESSION_COUNT="${SESSION_COUNT:-4096}"

mkdir -p "$OUT_DIR"
: > "$TMP_SQL"

cat > "$BOOTSTRAP_SQL" <<'SQL'
INSERT INTO users (id, role, display_name, city)
VALUES
  ('host_1', 'host', 'Demo Host', 'Hangzhou'),
  ('user_1', 'user', 'Demo User', 'Shanghai')
ON CONFLICT (id) DO NOTHING;

INSERT INTO rooms (id, host_id, status)
VALUES
  ('room_1', 'host_1', 'OPEN'),
  ('room_main', 'host_1', 'OPEN')
ON CONFLICT (id) DO UPDATE
SET host_id = EXCLUDED.host_id,
    status = EXCLUDED.status;
SQL

for file in "$MIGRATIONS_DIR"/*.sql; do
  awk '
    /^-- \+goose Up/ { in_up = 1; next }
    /^-- \+goose Down/ { in_up = 0; next }
    in_up { print }
  ' "$file" >> "$TMP_SQL"
  printf '\n' >> "$TMP_SQL"
done

sed -i \
  -e '/^CREATE TABLE IF NOT EXISTS /!s/^CREATE TABLE /CREATE TABLE IF NOT EXISTS /g' \
  -e '/^CREATE UNIQUE INDEX IF NOT EXISTS /!s/^CREATE UNIQUE INDEX /CREATE UNIQUE INDEX IF NOT EXISTS /g' \
  -e '/^CREATE INDEX IF NOT EXISTS /!s/^CREATE INDEX /CREATE INDEX IF NOT EXISTS /g' \
  -e '/^CREATE EXTENSION IF NOT EXISTS /!s/^CREATE EXTENSION /CREATE EXTENSION IF NOT EXISTS /g' \
  "$TMP_SQL"

perl -0pi -e "s/ALTER TABLE orders\\s+DROP CONSTRAINT orders_status_check,\\s+ADD CONSTRAINT orders_status_check CHECK \\(status IN \\('ORDER_PENDING','PAYMENT_INITIATED','PAYMENT_SUCCEEDED','PAID','ORDER_EXPIRED'\\)\\);/DO \\$\\$ BEGIN\\n  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orders_status_check') THEN\\n    ALTER TABLE orders DROP CONSTRAINT orders_status_check;\\n  END IF;\\n  ALTER TABLE orders ADD CONSTRAINT orders_status_check CHECK (status IN ('ORDER_PENDING','PAYMENT_INITIATED','PAYMENT_SUCCEEDED','PAID','ORDER_EXPIRED'));\\nEND \\$\\$;/s" "$TMP_SQL"

perl -0pi -e "s/ALTER TABLE orders\\s+ADD COLUMN provider_payment_id text,\\s+ADD COLUMN payment_initiated_at timestamptz,\\s+ADD COLUMN payment_succeeded_at timestamptz;/ALTER TABLE orders ADD COLUMN IF NOT EXISTS provider_payment_id text;\\nALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_initiated_at timestamptz;\\nALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_succeeded_at timestamptz;/s" "$TMP_SQL"

perl -0pi -e "s/ALTER TABLE outbox_delivery\\s+ADD COLUMN auction_id text,\\s+ADD COLUMN auction_seq bigint,\\s+ADD COLUMN event_created_at timestamptz;/ALTER TABLE outbox_delivery ADD COLUMN IF NOT EXISTS auction_id text;\\nALTER TABLE outbox_delivery ADD COLUMN IF NOT EXISTS auction_seq bigint;\\nALTER TABLE outbox_delivery ADD COLUMN IF NOT EXISTS event_created_at timestamptz;/s" "$TMP_SQL"

perl -0pi -e "s/ALTER TABLE outbox_delivery\\s+ADD COLUMN shard_id int;/ALTER TABLE outbox_delivery ADD COLUMN IF NOT EXISTS shard_id int;/s" "$TMP_SQL"

perl -0pi -e "s/CREATE TRIGGER trg_sync_outbox_delivery_event_fields\\nBEFORE INSERT ON outbox_delivery\\nFOR EACH ROW\\nEXECUTE FUNCTION sync_outbox_delivery_event_fields\\(\\);/DROP TRIGGER IF EXISTS trg_sync_outbox_delivery_event_fields ON outbox_delivery;\\nCREATE TRIGGER trg_sync_outbox_delivery_event_fields\\nBEFORE INSERT ON outbox_delivery\\nFOR EACH ROW\\nEXECUTE FUNCTION sync_outbox_delivery_event_fields();/s" "$TMP_SQL"

perl -0pi -e "s/ALTER TABLE outbox_events\\s+ADD COLUMN event_schema_version int NOT NULL DEFAULT 1,\\s+ADD COLUMN event_key text,\\s+ADD COLUMN payload_sha256 text;/ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS event_schema_version int NOT NULL DEFAULT 1;\\nALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS event_key text;\\nALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS payload_sha256 text;/s" "$TMP_SQL"

perl -0pi -e "s/ALTER TABLE outbox_delivery\\s+ADD COLUMN last_error_class text,\\s+ADD COLUMN last_error_retriable boolean,\\s+ADD COLUMN last_error_at timestamptz,\\s+ADD COLUMN last_published_watermark jsonb;/ALTER TABLE outbox_delivery ADD COLUMN IF NOT EXISTS last_error_class text;\\nALTER TABLE outbox_delivery ADD COLUMN IF NOT EXISTS last_error_retriable boolean;\\nALTER TABLE outbox_delivery ADD COLUMN IF NOT EXISTS last_error_at timestamptz;\\nALTER TABLE outbox_delivery ADD COLUMN IF NOT EXISTS last_published_watermark jsonb;/s" "$TMP_SQL"

echo "[1/7] Applying migrations to $DB_CONTAINER/$DB_NAME"
docker cp "$BOOTSTRAP_SQL" "$DB_CONTAINER:/tmp/live-auction-bootstrap-before-migrations.sql"
docker exec "$DB_CONTAINER" psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" -f /tmp/live-auction-bootstrap-before-migrations.sql
docker cp "$TMP_SQL" "$DB_CONTAINER:/tmp/live-auction-migrations-up.sql"
docker exec "$DB_CONTAINER" psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" -f /tmp/live-auction-migrations-up.sql

echo "[2/7] Checking Redis"
docker exec "$REDIS_CONTAINER" redis-cli ping

echo "[3/7] Seeding P1 pressure data"
(
  cd "$BACKEND_DIR"
  MINIO_ENDPOINT="${MINIO_ENDPOINT:-localhost:9000}" \
  MINIO_ROOT_USER="${MINIO_ROOT_USER:-liveauction}" \
  MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-liveauction123}" \
  S3_BUCKET="${S3_BUCKET:-live-auction-items}" \
  S3_USE_SSL="${S3_USE_SSL:-false}" \
  go run ./cmd/ensurebucket
  DATABASE_URL="$DATABASE_URL" REDIS_ADDR="$REDIS_ADDR" go run ./cmd/p1loadseed
)

echo "[4/7] Generating real session CSV for PTS"
{
  echo "user_id,token,role"
  docker exec -i "$DB_CONTAINER" psql -q -A -F ',' -t -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" -v session_count="$SESSION_COUNT" -f - < "$OUT_DIR/generate-pts-sessions.sql"
} > "$OUT_DIR/pts_sessions.csv"

echo "[5/7] Building backend"
(
  cd "$BACKEND_DIR"
  go build -o "$OUT_DIR/live-auction-server" ./cmd/server
)

echo "[6/7] Starting backend on $HTTP_ADDR"
if [ -f "$OUT_DIR/server.pid" ]; then
  old_pid="$(cat "$OUT_DIR/server.pid")"
  if kill -0 "$old_pid" 2>/dev/null; then
    kill "$old_pid" 2>/dev/null || true
    sleep 1
  fi
fi
while read -r listen_pid; do
  if [ -n "$listen_pid" ] && kill -0 "$listen_pid" 2>/dev/null; then
    echo "Stopping existing listener on :$HTTP_PORT pid=$listen_pid"
    kill "$listen_pid" 2>/dev/null || true
    sleep 1
  fi
done < <(
  ss -lntpH "sport = :$HTTP_PORT" 2>/dev/null |
    sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' |
    sort -u
)
(
  cd "$BACKEND_DIR"
  APP_ENV=local \
  HTTP_ADDR="$HTTP_ADDR" \
  DATABASE_URL="$DATABASE_URL" \
  REDIS_ADDR="$REDIS_ADDR" \
  ALLOW_MOCK_AUTH=false \
  ADMISSION_ENABLED=false \
  SESSION_TTL=12h \
  OUTBOX_WORKER_ID=pts-cloud-1 \
  SCHEDULER_WORKER_ID=pts-cloud-1 \
  setsid "$OUT_DIR/live-auction-server" > "$OUT_DIR/server.log" 2> "$OUT_DIR/server.err.log" < /dev/null &
  echo $! > "$OUT_DIR/server.pid"
)

echo "[7/7] Verifying backend"
sleep 1
curl -fsS "http://127.0.0.1:${HTTP_PORT}/readyz"
curl -fsS "http://127.0.0.1:${HTTP_PORT}/metrics" | grep 'auction_admission_enabled 0'
ss -lntpH "sport = :$HTTP_PORT" | grep '0.0.0.0'

echo "Prepared:"
echo "- Backend: http://47.113.223.90:${HTTP_PORT}"
echo "- JMX: $ROOT_DIR/tests/pts/live-auction-core-pressure.jmx"
echo "- CSV: $OUT_DIR/pts_sessions.csv"
echo "- Logs: $OUT_DIR/server.log and $OUT_DIR/server.err.log"
