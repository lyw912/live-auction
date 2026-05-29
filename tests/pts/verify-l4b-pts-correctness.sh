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
KAFKA_CONSUMER_GROUP="${KAFKA_CONSUMER_GROUP:-settlement-workers}"
FINAL_WAIT_SECONDS="${FINAL_WAIT_SECONDS:-0}"

mkdir -p "$OUT_DIR"

echo "[verify] writing $OUT_DIR/l4b-correctness.txt"

if [[ "$FINAL_WAIT_SECONDS" =~ ^[0-9]+$ ]] && [ "$FINAL_WAIT_SECONDS" -gt 0 ]; then
  echo "[verify] waiting ${FINAL_WAIT_SECONDS}s before final consistency checks"
  sleep "$FINAL_WAIT_SECONDS"
fi

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

\echo '## invariant gates'
with auction_row as (
  select id, status, current_price_cents, current_winner_id, accepted_bid_count,
         seq, engine_seq, engine_epoch, end_at, cap_price_cents
  from auctions
  where id = :'auction_id'
),
settlement as (
  select count(*) as total,
         count(*) filter (where result in ('ENGINE_ACCEPTED','ENGINE_SOLD')) as accepted_or_sold,
         count(*) filter (where status not in ('SETTLED','SKIPPED')) as non_terminal,
         count(*) filter (where ledger_source = 'kafka' and (ledger_topic is null or ledger_partition is null or ledger_offset is null)) as missing_kafka_position,
         count(distinct ledger_partition) filter (where ledger_source = 'kafka') as kafka_partitions
  from redis_engine_settlements
  where auction_id = :'auction_id'
),
bid_counts as (
  select count(*) filter (where status = 'ACCEPTED') as accepted,
         count(*) filter (where status = 'REJECTED') as rejected,
         count(*) filter (where status = 'ACCEPTED' and settlement_status <> 'SETTLED') as accepted_not_settled
  from bids
  where auction_id = :'auction_id'
),
accepted_gap as (
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
  select coalesce(count(expected.seq) filter (where accepted.seq is null), 0) as missing_count
  from expected
  left join accepted using (seq)
),
event_gap as (
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
  select coalesce(count(expected.seq) filter (where events.seq is null), 0) as missing_count
  from expected
  left join events using (seq)
),
duplicate_client_bid as (
  select count(*) as violations
  from (
    select client_bid_id
    from bids
    where auction_id = :'auction_id'
    group by client_bid_id
    having count(*) > 1
  ) d
),
duplicate_engine_seq as (
  select count(*) as violations
  from (
    select engine_epoch, engine_seq
    from bids
    where auction_id = :'auction_id'
      and status = 'ACCEPTED'
      and engine_seq is not null
    group by engine_epoch, engine_seq
    having count(*) > 1
  ) d
),
epoch_seq_violations as (
  select count(*) as violations
  from (
    select engine_epoch, engine_seq,
           lag(engine_epoch) over (order by engine_epoch, engine_seq, id) as prev_epoch,
           lag(engine_seq) over (partition by engine_epoch order by engine_seq, id) as prev_seq
    from redis_engine_settlements
    where auction_id = :'auction_id'
  ) o
  where (prev_epoch is not null and engine_epoch < prev_epoch)
     or (prev_seq is not null and engine_seq <= prev_seq)
),
kafka_order_violations as (
  select count(*) as violations
  from (
    select engine_epoch, engine_seq, ledger_partition, ledger_offset,
           lag(engine_seq) over (partition by ledger_partition order by ledger_offset) as prev_engine_seq,
           lag(ledger_offset) over (partition by engine_epoch order by engine_seq) as prev_offset_same_epoch
    from redis_engine_settlements
    where auction_id = :'auction_id'
      and ledger_source = 'kafka'
      and ledger_partition is not null
      and ledger_offset is not null
  ) o
  where (prev_engine_seq is not null and engine_seq <= prev_engine_seq)
     or (prev_offset_same_epoch is not null and ledger_offset <= prev_offset_same_epoch)
),
created_at_inversions as (
  select count(*) as violations
  from (
    select seq, created_at,
           lag(created_at) over (order by seq) as prev_created_at
    from bids
    where auction_id = :'auction_id'
      and status = 'ACCEPTED'
      and seq is not null
  ) o
  where prev_created_at is not null
    and created_at < prev_created_at
),
accepted_after_end as (
  select count(*) as violations
  from bids b
  join auctions a on a.id = b.auction_id
  where b.auction_id = :'auction_id'
    and b.status = 'ACCEPTED'
    and b.created_at > a.end_at
),
increment_violations as (
  select count(*) as violations
  from (
    select b.seq, b.amount_cents,
           lag(b.amount_cents) over (order by b.seq) as prev_amount,
           a.increment_cents
    from bids b
    join auctions a on a.id = b.auction_id
    where b.auction_id = :'auction_id'
      and b.status = 'ACCEPTED'
      and b.seq is not null
  ) o
  where prev_amount is not null
    and ((amount_cents - prev_amount) <= 0
         or (amount_cents - prev_amount) % increment_cents <> 0)
),
orders_count as (
  select count(*) as orders
  from orders
  where auction_id = :'auction_id'
),
outbox_not_published as (
  select count(*) as pending
  from outbox_delivery d
  join outbox_events e on e.id = d.outbox_id
  where e.auction_id = :'auction_id'
    and d.status <> 'PUBLISHED'
),
cross_auction_mismatch as (
  select count(*) as violations
  from auction_events e
  where e.auction_id = :'auction_id'
    and (
      (e.payload_json ? 'auction_id' and e.payload_json->>'auction_id' <> :'auction_id')
      or (e.payload_json ? 'bid_id' and not exists (
        select 1
        from bids b
        where b.auction_id = e.auction_id
          and b.id = e.payload_json->>'bid_id'
      ))
    )
)
select *
from (
  values
    ('P0', 'auction_exists', (select count(*) = 1 from auction_row), 'auction row must exist'),
    ('P0', 'no_non_terminal_settlements', (select non_terminal = 0 from settlement), 'no PROCESSING/FAILED/DLQ settlement rows after the chosen settle window'),
    ('P0', 'redis_kafka_pg_accepted_match', (select accepted_or_sold = accepted from settlement, bid_counts), 'accepted/sold Kafka ledger rows must match PG accepted bids'),
    ('P0', 'auction_accepted_count_matches_pg', (select accepted_bid_count = accepted from auction_row, bid_counts), 'auction accepted counter must match PG accepted bids'),
    ('P0', 'auction_seq_matches_engine_seq', (select seq = engine_seq from auction_row), 'auction seq and engine_seq must converge'),
    ('P0', 'kafka_position_present', (select missing_kafka_position = 0 from settlement), 'every Kafka ledger settlement must record topic/partition/offset'),
    ('P0', 'no_accepted_bid_seq_gap', (select missing_count = 0 from accepted_gap), 'accepted bid seq must be continuous'),
    ('P0', 'no_auction_event_seq_gap', (select missing_count = 0 from event_gap), 'auction event seq must be continuous'),
    ('P0', 'no_duplicate_client_bid_id', (select violations = 0 from duplicate_client_bid), 'client_bid_id must not create duplicate bid rows'),
    ('P0', 'no_duplicate_engine_seq', (select violations = 0 from duplicate_engine_seq), 'engine_epoch/engine_seq must identify at most one accepted bid'),
    ('P0', 'engine_epoch_seq_monotonic', (select violations = 0 from epoch_seq_violations), 'engine_epoch/engine_seq must be monotonic'),
    ('P0', 'kafka_offset_matches_engine_order', (select violations = 0 from kafka_order_violations), 'Kafka offset order must preserve engine_seq order for the auction partition'),
    ('P0', 'no_created_at_seq_inversion', (select violations = 0 from created_at_inversions), 'created_at must not go backward relative to accepted seq'),
    ('P0', 'no_accepted_after_final_end', (select violations = 0 from accepted_after_end), 'no accepted bid may appear after final end_at'),
    ('P0', 'increment_grid_valid', (select violations = 0 from increment_violations), 'accepted bid deltas must follow auction increment_cents'),
    ('P0', 'at_most_one_order', (select orders <= 1 from orders_count), 'one auction can create at most one order'),
    ('P0', 'no_cross_auction_event_payload_leak', (select violations = 0 from cross_auction_mismatch), 'event payload bid_id/auction_id must belong to the same auction'),
    ('P1', 'outbox_drained', (select pending = 0 from outbox_not_published), 'outbox should drain after the chosen settle window')
) as gates(severity, name, pass, detail)
order by severity, name;

