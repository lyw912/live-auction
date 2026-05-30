#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_DIR="${PTS_RUNTIME_DIR:-/tmp/live-auction-pts}"

export SESSION_COUNT="${SESSION_COUNT:-1000}"
export SESSION_CSV="${SESSION_CSV:-pts-1ab-1000vu-sessions.csv}"
export L4B_PROFILE="${L4B_PROFILE:-accepted}"
case "$L4B_PROFILE" in
  accepted|pts-1a|pts1a)
    default_jmx="$ROOT_DIR/tests/pts/pts-1a-accepted-ladder-1000vu-1m.jmx"
    ;;
  contention|pts-1b|pts1b|reject|bidonly|bid-only)
    default_jmx="$ROOT_DIR/tests/pts/pts-1b-contention-burst-1000vu-1m.jmx"
    ;;
  *)
    echo "unknown L4B_PROFILE=$L4B_PROFILE; expected pts-1a/accepted or pts-1b/contention" >&2
    exit 1
    ;;
esac
export JMX_PATH="${JMX_PATH:-$default_jmx}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6380}"
export KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}"
export BID_ENGINE_MODE="${BID_ENGINE_MODE:-redis_ledger}"
export ADMISSION_ENABLED=false
export P1_LOAD_SEED_TIMEOUT="${P1_LOAD_SEED_TIMEOUT:-5m}"
export P1_LOAD_AUC_LIVE_CAP_PRICE_CENTS="${P1_LOAD_AUC_LIVE_CAP_PRICE_CENTS:-100000000000000}"
export P1_LOAD_AUC_SIDE_CAP_PRICE_CENTS="${P1_LOAD_AUC_SIDE_CAP_PRICE_CENTS:-100000000000000}"
export P1_LOAD_AUCTION_END_MINUTES="${P1_LOAD_AUCTION_END_MINUTES:-90}"

cd "$ROOT_DIR"

docker compose -f infra/docker-compose.yml up -d postgres redis kafka kafka-init

if [ -f "$RUNTIME_DIR/server.pid" ]; then
  old_pid="$(cat "$RUNTIME_DIR/server.pid")"
  if kill -0 "$old_pid" 2>/dev/null; then
    kill "$old_pid" 2>/dev/null || true
    sleep 1
  fi
fi
while read -r listen_pid; do
  if [ -n "$listen_pid" ] && kill -0 "$listen_pid" 2>/dev/null; then
    kill "$listen_pid" 2>/dev/null || true
    sleep 1
  fi
done < <(
  ss -lntpH "sport = :18080" 2>/dev/null |
    sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' |
    sort -u
)

docker exec live-auction-postgres psql -U live_auction -d live_auction -v ON_ERROR_STOP=1 -c "
create temporary table pts_reset_auctions(id text primary key) on commit drop;
insert into pts_reset_auctions(id)
select id from auctions
where id is not null;

delete from system_anomaly_events
where auction_id in (select id from pts_reset_auctions)
   or auction_id is null
   or auction_id not in (select id from auctions);
delete from redis_engine_settlements where auction_id in (select id from pts_reset_auctions);
delete from scheduler_jobs
where target_id in (select id from pts_reset_auctions)
   or target_id in (select id from orders where auction_id in (select id from pts_reset_auctions));
delete from idempotency_records
where scope_id in (select id from pts_reset_auctions)
   or scope_id in (select id from orders where auction_id in (select id from pts_reset_auctions));
delete from bids where auction_id in (select id from pts_reset_auctions);
delete from outbox_delivery
where outbox_id in (select id from outbox_events where auction_id in (select id from pts_reset_auctions));
delete from outbox_events where auction_id in (select id from pts_reset_auctions);
delete from auction_events where auction_id in (select id from pts_reset_auctions);
delete from payment_events
where order_id in (select id from orders where auction_id in (select id from pts_reset_auctions));
delete from orders where auction_id in (select id from pts_reset_auctions);
delete from max_bid_intents where auction_id in (select id from pts_reset_auctions);
delete from auction_engine_checkpoints where auction_id in (select id from pts_reset_auctions);
delete from realtime_stream_epochs where auction_id in (select id from pts_reset_auctions);
delete from snapshot_rebuild_events where auction_id in (select id from pts_reset_auctions);
delete from auction_rules where auction_id in (select id from pts_reset_auctions) and auction_id not in ('auc_live','auc_side');
delete from auctions where id in (select id from pts_reset_auctions) and id not in ('auc_live','auc_side');
"

docker exec live-auction-redis sh -c "redis-cli --scan --pattern 'bid:{auc_*}:*' | xargs -r redis-cli del" >/dev/null
docker exec live-auction-redis sh -c "redis-cli --scan --pattern 'bid:{auc_live}:*' | xargs -r redis-cli del" >/dev/null
docker exec live-auction-redis sh -c "redis-cli --scan --pattern 'bid:{auc_side}:*' | xargs -r redis-cli del" >/dev/null
docker exec live-auction-redis sh -c "redis-cli --scan --pattern 'bid:{auc_engine_*}:*' | xargs -r redis-cli del" >/dev/null
docker exec live-auction-redis sh -c "redis-cli --scan --pattern 'bid:{auc_inv_*}:*' | xargs -r redis-cli del" >/dev/null
docker exec live-auction-redis redis-cli del bid:engine:pending:auctions auction:auc_live:events auction:auc_live:snapshot auction:auc_side:events auction:auc_side:snapshot >/dev/null

docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --delete --if-exists --topic auction.bid-events >/dev/null 2>&1 || true
docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --delete --if-exists --topic auction.dlq >/dev/null 2>&1 || true
for _ in $(seq 1 30); do
  if ! docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic auction.bid-events >/dev/null 2>&1 &&
     ! docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic auction.dlq >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --create --if-not-exists --topic auction.bid-events --partitions 16 --replication-factor 1
docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --create --if-not-exists --topic auction.dlq --partitions 16 --replication-factor 1
bid_topic_desc="$(docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic auction.bid-events)"
bid_partitions="$(printf '%s\n' "$bid_topic_desc" | sed -n 's/.*PartitionCount: \([0-9][0-9]*\).*/\1/p' | head -n 1)"
if [ "$bid_partitions" != "16" ]; then
  echo "auction.bid-events partition count is $bid_partitions, want 16" >&2
  exit 1
fi

bash tests/pts/prepare-cloud-pressure.sh

docker exec live-auction-postgres psql -U live_auction -d live_auction -v ON_ERROR_STOP=1 -c "
update auctions
set status='ACTIVE',
    current_price_cents=start_price_cents,
    current_winner_id=null,
    end_at=now() + interval '90 minutes',
    accepted_bid_count=0,
    seq=0,
    engine_seq=0,
    engine_epoch=1,
    engine_paused=false,
    engine_pause_reason=null,
    engine_paused_at=null,
    updated_at=now()
where id in ('auc_live','auc_side');
"

echo "L4B final-second pressure data reset complete"
echo "- Backend: http://47.113.223.90:18080"
echo "- Profile: ${L4B_PROFILE}"
echo "- JMX: ${JMX_PATH#$ROOT_DIR/}"
echo "- CSV: docs/perf/pts/${SESSION_CSV}"
echo "- Engine: BID_ENGINE_MODE=${BID_ENGINE_MODE}, ADMISSION_ENABLED=false, Redis=${REDIS_ADDR}, Kafka=${KAFKA_BROKERS}"
