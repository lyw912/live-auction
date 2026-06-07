#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
MIGRATIONS_DIR="$BACKEND_DIR/migrations"
OUT_DIR="$ROOT_DIR/docs/perf/pts"
RUNTIME_DIR="${PTS_RUNTIME_DIR:-/tmp/live-auction-pts}"
SESSION_CSV="${SESSION_CSV:-archive/data/pts_sessions.csv}"
JMX_PATH_WAS_SET="${JMX_PATH+x}"
JMX_PATH="${JMX_PATH:-$ROOT_DIR/tests/pts/archive/historical/live-auction-core-pressure.jmx}"
BOOTSTRAP_SQL="/tmp/live-auction-bootstrap-before-migrations.sql"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
KAFKA_CONTAINER="${KAFKA_CONTAINER:-live-auction-kafka}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"
HTTP_ADDR="${HTTP_ADDR:-0.0.0.0:18080}"
HTTP_PORT="${HTTP_ADDR##*:}"
DATABASE_URL="${DATABASE_URL:-postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable}"
REDIS_ADDR="${REDIS_ADDR:-localhost:6380}"
KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}"
KAFKA_BID_TOPIC="${KAFKA_BID_TOPIC:-auction.bid-events}"
KAFKA_DLQ_TOPIC="${KAFKA_DLQ_TOPIC:-auction.dlq}"
GOOSE_BIN="${GOOSE_BIN:-goose}"
SESSION_COUNT="${SESSION_COUNT:-4096}"
DB_MAX_CONNS="${DB_MAX_CONNS:-90}"
DB_MIN_CONNS="${DB_MIN_CONNS:-16}"
DB_MAX_CONN_LIFETIME="${DB_MAX_CONN_LIFETIME:-1h}"
DB_MAX_CONN_IDLE_TIME="${DB_MAX_CONN_IDLE_TIME:-30m}"
ALLOW_MOCK_AUTH="${ALLOW_MOCK_AUTH:-false}"
REDIS_POOL_SIZE="${REDIS_POOL_SIZE:-300}"
BID_LANE_WORKERS="${BID_LANE_WORKERS:-1}"
BID_LANE_QUEUE_SIZE="${BID_LANE_QUEUE_SIZE:-2048}"
BID_LANE_QUEUE_TIMEOUT="${BID_LANE_QUEUE_TIMEOUT:-3s}"
REDIS_ENGINE_SETTLEMENT_WORKERS="${REDIS_ENGINE_SETTLEMENT_WORKERS:-1}"
OTEL_TRACES_ENABLED="${OTEL_TRACES_ENABLED:-false}"
OTEL_EXPORTER_OTLP_ENDPOINT="${OTEL_EXPORTER_OTLP_ENDPOINT:-localhost:4318}"
OTEL_EXPORTER_OTLP_TIMEOUT="${OTEL_EXPORTER_OTLP_TIMEOUT:-3s}"
OTEL_TRACES_SAMPLER_RATIO="${OTEL_TRACES_SAMPLER_RATIO:-1}"
OTEL_SERVICE_NAME="${OTEL_SERVICE_NAME:-live-auction-backend}"

if [ -z "$JMX_PATH_WAS_SET" ] && [ "${ALLOW_HISTORICAL_PTS:-}" != "1" ]; then
  echo "prepare-cloud-pressure.sh default JMX is archived historical: tests/pts/archive/historical/live-auction-core-pressure.jmx" >&2
  echo "Use tests/pts/reset-l4b-final-second-pressure.sh for current PTS-1A/PTS-1B." >&2
  echo "To run the historical default intentionally, set ALLOW_HISTORICAL_PTS=1." >&2
  exit 2
fi

mkdir -p "$OUT_DIR" "$RUNTIME_DIR"

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

echo "[1/7] Applying migrations to $DB_CONTAINER/$DB_NAME"
docker cp "$BOOTSTRAP_SQL" "$DB_CONTAINER:/tmp/live-auction-bootstrap-before-migrations.sql"
docker exec "$DB_CONTAINER" psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" -f /tmp/live-auction-bootstrap-before-migrations.sql
if ! command -v "$GOOSE_BIN" >/dev/null 2>&1; then
  if [ -x /root/go/bin/goose ]; then
    GOOSE_BIN=/root/go/bin/goose
  else
    echo "goose is required; install with: go install github.com/pressly/goose/v3/cmd/goose@latest" >&2
    exit 1
  fi
fi
"$GOOSE_BIN" -dir "$MIGRATIONS_DIR" postgres "$DATABASE_URL" up

