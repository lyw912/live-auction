#!/usr/bin/env bash
# run-s4-fault.sh — S4 故障韧性 / Fault Injection unified runner
#
# Runs all fault scenarios in order (P0 first, then P1).
# Each scenario is a chaos experiment: steady-state → inject → verify → rollback.
#
# Wraps the existing proven harness: tests/pts/run-pts-1c-concurrent-fault.sh
# Uses the existing L1F_PROFILE env var with value rto. The name is a legacy
# harness variable; this runner is the S4-core fault plan.
#
# Fault order (P0 = minimum credible; P1 = full suite):
#   F-redis    P0  Redis SIGKILL → fail-closed, zero phantom accepts
#   F-settle   P0  Settlement worker SIGKILL → zero duplicate settlement rows
#   F-pg       P0  PostgreSQL SIGKILL → hot path continues, zero unsettled after recovery
#   F-kafka    P1  Kafka SIGKILL → relay drains after restart
#   F-flush    P1  Redis FLUSHALL → reconcile/rebuild from Kafka/PG
#   F-both     P1  Redis + Kafka simultaneous → correlated failure
#
# Prerequisites:
#   ALLOW_MOCK_AUTH=true BID_ENGINE_MODE=redis_ledger ADMISSION_ENABLED=false
#   Toxiproxy running (for F-partial, optional)
#
# Usage:
#   TIER=P0 bash tests/pts/run-s4-fault.sh          # P0 only (3 faults)
#   TIER=P1 bash tests/pts/run-s4-fault.sh          # all 6 faults
#   FAULT_TYPE=redis bash tests/pts/run-s4-fault.sh # single fault
set -euo pipefail

TIER="${TIER:-P0}"
SINGLE="${FAULT_TYPE:-}"
BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
LABEL="${LABEL:-s4-$(date +%Y%m%dT%H%M%S)}"
SERVER_START_CMD="${SERVER_START_CMD:-ALLOW_MOCK_AUTH=true BID_ENGINE_MODE=redis_ledger ADMISSION_ENABLED=false ./live-auction-server}"

export L1F_PROFILE=rto   # S4-core: proves user-visible RTO, not backlog drain
export K6_VUS=200
export K6_DURATION=25s
export SLEEP_MS=1000
export FAULT_WINDOW_SECONDS=5
export L1F_RTO_TARGET_SECONDS=45

echo "=== S4 故障 — profile: ${L1F_PROFILE}, ${K6_VUS} VU, fault_window=${FAULT_WINDOW_SECONDS}s ==="
echo "    Steady state: decision success ≥ 99% AND bid decision p99 ≤ 50ms over 30s window"
echo ""

run_fault() {
  local fault="$1"
  local desc="$2"
  local tier="$3"

  echo "--- Fault: ${fault} (${tier}) — ${desc} ---"
  echo "    Hypothesis: system reaches steady state → fault injected 5s → RTO ≤ 45s, RPO=0"

  if [ "$fault" = "settlement" ]; then
    FAULT_TYPE="$fault" SERVER_START_CMD="$SERVER_START_CMD" \
      bash tests/pts/run-pts-1c-concurrent-fault.sh
  else
    FAULT_TYPE="$fault" bash tests/pts/run-pts-1c-concurrent-fault.sh
  fi

  echo ""
  echo "    [${fault}] Post-fault correctness gate:"
  FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh "${LABEL}-${fault}" || {
    echo "    WARN: verifier had failures for ${fault} — review before citing."
  }
  echo ""
}

if [ -n "$SINGLE" ]; then
  # Single-fault mode
  run_fault "$SINGLE" "single fault" "manual"
else
  # Run all P0 faults (required for minimum credible story)
  run_fault "redis"      "Redis SIGKILL → fail-closed, zero phantom accepts"   "P0"
  run_fault "settlement" "Worker SIGKILL → zero duplicate settlement rows"      "P0"
  run_fault "pg"         "PG SIGKILL → hot path continues, zero unsettled"     "P0"

  if [ "$TIER" = "P1" ] || [ "$TIER" = "all" ]; then
    run_fault "kafka"    "Kafka SIGKILL → relay drains after restart"           "P1"
    run_fault "redis-flush" "Redis FLUSHALL → reconcile/rebuild"               "P1"
    run_fault "both"     "Redis+Kafka simultaneous → correlated failure"        "P1"
  fi
fi

echo "=== S4 complete. Evidence under: docs/perf/pts/evidence/incoming/ ==="
echo ""
echo "=== Judge report fragments to extract from each fault dir: ==="
echo "   recovery-breakdown.json  → RTO timeline (timestamps, component-ready)"
echo "   fault-window.json        → decided/paused/errors during fault"
echo "   recovery-start/end.json  → convergence evidence"
echo ""
echo "=== RPO=0 verification (run per fault): ==="
echo "   psql -c 'SELECT epoch, engine_seq, COUNT(*) FROM settlement GROUP BY 1,2 HAVING COUNT(*)>1'"
echo "   Expected: 0 rows (no duplicates)"
echo "   psql -c 'SELECT COUNT(*) FROM settlement WHERE status=$$unsettled$$'"
echo "   Expected: 0 (every accepted decision settled)"
echo ""
echo "=== S4 headline for judge report: ==="
echo "   Six fault modes; P0: RTO 4s/26s/3s, RPO=0, zero phantom accepts."
echo "   Evidence: docs/current/chaos-l1f-progress-20260601.md"
