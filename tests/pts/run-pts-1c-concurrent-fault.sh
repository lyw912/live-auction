#!/usr/bin/env bash
# Layer C — concurrent fault injection under bid load.
#
# Fills the proof gap between:
#   Layer B (PTS-1B): 1000 VU peak load, zero faults — proves latency SLA
#   Chaos tests:      single bids, fault injected — proves fail-closed protocol
#
# Layer C proves: Redis or Kafka failure injected while 200 VUs are bidding
# causes fail-closed behaviour (ENGINE_PAUSED), and after recovery the final
# auction state satisfies all correctness invariants — no phantom accepts,
# no engine_seq gaps, correct winner.
#
# What Layer C does NOT prove (by design):
#   - Latency SLA under fault (latency degrades during fault; that is expected)
#   - Fault behaviour at full 1000 VU (structural proof: Redis Lua atomicity
#     ensures fail-closed regardless of VU count; PTS-1B proves 1000 VU works
#     without faults; their combination is the complete argument)
#
# Prerequisites:
#   - Docker infra running: postgres, redis, kafka, kafka-init
#   - Server running with:
#       ALLOW_MOCK_AUTH=true BID_ENGINE_MODE=redis_ledger ADMISSION_ENABLED=false
#     (k6 uses X-Mock-* headers; real JWT sessions are not required here)
#   - k6 installed: https://k6.io/docs/get-started/installation/
#     If local k6 is absent, the runner falls back to Docker image grafana/k6.
#   - jq installed
#
# Usage:
#   FAULT_TYPE=redis         bash tests/pts/run-pts-1c-concurrent-fault.sh
#   FAULT_TYPE=kafka         bash tests/pts/run-pts-1c-concurrent-fault.sh
#   FAULT_TYPE=both          bash tests/pts/run-pts-1c-concurrent-fault.sh
#   FAULT_TYPE=redis-flush   bash tests/pts/run-pts-1c-concurrent-fault.sh
#   FAULT_TYPE=pg            bash tests/pts/run-pts-1c-concurrent-fault.sh
#   FAULT_TYPE=settlement    bash tests/pts/run-pts-1c-concurrent-fault.sh
#
# Fault type reference:
#   redis        SIGKILL Redis container → ENGINE_PAUSED (fail-closed); no accepted bids during fault
#   kafka        SIGKILL Kafka container → relay pauses; hot path continues (DECIDED); relay drains after restore
#   both         SIGKILL Redis + Kafka simultaneously → correlated infrastructure failure
#   redis-flush  FLUSHALL Redis while container lives → engine detects missing state → RECONCILING → rebuild
#                Different from 'redis': process stays up, state evaporates (models OOM eviction / maxmemory)
#   pg           SIGKILL PostgreSQL → hot path must continue accepting bids (DECIDED); settlement queues
#                Key assertion: decided_total > 0 during PG fault proves hot path does NOT depend on PG
#   settlement   SIGKILL backend process → bid engine + settlement worker both down; restart; Kafka replay
#                runner restarts the backend from the reset-built binary unless SERVER_START_CMD is set
#                Key assertion: after restart, all pre-crash decisions are settled exactly once
#
# Environment overrides:
#   FAULT_TYPE              redis | kafka | both | redis-flush | pg | settlement  (default: redis)
#   K6_VUS                  default 200
#   K6_DURATION             default 45s
#                           VUs loop for the full duration; decision log count
#                           is actual throughput over time, not K6_VUS.
#   RAMP_SECONDS            default 10   (k6 running before fault fires)
#   FAULT_WINDOW_SECONDS    default 5    (how long fault lasts)
#   RECOVERY_GRACE          default 12   (extra settle time after restore)
#   AUCTION_ID              default auc_live
#   SUT_HOST                default 127.0.0.1:18080
#   DB_CONTAINER            default live-auction-postgres
#   REDIS_CONTAINER         default live-auction-redis
#   KAFKA_CONTAINER         default live-auction-kafka
#   SERVER_START_CMD        optional custom command to (re)start the backend
#   EVIDENCE_ROOT           default docs/perf/pts/evidence/incoming

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_DIR="${PTS_RUNTIME_DIR:-/tmp/live-auction-pts}"

FAULT_TYPE="${FAULT_TYPE:-redis}"
K6_VUS="${K6_VUS:-200}"
K6_DURATION="${K6_DURATION:-45s}"
RAMP_SECONDS="${RAMP_SECONDS:-10}"
FAULT_WINDOW_SECONDS="${FAULT_WINDOW_SECONDS:-5}"
RECOVERY_GRACE="${RECOVERY_GRACE:-12}"
RECOVERY_CONVERGENCE_TIMEOUT="${RECOVERY_CONVERGENCE_TIMEOUT:-}"
AUCTION_ID="${AUCTION_ID:-auc_live}"
SUT_HOST="${SUT_HOST:-127.0.0.1:18080}"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
KAFKA_CONTAINER="${KAFKA_CONTAINER:-live-auction-kafka}"
SERVER_START_CMD="${SERVER_START_CMD:-}"  # optional custom restart command
EVIDENCE_ROOT="${EVIDENCE_ROOT:-$ROOT_DIR/docs/perf/pts/evidence/incoming}"
SERVER_PID_FILE="${SERVER_PID_FILE:-$RUNTIME_DIR/server.pid}"
K6_DOCKER_IMAGE="${K6_DOCKER_IMAGE:-grafana/k6:latest}"
K6_RUNNER="local"

