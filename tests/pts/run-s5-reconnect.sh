#!/usr/bin/env bash
# run-s5-reconnect.sh — S5 断连重连 / Reconnect & Recovery runner
#
# Tests that a WebSocket client reconnecting with a stale last_seq correctly
# catches up to the current auction state without gaps, duplicates, or stale truth.
#
# Two modes (controlled by DISCONNECT_MODE env):
#   clean   — clients disconnect cleanly and reconnect with stale seq (default)
#   network — Toxiproxy reset_peer simulates abrupt network drop (more realistic)
#
# Toxiproxy setup for "network" mode (run before this script):
#   # Add Toxiproxy proxy for the WS port:
#   curl -s -X POST http://localhost:8474/proxies \
#     -d '{"name":"ws-port","listen":"0.0.0.0:18081","upstream":"127.0.0.1:18080"}'
#   # Then set DISCONNECT_MODE=network WS_URL=ws://127.0.0.1:18081
#
# Usage:
#   bash tests/pts/run-s5-reconnect.sh                                 # clean mode
#   DISCONNECT_MODE=network WS_URL=ws://127.0.0.1:18081 \
#     bash tests/pts/run-s5-reconnect.sh                               # toxiproxy mode
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
WS_URL="${WS_URL:-ws://127.0.0.1:18080}"
DISCONNECT_MODE="${DISCONNECT_MODE:-clean}"
VUS="${VUS:-20}"
DURATION="${DURATION:-2m}"
SEQ_LAG="${SEQ_LAG:-5}"     # simulated stale gap (client missed last N events)
LABEL="${LABEL:-s5-$(date +%Y%m%dT%H%M%S)}"

echo "=== S5 断连重连 — mode=${DISCONNECT_MODE}, VUs=${VUS}, duration=${DURATION} ==="
echo "    Each VU: initial connect → simulate disconnect → reconnect with last_seq=(current-${SEQ_LAG})"
echo "    Measures: time-to-current-state p99, seq gaps, server-truth consistency"

# Pre-check: ensure there are live price updates happening (so reconnect has something to catch up to)
echo ""
echo "Starting a background bid source to generate price updates..."
k6 run \
  --env BASE_URL="$BASE_URL" \
  --env STAGE1_RATE=3 --env STAGE2_RATE=3 --env STAGE3_RATE=3 \
  --env STAGE_DUR="$DURATION" \
  tests/load/s2-steady-soak.js \
  --no-summary &
BID_PID=$!
trap 'kill $BID_PID 2>/dev/null || true' EXIT

sleep 5   # let price move before reconnect wave starts
echo ""

mkdir -p "docs/perf/pts/evidence/incoming/${LABEL}"

echo "Running S5 reconnect recovery test..."
k6 run \
  --env BASE_URL="$BASE_URL" \
  --env WS_URL="$WS_URL" \
  --env DISCONNECT_MODE="$DISCONNECT_MODE" \
  --env VUS="$VUS" \
  --env DURATION="$DURATION" \
  --env INITIAL_SEQ_LAG="$SEQ_LAG" \
  tests/load/s5-reconnect-recovery.js \
  --out "json=docs/perf/pts/evidence/incoming/${LABEL}/s5-k6-reconnect.json"

kill $BID_PID 2>/dev/null || true

echo ""
echo "=== S5 done. Evidence: docs/perf/pts/evidence/incoming/${LABEL}/ ==="
echo ""
echo "   Key metrics to report:"
echo "     s5_ttcs_ms p99        — time-to-current-state p99 (target ≤ 2s)"
echo "     s5_recovered_total    — connections that successfully caught up"
echo "     s5_recovery_errors    — must be 0"
echo "     s5_seq_gaps_after_reconnect — must be 0"
echo ""
echo "   Recovery source: server logs will show 'incremental replay' vs 'snapshot rebuild'."
echo "   grep the server logs for 'reconnect' or 'snapshot' to get the distribution."