\echo '## l4b hard gate summary'
with auction_row as (
  select id, status, current_price_cents, current_winner_id, accepted_bid_count,
         seq, engine_seq, engine_epoch, end_at
  from auctions
  where id = :'auction_id'
),
settlement as (
  select count(*) as total,
         count(*) filter (where result in ('ENGINE_ACCEPTED','ENGINE_SOLD')) as accepted_or_sold,
         count(*) filter (where status not in ('SETTLED','SKIPPED')) as open_or_failed,
         count(*) filter (where ledger_source = 'kafka' and (ledger_topic is null or ledger_partition is null or ledger_offset is null)) as missing_kafka_position,
         count(distinct ledger_partition) filter (where ledger_source = 'kafka') as kafka_partitions
  from redis_engine_settlements
  where auction_id = :'auction_id'
),
bid_counts as (
  select count(*) filter (where status = 'ACCEPTED') as accepted,
         count(*) filter (where status = 'REJECTED') as rejected,
         count(*) filter (where status = 'ACCEPTED' and settlement_status <> 'SETTLED') as accepted_not_settled
  from bids
  where auction_id = :'auction_id'
),
event_counts as (
  select count(*) as events
  from auction_events
  where auction_id = :'auction_id'
),
orders_count as (
  select count(*) as orders
  from orders
  where auction_id = :'auction_id'
)
select auction_row.accepted_bid_count as auction_accepted,
       auction_row.seq as auction_seq,
       auction_row.engine_seq as auction_engine_seq,
       settlement.total as settlement_total,
       settlement.accepted_or_sold as settlement_accepted_or_sold,
       settlement.open_or_failed as open_or_failed_settlements,
       settlement.missing_kafka_position,
       settlement.kafka_partitions,
       bid_counts.accepted as pg_accepted_bids,
       bid_counts.accepted_not_settled,
       event_counts.events as auction_events,
       orders_count.orders