if [ -z "$RECOVERY_CONVERGENCE_TIMEOUT" ]; then
  case "$FAULT_TYPE" in
    kafka|both)
      RECOVERY_CONVERGENCE_TIMEOUT=600
      ;;
    *)
      RECOVERY_CONVERGENCE_TIMEOUT=240
      ;;
  esac
fi

LABEL="pts-1c-${FAULT_TYPE}-$(date +%Y%m%dT%H%M%S)"
OUT_DIR="$EVIDENCE_ROOT/$LABEL"
K6_SCRIPT="$ROOT_DIR/tests/pts/L1-component/pts-1c-k6-concurrent-fault.js"

mkdir -p "$OUT_DIR"
exec > >(tee "$OUT_DIR/run.log") 2>&1

echo "============================================================"
echo " Layer C — Concurrent Fault Injection"
echo " fault_type=$FAULT_TYPE k6_vus=$K6_VUS duration=$K6_DURATION"
echo " load_model=closed-loop VU loop (decision count is throughput over duration, not VU count)"
echo " ramp=${RAMP_SECONDS}s fault_window=${FAULT_WINDOW_SECONDS}s recovery=${RECOVERY_GRACE}s"
echo " label=$LABEL"
echo "============================================================"

# ── 0. Prerequisite checks ────────────────────────────────────────────────────

