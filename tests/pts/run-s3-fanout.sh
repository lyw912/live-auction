#!/usr/bin/env bash
# run-s3-fanout.sh — S3 room fanout run sequence
#
# Two parts:
#   Part A (PTS JMeter, mixed final burst): uploads the current S3 single-branch
#     JMX and mixed CSV. This avoids PTS redistributing separate main Thread
#     Groups and makes role counts verifiable from sampler counts.
#
#   Part B (local/independent-source k6 live-only fanout): holds VIEWER_VUS WS
#     connections and measures live published_at_ms -> receive latency.
#
# Prerequisites:
#   - ulimit -n raised to at least VIEWER_VUS + 10000
#   - fs.nr_open raised (sysctl -w fs.nr_open=2000000)
#   - net.ipv4.ip_local_port_range widened if running >28k conns locally
#
# PTS Part A config:
#   JMX: tests/pts/scenarios/s3-room-fanout/s3-mixed-final-burst-4500vu.jmx
#   CSV: tests/pts/inputs/s1-s5/s3-mixed-final-burst-4500-sessions.csv
#   压力模式=虚拟用户模式, 最大VU=4500, 指定IP数=9
#   指定循环=是, 循环次数=1, 时长=3min, 采样率=1% (100% only for smoke/debug)
#   Screenshots: S3 live fanout receive p99, S3 POST accepted-update bid p99,
#     S3 WS handshake complete, S3 WS first snapshot/business message.
#
# Usage (Part B only for cheap soak):
#   VIEWER_VUS=10000 HOLD_SECONDS=600 bash tests/pts/run-s3-fanout.sh --soak-only
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
VIEWER_VUS="${VIEWER_VUS:-10000}"
HOLD_SECONDS="${HOLD_SECONDS:-600}"
LABEL="${LABEL:-s3-$(date +%Y%m%dT%H%M%S)}"
SOAK_ONLY="${1:-}"

echo "=== S3 prep ==="
bash tests/pts/prepare-s3-room-fanout-pressure.sh

# Node prep for high connection count
echo "Checking ulimit..."
ULIMIT=$(ulimit -n)
if [ "$ULIMIT" -lt "$((VIEWER_VUS + 5000))" ]; then
  echo "WARNING: ulimit -n=$ULIMIT is below VIEWER_VUS+5000=$((VIEWER_VUS + 5000))"
  echo "         Run: ulimit -n 1048576"
  echo "         And: sysctl -w fs.nr_open=2000000 net.core.somaxconn=65535"
fi

if [ "$SOAK_ONLY" != "--soak-only" ]; then
  echo ""
  echo "=== Part A: PTS JMeter S3-mixed-final-burst ==="
  echo "   JMX: tests/pts/scenarios/s3-room-fanout/s3-mixed-final-burst-4500vu.jmx"
  echo "   CSV: tests/pts/inputs/s1-s5/s3-mixed-final-burst-4500-sessions.csv"
  echo "   PTS: 最大VU=4500, 指定IP数=9, 指定循环=是, 循环次数=1, 时长=3min, 采样率=1%"
  echo "   Smoke first if needed:"
  echo "     JMX: tests/pts/scenarios/s3-room-fanout/s3-mixed-smoke-30vu.jmx"
  echo "     CSV: tests/pts/inputs/s1-s5/s3-mixed-smoke-30-sessions.csv"
  echo "     最大VU=30, 指定IP数=1, 指定循环=是, 循环次数=1, 时长=1min, 采样率=100%"
  echo ""
  read -r -p "Press ENTER when PTS run is complete: "
  read -r -p "PTS report ID: " PTS_REPORT_ID_A
fi

echo ""
echo "=== Part B: local/independent-source k6 S3-live-only-fanout (${VIEWER_VUS} WS, ${HOLD_SECONDS}s) ==="
echo "   Open Grafana now. Watch:"
echo "     process_resident_memory_bytes (RAM/conn = RSS / VIEWER_VUS)"
echo "     go_goroutines                 (should not climb monotonically)"
echo "     process_open_fds              (bounded by ulimit; should plateau)"
echo "     auction_fanout_latency_seconds (server-side fanout histogram)"

mkdir -p "artifacts/pts/evidence/incoming/${LABEL}"

k6 run \
  --env BASE_URL="$BASE_URL" \
  --env VIEWER_VUS="$VIEWER_VUS" \
  --env HOLD_SECONDS="$HOLD_SECONDS" \
  --env BID_RATE_PER_S=5 \
  tests/load/s3-fanout-soak.js \
  --out "json=artifacts/pts/evidence/incoming/${LABEL}/s3-k6-fanout.json" \
  || true

echo ""
echo "=== S3 post-run: collect server evidence ==="
BASE_URL="$BASE_URL" bash tests/pts/collect-server-evidence.sh "${LABEL}"

echo ""
echo "=== S3 done. Evidence: artifacts/pts/evidence/incoming/${LABEL}/ ==="
[ -n "${PTS_REPORT_ID_A:-}" ] && echo "   PTS report A (headline): ${PTS_REPORT_ID_A}"
echo "   M4 evidence: Grafana screenshot showing stable heap/goroutines/fd over soak."
echo "   M2 evidence: k6 summary s3_fanout_latency_ms p99."
echo ""
echo "   Judge framing:"
echo "     PTS: S3 live fanout receive p99 = __ ms; sampler counts match expected roles."
echo "     Soak: goroutines/fd/RSS plateau at __ over ${HOLD_SECONDS}s; no leak."
echo "     RAM/conn ≈ \$(( \$(cat /proc/\$(pgrep live-auction)/status | grep VmRSS | awk '{print \$2}') / ${VIEWER_VUS} )) KB/conn"