from auction_row, settlement, bid_counts, event_counts, orders_count;

\echo '## non-terminal settlements'
select id, engine_epoch, engine_seq, status, result, attempts,
       last_error, dlq_error, created_at, updated_at
from redis_engine_settlements
where auction_id = :'auction_id'
  and status not in ('SETTLED','SKIPPED')
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

\echo '## duplicate successful idempotency keys'
select scope_type, scope_id, user_id, idempotency_key, count(*) as count
from idempotency_records
where scope_type = 'bid'
  and scope_id = :'auction_id'
  and status = 'COMPLETED'
group by scope_type, scope_id, user_id, idempotency_key
having count(*) > 1
order by count desc, user_id, idempotency_key
limit 50;

\echo '## duplicate engine sequence rows'
select engine_epoch, engine_seq,
       count(*) as bids,
       count(distinct client_bid_id) as client_bid_ids
from bids
where auction_id = :'auction_id'
  and status = 'ACCEPTED'
  and engine_seq is not null
group by engine_epoch, engine_seq
having count(*) > 1
order by engine_epoch, engine_seq
limit 50;

\echo '## engine epoch/seq monotonicity violations in settlements'
with ordered as (
  select engine_epoch, engine_seq,
         lag(engine_epoch) over (order by engine_epoch, engine_seq, id) as prev_epoch,
         lag(engine_seq) over (partition by engine_epoch order by engine_seq, id) as prev_seq
  from redis_engine_settlements
  where auction_id = :'auction_id'
)
select *
from ordered
where (prev_epoch is not null and engine_epoch < prev_epoch)
   or (prev_seq is not null and engine_seq <= prev_seq)
limit 50;

\echo '## kafka offset ordering violations'
with ordered as (
  select engine_epoch, engine_seq, ledger_partition, ledger_offset,
         lag(engine_seq) over (partition by ledger_partition order by ledger_offset) as prev_engine_seq,
         lag(ledger_offset) over (partition by engine_epoch order by engine_seq) as prev_offset_same_epoch
  from redis_engine_settlements
  where auction_id = :'auction_id'
    and ledger_source = 'kafka'
    and ledger_partition is not null
    and ledger_offset is not null
)
select *
from ordered
where (prev_engine_seq is not null and engine_seq <= prev_engine_seq)
   or (prev_offset_same_epoch is not null and ledger_offset <= prev_offset_same_epoch)
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

\echo '## created_at inversion by seq'
with ordered as (
  select seq, created_at,
         lag(created_at) over (order by seq) as prev_created_at
  from bids
  where auction_id = :'auction_id'
    and status = 'ACCEPTED'
    and seq is not null
)
select count(*) as inversion_count
from ordered
where prev_created_at is not null
  and created_at < prev_created_at;

\echo '## soft close accepted after final end_at'
select count(*) as accepted_after_final_end
from bids b
join auctions a on a.id = b.auction_id
where b.auction_id = :'auction_id'
  and b.status = 'ACCEPTED'
  and b.created_at > a.end_at;

