#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LABEL="${1:-before-l4b-pts-preflight}"
OUT_DIR="$ROOT_DIR/docs/perf/pts/evidence/$LABEL"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
KAFKA_CONTAINER="${KAFKA_CONTAINER:-live-auction-kafka}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"
BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
KAFKA_BOOTSTRAP="${KAFKA_BOOTSTRAP:-localhost:9092}"
KAFKA_BID_TOPIC="${KAFKA_BID_TOPIC:-auction.bid-events}"
KAFKA_DLQ_TOPIC="${KAFKA_DLQ_TOPIC:-auction.dlq}"
KAFKA_CONSUMER_GROUP="${KAFKA_CONSUMER_GROUP:-settlement-workers}"

mkdir -p "$OUT_DIR"

gate_file="$OUT_DIR/l4b-preflight-gates.tsv"
report_file="$OUT_DIR/l4b-preflight.txt"

pass_fail() {
  if [ "$1" = "0" ]; then
    printf 'PASS'
  else
    printf 'FAIL'
  fi
}

require_rg() {
  local name="$1"
  local pattern="$2"
  local path="$3"
  if rg -q "$pattern" "$path"; then
    printf 'P0\t%s\tPASS\t%s\n' "$name" "$pattern"
  else
    printf 'P0\t%s\tFAIL\tmissing %s\n' "$name" "$pattern"
  fi
}

{
  echo "# L4B PTS preflight guard verification"
  echo "label=$LABEL"
  echo "collected_at=$(date -Is)"
  echo

  echo "## service"
  curl -fsS "$BASE_URL/readyz" || true
  echo
  curl -fsS "$BASE_URL/metrics" | rg 'auction_admission_enabled|auction_bid_lane_config|db_pool_max_conns|runtime_goroutines' || true
  echo

  echo "## postgres indexes"
  docker exec -i "$DB_CONTAINER" psql -q -A -F $'\t' -U "$DB_USER" -d "$DB_NAME" -f - <<'SQL' || true
select indexname, indexdef
from pg_indexes
where indexname in (
  'ux_auction_events_engine_seq',
  'ux_bids_engine_seq',
  'ux_redis_engine_settlements_kafka_offset',
  'ix_redis_engine_settlements_ledger_lag'
)
order by indexname;
SQL
  echo

  echo "## redis"
  docker exec "$REDIS_CONTAINER" redis-cli INFO memory | grep -E '^(used_memory:|maxmemory:|maxmemory_policy:)' || true
  docker exec "$REDIS_CONTAINER" redis-cli INFO stats | grep -E '^(evicted_keys:|rejected_connections:|total_error_replies:)' || true
  docker exec "$REDIS_CONTAINER" redis-cli INFO persistence | grep -E '^(aof_enabled:|aof_last_write_status:|aof_pending_bio_fsync:|rdb_last_bgsave_status:)' || true
  echo

  echo "## kafka topics"
  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --topic "$KAFKA_BID_TOPIC" || true
  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --topic "$KAFKA_DLQ_TOPIC" || true
  echo

  echo "## kafka consumer group"
  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --group "$KAFKA_CONSUMER_GROUP" 2>&1 || true
} > "$report_file"

