#!/usr/bin/env bash
# run-s2-steady.sh — S2 正常竞价 full run sequence
#
# Two parts:
#   Part A (local k6 soak, M4 leak gate): runs s2-steady-soak.js locally for
#     SOAK_MINUTES; watch Grafana for heap floor / goroutines / fd slope.
#   Part B (optional PTS JMeter chart): run pts-2p4-steady-interactive-auction.jmx
#     only when you need a polished PTS PDF.
#
# Optional PTS Part B config:
#   JMX=tests/pts/L2-protocol/pts-2p4-steady-interactive-auction.jmx
#   压力模式=虚拟用户模式, 最大VU=3000, 指定IP数=6
#   压测时长=10min, 是否指定循环=否, 采样率=1%
#
# Usage:
#   SOAK_MINUTES=30 bash tests/pts/run-s2-steady.sh
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
SOAK_MINUTES="${SOAK_MINUTES:-30}"
LABEL="${LABEL:-s2-$(date +%Y%m%dT%H%M%S)}"
EVIDENCE_DIR="docs/perf/pts/evidence/incoming/${LABEL}"
ALLOW_MOCK_AUTH="${ALLOW_MOCK_AUTH:-true}"
STAGE1_RATE="${STAGE1_RATE:-20}"
STAGE2_RATE="${STAGE2_RATE:-60}"
STAGE3_RATE="${STAGE3_RATE:-100}"
PRE_ALLOC_VUS="${PRE_ALLOC_VUS:-50}"
MAX_VUS="${MAX_VUS:-200}"
S2_CONVERGENCE_TIMEOUT_SECONDS="${S2_CONVERGENCE_TIMEOUT_SECONDS:-120}"
S2_CONVERGENCE_POLL_SECONDS="${S2_CONVERGENCE_POLL_SECONDS:-2}"
S2_RUNTIME_SAMPLE_SECONDS="${S2_RUNTIME_SAMPLE_SECONDS:-5}"

DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
KAFKA_CONTAINER="${KAFKA_CONTAINER:-live-auction-kafka}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"
KAFKA_BOOTSTRAP="${KAFKA_BOOTSTRAP:-localhost:9092}"
KAFKA_BID_TOPIC="${KAFKA_BID_TOPIC:-auction.bid-events}"
KAFKA_CONSUMER_GROUP="${KAFKA_CONSUMER_GROUP:-settlement-workers}"
AUCTION_ID="${AUCTION_ID:-auc_live}"

mkdir -p "$EVIDENCE_DIR"

kafka_group_lag() {
  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-consumer-groups.sh \
    --bootstrap-server "$KAFKA_BOOTSTRAP" \
    --describe \
    --group "$KAFKA_CONSUMER_GROUP" 2>/dev/null |
    awk 'NR > 1 && $6 ~ /^[0-9]+$/ {sum += $6} END {print sum + 0}'
}

db_scalar() {
  local sql="$1"
  docker exec -i "$DB_CONTAINER" psql -q -A -t -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<SQL | tr -d '[:space:]'
$sql
SQL
}

redis_scalar() {
  docker exec "$REDIS_CONTAINER" redis-cli "$@" 2>/dev/null | tr -d '\r[:space:]'
}

metric_scalar() {
  local metric="$1"
  local metrics_text="$2"
  awk -v name="$metric" '$1 == name { print $2; found=1; exit } END { if (!found) print "nan" }' "$metrics_text"
}

