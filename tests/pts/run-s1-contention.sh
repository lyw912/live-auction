#!/usr/bin/env bash
# run-s1-contention.sh — S1 绝杀时刻 full run sequence
#
# Runs both sub-tests:
#   S1-burst  (asset alias L1-C1): tight contention band, decision p99 ≤ 50ms, M1 headline
#   S1-ladder (asset alias L1-C0): strictly increasing amounts, accept-path control
#
# Upload the JMX to PTS manually; this script handles server-side prep + post-run verify.
# PTS config: 压力模式=虚拟用户模式, 最大VU=1000, 指定循环=是/1, 指定IP数=2, 时长=2min, 采样率=100% for judge-forensics runs
# S1-burst default JMX release model: contention_release_window_ms=500,
# i.e. 1000 one-shot bids spread deterministically inside a short final-second
# window. The actual arrival span is validated from sampling logs/server metrics.
# Diagnostic zero-ms microburst: set contention_release_window_ms=0 in the PTS
# JMeter environment properties, and label the report as diagnostic only.
#
# Usage:
#   LABEL=run-$(date +%Y%m%dT%H%M%S) bash tests/pts/run-s1-contention.sh
set -euo pipefail

LABEL="${LABEL:-s1-$(date +%Y%m%dT%H%M%S)}"
BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"

echo "=== S1 绝杀 — pre-flight ==="
# Reset auction state and pre-generate session CSV
L4B_PROFILE=pts-1b SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
# Verify Redis/Kafka/settlement protections before loading PTS
BASE_URL="$BASE_URL" bash tests/pts/preflight-l4b-pts-guards.sh "before-${LABEL}"

echo ""
echo "=== S1 ready — upload JMX to PTS now ==="
echo "   JMX:       tests/pts/L1-component/pts-1b-contention-burst-1000vu-1m.jmx   (S1-burst)"
echo "   CSV:       docs/perf/pts/pts-1ab-1000vu-sessions.csv"
echo "   Sampler to screenshot: '出价决策 bid-decision'"
echo "   JMX default: contention_release_window_ms=500 (short final-second release window)"
echo "   Optional diagnostic property: contention_release_window_ms=0 (strict microburst, not the judge-facing default)"
echo "   Optional conservative property: contention_release_window_ms=1000 (one-second final-window burst)"
echo ""
echo "   [optional S1-ladder control]"
echo "   JMX:       tests/pts/L1-component/pts-1a-accepted-ladder-1000vu-1m.jmx"
echo "   CSV:       docs/perf/pts/pts-1ab-1000vu-sessions.csv"
echo ""
read -r -p "Press ENTER when PTS run is complete and you have the report ID: "
read -r -p "PTS report ID: " PTS_REPORT_ID

echo ""
echo "=== S1 post-run: collect server evidence ==="
BASE_URL="$BASE_URL" bash tests/pts/collect-server-evidence.sh "${LABEL}"

echo ""
echo "=== S1 post-run: correctness verifier (M3 gate) ==="
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh "${LABEL}"

echo ""
echo "=== S1 done. Evidence at: docs/perf/pts/evidence/incoming/${LABEL}/ ==="
echo "   PTS report: ${PTS_REPORT_ID}"
echo "   Screenshot '出价决策 bid-decision' p99 from the PTS report → judge packet."
echo "   Export PTS PDF (无水印) and attach as S1-burst evidence."
echo "   For 100% sampling evidence, run:"
echo "     PAGE_SIZE=100 bash tests/pts/fetch-pts-sampling-logs.sh ${PTS_REPORT_ID}"
echo "     bash tests/pts/review-s1-pts-run.sh ${PTS_REPORT_ID}"