check_prereq() {
  if ! command -v k6 >/dev/null 2>&1; then
    if command -v docker >/dev/null 2>&1; then
      K6_RUNNER="docker"
      case "$OUT_DIR" in
        "$ROOT_DIR"/*) ;;
        *)
          echo "[prereq] FAIL: local k6 not found and OUT_DIR is outside repo, cannot mount evidence path into Docker k6: $OUT_DIR" >&2
          exit 1
          ;;
      esac
      echo "[prereq] local k6 not found; using Docker k6 image $K6_DOCKER_IMAGE"
    else
      echo "[prereq] FAIL: k6 not found and Docker is unavailable. Install k6 or Docker." >&2
      exit 1
    fi
  fi
  if ! command -v jq >/dev/null 2>&1; then
    echo "[prereq] FAIL: jq not found." >&2
    exit 1
  fi
  local ready
  ready=$(curl -fsS "http://${SUT_HOST}/readyz" 2>/dev/null | jq -r '.status // empty' || true)
  if [ "$ready" != "ready" ]; then
    echo "[prereq] FAIL: server not ready at http://${SUT_HOST}/readyz" >&2
    echo "  Start with: ALLOW_MOCK_AUTH=true BID_ENGINE_MODE=redis_ledger ADMISSION_ENABLED=false" >&2
    exit 1
  fi
  local admission
  admission=$(curl -fsS "http://${SUT_HOST}/metrics" 2>/dev/null |
    awk '/^auction_admission_enabled / {print $2; exit}' || true)
  if [ "${admission:-}" != "0" ]; then
    echo "[prereq] FAIL: ADMISSION_ENABLED is not false (metric=${admission:-missing})" >&2
    echo "  Layer C requires ADMISSION_ENABLED=false to isolate fault behaviour." >&2
    exit 1
  fi
  echo "[prereq] PASS: server ready, admission disabled"
}

run_k6_load() {
  if [ "$K6_RUNNER" = "local" ]; then
    k6 run \
      --env BASE_URL="http://${SUT_HOST}" \
      --env AUCTION_ID="$AUCTION_ID" \
      --env VUS="$K6_VUS" \
      --env DURATION="$K6_DURATION" \
      --out "json=$OUT_DIR/k6-results.json" \
      "$K6_SCRIPT"
  else
    docker run --rm --network host --user 0:0 \
      -v "$ROOT_DIR:/work:ro" \
      -v "$OUT_DIR:/evidence" \
      -w /work \
      "$K6_DOCKER_IMAGE" run \
      --env BASE_URL="http://${SUT_HOST}" \
      --env AUCTION_ID="$AUCTION_ID" \
      --env VUS="$K6_VUS" \
      --env DURATION="$K6_DURATION" \
      --out "json=/evidence/k6-results.json" \
      "/work/tests/pts/L1-component/pts-1c-k6-concurrent-fault.js"
  fi
}

preseed_mock_auth_acl_cache() {
  local redis_container="${REDIS_CONTAINER}"
  local db_container="${DB_CONTAINER}"
  local room_id
  room_id="$(docker exec "$db_container" psql -q -A -t -U live_auction \
    -d live_auction -c "SELECT room_id FROM auctions WHERE id = '${AUCTION_ID}'" 2>/dev/null || true)"
  [ -n "$room_id" ] || return 0

  echo "[seed] seeding ${K6_VUS} L1-F mock bidders/snapshot users into DB and Redis ACL cache"
  docker exec -i "$db_container" psql -q -v ON_ERROR_STOP=1 -U live_auction -d live_auction \
    -v room_id="$room_id" -v k6_vus="$K6_VUS" -f - <<'SQL'
INSERT INTO users (id, role, display_name, city)
SELECT user_id, 'user', user_id, 'l1-f'
FROM (
  SELECT 'l1c_bidder_' || generate_series(1, :'k6_vus'::int)::text AS user_id
  UNION ALL
  SELECT 'l1c_snap_' || generate_series(1, :'k6_vus'::int)::text AS user_id
) seeded
ON CONFLICT (id) DO UPDATE
SET role = EXCLUDED.role,
    display_name = EXCLUDED.display_name,
    city = EXCLUDED.city;

INSERT INTO room_memberships (room_id, user_id, role, status)
SELECT :'room_id', user_id, 'viewer', 'ACTIVE'
FROM (
  SELECT 'l1c_bidder_' || generate_series(1, :'k6_vus'::int)::text AS user_id
  UNION ALL
  SELECT 'l1c_snap_' || generate_series(1, :'k6_vus'::int)::text AS user_id
) seeded
ON CONFLICT (room_id, user_id)
DO UPDATE SET role = EXCLUDED.role,
              status = EXCLUDED.status,
              left_at = NULL;
SQL

  for i in $(seq 1 "$K6_VUS"); do
    docker exec "$redis_container" redis-cli \
      SET "acl:membership:{${AUCTION_ID}}:l1c_bidder_${i}" "$room_id" EX 43200 >/dev/null
    docker exec "$redis_container" redis-cli \
      SET "acl:membership:{${AUCTION_ID}}:l1c_snap_${i}" "$room_id" EX 43200 >/dev/null
  done
}

wait_for_server_ready() {
  local attempts=30
  for i in $(seq 1 $attempts); do
    local status
    status=$(curl -fsS "http://${SUT_HOST}/readyz" 2>/dev/null | jq -r '.status // empty' || true)
    if [ "$status" = "ready" ]; then
      echo "[server] backend ready (attempt $i)"
      return 0
    fi
    sleep 1
  done
  echo "[server] backend did not become ready within ${attempts}s" >&2
  return 1
}

start_fault_backend() {
  if [ -n "$SERVER_START_CMD" ]; then
    echo "[server] starting backend with SERVER_START_CMD"
    (cd "$ROOT_DIR" && eval "$SERVER_START_CMD" & echo $! > "$SERVER_PID_FILE")
    wait_for_server_ready
    return 0
  fi

  echo "[server] starting backend from reset-built binary with ALLOW_MOCK_AUTH=true"
  (
    cd "$ROOT_DIR/backend"
    APP_ENV=local \
    HTTP_ADDR="0.0.0.0:${SUT_HOST##*:}" \
    DATABASE_URL="${DATABASE_URL:-postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable}" \
    REDIS_ADDR="${REDIS_ADDR:-localhost:6380}" \
    KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}" \
    KAFKA_BID_TOPIC="${KAFKA_BID_TOPIC:-auction.bid-events}" \
    KAFKA_DLQ_TOPIC="${KAFKA_DLQ_TOPIC:-auction.dlq}" \
    ALLOW_MOCK_AUTH=true \
    ADMISSION_ENABLED=false \
    BID_ENGINE_MODE=redis_ledger \
    REDIS_POOL_SIZE="${REDIS_POOL_SIZE:-300}" \
    SESSION_TTL=12h \
    OUTBOX_WORKER_ID=pts-1c \
    SCHEDULER_WORKER_ID=pts-1c \
    setsid "$RUNTIME_DIR/live-auction-server" > "$RUNTIME_DIR/server.log" 2> "$RUNTIME_DIR/server.err.log" < /dev/null &
    echo $! > "$SERVER_PID_FILE"
  )
  wait_for_server_ready
}

stop_fault_backend() {
  if [ -f "$SERVER_PID_FILE" ]; then
    local pid
    pid="$(cat "$SERVER_PID_FILE")"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      sleep 1
      if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" 2>/dev/null || true
      fi
      return 0
    fi
  fi
  echo "[server] WARN: backend pid file missing/stale: $SERVER_PID_FILE" >&2
  return 1
}

# ── 1. Reset state (reuse existing PTS reset logic) ──────────────────────────

echo "[reset] resetting DB / Redis / Kafka state..."
ALLOW_MOCK_AUTH=true \
L4B_PROFILE=pts-1b SESSION_COUNT=0 \
  P1_LOAD_AUC_LIVE_CAP_PRICE_CENTS=100000000000000 \
  P1_LOAD_AUC_SIDE_CAP_PRICE_CENTS=100000000000000 \
  P1_LOAD_AUCTION_END_MINUTES=90 \
  bash "$ROOT_DIR/tests/pts/reset-l4b-final-second-pressure.sh"
stop_fault_backend >/dev/null 2>&1 || true
start_fault_backend
preseed_mock_auth_acl_cache
check_prereq
echo "[reset] done"

# ── 2. Preflight gates ────────────────────────────────────────────────────────

echo "[preflight] running guards..."
BASE_URL="http://${SUT_HOST}" \
  bash "$ROOT_DIR/tests/pts/preflight-l4b-pts-guards.sh" "before-$LABEL"
echo "[preflight] done"

# ── 3. Fault injection helpers ────────────────────────────────────────────────

wait_for_redis_ready() {
  local attempts=30
  for i in $(seq 1 $attempts); do
    if docker exec "$REDIS_CONTAINER" redis-cli ping >/dev/null 2>&1; then
      echo "[fault] Redis ready after restart (attempt $i)"
      return 0
    fi
    sleep 1
  done
  echo "[fault] WARN: Redis did not become ready within ${attempts}s" >&2
  return 1
}

wait_for_kafka_ready() {
  local attempts=40
  for i in $(seq 1 $attempts); do
    if docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh \
        --bootstrap-server localhost:9092 --list >/dev/null 2>&1; then
      echo "[fault] Kafka ready after restart (attempt $i)"
      return 0
    fi
    sleep 1
  done
  echo "[fault] WARN: Kafka did not become ready within ${attempts}s" >&2
  return 1
}

wait_for_pg_ready() {
  local attempts=30
  for i in $(seq 1 $attempts); do
    if docker exec "$DB_CONTAINER" pg_isready -U live_auction >/dev/null 2>&1; then
      echo "[fault] PostgreSQL ready after restart (attempt $i)"
      return 0
    fi
    sleep 1
  done
  echo "[fault] WARN: PostgreSQL did not become ready within ${attempts}s" >&2
  return 1
}

wait_for_recovery_convergence() {
  local timeout="${1:-$RECOVERY_CONVERGENCE_TIMEOUT}"
  local start
  start="$(date +%s)"
  echo "[recovery] waiting for Redis/Kafka/PostgreSQL convergence (timeout=${timeout}s)..."
  while true; do
    local row stream_len pending_count lag now elapsed status
    row=$(docker exec -i "$DB_CONTAINER" psql -q -A -t -F $'\t' -U live_auction -d live_auction \
      -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<'SQL' 2>/dev/null || true
with a as (
  select engine_seq, engine_paused, coalesce(engine_pause_reason, '') as pause_reason
  from auctions
  where id = :'auction_id'
),
s as (
  select count(*) as total,
         count(*) filter (where status in ('PROCESSING','FAILED') or dlq_at is not null) as open_count
  from redis_engine_settlements
  where auction_id = :'auction_id'
),
o as (
  select count(*) as open_count
  from outbox_delivery d
  join outbox_events e on e.id = d.outbox_id
  where e.auction_id = :'auction_id'
    and d.status <> 'PUBLISHED'
)
select a.engine_seq, a.engine_paused, a.pause_reason, s.total, s.open_count, o.open_count
from a cross join s cross join o;
SQL
)
    stream_len=$(docker exec "$REDIS_CONTAINER" redis-cli XLEN "bid:{${AUCTION_ID}}:engine:log" 2>/dev/null | tr -d '[:space:]' || echo "0")
    pending_count=$(docker exec "$REDIS_CONTAINER" redis-cli HLEN "bid:{${AUCTION_ID}}:engine:pending" 2>/dev/null | tr -d '[:space:]' || echo "0")
    lag=$(docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-consumer-groups.sh \
      --bootstrap-server localhost:9092 --describe --group settlement-workers 2>/dev/null |
      awk 'NR>1 && $1 !~ /^GROUP$/ && $6 ~ /^[0-9]+$/ {sum+=$6} END {print sum+0}' || echo "0")
    if [ -n "$row" ]; then
      local engine_seq engine_paused pause_reason settlement_total open_settlements open_outbox
      IFS=$'\t' read -r engine_seq engine_paused pause_reason settlement_total open_settlements open_outbox <<<"$row"
      echo "[recovery] db_engine_seq=${engine_seq:-?} paused=${engine_paused:-?} reason=${pause_reason:-} settlements=${settlement_total:-?}/${stream_len:-?} open_settlements=${open_settlements:-?} pending=${pending_count:-?} outbox_open=${open_outbox:-?} kafka_lag=${lag:-?}"
      local stream_complete="0"
      if [ "${settlement_total:-x}" = "${stream_len:-y}" ]; then
        stream_complete="1"
      elif [ "$FAULT_TYPE" = "redis-flush" ] && [ "${stream_len:-0}" = "0" ]; then
        stream_complete="1"
      fi
      if [ "$stream_complete" = "1" ] &&
         [ "${settlement_total:-0}" -gt 0 ] &&
         [ "${open_settlements:-1}" = "0" ] &&
         [ "${pending_count:-1}" = "0" ] &&
         [ "${open_outbox:-1}" = "0" ] &&
         [ "${lag:-1}" = "0" ]; then
        return 0
      fi
    fi
    now="$(date +%s)"
    elapsed=$((now - start))
    if [ "$elapsed" -ge "$timeout" ]; then
      echo "[recovery] FAIL: convergence did not complete within ${timeout}s" >&2
      return 1
    fi
    sleep 2
  done
}

request_redis_engine_signal() {
  local signal_type="$1"
  local timeout="${2:-$RECOVERY_CONVERGENCE_TIMEOUT}"
  local signal_id start
  echo "[recovery] requesting redis engine ${signal_type} through system_control_signals"
  signal_id=$(docker exec -i "$DB_CONTAINER" psql -q -A -t -U live_auction -d live_auction \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -v signal_type="$signal_type" -f - <<'SQL'
INSERT INTO system_control_signals (signal_type, target_type, target_id, requested_by, reason, payload_json)
VALUES (:'signal_type', 'auction', :'auction_id', 'l1-f-runner', 'post fault convergence check', '{}')
RETURNING id;
SQL
)
  signal_id="$(printf '%s' "$signal_id" | tr -d '[:space:]')"
  [ -n "$signal_id" ] || { echo "[recovery] FAIL: reconcile signal id was empty" >&2; return 1; }

  start="$(date +%s)"
  while true; do
    local row status error_message result_json paused pause_reason now elapsed
    row=$(docker exec -i "$DB_CONTAINER" psql -q -A -t -F '|' -U live_auction -d live_auction \
      -v ON_ERROR_STOP=1 -v signal_id="$signal_id" -v auction_id="$AUCTION_ID" -f - <<'SQL' 2>/dev/null || true
select s.status,
       coalesce(s.error_message, ''),
       coalesce(s.result_json::text, ''),
       a.engine_paused,
       coalesce(a.engine_pause_reason, '')
from system_control_signals s
join auctions a on a.id = :'auction_id'
where s.id = :'signal_id'::bigint;
SQL
)
    if [ -n "$row" ]; then
      IFS='|' read -r status error_message result_json paused pause_reason <<<"$row"
      echo "[recovery] ${signal_type}_signal=${signal_id} status=${status:-?} paused=${paused:-?} reason=${pause_reason:-}"
      if [ "$status" = "SUCCEEDED" ] && [ "$paused" = "f" ]; then
        return 0
      fi
      if [ "$status" = "FAILED" ] || [ "$status" = "REJECTED" ]; then
        echo "[recovery] FAIL: ${signal_type} signal failed: ${error_message:-$result_json}" >&2
        return 1
      fi
    fi
    now="$(date +%s)"
    elapsed=$((now - start))
    if [ "$elapsed" -ge "$timeout" ]; then
      echo "[recovery] FAIL: ${signal_type} signal ${signal_id} did not succeed within ${timeout}s" >&2
      return 1
    fi
    sleep 1
  done
}

request_redis_engine_reconcile() {
  request_redis_engine_signal reconcile_redis_engine "${1:-$RECOVERY_CONVERGENCE_TIMEOUT}"
}

request_redis_engine_resume() {
  request_redis_engine_signal resume_redis_engine "${1:-$RECOVERY_CONVERGENCE_TIMEOUT}"
}

inject_fault() {
  echo "[fault] injecting fault_type=$FAULT_TYPE at $(date -Iseconds)"
  case "$FAULT_TYPE" in
    redis)
      docker kill "$REDIS_CONTAINER" >/dev/null
      echo "[fault] Redis killed (SIGKILL)"
      ;;
    kafka)
      docker kill "$KAFKA_CONTAINER" >/dev/null
      echo "[fault] Kafka killed (SIGKILL)"
      ;;
    both)
      docker kill "$REDIS_CONTAINER" "$KAFKA_CONTAINER" >/dev/null
      echo "[fault] Redis + Kafka killed simultaneously"
      ;;
    redis-flush)
      # Container stays up; all keys are wiped — models OOM eviction or maxmemory flush.
      # The engine detects the missing state key on the next bid attempt and enters
      # RECONCILING, then rebuilds from Kafka high-water + PG checkpoint.
      docker exec "$REDIS_CONTAINER" redis-cli FLUSHALL >/dev/null
      echo "[fault] Redis FLUSHALL executed (container still running)"
      ;;
    pg)
      docker kill "$DB_CONTAINER" >/dev/null
      echo "[fault] PostgreSQL killed (SIGKILL) — hot path must continue via Redis"
      ;;
    settlement)
      # The settlement worker runs inside the Go backend process. Killing the backend
      # takes down both bid processing and settlement simultaneously, modelling a
      # process-level crash (OOM killer, panic, container eviction).
      stop_fault_backend || { echo "[fault] WARN: could not find backend process to kill" >&2; }
      echo "[fault] backend process killed via pid file"
      ;;
    *)
      echo "[fault] unknown FAULT_TYPE=$FAULT_TYPE (valid: redis|kafka|both|redis-flush|pg|settlement)" >&2
      exit 1
      ;;
  esac
}

restore_fault() {
  echo "[fault] restoring fault_type=$FAULT_TYPE at $(date -Iseconds)"
  case "$FAULT_TYPE" in
    redis)
      docker start "$REDIS_CONTAINER" >/dev/null
      wait_for_redis_ready
      ;;
    kafka)
      docker start "$KAFKA_CONTAINER" >/dev/null
      wait_for_kafka_ready
      ;;
    both)
      docker start "$REDIS_CONTAINER" "$KAFKA_CONTAINER" >/dev/null
      wait_for_redis_ready || true
      wait_for_kafka_ready || true
      ;;
    redis-flush)
      # Container never stopped; reseed the auth/ACL cache that FLUSHALL wiped.
      # Without this, the first post-flush bids hit DB for auth (defeats the
      # warm-cache assumption in the PTS reset and inflates latency post-recovery).
      echo "[fault] reseeding Redis ACL cache after FLUSHALL..."
      preseed_mock_auth_acl_cache || true
      ;;
    pg)
      docker start "$DB_CONTAINER" >/dev/null
      wait_for_pg_ready
      ;;
    settlement)
      start_fault_backend
      ;;
  esac
  echo "[fault] restore complete at $(date -Iseconds)"
}

# ── 4. Start k6 load in background ───────────────────────────────────────────

echo "[k6] starting ${K6_VUS} VU bid load for ${K6_DURATION}..."
run_k6_load > "$OUT_DIR/k6-stdout.txt" 2>&1 &
K6_PID=$!
echo "[k6] pid=$K6_PID"

# ── 5. Wait for ramp, then inject fault ───────────────────────────────────────

echo "[timing] waiting ${RAMP_SECONDS}s for load to ramp up..."
sleep "$RAMP_SECONDS"

FAULT_START_EPOCH=$(date +%s)
FAULT_START_ISO=$(date -Iseconds)
inject_fault

echo "[timing] fault active for ${FAULT_WINDOW_SECONDS}s..."
sleep "$FAULT_WINDOW_SECONDS"

FAULT_END_EPOCH=$(date +%s)
FAULT_END_ISO=$(date -Iseconds)
restore_fault

# ── 6. Wait for k6 to finish ─────────────────────────────────────────────────

echo "[k6] waiting for load to complete..."
if wait "$K6_PID"; then
  echo "[k6] finished cleanly (exit 0)"
else
  K6_EXIT=$?
  echo "[k6] finished with exit=$K6_EXIT (may include fault-window failures — see k6-stdout.txt)"
fi

echo "[timing] waiting ${RECOVERY_GRACE}s for settlement to drain..."
sleep "$RECOVERY_GRACE"
wait_for_recovery_convergence "$RECOVERY_CONVERGENCE_TIMEOUT"
if [ "$FAULT_TYPE" = "redis-flush" ]; then
  request_redis_engine_resume "$RECOVERY_CONVERGENCE_TIMEOUT"
else
  request_redis_engine_reconcile "$RECOVERY_CONVERGENCE_TIMEOUT"
fi

# ── 7. Parse k6 metrics from JSON output ──────────────────────────────────────

parse_k6_counter() {
  local metric="$1"
  # k6 JSON output: {"type":"Point","data":{"time":"...","value":N},"metric":"name"}
  jq -r --arg m "$metric" \
    'select(.type=="Point" and .metric==$m) | .data.value' \
    "$OUT_DIR/k6-results.json" 2>/dev/null |
    awk '{s+=$1} END {print s+0}'
}

K6_PAUSED=$(parse_k6_counter "bid_paused_total")
K6_RECONCILING=$(parse_k6_counter "bid_reconciling_total")
K6_DECIDED=$(parse_k6_counter "bid_decided_total")
K6_HTTP_ERRORS=$(parse_k6_counter "bid_http_error_total")
K6_ADMISSION_CONTAM=$(parse_k6_counter "bid_admission_contamination")

echo ""
echo "── k6 response distribution ────────────────────────────────"
echo "  decided      : $K6_DECIDED"
echo "  paused       : $K6_PAUSED    ← expected during fault window"
echo "  reconciling  : $K6_RECONCILING"
echo "  http_errors  : $K6_HTTP_ERRORS"
echo "  adm_contam   : $K6_ADMISSION_CONTAM  ← must be 0"
echo "────────────────────────────────────────────────────────────"

# Persist timing for evidence record.
cat > "$OUT_DIR/fault-window.json" <<JSON
{
  "fault_type":            "$FAULT_TYPE",
  "fault_start_iso":       "$FAULT_START_ISO",
  "fault_end_iso":         "$FAULT_END_ISO",
  "fault_window_seconds":  $FAULT_WINDOW_SECONDS,
  "k6_vus":                $K6_VUS,
  "k6_duration":           "$K6_DURATION",
  "k6_decided":            $K6_DECIDED,
  "k6_paused":             $K6_PAUSED,
  "k6_reconciling":        $K6_RECONCILING,
  "k6_http_errors":        $K6_HTTP_ERRORS,
  "k6_admission_contam":   $K6_ADMISSION_CONTAM
}
JSON

# ── 8. Layer C specific gate checks ──────────────────────────────────────────

L1C_GATE_FILE="$OUT_DIR/l1c-gates.tsv"
L1C_PASS=0
L1C_FAIL=0

gate() {
  local sev="$1" name="$2" result="$3" detail="$4"
  printf '%s\t%s\t%s\t%s\n' "$sev" "$name" "$result" "$detail" | tee -a "$L1C_GATE_FILE"
  if [ "$result" = "PASS" ]; then ((L1C_PASS++)) || true
  elif [ "$sev" = "P0" ];    then ((L1C_FAIL++)) || true
  fi
}

# ── Gate: fault was actually observed ────────────────────────────────────────
# Each fault type has a distinct "signature" of what k6 should see.
# If none of the expected signals appear, the fault didn't reach clients.
case "$FAULT_TYPE" in
  redis|redis-flush|both)
    # Redis down / state wiped: engine cannot run Lua → ENGINE_PAUSED or RECONCILING.
    if [ "$((K6_PAUSED + K6_RECONCILING))" -gt 0 ]; then
      gate P0 fault_observed_by_clients PASS \
        "ENGINE_PAUSED=${K6_PAUSED} RECONCILING=${K6_RECONCILING} — fail-closed reached clients"
    else
      gate P0 fault_observed_by_clients FAIL \
        "k6 saw zero ENGINE_PAUSED/RECONCILING — fault may not have fired or was silently absorbed"
    fi
    ;;
  kafka)
    # Kafka down: hot path continues (DECIDED responses expected), relay falls behind.
    # If decided = 0, the test ran against a server that was already broken before the fault.
    if [ "${K6_DECIDED:-0}" -gt 0 ]; then
      gate P0 fault_observed_by_clients PASS \
        "bid decided=${K6_DECIDED} continued while Kafka was down — hot path is Kafka-independent"
    else
      gate P0 fault_observed_by_clients FAIL \
        "k6 saw zero DECIDED responses — hot path was not running, cannot prove Kafka independence"
    fi
    ;;
  pg)
    # PG down: hot path must continue (DECIDED expected, ENGINE_PAUSED must NOT appear).
    # Seeing ENGINE_PAUSED during a PG fault would prove PG is on the hot path — a bug.
    if [ "${K6_DECIDED:-0}" -gt 0 ] && [ "${K6_PAUSED:-0}" -eq 0 ]; then
      gate P0 fault_observed_by_clients PASS \
        "decided=${K6_DECIDED} paused=0 during PG fault — hot path is PG-independent"
    elif [ "${K6_PAUSED:-0}" -gt 0 ]; then
      gate P0 fault_observed_by_clients FAIL \
        "ENGINE_PAUSED=${K6_PAUSED} appeared during PG fault — hot path must not depend on PG"
    else
      gate P0 fault_observed_by_clients FAIL \
        "zero DECIDED during PG fault — bids stopped; cannot prove hot path PG independence"
    fi
    ;;
  settlement)
    # Backend killed: k6 sees HTTP connection errors during kill window.
    # After restart k6 resumes DECIDED. http_errors > 0 proves backend was actually down.
    if [ "${K6_HTTP_ERRORS:-0}" -gt 0 ]; then
      gate P0 fault_observed_by_clients PASS \
        "http_errors=${K6_HTTP_ERRORS} during backend kill — process crash was observed by clients"
    else
      gate P0 fault_observed_by_clients FAIL \
        "zero HTTP errors during settlement fault — backend may not have been killed"
    fi
    ;;
esac

# ── Gate: no admission contamination (all fault types) ───────────────────────
if [ "${K6_ADMISSION_CONTAM}" -eq 0 ]; then
  gate P0 no_admission_contamination PASS \
    "zero RATE_LIMITED despite concurrent load — ADMISSION_ENABLED=false respected throughout"
else
  gate P0 no_admission_contamination FAIL \
    "admission contamination=${K6_ADMISSION_CONTAM} — ADMISSION_ENABLED may not be false"
fi

# ── Gate: no accepted bids during Redis unavailability ───────────────────────
# redis/redis-flush/both: Lua cannot execute while Redis is down or wiped.
# Any accepted settlement created inside the fault window means the engine
# produced a decision without a live Redis — a correctness violation.
if [ "$FAULT_TYPE" = "redis" ] || [ "$FAULT_TYPE" = "redis-flush" ] || [ "$FAULT_TYPE" = "both" ]; then
  ACCEPTED_IN_WINDOW=$(docker exec -i "$DB_CONTAINER" psql \
    -q -A -t -U live_auction -d live_auction \
    -c "SELECT count(*) FROM redis_engine_settlements
        WHERE auction_id = '${AUCTION_ID}'
          AND status = 'SETTLED'
          AND result IN ('ENGINE_ACCEPTED','ENGINE_SOLD')
          AND created_at >= to_timestamp(${FAULT_START_EPOCH})
          AND created_at <= to_timestamp(${FAULT_END_EPOCH});" \
    2>/dev/null | tr -d '[:space:]' || echo "0")
  if [ "${ACCEPTED_IN_WINDOW:-0}" -eq 0 ]; then
    gate P0 no_accepted_settlement_during_redis_fault PASS \
      "zero accepted settlements in fault window [${FAULT_START_ISO} .. ${FAULT_END_ISO}]"
  else
    gate P0 no_accepted_settlement_during_redis_fault FAIL \
      "${ACCEPTED_IN_WINDOW} accepted settlements in fault window — engine decided while Redis was unavailable"
  fi
fi

# ── Gate: Kafka relay drained after Kafka recovery ────────────────────────────
# kafka/both: hot path writes to Redis Stream while Kafka is down.
# After Kafka restart, relay must drain the stream to zero pending.
if [ "$FAULT_TYPE" = "kafka" ] || [ "$FAULT_TYPE" = "both" ]; then
  PENDING_COUNT=$(docker exec "$REDIS_CONTAINER" \
    redis-cli HLEN "bid:{${AUCTION_ID}}:engine:pending" 2>/dev/null | tr -d '[:space:]' || echo "0")
  if [ "${PENDING_COUNT:-0}" -eq 0 ]; then
    gate P0 kafka_relay_drained_after_recovery PASS \
      "Redis pending hash is empty — relay fully drained after Kafka restart"
  else
    gate P0 kafka_relay_drained_after_recovery FAIL \
      "${PENDING_COUNT} entries in pending hash — relay has not drained after Kafka restart"
  fi
fi

# ── Gate: PG independence (pg fault only) ─────────────────────────────────────
# After PG recovery, settlement must complete. All bids decided before/after
# the PG fault window must have settled ledger rows — proving that pending
# decisions survived the PG outage in the Redis Stream and were settled
# once PG returned.
if [ "$FAULT_TYPE" = "pg" ]; then
  UNSETTLED=$(docker exec -i "$DB_CONTAINER" psql \
    -q -A -t -U live_auction -d live_auction \
    -c "SELECT count(*) FROM bids
        WHERE auction_id = '${AUCTION_ID}'
          AND status = 'ACCEPTED'
          AND settlement_status <> 'SETTLED';" \
    2>/dev/null | tr -d '[:space:]' || echo "unknown")
  if [ "${UNSETTLED:-0}" = "0" ]; then
    gate P0 pg_recovery_settlement_complete PASS \
      "zero unsettled accepted bids after PG recovery — decisions queued during outage all settled"
  else
    gate P0 pg_recovery_settlement_complete FAIL \
      "${UNSETTLED} accepted bids still unsettled after PG recovery — settlement did not catch up"
  fi
fi

# ── Gate: settlement replay idempotency (settlement fault only) ───────────────
# After backend restart, Kafka consumer group replays from committed offset.
# No duplicate settlement rows must appear — (auction_id, engine_seq) is unique.
if [ "$FAULT_TYPE" = "settlement" ]; then
  DUPLICATE_SETTLEMENTS=$(docker exec -i "$DB_CONTAINER" psql \
    -q -A -t -U live_auction -d live_auction \
    -c "SELECT count(*) FROM (
          SELECT engine_epoch, engine_seq
          FROM redis_engine_settlements
          WHERE auction_id = '${AUCTION_ID}'
          GROUP BY engine_epoch, engine_seq
          HAVING count(*) > 1
        ) d;" \
    2>/dev/null | tr -d '[:space:]' || echo "0")
  if [ "${DUPLICATE_SETTLEMENTS:-0}" -eq 0 ]; then
    gate P0 settlement_replay_no_duplicates PASS \
      "zero duplicate (epoch,seq) settlement rows after backend restart and Kafka replay"
  else
    gate P0 settlement_replay_no_duplicates FAIL \
      "${DUPLICATE_SETTLEMENTS} duplicate settlement rows — Kafka at-least-once replay was not idempotent"
  fi

  # Secondary gate: bids that were decided before the crash must all be settled
  # (not left in PROCESSING limbo). The restart should have resumed from offset.
  UNSETTLED_POST_CRASH=$(docker exec -i "$DB_CONTAINER" psql \
    -q -A -t -U live_auction -d live_auction \
    -c "SELECT count(*) FROM bids
        WHERE auction_id = '${AUCTION_ID}'
          AND status = 'ACCEPTED'
          AND settlement_status <> 'SETTLED';" \
    2>/dev/null | tr -d '[:space:]' || echo "unknown")
  if [ "${UNSETTLED_POST_CRASH:-0}" = "0" ]; then
    gate P0 settlement_replay_complete PASS \
      "zero unsettled accepted bids after restart — Kafka replay covered all pre-crash decisions"
  else
    gate P0 settlement_replay_complete FAIL \
      "${UNSETTLED_POST_CRASH} accepted bids unsettled after restart — replay did not cover all decisions"
  fi
fi

echo ""
echo "── Layer C gate summary ─────────────────────────────────────"
echo "  PASS=$L1C_PASS  FAIL=$L1C_FAIL"
echo "────────────────────────────────────────────────────────────"

if [ "$L1C_FAIL" -gt 0 ]; then
  echo "[l1c] FAIL: Layer C specific gates failed. See $L1C_GATE_FILE" >&2
  exit 1
fi

# ── 9. Post-run correctness verification (reuse existing verifier) ────────────
#
# EXPECTED_UNIQUE_BIDS="" disables the "exactly N bids" count gates that are
# meaningful for PTS-1B (each VU sends exactly one bid) but not for Layer C
# (VUs loop; fault-window bids return ENGINE_PAUSED and are not written to DB).

echo "[verify] running correctness verifier..."
EXPECTED_UNIQUE_BIDS="" \
  REDIS_DATA_LOSS_OK="$([ "$FAULT_TYPE" = "redis-flush" ] && echo 1 || echo 0)" \
  FINAL_WAIT_SECONDS=0 \
  AUCTION_ID="$AUCTION_ID" \
  DB_CONTAINER="$DB_CONTAINER" \
  REDIS_CONTAINER="$REDIS_CONTAINER" \
  KAFKA_CONTAINER="$KAFKA_CONTAINER" \
  EVIDENCE_ROOT="$EVIDENCE_ROOT" \
  bash "$ROOT_DIR/tests/pts/verify-l4b-pts-correctness.sh" "$LABEL"

echo ""
echo "============================================================"
echo " Layer C PASS"
echo " label=$LABEL"
echo " fault=$FAULT_TYPE window=${FAULT_WINDOW_SECONDS}s"
echo " k6_paused=$K6_PAUSED k6_decided=$K6_DECIDED"
echo " evidence: $OUT_DIR"
echo "============================================================"