\echo '## increment grid violations'
with ordered as (
  select b.seq, b.amount_cents,
         lag(b.amount_cents) over (order by b.seq) as prev_amount,
         a.increment_cents
  from bids b
  join auctions a on a.id = b.auction_id
  where b.auction_id = :'auction_id'
    and b.status = 'ACCEPTED'
    and b.seq is not null
)
select count(*) as off_grid_count
from ordered
where prev_amount is not null
  and ((amount_cents - prev_amount) <= 0
       or (amount_cents - prev_amount) % increment_cents <> 0);

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

\echo '## duplicate order rows'
select auction_id, count(*) as count
from orders
where auction_id = :'auction_id'
group by auction_id
having count(*) > 1;

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
  echo "## machine-readable invariant gates"
  docker exec -i "$DB_CONTAINER" psql -q -A -F $'\t' -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<'SQL'
with auction_row as (
  select id, status, current_price_cents, current_winner_id, accepted_bid_count,
         seq, engine_seq, engine_epoch, end_at, cap_price_cents
  from auctions
  where id = :'auction_id'
),
settlement as (
  select count(*) as total,
         count(*) filter (where result in ('ENGINE_ACCEPTED','ENGINE_SOLD')) as accepted_or_sold,
         count(*) filter (where status not in ('SETTLED','SKIPPED')) as non_terminal,
         count(*) filter (where ledger_source = 'kafka' and (ledger_topic is null or ledger_partition is null or ledger_offset is null)) as missing_kafka_position
  from redis_engine_settlements
  where auction_id = :'auction_id'
),
bid_counts as (
  select count(*) filter (where status = 'ACCEPTED') as accepted,
         count(*) filter (where status = 'ACCEPTED' and settlement_status <> 'SETTLED') as accepted_not_settled
  from bids
  where auction_id = :'auction_id'
),
accepted_gap as (
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
  select coalesce(count(expected.seq) filter (where accepted.seq is null), 0) as missing_count
  from expected
  left join accepted using (seq)
),
event_gap as (
  with events as (
    select seq
    from auction_events
    where auction_id = :'auction_id'
  ),
  bounds as (
    select min(seq) as min_seq, max(seq) as max_seq
    from events
  ),
  expected as (
    select generate_series((select min_seq from bounds), (select max_seq from bounds)) as seq
  )
  select coalesce(count(expected.seq) filter (where events.seq is null), 0) as missing_count
  from expected
  left join events using (seq)
),
duplicate_client_bid as (
  select count(*) as violations
  from (
    select client_bid_id
    from bids
    where auction_id = :'auction_id'
    group by client_bid_id
    having count(*) > 1
  ) d
),
duplicate_engine_seq as (
  select count(*) as violations
  from (
    select engine_epoch, engine_seq
    from bids
    where auction_id = :'auction_id'
      and status = 'ACCEPTED'
      and engine_seq is not null
    group by engine_epoch, engine_seq
    having count(*) > 1
  ) d
),
epoch_seq_violations as (
  select count(*) as violations
  from (
    select engine_epoch, engine_seq,
           lag(engine_epoch) over (order by engine_epoch, engine_seq, id) as prev_epoch,
           lag(engine_seq) over (partition by engine_epoch order by engine_seq, id) as prev_seq
    from redis_engine_settlements
    where auction_id = :'auction_id'
  ) o
  where (prev_epoch is not null and engine_epoch < prev_epoch)
     or (prev_seq is not null and engine_seq <= prev_seq)
),
kafka_order_violations as (
  select count(*) as violations
  from (
    select engine_epoch, engine_seq, ledger_partition, ledger_offset,
           lag(engine_seq) over (partition by ledger_partition order by ledger_offset) as prev_engine_seq,
           lag(ledger_offset) over (partition by engine_epoch order by engine_seq) as prev_offset_same_epoch
    from redis_engine_settlements
    where auction_id = :'auction_id'
      and ledger_source = 'kafka'
      and ledger_partition is not null
      and ledger_offset is not null
  ) o
  where (prev_engine_seq is not null and engine_seq <= prev_engine_seq)
     or (prev_offset_same_epoch is not null and ledger_offset <= prev_offset_same_epoch)
),
created_at_inversions as (
  select count(*) as violations
  from (
    select seq, created_at,
           lag(created_at) over (order by seq) as prev_created_at
    from bids
    where auction_id = :'auction_id'
      and status = 'ACCEPTED'
      and seq is not null
  ) o
  where prev_created_at is not null
    and created_at < prev_created_at
),
accepted_after_end as (
  select count(*) as violations
  from bids b
  join auctions a on a.id = b.auction_id
  where b.auction_id = :'auction_id'
    and b.status = 'ACCEPTED'
    and b.created_at > a.end_at
),
increment_violations as (
  select count(*) as violations
  from (
    select b.seq, b.amount_cents,
           lag(b.amount_cents) over (order by b.seq) as prev_amount,
           a.increment_cents
    from bids b
    join auctions a on a.id = b.auction_id
    where b.auction_id = :'auction_id'
      and b.status = 'ACCEPTED'
      and b.seq is not null
  ) o
  where prev_amount is not null
    and ((amount_cents - prev_amount) <= 0
         or (amount_cents - prev_amount) % increment_cents <> 0)
),
orders_count as (
  select count(*) as orders
  from orders
  where auction_id = :'auction_id'
),
outbox_not_published as (
  select count(*) as pending
  from outbox_delivery d
  join outbox_events e on e.id = d.outbox_id
  where e.auction_id = :'auction_id'
    and d.status <> 'PUBLISHED'
),
cross_auction_mismatch as (
  select count(*) as violations
  from auction_events e
  where e.auction_id = :'auction_id'
    and (
      (e.payload_json ? 'auction_id' and e.payload_json->>'auction_id' <> :'auction_id')
      or (e.payload_json ? 'bid_id' and not exists (
        select 1
        from bids b
        where b.auction_id = e.auction_id
          and b.id = e.payload_json->>'bid_id'
      ))
    )
)
select severity, name, case when pass then 'PASS' else 'FAIL' end as status, detail
from (
  values
    ('P0', 'auction_exists', (select count(*) = 1 from auction_row), 'auction row must exist'),
    ('P0', 'no_non_terminal_settlements', (select non_terminal = 0 from settlement), 'no PROCESSING/FAILED/DLQ settlement rows after the chosen settle window'),
    ('P0', 'redis_kafka_pg_accepted_match', (select accepted_or_sold = accepted from settlement, bid_counts), 'accepted/sold Kafka ledger rows must match PG accepted bids'),
    ('P0', 'auction_accepted_count_matches_pg', (select accepted_bid_count = accepted from auction_row, bid_counts), 'auction accepted counter must match PG accepted bids'),
    ('P0', 'auction_seq_matches_engine_seq', (select seq = engine_seq from auction_row), 'auction seq and engine_seq must converge'),
    ('P0', 'kafka_position_present', (select missing_kafka_position = 0 from settlement), 'every Kafka ledger settlement must record topic/partition/offset'),
    ('P0', 'no_accepted_bid_seq_gap', (select missing_count = 0 from accepted_gap), 'accepted bid seq must be continuous'),
    ('P0', 'no_auction_event_seq_gap', (select missing_count = 0 from event_gap), 'auction event seq must be continuous'),
    ('P0', 'no_duplicate_client_bid_id', (select violations = 0 from duplicate_client_bid), 'client_bid_id must not create duplicate bid rows'),
    ('P0', 'no_duplicate_engine_seq', (select violations = 0 from duplicate_engine_seq), 'engine_epoch/engine_seq must identify at most one accepted bid'),
    ('P0', 'engine_epoch_seq_monotonic', (select violations = 0 from epoch_seq_violations), 'engine_epoch/engine_seq must be monotonic'),
    ('P0', 'kafka_offset_matches_engine_order', (select violations = 0 from kafka_order_violations), 'Kafka offset order must preserve engine_seq order for the auction partition'),
    ('P0', 'no_created_at_seq_inversion', (select violations = 0 from created_at_inversions), 'created_at must not go backward relative to accepted seq'),
    ('P0', 'no_accepted_after_final_end', (select violations = 0 from accepted_after_end), 'no accepted bid may appear after final end_at'),
    ('P0', 'increment_grid_valid', (select violations = 0 from increment_violations), 'accepted bid deltas must follow auction increment_cents'),
    ('P0', 'at_most_one_order', (select orders <= 1 from orders_count), 'one auction can create at most one order'),
    ('P0', 'no_cross_auction_event_payload_leak', (select violations = 0 from cross_auction_mismatch), 'event payload bid_id/auction_id must belong to the same auction'),
    ('P1', 'outbox_drained', (select pending = 0 from outbox_not_published), 'outbox should drain after the chosen settle window')
) as gates(severity, name, pass, detail)
order by severity, name;
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
  echo "## kafka consumer group offsets"
  docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-consumer-groups.sh \
    --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --group "$KAFKA_CONSUMER_GROUP" 2>&1 || true

  echo
  echo "## redis auction keys"
  docker exec "$REDIS_CONTAINER" redis-cli --scan --pattern "bid:{$AUCTION_ID}:*" || true
  docker exec "$REDIS_CONTAINER" redis-cli exists "auction:$AUCTION_ID:snapshot" "auction:$AUCTION_ID:events" || true
  echo
  echo "## redis pending decisions"
  docker exec "$REDIS_CONTAINER" redis-cli HLEN "bid:{$AUCTION_ID}:engine:pending" || true
  docker exec "$REDIS_CONTAINER" redis-cli HGETALL "bid:{$AUCTION_ID}:engine:pending" || true
  echo
  echo "## redis memory hard gates"
  docker exec "$REDIS_CONTAINER" redis-cli INFO memory | grep -E '^(used_memory:|maxmemory:|maxmemory_policy:)' || true
  docker exec "$REDIS_CONTAINER" redis-cli INFO stats | grep -E '^(evicted_keys:|rejected_connections:|total_error_replies:)' || true
} > "$OUT_DIR/l4b-correctness.txt"