{
  require_rg "lua_writes_pending_before_kafka" "redis\\.call\\('HSET', pending_key" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "lua_indexes_pending_auction" "redis\\.call\\('SADD', pending_auctions_key, auction_id\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "handler_returns_pending_without_kafka_append" "return result\\.response\\(\\), nil" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "worker_processes_pending_before_kafka" "ProcessPendingAppends\\(loopCtx, 100\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "worker_logs_loop_errors" "redis engine worker loop failed" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "worker_logs_loop_progress" "redis engine worker loop processed" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "worker_splits_kafka_settlement_loop" "\"kafka-settlement\"" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "server_passes_logger_to_worker" "WithLogger\\(log\\)\\.Run" "$ROOT_DIR/backend/cmd/server/main.go"
  require_rg "worker_uses_pending_auction_index" "pendingAuctionIDs\\(ctx, limit\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_append_has_per_auction_lock" "pendingAppendLockKey\\(auctionID\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_append_lock_uses_setnx" "SetNX\\(ctx, lockKey, lockToken" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_append_lock_uses_unique_token" "lockOwner \\+ \":\" \\+ uuid\\.NewString\\(\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_append_lock_uses_long_lease" "pendingAppendLockTTL\\s+=\\s+2 \\* time\\.Minute" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_append_lock_is_refreshed" "refreshPendingAppendLock\\(lockCtx, lockKey, lockToken\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_append_checks_lock_before_delete" "pending append lock lost after kafka append" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "reconcile_does_not_pause_inflight_append" "REDIS_PENDING_APPEND_IN_PROGRESS" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_append_appends_then_deletes" "w\\.ledger\\.Append\\(ctx, decision\\.result\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_append_deletes_after_kafka_append" "HDel\\(ctx, pendingKey, decision\\.field\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "pending_index_removed_when_empty" "SRem\\(ctx, redisx\\.BidEnginePendingAuctionsKey\\(\\), auctionID\\)" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "empty_pending_index_entry_is_cleaned" "len\\(decisions\\) == 0" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "reconciler_recovers_pending" "recoverPendingDecisions" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "settlement_rejects_stale_epoch" "result\\.EngineEpoch != dbEpoch" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "settlement_enforces_seq_next" "result\\.EngineSeq != dbSeq\\+1" "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "accepted_settlement_fenced_update" 'WHERE id = \$1 AND engine_epoch = \$7 AND engine_seq = \$6 - 1' "$ROOT_DIR/backend/internal/redisengine/engine.go"
  require_rg "kafka_writer_requires_all_acks" "RequiredAcks:\\s+kafka\\.RequireAll" "$ROOT_DIR/backend/internal/redisengine/kafka_ledger.go"
  require_rg "kafka_writer_is_synchronous" "Async:\\s+false" "$ROOT_DIR/backend/internal/redisengine/kafka_ledger.go"
  require_rg "kafka_writer_retries_transient_failures" "MaxAttempts:\\s+10" "$ROOT_DIR/backend/internal/redisengine/kafka_ledger.go"
  require_rg "kafka_message_key_auction_id" "key := result\\.AuctionID" "$ROOT_DIR/backend/internal/redisengine/kafka_ledger.go"
  require_rg "kafka_headers_include_engine_epoch" "Key: \"engine_epoch\"" "$ROOT_DIR/backend/internal/redisengine/kafka_ledger.go"
  require_rg "kafka_headers_include_engine_seq" "Key: \"engine_seq\"" "$ROOT_DIR/backend/internal/redisengine/kafka_ledger.go"
  require_rg "db_unique_engine_seq_bids" "CREATE UNIQUE INDEX ux_bids_engine_seq" "$ROOT_DIR/backend/migrations/202605280001_redis_ledger_engine.sql"
  require_rg "db_unique_engine_seq_events" "CREATE UNIQUE INDEX ux_auction_events_engine_seq" "$ROOT_DIR/backend/migrations/202605280001_redis_ledger_engine.sql"
  require_rg "db_unique_kafka_offset" "CREATE UNIQUE INDEX ux_redis_engine_settlements_kafka_offset" "$ROOT_DIR/backend/migrations/202605290001_kafka_bid_ledger.sql"

  admission="$(curl -fsS "$BASE_URL/metrics" | awk '/^auction_admission_enabled / {print $2; exit}' || true)"
  printf 'P0\tadmission_disabled\t%s\tauction_admission_enabled must be 0 for downstream pressure\n' "$([ "${admission:-}" = "0" ] && echo PASS || echo FAIL)"

  mode_metric="$(curl -fsS "$BASE_URL/readyz" >/dev/null && echo ok || echo fail)"
  printf 'P0\tservice_ready\t%s\treadyz must pass\n' "$([ "$mode_metric" = "ok" ] && echo PASS || echo FAIL)"

  redis_policy="$(docker exec "$REDIS_CONTAINER" redis-cli INFO memory | awk -F: '/^maxmemory_policy:/ {gsub(/\r/, "", $2); print $2}')"
  evicted_keys="$(docker exec "$REDIS_CONTAINER" redis-cli INFO stats | awk -F: '/^evicted_keys:/ {gsub(/\r/, "", $2); print $2}')"
  redis_aof_enabled="$(docker exec "$REDIS_CONTAINER" redis-cli INFO persistence | awk -F: '/^aof_enabled:/ {gsub(/\r/, "", $2); print $2}')"
  redis_aof_write_status="$(docker exec "$REDIS_CONTAINER" redis-cli INFO persistence | awk -F: '/^aof_last_write_status:/ {gsub(/\r/, "", $2); print $2}')"
  printf 'P0\tredis_noeviction_policy\t%s\tRedis maxmemory policy must not evict hot auction state\n' "$([ "${redis_policy:-}" = "noeviction" ] && echo PASS || echo FAIL)"
  printf 'P0\tredis_evicted_keys_zero\t%s\tRedis evicted_keys must be zero before PTS\n' "$([ "${evicted_keys:-0}" = "0" ] && echo PASS || echo FAIL)"
  printf 'P0\tredis_aof_enabled\t%s\tRedis AOF must protect pending accepted decisions before Kafka append\n' "$([ "${redis_aof_enabled:-}" = "1" ] && echo PASS || echo FAIL)"
  printf 'P0\tredis_aof_last_write_ok\t%s\tRedis AOF last write status must be ok\n' "$([ "${redis_aof_write_status:-}" = "ok" ] && echo PASS || echo FAIL)"

  docker exec -i "$DB_CONTAINER" psql -q -A -t -F $'\t' -U "$DB_USER" -d "$DB_NAME" -f - <<'SQL'
select 'P0', 'db_engine_indexes_present',
       case when count(*) = 4 then 'PASS' else 'FAIL' end,
       'engine seq and kafka offset indexes must exist'
from pg_indexes
where indexname in (
  'ux_auction_events_engine_seq',
  'ux_bids_engine_seq',
  'ux_redis_engine_settlements_kafka_offset',
  'ix_redis_engine_settlements_ledger_lag'
);
SQL

  bid_topic_desc="$(docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --topic "$KAFKA_BID_TOPIC" 2>/dev/null || true)"
  dlq_topic_desc="$(docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --topic "$KAFKA_DLQ_TOPIC" 2>/dev/null || true)"
  bid_topic_partitions="$(printf '%s\n' "$bid_topic_desc" | awk -F'PartitionCount: ' '/PartitionCount:/ {split($2, a, "\t"); print a[1]; exit}')"
  printf 'P0\tkafka_bid_topic_exists\t%s\tbid ledger topic must exist\n' "$([[ "$bid_topic_desc" == *"Topic: $KAFKA_BID_TOPIC"* ]] && echo PASS || echo FAIL)"
  printf 'P0\tkafka_bid_topic_partitions_16\t%s\tbid ledger topic must have 16 partitions before PTS\n' "$([ "${bid_topic_partitions:-}" = "16" ] && echo PASS || echo FAIL)"
  printf 'P0\tkafka_dlq_topic_exists\t%s\tDLQ topic must exist\n' "$([[ "$dlq_topic_desc" == *"Topic: $KAFKA_DLQ_TOPIC"* ]] && echo PASS || echo FAIL)"
} > "$gate_file"

if awk -F '\t' '$1 == "P0" && $3 == "FAIL" { found=1 } END { exit found ? 0 : 1 }' "$gate_file"; then
  echo "[preflight] P0 guard failure; see $gate_file" >&2
  exit 1
fi

echo "[preflight] done: $report_file"
echo "[preflight] gates: $gate_file"
