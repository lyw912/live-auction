#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

export SESSION_COUNT="${SESSION_COUNT:-512}"
export SESSION_CSV="${SESSION_CSV:-pts_l4b_final_second_100vu_sessions.csv}"
export JMX_PATH="${JMX_PATH:-$ROOT_DIR/tests/pts/live-auction-l4b-final-second-100vu.jmx}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6380}"
export KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}"
export BID_ENGINE_MODE="${BID_ENGINE_MODE:-redis_ledger}"
export ADMISSION_ENABLED=false
export P1_LOAD_SEED_TIMEOUT="${P1_LOAD_SEED_TIMEOUT:-5m}"
export P1_LOAD_AUC_LIVE_CAP_PRICE_CENTS="${P1_LOAD_AUC_LIVE_CAP_PRICE_CENTS:-100000000000000}"
export P1_LOAD_AUC_SIDE_CAP_PRICE_CENTS="${P1_LOAD_AUC_SIDE_CAP_PRICE_CENTS:-100000000000000}"
export P1_LOAD_AUCTION_END_MINUTES="${P1_LOAD_AUCTION_END_MINUTES:-30}"

cd "$ROOT_DIR"

docker compose -f infra/docker-compose.yml up -d postgres redis kafka kafka-init

docker exec live-auction-postgres psql -U live_auction -d live_auction -v ON_ERROR_STOP=1 -c "
delete from redis_engine_settlements where auction_id in ('auc_live','auc_side');
delete from scheduler_jobs
where target_id in ('auc_live','auc_side')
   or target_id in (select id from orders where auction_id in ('auc_live','auc_side'));
delete from idempotency_records
where scope_id in ('auc_live','auc_side')
   or scope_id in (select id from orders where auction_id in ('auc_live','auc_side'));
delete from bids where auction_id in ('auc_live','auc_side');
delete from outbox_delivery
where outbox_id in (select id from outbox_events where auction_id in ('auc_live','auc_side'));
delete from outbox_events where auction_id in ('auc_live','auc_side');
delete from auction_events where auction_id in ('auc_live','auc_side');
delete from payment_events
where order_id in (select id from orders where auction_id in ('auc_live','auc_side'));
delete from orders where auction_id in ('auc_live','auc_side');
"

docker exec live-auction-redis sh -c "redis-cli --scan --pattern 'bid:{auc_live}:*' | xargs -r redis-cli del" >/dev/null
docker exec live-auction-redis sh -c "redis-cli --scan --pattern 'bid:{auc_side}:*' | xargs -r redis-cli del" >/dev/null
docker exec live-auction-redis redis-cli del auction:auc_live:events auction:auc_live:snapshot auction:auc_side:events auction:auc_side:snapshot >/dev/null

docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --delete --if-exists --topic auction.bid-events >/dev/null 2>&1 || true
docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --delete --if-exists --topic auction.dlq >/dev/null 2>&1 || true
sleep 2
docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --create --if-not-exists --topic auction.bid-events --partitions 16 --replication-factor 1
docker exec live-auction-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --create --if-not-exists --topic auction.dlq --partitions 16 --replication-factor 1

bash tests/pts/prepare-cloud-pressure.sh

docker exec live-auction-postgres psql -U live_auction -d live_auction -v ON_ERROR_STOP=1 -c "
update auctions
set status='ACTIVE',
    current_price_cents=start_price_cents,
    current_winner_id=null,
    end_at=now() + interval '30 minutes',
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
echo "- JMX: tests/pts/live-auction-l4b-final-second-100vu.jmx"
echo "- CSV: docs/perf/pts/${SESSION_CSV}"
echo "- Engine: BID_ENGINE_MODE=${BID_ENGINE_MODE}, ADMISSION_ENABLED=false, Redis=${REDIS_ADDR}, Kafka=${KAFKA_BROKERS}"