docker exec -i "$DB_CONTAINER" psql -q -A -t -F $'\t' -U "$DB_USER" -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - > "$OUT_DIR/l4b-invariant-gates.tsv" <<'SQL'
with auction_row as (
  select id, status, current_price_cents, current_winner_id, accepted_bid_count,
         seq, engine_seq, engine_epoch, end_at
  from auctions
  where id = :'auction_id'
),
settlement as (
  select count(*) as total,
         count(*) filter (where result in ('ENGINE_ACCEPTED','ENGINE_SOLD')) as accepted_or_sold,
         count(*) filter (where status not in ('SETTLED','SKIPPED')) as non_terminal,
         count(*) filter (where ledger_source = 'kafka' and (ledger_topic is null or ledger_partition is null or ledger_offset is null)) as missing_kafka_position
  from redis_engine_settlements
  where auction_id = :'auction_id'
),
bid_counts as (
  select count(*) filter (where status = 'ACCEPTED') as accepted,
         count(*) filter (where status = 'ACCEPTED' and settlement_status <> 'SETTLED') as accepted_not_settled
  from bids
  where auction_id = :'auction_id'
),
accepted_gap as (
  with accepted as (
    select seq from bids where auction_id = :'auction_id' and status = 'ACCEPTED' and seq is not null
  ),
  bounds as (
    select min(seq) as min_seq, max(seq) as max_seq from accepted
  ),
  expected as (
    select generate_series((select min_seq from bounds), (select max_seq from bounds)) as seq
  )
  select coalesce(count(expected.seq) filter (where accepted.seq is null), 0) as missing_count
  from expected left join accepted using (seq)
),
event_gap as (
  with events as (
    select seq from auction_events where auction_id = :'auction_id'
  ),
  bounds as (
    select min(seq) as min_seq, max(seq) as max_seq from events
  ),
  expected as (
    select generate_series((select min_seq from bounds), (select max_seq from bounds)) as seq
  )
  select coalesce(count(expected.seq) filter (where events.seq is null), 0) as missing_count
  from expected left join events using (seq)
),
duplicate_client_bid as (
  select count(*) as violations from (
    select client_bid_id from bids where auction_id = :'auction_id' group by client_bid_id having count(*) > 1
  ) d
),
duplicate_engine_seq as (
  select count(*) as violations from (
    select engine_epoch, engine_seq
    from bids
    where auction_id = :'auction_id' and status = 'ACCEPTED' and engine_seq is not null
    group by engine_epoch, engine_seq
    having count(*) > 1
  ) d
),
epoch_seq_violations as (
  select count(*) as violations from (
    select engine_epoch, engine_seq,
           lag(engine_epoch) over (order by engine_epoch, engine_seq, id) as prev_epoch,
           lag(engine_seq) over (partition by engine_epoch order by engine_seq, id) as prev_seq
    from redis_engine_settlements
    where auction_id = :'auction_id'
  ) o
  where (prev_epoch is not null and engine_epoch < prev_epoch)
     or (prev_seq is not null and engine_seq <= prev_seq)
),
kafka_order_violations as (
  select count(*) as violations from (
    select engine_epoch, engine_seq, ledger_partition, ledger_offset,
           lag(engine_seq) over (partition by ledger_partition order by ledger_offset) as prev_engine_seq,
           lag(ledger_offset) over (partition by engine_epoch order by engine_seq) as prev_offset_same_epoch
    from redis_engine_settlements
    where auction_id = :'auction_id'
      and ledger_source = 'kafka'
      and ledger_partition is not null
      and ledger_offset is not null
  ) o
  where (prev_engine_seq is not null and engine_seq <= prev_engine_seq)
     or (prev_offset_same_epoch is not null and ledger_offset <= prev_offset_same_epoch)
),
created_at_inversions as (
  select count(*) as violations from (
    select seq, created_at, lag(created_at) over (order by seq) as prev_created_at
    from bids
    where auction_id = :'auction_id' and status = 'ACCEPTED' and seq is not null
  ) o
  where prev_created_at is not null and created_at < prev_created_at
),
accepted_after_end as (
  select count(*) as violations
  from bids b join auctions a on a.id = b.auction_id
  where b.auction_id = :'auction_id' and b.status = 'ACCEPTED' and b.created_at > a.end_at
),
increment_violations as (
  select count(*) as violations from (
    select b.seq, b.amount_cents, lag(b.amount_cents) over (order by b.seq) as prev_amount, a.increment_cents
    from bids b join auctions a on a.id = b.auction_id
    where b.auction_id = :'auction_id' and b.status = 'ACCEPTED' and b.seq is not null
  ) o
  where prev_amount is not null
    and ((amount_cents - prev_amount) <= 0 or (amount_cents - prev_amount) % increment_cents <> 0)
),
orders_count as (
  select count(*) as orders from orders where auction_id = :'auction_id'
),
outbox_not_published as (
  select count(*) as pending
  from outbox_delivery d join outbox_events e on e.id = d.outbox_id
  where e.auction_id = :'auction_id' and d.status <> 'PUBLISHED'
),
cross_auction_mismatch as (
  select count(*) as violations
  from auction_events e
  where e.auction_id = :'auction_id'
    and (
      (e.payload_json ? 'auction_id' and e.payload_json->>'auction_id' <> :'auction_id')
      or (e.payload_json ? 'bid_id' and not exists (
        select 1
        from bids b
        where b.auction_id = e.auction_id
          and b.id = e.payload_json->>'bid_id'
      ))
    )
)
select severity, name, case when pass then 'PASS' else 'FAIL' end as status, detail
from (
  values
    ('P0', 'auction_exists', (select count(*) = 1 from auction_row), 'auction row must exist'),
    ('P0', 'no_non_terminal_settlements', (select non_terminal = 0 from settlement), 'no PROCESSING/FAILED/DLQ settlement rows after the chosen settle window'),
    ('P0', 'redis_kafka_pg_accepted_match', (select accepted_or_sold = accepted from settlement, bid_counts), 'accepted/sold Kafka ledger rows must match PG accepted bids'),
    ('P0', 'auction_accepted_count_matches_pg', (select accepted_bid_count = accepted from auction_row, bid_counts), 'auction accepted counter must match PG accepted bids'),
    ('P0', 'auction_seq_matches_engine_seq', (select seq = engine_seq from auction_row), 'auction seq and engine_seq must converge'),
    ('P0', 'kafka_position_present', (select missing_kafka_position = 0 from settlement), 'every Kafka ledger settlement must record topic/partition/offset'),
    ('P0', 'no_accepted_bid_seq_gap', (select missing_count = 0 from accepted_gap), 'accepted bid seq must be continuous'),
    ('P0', 'no_auction_event_seq_gap', (select missing_count = 0 from event_gap), 'auction event seq must be continuous'),
    ('P0', 'no_duplicate_client_bid_id', (select violations = 0 from duplicate_client_bid), 'client_bid_id must not create duplicate bid rows'),
    ('P0', 'no_duplicate_engine_seq', (select violations = 0 from duplicate_engine_seq), 'engine_epoch/engine_seq must identify at most one accepted bid'),
    ('P0', 'engine_epoch_seq_monotonic', (select violations = 0 from epoch_seq_violations), 'engine_epoch/engine_seq must be monotonic'),
    ('P0', 'kafka_offset_matches_engine_order', (select violations = 0 from kafka_order_violations), 'Kafka offset order must preserve engine_seq order for the auction partition'),
    ('P0', 'no_created_at_seq_inversion', (select violations = 0 from created_at_inversions), 'created_at must not go backward relative to accepted seq'),
    ('P0', 'no_accepted_after_final_end', (select violations = 0 from accepted_after_end), 'no accepted bid may appear after final end_at'),
    ('P0', 'increment_grid_valid', (select violations = 0 from increment_violations), 'accepted bid deltas must follow auction increment_cents'),
    ('P0', 'at_most_one_order', (select orders <= 1 from orders_count), 'one auction can create at most one order'),
    ('P0', 'no_cross_auction_event_payload_leak', (select violations = 0 from cross_auction_mismatch), 'event payload bid_id/auction_id must belong to the same auction'),
    ('P1', 'outbox_drained', (select pending = 0 from outbox_not_published), 'outbox should drain after the chosen settle window')
) as gates(severity, name, pass, detail)
order by severity, name;
SQL

