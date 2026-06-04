#!/usr/bin/env bash
# Thin wrapper for running k6 from the independent ECS and keeping host metrics
# next to the k6 summary. This does not start or reset the service under test.
set -euo pipefail

SCENARIO="${1:-}"
if [ -z "$SCENARIO" ]; then
  echo "usage: $0 <s2-long-soak|s2-read-interference|s3-ws-sanity|s5-clean|custom> [k6 args...]" >&2
  exit 2
fi
shift || true

LABEL="${LABEL:-${SCENARIO}-$(date +%Y%m%dT%H%M%S)}"
EVIDENCE_DIR="${EVIDENCE_DIR:-docs/perf/pts/evidence/incoming/${LABEL}}"
BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
if [ -z "${WS_URL:-}" ]; then
  case "$BASE_URL" in
    http://*) WS_URL="ws://${BASE_URL#http://}" ;;
    https://*) WS_URL="wss://${BASE_URL#https://}" ;;
    *) WS_URL="ws://${BASE_URL}" ;;
  esac
fi

mkdir -p "$EVIDENCE_DIR"

METRICS_DIR="${EVIDENCE_DIR}/k6-host"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-5}" \
  bash scripts/perf/collect-k6-host-metrics.sh "$METRICS_DIR" &
COLLECTOR_PID=$!

cleanup() {
  kill "$COLLECTOR_PID" >/dev/null 2>&1 || true
  wait "$COLLECTOR_PID" >/dev/null 2>&1 || true
}
trap cleanup EXIT

K6_EXIT=0
case "$SCENARIO" in
  s2-long-soak)
    STAGE_DUR="${STAGE_DUR:-10m}"
    STAGE1_RATE="${STAGE1_RATE:-20}"
    STAGE2_RATE="${STAGE2_RATE:-60}"
    STAGE3_RATE="${STAGE3_RATE:-100}"
    PRE_ALLOC_VUS="${PRE_ALLOC_VUS:-80}"
    MAX_VUS="${MAX_VUS:-300}"
    k6 run \
      --env "BASE_URL=${BASE_URL}" \
      --env "STAGE_DUR=${STAGE_DUR}" \
      --env "STAGE1_RATE=${STAGE1_RATE}" \
      --env "STAGE2_RATE=${STAGE2_RATE}" \
      --env "STAGE3_RATE=${STAGE3_RATE}" \
      --env "PRE_ALLOC_VUS=${PRE_ALLOC_VUS}" \
      --env "MAX_VUS=${MAX_VUS}" \
      --summary-export "${EVIDENCE_DIR}/k6-summary.json" \
      --out "json=${EVIDENCE_DIR}/k6-samples.jsonl" \
      tests/load/s2-steady-soak.js "$@" || K6_EXIT=$?
    ;;
  s2-read-interference)
    STAGE_DUR="${STAGE_DUR:-5m}"
    BID_STAGE1_RATE="${BID_STAGE1_RATE:-20}"
    BID_STAGE2_RATE="${BID_STAGE2_RATE:-60}"
    BID_STAGE3_RATE="${BID_STAGE3_RATE:-100}"
    READ_STAGE1_RATE="${READ_STAGE1_RATE:-200}"
    READ_STAGE2_RATE="${READ_STAGE2_RATE:-600}"
    READ_STAGE3_RATE="${READ_STAGE3_RATE:-1000}"
    BID_PRE_ALLOC_VUS="${BID_PRE_ALLOC_VUS:-80}"
    BID_MAX_VUS="${BID_MAX_VUS:-300}"
    READ_PRE_ALLOC_VUS="${READ_PRE_ALLOC_VUS:-160}"
    READ_MAX_VUS="${READ_MAX_VUS:-600}"
    k6 run \
      --env "BASE_URL=${BASE_URL}" \
      --env "STAGE_DUR=${STAGE_DUR}" \
      --env "BID_STAGE1_RATE=${BID_STAGE1_RATE}" \
      --env "BID_STAGE2_RATE=${BID_STAGE2_RATE}" \
      --env "BID_STAGE3_RATE=${BID_STAGE3_RATE}" \
      --env "READ_STAGE1_RATE=${READ_STAGE1_RATE}" \
      --env "READ_STAGE2_RATE=${READ_STAGE2_RATE}" \
      --env "READ_STAGE3_RATE=${READ_STAGE3_RATE}" \
      --env "BID_PRE_ALLOC_VUS=${BID_PRE_ALLOC_VUS}" \
      --env "BID_MAX_VUS=${BID_MAX_VUS}" \
      --env "READ_PRE_ALLOC_VUS=${READ_PRE_ALLOC_VUS}" \
      --env "READ_MAX_VUS=${READ_MAX_VUS}" \
      --summary-export "${EVIDENCE_DIR}/k6-summary.json" \
      --out "json=${EVIDENCE_DIR}/k6-samples.jsonl" \
      tests/load/s2-read-interference.js "$@" || K6_EXIT=$?
    ;;
  s3-ws-sanity)
    VIEWER_VUS="${VIEWER_VUS:-1000}"
    HOLD_SECONDS="${HOLD_SECONDS:-120}"
    BIDDER_VUS="${BIDDER_VUS:-3}"
    k6 run \
      --env "BASE_URL=${BASE_URL}" \
      --env "WS_URL=${WS_URL}" \
      --env "VIEWER_VUS=${VIEWER_VUS}" \
      --env "HOLD_SECONDS=${HOLD_SECONDS}" \
      --env "BIDDER_VUS=${BIDDER_VUS}" \
      --summary-export "${EVIDENCE_DIR}/k6-summary.json" \
      --out "json=${EVIDENCE_DIR}/k6-samples.jsonl" \
      tests/load/s3-fanout-soak.js "$@" || K6_EXIT=$?
    ;;
  s5-clean)
    VUS="${VUS:-100}"
    DURATION="${DURATION:-2m}"
    MISSED_EVENTS="${MISSED_EVENTS:-3}"
    BID_RATE_PER_S="${BID_RATE_PER_S:-10}"
    BID_SOURCE_VUS="${BID_SOURCE_VUS:-5}"
    SESSION_CSV="${SESSION_CSV:-../../docs/perf/pts/pts-1ab-1000vu-sessions.csv}"
    k6 run \
      --env "BASE_URL=${BASE_URL}" \
      --env "WS_URL=${WS_URL}" \
      --env "DISCONNECT_MODE=clean" \
      --env "VUS=${VUS}" \
      --env "DURATION=${DURATION}" \
      --env "MISSED_EVENTS=${MISSED_EVENTS}" \
      --env "BID_RATE_PER_S=${BID_RATE_PER_S}" \
      --env "BID_SOURCE_VUS=${BID_SOURCE_VUS}" \
      --env "SESSION_CSV=${SESSION_CSV}" \
      --summary-export "${EVIDENCE_DIR}/k6-summary.json" \
      --out "json=${EVIDENCE_DIR}/k6-samples.jsonl" \
      tests/load/s5-reconnect-recovery.js "$@" || K6_EXIT=$?
    ;;
  custom)
    k6 run \
      --summary-export "${EVIDENCE_DIR}/k6-summary.json" \
      --out "json=${EVIDENCE_DIR}/k6-samples.jsonl" \
      "$@" || K6_EXIT=$?
    ;;
  *)
    echo "unknown scenario: ${SCENARIO}" >&2
    exit 2
    ;;
esac

echo "$K6_EXIT" > "${EVIDENCE_DIR}/k6-exit.txt"
echo "evidence_dir=${EVIDENCE_DIR}"
exit "$K6_EXIT"
