#!/usr/bin/env bash
# run-s5-reconnect.sh — S5 断连重连 / Reconnect & Recovery runner
#
# Tests that a WebSocket client first connects, disconnects, misses real public
# seq updates, then reconnects with last_seq and catches up without gaps,
# duplicates, or stale truth.
#
# Two modes (controlled by DISCONNECT_MODE env):
#   clean   — clients disconnect cleanly and reconnect with stale seq (default)
#   network — Toxiproxy reset_peer simulates abrupt network drop (more realistic)
#
# Usage:
#   bash tests/pts/run-s5-reconnect.sh                                 # clean mode
#   DISCONNECT_MODE=network bash tests/pts/run-s5-reconnect.sh          # toxiproxy mode
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
WS_URL="${WS_URL:-}"
INITIAL_WS_URL="${INITIAL_WS_URL:-}"
DISCONNECT_MODE="${DISCONNECT_MODE:-clean}"
VUS="${VUS:-20}"
DURATION="${DURATION:-2m}"
MISSED_EVENTS="${MISSED_EVENTS:-3}"
BID_RATE_PER_S="${BID_RATE_PER_S:-10}"
BID_SOURCE_VUS="${BID_SOURCE_VUS:-5}"
SESSION_CSV="${SESSION_CSV:-../../tests/pts/inputs/s1-s5/s1-s5-1000-user-sessions.csv}"
K6_DOCKER_IMAGE="${K6_DOCKER_IMAGE:-grafana/k6:latest}"
LABEL="${LABEL:-s5-$(date +%Y%m%dT%H%M%S)}"
EVIDENCE_DIR="artifacts/pts/evidence/incoming/${LABEL}"

echo "=== S5 断连重连 — mode=${DISCONNECT_MODE}, VUs=${VUS}, duration=${DURATION} ==="
echo "    Each VU: initial connect → disconnect → miss ${MISSED_EVENTS} real seqs → reconnect with last_seq"
echo "    Measures: time-to-current-state p99, seq gaps, server-truth consistency"

mkdir -p "$EVIDENCE_DIR"

if [[ "$DISCONNECT_MODE" == "network" ]]; then
  echo "Starting Toxiproxy WS proxy on :18081 -> host.docker.internal:18080..."
  docker compose -f infra/docker-compose.yml -f infra/docker-compose.toxiproxy.yml up -d toxiproxy
  node tests/chaos/s5-toxiproxy-ws-fault.mjs inject | tee "$EVIDENCE_DIR/toxiproxy-ws.json"
  INITIAL_WS_URL="${INITIAL_WS_URL:-${BASE_URL/#http/ws}}"
  WS_URL="${WS_URL:-ws://127.0.0.1:18081}"
fi
WS_URL="${WS_URL:-${BASE_URL/#http/ws}}"
INITIAL_WS_URL="${INITIAL_WS_URL:-$WS_URL}"

curl -s "$BASE_URL/readyz" > "$EVIDENCE_DIR/readyz-before.json" || true
if command -v docker >/dev/null 2>&1; then
  docker exec live-auction-postgres psql -U live_auction -d live_auction -A -F ',' -c \
    "select event_type, payload_json->>'source' as source, count(*) from user_activity_events where created_at >= now() - interval '10 minutes' and event_type in ('ws_reconnect','ws_recovered','ws_slow_consumer_closed') group by 1,2 order by 1,2;" \
    > "$EVIDENCE_DIR/recovery-before.csv" 2>/dev/null || true
fi

cat > "$EVIDENCE_DIR/run-env.json" <<EOF
{
  "label": "$LABEL",
  "base_url": "$BASE_URL",
  "ws_url": "$WS_URL",
  "initial_ws_url": "$INITIAL_WS_URL",
  "disconnect_mode": "$DISCONNECT_MODE",
  "vus": $VUS,
  "duration": "$DURATION",
  "missed_events": $MISSED_EVENTS,
  "bid_rate_per_s": $BID_RATE_PER_S,
  "bid_source_vus": $BID_SOURCE_VUS,
  "session_csv": "$SESSION_CSV"
}
EOF

echo "Running S5 reconnect recovery test..."
K6_ARGS=(
  run
  --env "BASE_URL=$BASE_URL"
  --env "WS_URL=$WS_URL"
  --env "INITIAL_WS_URL=$INITIAL_WS_URL"
  --env "DISCONNECT_MODE=$DISCONNECT_MODE"
  --env "VUS=$VUS"
  --env "DURATION=$DURATION"
  --env "MISSED_EVENTS=$MISSED_EVENTS"
  --env "BID_RATE_PER_S=$BID_RATE_PER_S"
  --env "BID_SOURCE_VUS=$BID_SOURCE_VUS"
  --env "SESSION_CSV=$SESSION_CSV"
  --summary-export "$EVIDENCE_DIR/s5-k6-summary.json"
  --out "json=$EVIDENCE_DIR/s5-k6-reconnect.json"
  tests/load/s5-reconnect-recovery.js \
)

K6_EXIT=0
if command -v k6 >/dev/null 2>&1; then
  k6 "${K6_ARGS[@]}" || K6_EXIT=$?
else
  echo "   local k6 not found; using Docker image ${K6_DOCKER_IMAGE}"
  docker run --rm --network host --user "$(id -u):$(id -g)" \
    -v "$PWD:/work" -w /work \
    "$K6_DOCKER_IMAGE" "${K6_ARGS[@]}" || K6_EXIT=$?
fi

curl -s "$BASE_URL/readyz" > "$EVIDENCE_DIR/readyz-after.json" || true
if command -v docker >/dev/null 2>&1; then
  docker exec live-auction-postgres psql -U live_auction -d live_auction -A -F ',' -c \
    "select event_type, payload_json->>'source' as source, count(*) from user_activity_events where created_at >= now() - interval '10 minutes' and event_type in ('ws_reconnect','ws_recovered','ws_slow_consumer_closed') group by 1,2 order by 1,2;" \
    > "$EVIDENCE_DIR/recovery-after.csv" 2>/dev/null || true
fi

if [[ "$DISCONNECT_MODE" == "network" ]]; then
  node tests/chaos/s5-toxiproxy-ws-fault.mjs clear > "$EVIDENCE_DIR/toxiproxy-clear.json" || true
fi

echo "$K6_EXIT" > "$EVIDENCE_DIR/k6-exit.txt"

echo ""
echo "=== S5 done. Evidence: ${EVIDENCE_DIR}/ ==="
echo ""
echo "   Key metrics to report:"
echo "     s5_ttcs_ms p99        — time-to-current-state p99 (target ≤ 2s)"
echo "     s5_recovered_total    — connections that successfully caught up"
echo "     s5_recovery_errors    — must be 0"
echo "     s5_seq_gaps_after_reconnect — must be 0"
echo "     s5_duplicate_seq_after_reconnect — must be 0"
echo ""
echo "   Recovery source distribution is captured in recovery-before/after monitor snapshots."

exit "$K6_EXIT"