redis_gate_file="$OUT_DIR/l4b-redis-gates.tsv"
{
  evicted_keys="$(docker exec "$REDIS_CONTAINER" redis-cli INFO stats | awk -F: '/^evicted_keys:/ {gsub(/\r/, "", $2); print $2}')"
  rejected_connections="$(docker exec "$REDIS_CONTAINER" redis-cli INFO stats | awk -F: '/^rejected_connections:/ {gsub(/\r/, "", $2); print $2}')"
  maxmemory_policy="$(docker exec "$REDIS_CONTAINER" redis-cli INFO memory | awk -F: '/^maxmemory_policy:/ {gsub(/\r/, "", $2); print $2}')"
  printf 'P0\tredis_no_eviction\t%s\tevicted_keys must stay zero\n' "$([ "${evicted_keys:-0}" = "0" ] && echo PASS || echo FAIL)"
  printf 'P1\tredis_no_rejected_connections\t%s\trejected_connections should stay zero\n' "$([ "${rejected_connections:-0}" = "0" ] && echo PASS || echo FAIL)"
  printf 'P0\tredis_noeviction_policy\t%s\tRedis maxmemory policy must not evict hot auction state\n' "$([ "${maxmemory_policy:-}" = "noeviction" ] && echo PASS || echo FAIL)"
} > "$redis_gate_file"