echo "[2/7] Checking Redis"
docker exec "$REDIS_CONTAINER" redis-cli ping

echo "[2b/7] Checking Kafka topics"
docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --create --if-not-exists --topic "$KAFKA_BID_TOPIC" --partitions 16 --replication-factor 1
docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --create --if-not-exists --topic "$KAFKA_DLQ_TOPIC" --partitions 16 --replication-factor 1

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
} > "$OUT_DIR/$SESSION_CSV"

echo "[5/7] Building backend"
(
  cd "$BACKEND_DIR"
  go build -o "$RUNTIME_DIR/live-auction-server" ./cmd/server
)

echo "[6/7] Starting backend on $HTTP_ADDR"
if [ -f "$RUNTIME_DIR/server.pid" ]; then
  old_pid="$(cat "$RUNTIME_DIR/server.pid")"
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
  KAFKA_BROKERS="$KAFKA_BROKERS" \
  KAFKA_BID_TOPIC="$KAFKA_BID_TOPIC" \
  KAFKA_DLQ_TOPIC="$KAFKA_DLQ_TOPIC" \
  DB_MAX_CONNS="$DB_MAX_CONNS" \
  DB_MIN_CONNS="$DB_MIN_CONNS" \
  DB_MAX_CONN_LIFETIME="$DB_MAX_CONN_LIFETIME" \
  DB_MAX_CONN_IDLE_TIME="$DB_MAX_CONN_IDLE_TIME" \
  ALLOW_MOCK_AUTH="$ALLOW_MOCK_AUTH" \
  ADMISSION_ENABLED=false \
  BID_LANE_WORKERS="$BID_LANE_WORKERS" \
  BID_LANE_QUEUE_SIZE="$BID_LANE_QUEUE_SIZE" \
  BID_LANE_QUEUE_TIMEOUT="$BID_LANE_QUEUE_TIMEOUT" \
  REDIS_ENGINE_SETTLEMENT_WORKERS="$REDIS_ENGINE_SETTLEMENT_WORKERS" \
  OTEL_TRACES_ENABLED="$OTEL_TRACES_ENABLED" \
  OTEL_EXPORTER_OTLP_ENDPOINT="$OTEL_EXPORTER_OTLP_ENDPOINT" \
  OTEL_EXPORTER_OTLP_TIMEOUT="$OTEL_EXPORTER_OTLP_TIMEOUT" \
  OTEL_TRACES_SAMPLER_RATIO="$OTEL_TRACES_SAMPLER_RATIO" \
  REDIS_POOL_SIZE="$REDIS_POOL_SIZE" \
  OTEL_SERVICE_NAME="$OTEL_SERVICE_NAME" \
  SESSION_TTL=12h \
  OUTBOX_WORKER_ID=pts-cloud-1 \
  SCHEDULER_WORKER_ID=pts-cloud-1 \
  setsid "$RUNTIME_DIR/live-auction-server" > "$RUNTIME_DIR/server.log" 2> "$RUNTIME_DIR/server.err.log" < /dev/null &
  echo $! > "$RUNTIME_DIR/server.pid"
)

echo "[7/7] Verifying backend"
sleep 1
curl -fsS "http://127.0.0.1:${HTTP_PORT}/readyz"
curl -fsS "http://127.0.0.1:${HTTP_PORT}/metrics" | grep 'auction_admission_enabled 0'
ss -lntpH "sport = :$HTTP_PORT" | grep -E '(\*:|0\.0\.0\.0:|\[::\]:)'"$HTTP_PORT"

echo "Prepared:"
echo "- Backend: http://47.113.223.90:${HTTP_PORT}"
echo "- JMX: $JMX_PATH"
echo "- CSV: $OUT_DIR/$SESSION_CSV"
echo "- Runtime: $RUNTIME_DIR"
echo "- Logs: $RUNTIME_DIR/server.log and $RUNTIME_DIR/server.err.log"
echo "- Exploration profile: redis_ledger + kafka_ack, ADMISSION_ENABLED=false REDIS_ADDR=$REDIS_ADDR KAFKA_BROKERS=$KAFKA_BROKERS"
echo "- Tracing: OTEL_TRACES_ENABLED=$OTEL_TRACES_ENABLED OTEL_EXPORTER_OTLP_ENDPOINT=$OTEL_EXPORTER_OTLP_ENDPOINT OTEL_TRACES_SAMPLER_RATIO=$OTEL_TRACES_SAMPLER_RATIO"
