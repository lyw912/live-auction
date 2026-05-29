#!/usr/bin/env bash
set -euo pipefail

AUCTION_ID="${AUCTION_ID:-auc_live}"
LABEL="${1:-after-l4b-pts}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/docs/perf/pts/evidence/$LABEL"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
KAFKA_CONTAINER="${KAFKA_CONTAINER:-live-auction-kafka}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"
KAFKA_BOOTSTRAP="${KAFKA_BOOTSTRAP:-localhost:9092}"
KAFKA_BID_TOPIC="${KAFKA_BID_TOPIC:-auction.bid-events}"
KAFKA_DLQ_TOPIC="${KAFKA_DLQ_TOPIC:-auction.dlq}"

mkdir -p "$OUT_DIR"

echo "[verify] writing $OUT_DIR/l4b-correctness.txt"

{
  echo "# L4B PTS correctness verification"
  echo "label=$LABEL"
  echo "auction_id=$AUCTION_ID"
  echo "collected_at=$(date -Is)"
  echo

  docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<'SQL'
\pset pager off
\timing on

\echo '## auction state'
select id, status, current_price_cents, current_winner_id,
       accepted_bid_count, seq, engine_seq, engine_epoch,
       engine_paused, engine_pause_reason, end_at
from auctions
where id = :'auction_id';

\echo '## bid status counts'
select status, settlement_status, count(*) as count
from bids
where auction_id = :'auction_id'
group by status, settlement_status
order by status, settlement_status;

\echo '## settlement status counts'
select status, result, ledger_source, count(*) as count,
       min(engine_seq) as min_engine_seq,
       max(engine_seq) as max_engine_seq,
       min(ledger_offset) as min_ledger_offset,
       max(ledger_offset) as max_ledger_offset
from redis_engine_settlements
where auction_id = :'auction_id'
group by status, result, ledger_source
order by status, result, ledger_source;

\echo '## non-terminal settlements'
select id, engine_epoch, engine_seq, status, result, attempts,
       last_error, dlq_error, created_at, updated_at
from redis_engine_settlements
where auction_id = :'auction_id'
  and status not in ('SETTLED','REJECTED','FAILED','DLQ')
order by engine_seq
limit 50;

\echo '## duplicate client_bid_id'
select client_bid_id, count(*) as count
from bids
where auction_id = :'auction_id'
group by client_bid_id
having count(*) > 1
order by count desc, client_bid_id
limit 50;

\echo '## accepted bid sequence gap count'
with accepted as (
  select seq
  from bids
  where auction_id = :'auction_id'
    and status = 'ACCEPTED'
    and seq is not null
),
bounds as (
  select min(seq) as min_seq, max(seq) as max_seq, count(*) as actual_count
  from accepted
),
expected as (
  select generate_series((select min_seq from bounds), (select max_seq from bounds)) as seq
)
select (select min_seq from bounds) as min_seq,
       (select max_seq from bounds) as max_seq,
       (select actual_count from bounds) as actual_count,
       count(expected.seq) filter (where accepted.seq is null) as missing_count
from expected
left join accepted using (seq);

\echo '## accepted bid missing sequences'
with accepted as (
  select seq
  from bids
  where auction_id = :'auction_id'
    and status = 'ACCEPTED'
    and seq is not null
),
bounds as (
  select min(seq) as min_seq, max(seq) as max_seq
  from accepted
),
expected as (
  select generate_series((select min_seq from bounds), (select max_seq from bounds)) as seq
)
select expected.seq
from expected
left join accepted using (seq)
where accepted.seq is null
order by expected.seq
limit 50;

\echo '## auction_events sequence gap count'
with events as (
  select seq
  from auction_events
  where auction_id = :'auction_id'
),
bounds as (
  select min(seq) as min_seq, max(seq) as max_seq, count(*) as actual_count
  from events
),
expected as (
  select generate_series((select min_seq from bounds), (select max_seq from bounds)) as seq
)
select (select min_seq from bounds) as min_seq,
       (select max_seq from bounds) as max_seq,
       (select actual_count from bounds) as actual_count,
       count(expected.seq) filter (where events.seq is null) as missing_count
from expected
left join events using (seq);

\echo '## outbox status counts'
select d.status, count(*) as count,
       max(now() - d.event_created_at) as max_age
from outbox_delivery d
join outbox_events e on e.id = d.outbox_id
where e.auction_id = :'auction_id'
group by d.status
order by d.status;

\echo '## order count'
select count(*) as orders,
       count(distinct winner_id) as winners,
       min(amount_cents) as min_amount,
       max(amount_cents) as max_amount
from orders
where auction_id = :'auction_id';

\echo '## recent settlement errors'
select id, engine_seq, status, result, attempts, last_error,
       dlq_topic, dlq_error, dlq_at
from redis_engine_settlements
where auction_id = :'auction_id'
  and (last_error is not null or dlq_error is not null or status in ('FAILED','DLQ'))
order by updated_at desc
limit 50;
SQL

  echo
  echo "## kafka topics"
  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --topic "$KAFKA_BID_TOPIC" || true
  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --topic "$KAFKA_DLQ_TOPIC" || true

  echo
  echo "## kafka dlq sample"
  echo "A timeout with zero processed messages means the sampled DLQ is empty."
  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-console-consumer.sh \
    --bootstrap-server "$KAFKA_BOOTSTRAP" --topic "$KAFKA_DLQ_TOPIC" \
    --from-beginning --timeout-ms 3000 --max-messages 20 2>&1 || true

  echo
  echo "## redis auction keys"
  docker exec "$REDIS_CONTAINER" redis-cli --scan --pattern "bid:{$AUCTION_ID}:*" || true
  docker exec "$REDIS_CONTAINER" redis-cli exists "auction:$AUCTION_ID:snapshot" "auction:$AUCTION_ID:events" || true
} > "$OUT_DIR/l4b-correctness.txt"

echo "[verify] done: $OUT_DIR/l4b-correctness.txt"