collect_runtime_samples() {
  local out_file="${EVIDENCE_DIR}/s2-runtime-samples.tsv"
  printf 'epoch_ms\telapsed_seconds\truntime_rss_bytes\truntime_heap_alloc_bytes\truntime_heap_inuse_bytes\truntime_heap_sys_bytes\truntime_goroutines\truntime_open_fds\tdb_pool_acquired\tdb_pool_total\n' > "$out_file"
  local start_ts
  start_ts="$(date +%s)"
  while true; do
    local tmp now_ts epoch_ms elapsed rss heap_alloc heap_inuse heap_sys goroutines open_fds db_acquired db_total
    tmp="$(mktemp)"
    if curl -fsS "${BASE_URL}/metrics" > "$tmp"; then
      now_ts="$(date +%s)"
      epoch_ms="$(date +%s%3N)"
      elapsed=$(( now_ts - start_ts ))
      rss="$(metric_scalar runtime_rss_bytes "$tmp")"
      heap_alloc="$(metric_scalar runtime_heap_alloc_bytes "$tmp")"
      heap_inuse="$(metric_scalar runtime_heap_inuse_bytes "$tmp")"
      heap_sys="$(metric_scalar runtime_heap_sys_bytes "$tmp")"
      goroutines="$(metric_scalar runtime_goroutines "$tmp")"
      open_fds="$(metric_scalar runtime_open_fds "$tmp")"
      db_acquired="$(awk '$1 == "db_pool_conns{state=\"acquired\"}" { print $2; found=1; exit } END { if (!found) print "nan" }' "$tmp")"
      db_total="$(awk '$1 == "db_pool_conns{state=\"total\"}" { print $2; found=1; exit } END { if (!found) print "nan" }' "$tmp")"
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$epoch_ms" "$elapsed" "$rss" "$heap_alloc" "$heap_inuse" "$heap_sys" "$goroutines" "$open_fds" "$db_acquired" "$db_total" >> "$out_file"
    fi
    rm -f "$tmp"
    sleep "$S2_RUNTIME_SAMPLE_SECONDS"
  done
}

write_runtime_summary() {
  local samples="${EVIDENCE_DIR}/s2-runtime-samples.tsv"
  local out="${EVIDENCE_DIR}/s2-runtime-summary.env"
  if [ ! -f "$samples" ]; then
    echo "runtime_samples_status=missing" > "$out"
    return 0
  fi
  awk -F '\t' '
    NR == 1 {
      for (i = 1; i <= NF; i++) idx[$i] = i
      next
    }
    function numeric(v) { return v != "" && v != "nan" }
    {
      n++
      fields["runtime_rss_bytes"] = idx["runtime_rss_bytes"]
      fields["runtime_heap_alloc_bytes"] = idx["runtime_heap_alloc_bytes"]
      fields["runtime_heap_inuse_bytes"] = idx["runtime_heap_inuse_bytes"]
      fields["runtime_heap_sys_bytes"] = idx["runtime_heap_sys_bytes"]
      fields["runtime_goroutines"] = idx["runtime_goroutines"]
      fields["runtime_open_fds"] = idx["runtime_open_fds"]
      fields["db_pool_acquired"] = idx["db_pool_acquired"]
      fields["db_pool_total"] = idx["db_pool_total"]
      for (name in fields) {
        col = fields[name]
        if (numeric($col)) {
          value = $col + 0
          if (!(name in seen)) {
            first[name] = value
            min[name] = value
            max[name] = value
            seen[name] = 1
          }
          last[name] = value
          if (value < min[name]) min[name] = value
          if (value > max[name]) max[name] = value
        }
      }
    }
    END {
      print "runtime_samples=" n
      for (name in seen) {
        printf "%s_first=%s\n", name, first[name]
        printf "%s_last=%s\n", name, last[name]
        printf "%s_min=%s\n", name, min[name]
        printf "%s_max=%s\n", name, max[name]
        printf "%s_delta=%s\n", name, last[name] - first[name]
      }
    }
  ' "$samples" > "$out"
}

write_s2_backlog_snapshot() {
  local suffix="$1"
  {
    echo "collected_at=$(date -Is)"
    echo "auction_id=$AUCTION_ID"
    echo "kafka_group_lag=$(kafka_group_lag)"
    echo "non_terminal_settlements=$(db_scalar "select count(*) from redis_engine_settlements where auction_id = :'auction_id' and status not in ('SETTLED','SKIPPED');")"
    echo "settlement_total=$(db_scalar "select count(*) from redis_engine_settlements where auction_id = :'auction_id';")"
    echo "auction_engine_seq=$(db_scalar "select coalesce(engine_seq, 0) from auctions where id = :'auction_id';")"
    echo "redis_stream_len=$(redis_scalar XLEN "bid:{$AUCTION_ID}:engine:log")"
    echo "redis_pending_count=$(redis_scalar HLEN "bid:{$AUCTION_ID}:engine:pending")"
    echo "outbox_unpublished=$(db_scalar "select count(*) from outbox_delivery d join outbox_events e on e.id = d.outbox_id where e.auction_id = :'auction_id' and d.status <> 'PUBLISHED';")"
  } > "${EVIDENCE_DIR}/s2-${suffix}-backlog.env"

  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-consumer-groups.sh \
    --bootstrap-server "$KAFKA_BOOTSTRAP" \
    --describe \
    --group "$KAFKA_CONSUMER_GROUP" \
    > "${EVIDENCE_DIR}/s2-${suffix}-kafka-consumer-lag.txt" 2>&1 || true

  docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -c "
select id,status,current_price_cents,current_winner_id,accepted_bid_count,engine_seq,engine_paused,engine_pause_reason
from auctions
where id = :'auction_id';
select status,result,count(*) as count,min(engine_seq) as min_engine_seq,max(engine_seq) as max_engine_seq
from redis_engine_settlements
where auction_id = :'auction_id'
group by status,result
order by status,result;
" > "${EVIDENCE_DIR}/s2-${suffix}-postgres-backlog.txt" 2>&1 || true
}

wait_s2_convergence() {
  local start_ts now_ts elapsed lag non_terminal stream_len settlement_total pending_count outbox_unpublished
  start_ts="$(date +%s)"
  printf 'elapsed_seconds\tkafka_group_lag\tnon_terminal_settlements\tredis_stream_len\tsettlement_total\tredis_pending_count\toutbox_unpublished\n' > "${EVIDENCE_DIR}/s2-convergence.tsv"

  while true; do
    now_ts="$(date +%s)"
    elapsed=$(( now_ts - start_ts ))
    lag="$(kafka_group_lag)"
    non_terminal="$(db_scalar "select count(*) from redis_engine_settlements where auction_id = :'auction_id' and status not in ('SETTLED','SKIPPED');")"
    stream_len="$(redis_scalar XLEN "bid:{$AUCTION_ID}:engine:log")"
    settlement_total="$(db_scalar "select count(*) from redis_engine_settlements where auction_id = :'auction_id';")"
    pending_count="$(redis_scalar HLEN "bid:{$AUCTION_ID}:engine:pending")"
    outbox_unpublished="$(db_scalar "select count(*) from outbox_delivery d join outbox_events e on e.id = d.outbox_id where e.auction_id = :'auction_id' and d.status <> 'PUBLISHED';")"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$elapsed" "$lag" "$non_terminal" "$stream_len" "$settlement_total" "$pending_count" "$outbox_unpublished" >> "${EVIDENCE_DIR}/s2-convergence.tsv"

    if [ "${lag:-0}" = "0" ] &&
       [ "${non_terminal:-0}" = "0" ] &&
       [ "${stream_len:-0}" = "${settlement_total:-0}" ] &&
       [ "${pending_count:-0}" = "0" ] &&
       [ "${outbox_unpublished:-0}" = "0" ]; then
      echo "converged_seconds=$elapsed" > "${EVIDENCE_DIR}/s2-convergence-summary.env"
      echo "convergence_status=PASS" >> "${EVIDENCE_DIR}/s2-convergence-summary.env"
      return 0
    fi

    if [ "$elapsed" -ge "$S2_CONVERGENCE_TIMEOUT_SECONDS" ]; then
      echo "converged_seconds=$elapsed" > "${EVIDENCE_DIR}/s2-convergence-summary.env"
      echo "convergence_status=FAIL" >> "${EVIDENCE_DIR}/s2-convergence-summary.env"
      return 1
    fi

    sleep "$S2_CONVERGENCE_POLL_SECONDS"
  done
}

echo "=== S2 稳态 — prep ==="
ALLOW_MOCK_AUTH="$ALLOW_MOCK_AUTH" bash tests/pts/prepare-l2p4-steady-pressure.sh

preseed_local_k6_bidders() {
  local db_container="${DB_CONTAINER:-live-auction-postgres}"
  local redis_container="${REDIS_CONTAINER:-live-auction-redis}"
  local db_user="${DB_USER:-live_auction}"
  local db_name="${DB_NAME:-live_auction}"
  local room_id
  room_id="$(docker exec "$db_container" psql -q -A -t -U "$db_user" -d "$db_name" \
    -c "SELECT room_id FROM auctions WHERE id = 'auc_live'" 2>/dev/null || true)"
  [ -n "$room_id" ] || return 0

  docker exec -i "$db_container" psql -q -v ON_ERROR_STOP=1 -U "$db_user" -d "$db_name" \
    -v room_id="$room_id" -v max_vus="$MAX_VUS" -f - <<'SQL'
INSERT INTO users (id, role, display_name, city)
SELECT 'k6_bidder_' || generate_series(1, :'max_vus'::int)::text, 'user',
       'k6_bidder_' || generate_series(1, :'max_vus'::int)::text, 's2'
ON CONFLICT (id) DO NOTHING;

INSERT INTO room_memberships (room_id, user_id, role, status)
SELECT :'room_id', 'k6_bidder_' || generate_series(1, :'max_vus'::int)::text, 'viewer', 'ACTIVE'
ON CONFLICT (room_id, user_id) DO UPDATE
SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL;
SQL

  {
    for i in $(seq 1 "$MAX_VUS"); do
      key="acl:membership:{auc_live}:k6_bidder_${i}"
      printf '*5\r\n'
      printf '$3\r\nSET\r\n'
      printf '$%s\r\n%s\r\n' "${#key}" "$key"
      printf '$%s\r\n%s\r\n' "${#room_id}" "$room_id"
      printf '$2\r\nEX\r\n'
      printf '$5\r\n43200\r\n'
    done
  } | docker exec -i "$redis_container" redis-cli --pipe >/dev/null
}

preseed_local_k6_bidders

echo ""
echo "=== Part A: local k6 soak (M4 leak gate, ${SOAK_MINUTES} min) ==="
echo "   Runtime samples: ${EVIDENCE_DIR}/s2-runtime-samples.tsv every ${S2_RUNTIME_SAMPLE_SECONDS}s."
echo "   Watch runtime_rss_bytes, runtime_heap_inuse_bytes, runtime_goroutines, runtime_open_fds."
echo "   Auth profile: ALLOW_MOCK_AUTH=${ALLOW_MOCK_AUTH} for local k6 role headers."
echo ""
curl -fsS "${BASE_URL}/readyz" > "${EVIDENCE_DIR}/readyz-before.json" || true
curl -fsS "${BASE_URL}/metrics" > "${EVIDENCE_DIR}/metrics-before.prom" || true
if curl -fsS "${BASE_URL}/debug/pprof/heap" > "${EVIDENCE_DIR}/heap-before.pprof"; then
  HAS_PPROF_HEAP=1
else
  HAS_PPROF_HEAP=0
  rm -f "${EVIDENCE_DIR}/heap-before.pprof"
  echo "   pprof heap endpoint unavailable; using /metrics runtime gauges for M4."
fi

collect_runtime_samples &
RUNTIME_SAMPLER_PID=$!
cleanup_runtime_sampler() {
  if [ -n "${RUNTIME_SAMPLER_PID:-}" ] && kill -0 "$RUNTIME_SAMPLER_PID" 2>/dev/null; then
    kill "$RUNTIME_SAMPLER_PID" 2>/dev/null || true
    wait "$RUNTIME_SAMPLER_PID" 2>/dev/null || true
  fi
}
trap cleanup_runtime_sampler EXIT

# Stage durations for soak: total ~= SOAK_MINUTES plus 30s ramp-down.
STAGE_SECONDS=$(( SOAK_MINUTES * 60 / 3 ))
if [ "$STAGE_SECONDS" -lt 1 ]; then
  STAGE_SECONDS=1
fi
STAGE_DUR="${STAGE_SECONDS}s"

K6_ARGS=(
  run
  --env "BASE_URL=$BASE_URL"
  --env "STAGE1_RATE=$STAGE1_RATE"
  --env "STAGE2_RATE=$STAGE2_RATE"
  --env "STAGE3_RATE=$STAGE3_RATE"
  --env "STAGE_DUR=${STAGE_DUR}"
  --env "PRE_ALLOC_VUS=$PRE_ALLOC_VUS"
  --env "MAX_VUS=$MAX_VUS"
  --summary-export "${EVIDENCE_DIR}/s2-k6-summary.json"
  --out "json=${EVIDENCE_DIR}/s2-k6-soak.json"
  tests/load/s2-steady-soak.js
)

if command -v k6 >/dev/null 2>&1; then
  k6 "${K6_ARGS[@]}" || K6_EXIT=$?   # threshold breach: still collect evidence, fail at the end
else
  echo "   local k6 not found; using docker image grafana/k6:latest"
  docker run --rm --network host --user "$(id -u):$(id -g)" \
    -v "$PWD:/work" -w /work \
    grafana/k6:latest "${K6_ARGS[@]}" || K6_EXIT=$?
fi
K6_EXIT="${K6_EXIT:-0}"
cleanup_runtime_sampler
trap - EXIT
write_runtime_summary

echo ""
echo "=== S2 post-run: runtime snapshots ==="
curl -fsS "${BASE_URL}/readyz" > "${EVIDENCE_DIR}/readyz-after.json" || true
curl -fsS "${BASE_URL}/metrics" > "${EVIDENCE_DIR}/metrics-after.prom" || true
write_s2_backlog_snapshot "immediate"
if [ "$HAS_PPROF_HEAP" = "1" ]; then
  curl -fsS "${BASE_URL}/debug/pprof/heap" > "${EVIDENCE_DIR}/heap-after.pprof" || true
  echo "   Optional pprof: go tool pprof -base ${EVIDENCE_DIR}/heap-before.pprof ${EVIDENCE_DIR}/heap-after.pprof"
else
  echo "   pprof heap was unavailable; inspect metrics-before/after.prom and Grafana resource slope."
fi

echo ""
echo "=== S2 post-run: bounded async convergence wait ==="
echo "   Waiting up to ${S2_CONVERGENCE_TIMEOUT_SECONDS}s for Kafka settlement, Redis relay, and outbox drain."
if wait_s2_convergence; then
  CONVERGENCE_EXIT=0
  echo "   convergence PASS ($(cat "${EVIDENCE_DIR}/s2-convergence-summary.env" | tr '\n' ' '))"
else
  CONVERGENCE_EXIT=1
  echo "   convergence FAIL ($(cat "${EVIDENCE_DIR}/s2-convergence-summary.env" | tr '\n' ' '))"
fi
write_s2_backlog_snapshot "final"

echo ""
echo "=== S2 post-run: correctness verifier ==="
BASE_URL="$BASE_URL" bash tests/pts/collect-server-evidence.sh "${LABEL}"
# S2 is an open-arrival soak, so the exact request count depends on rate and
# duration. Reuse the hard consistency gates, but disable L4B's fixed 1000-bid
# PTS identity gate.
EXPECTED_UNIQUE_BIDS="" FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh "${LABEL}" || VERIFIER_EXIT=$?
VERIFIER_EXIT="${VERIFIER_EXIT:-0}"

echo ""
echo "=== S2 done. Evidence: ${EVIDENCE_DIR}/ ==="
echo "   M4 evidence:  Grafana screenshot (resource slope over soak)."
echo ""
echo "Optional Part B PTS chart:"
echo "   JMX: tests/pts/L2-protocol/pts-2p4-steady-interactive-auction.jmx"
echo "   CSVs: pts-l2p4-bidder/viewer/reader session files"
echo "   PTS: 最大VU=3000, 指定IP数=6, 压测时长=10min, 是否指定循环=否"

if [ "$K6_EXIT" -ne 0 ] || [ "$CONVERGENCE_EXIT" -ne 0 ] || [ "$VERIFIER_EXIT" -ne 0 ]; then
  echo ""
  echo "=== S2 failed ==="
  echo "   k6_exit=${K6_EXIT} convergence_exit=${CONVERGENCE_EXIT} verifier_exit=${VERIFIER_EXIT}"
  exit 1
fi