cat "$redis_gate_file" >> "$OUT_DIR/l4b-invariant-gates.tsv"

kafka_gate_file="$OUT_DIR/l4b-kafka-gates.tsv"
{
  dlq_offsets="$(docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-get-offsets.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --topic "$KAFKA_DLQ_TOPIC" 2>/dev/null || true)"
  dlq_total="$(printf '%s\n' "$dlq_offsets" | awk -F: 'NF >= 3 {sum += $3} END {print sum + 0}')"
  group_lag="$(docker exec "$KAFKA_CONTAINER" /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server "$KAFKA_BOOTSTRAP" --describe --group "$KAFKA_CONSUMER_GROUP" 2>/dev/null | awk 'NR > 1 && $6 ~ /^[0-9]+$/ {sum += $6} END {print sum + 0}')"
  printf 'P0\tdlq_empty\t%s\tKafka DLQ must stay empty or be explained before release\n' "$([ "${dlq_total:-0}" = "0" ] && echo PASS || echo FAIL)"
  printf 'P1\tkafka_consumer_group_lag_zero\t%s\tSettlement consumer group lag should drain to zero after the chosen settle window\n' "$([ "${group_lag:-0}" = "0" ] && echo PASS || echo FAIL)"
} > "$kafka_gate_file"

cat "$kafka_gate_file" >> "$OUT_DIR/l4b-invariant-gates.tsv"

redis_pending_gate_file="$OUT_DIR/l4b-redis-pending-gates.tsv"
{
  pending_count="$(docker exec "$REDIS_CONTAINER" redis-cli HLEN "bid:{$AUCTION_ID}:engine:pending" | tr -d '\r')"
  printf 'P0\tredis_pending_decisions_empty\t%s\tRedis pending decisions must be zero; otherwise Redis accepted work may not have reached Kafka\n' "$([ "${pending_count:-0}" = "0" ] && echo PASS || echo FAIL)"
} > "$redis_pending_gate_file"

cat "$redis_pending_gate_file" >> "$OUT_DIR/l4b-invariant-gates.tsv"

if awk -F '\t' '$1 == "P0" && $3 == "FAIL" { found=1 } END { exit found ? 0 : 1 }' "$OUT_DIR/l4b-invariant-gates.tsv"; then
  echo "[verify] P0 invariant violation; see $OUT_DIR/l4b-invariant-gates.tsv" >&2
  exit 1
fi

echo "[verify] done: $OUT_DIR/l4b-correctness.txt"
echo "[verify] gates: $OUT_DIR/l4b-invariant-gates.tsv"
