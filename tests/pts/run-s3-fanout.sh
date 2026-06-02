#!/usr/bin/env bash
# run-s3-fanout.sh — S3 万人围观 / Room Fanout full run sequence
#
# Two parts:
#   Part A (PTS JMeter, judge chart): uploads pts-2p1-bid-plus-ws-fanout.jmx.
#     Proves fanout p99 ≤ 1s at scale with distributed source IPs.
#     Sampler to screenshot: '广播接收 ws-fanout-receive'
#
#   Part B (local k6 soak, M4 + 10k headline): holds VIEWER_VUS WS connections
#     locally. Needed for 10k soak (PTS would cost ≈¥150; local is free).
#
# Prerequisites:
#   - ulimit -n raised to at least VIEWER_VUS + 10000
#   - fs.nr_open raised (sysctl -w fs.nr_open=2000000)
#   - net.ipv4.ip_local_port_range widened if running >28k conns locally
#
# PTS Part A config (fanout headline):
#   JMX: tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx
#   CSVs: pts-l2-bidder-1000-sessions.csv, pts-l2-viewer-<N>-sessions.csv
#   压力模式=虚拟用户模式, 最大VU=9000 (or 10000), 指定IP数=18 (or 20)
#   指定循环=是/1, 时长=2min, 采样率=1%
#   Screenshots: '广播接收 ws-fanout-receive' p99, '建立连接 ws-connect' p99
#
# Usage (Part B only for cheap soak):
#   VIEWER_VUS=10000 HOLD_SECONDS=600 bash tests/pts/run-s3-fanout.sh --soak-only
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
VIEWER_VUS="${VIEWER_VUS:-10000}"
HOLD_SECONDS="${HOLD_SECONDS:-600}"
LABEL="${LABEL:-s3-$(date +%Y%m%dT%H%M%S)}"
SOAK_ONLY="${1:-}"

echo "=== S3 围观 — prep ==="
# Session CSVs for viewers
bash tests/pts/prepare-l2-protocol-pressure.sh

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
  echo "=== Part A: PTS JMeter fanout headline (judge chart) ==="
  echo "   JMX: tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx"
  echo "   CSV (bidders): docs/perf/pts/pts-l2-bidder-1000-sessions.csv"
  echo "   CSV (viewers): docs/perf/pts/pts-l2-viewer-10000-sessions.csv"
  echo "   PTS: 最大VU=10000, 指定IP数=20, 指定循环=是/1, 时长=2min, 采样率=1%"
  echo "   Sampler to screenshot: '广播接收 ws-fanout-receive' p99"
  echo "   Also screenshot: '建立连接 ws-connect' p99 (join latency)"
  echo ""
  echo "   Cost-variant (cheaper): 2000 WS viewers"
  echo "   PTS: 最大VU=3000, 指定IP数=6, JMX ws_threads=2000"
  echo ""
  read -r -p "Press ENTER when PTS run is complete: "
  read -r -p "PTS report ID: " PTS_REPORT_ID_A
fi

echo ""
echo "=== Part B: local k6 soak (M4, ${VIEWER_VUS} WS, ${HOLD_SECONDS}s) ==="
echo "   Open Grafana now. Watch:"
echo "     process_resident_memory_bytes (RAM/conn = RSS / VIEWER_VUS)"
echo "     go_goroutines                 (should not climb monotonically)"
echo "     process_open_fds              (bounded by ulimit; should plateau)"
echo "     auction_fanout_latency_seconds (server-side fanout histogram)"

mkdir -p "docs/perf/pts/evidence/incoming/${LABEL}"

k6 run \
  --env BASE_URL="$BASE_URL" \
  --env VIEWER_VUS="$VIEWER_VUS" \
  --env HOLD_SECONDS="$HOLD_SECONDS" \
  --env BID_RATE_PER_S=5 \
  tests/load/s3-fanout-soak.js \
  --out "json=docs/perf/pts/evidence/incoming/${LABEL}/s3-k6-fanout.json" \
  || true

echo ""
echo "=== S3 post-run: collect server evidence ==="
BASE_URL="$BASE_URL" bash tests/pts/collect-server-evidence.sh "${LABEL}"

echo ""
echo "=== S3 done. Evidence: docs/perf/pts/evidence/incoming/${LABEL}/ ==="
[ -n "${PTS_REPORT_ID_A:-}" ] && echo "   PTS report A (headline): ${PTS_REPORT_ID_A}"
echo "   M4 evidence: Grafana screenshot showing stable heap/goroutines/fd over soak."
echo "   M2 evidence: k6 summary s3_fanout_latency_ms p99."
echo ""
echo "   Judge framing:"
echo "     PTS: '广播接收 ws-fanout-receive' p99 = __ ms @ ${VIEWER_VUS} WS (same-region, NTP clock)"
echo "     Soak: goroutines/fd/RSS plateau at __ over ${HOLD_SECONDS}s — no leak."
echo "     RAM/conn ≈ \$(( \$(cat /proc/\$(pgrep live-auction)/status | grep VmRSS | awk '{print \$2}') / ${VIEWER_VUS} )) KB/conn"
