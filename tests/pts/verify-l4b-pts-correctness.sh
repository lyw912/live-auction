#!/usr/bin/env bash
set -euo pipefail

AUCTION_ID="${AUCTION_ID:-auc_live}"
LABEL="${1:-after-l4b-pts}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVIDENCE_ROOT="${EVIDENCE_ROOT:-$ROOT_DIR/docs/perf/pts/evidence/incoming}"
OUT_DIR="$EVIDENCE_ROOT/$LABEL"
DB_CONTAINER="${DB_CONTAINER:-live-auction-postgres}"
KAFKA_CONTAINER="${KAFKA_CONTAINER:-live-auction-kafka}"
REDIS_CONTAINER="${REDIS_CONTAINER:-live-auction-redis}"
DB_USER="${DB_USER:-live_auction}"
DB_NAME="${DB_NAME:-live_auction}"
KAFKA_BOOTSTRAP="${KAFKA_BOOTSTRAP:-localhost:9092}"
KAFKA_BID_TOPIC="${KAFKA_BID_TOPIC:-auction.bid-events}"
KAFKA_DLQ_TOPIC="${KAFKA_DLQ_TOPIC:-auction.dlq}"
KAFKA_CONSUMER_GROUP="${KAFKA_CONSUMER_GROUP:-settlement-workers}"
FINAL_WAIT_SECONDS="${FINAL_WAIT_SECONDS:-30}"
REDIS_DATA_LOSS_OK="${REDIS_DATA_LOSS_OK:-0}"
if [ "${EXPECTED_UNIQUE_BIDS+x}" = "x" ]; then
  EXPECTED_UNIQUE_BIDS="${EXPECTED_UNIQUE_BIDS}"
elif [ "${EXPECTED_BIDS+x}" = "x" ]; then
  EXPECTED_UNIQUE_BIDS="${EXPECTED_BIDS}"
else
  EXPECTED_UNIQUE_BIDS="${SESSION_COUNT:-1000}"
fi

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
  echo "expected_unique_bids=${EXPECTED_UNIQUE_BIDS:-disabled}"
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
         seq, engine_seq, engine_epoch, end_at, cap_price_cents,
         engine_paused, engine_pause_reason
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
engine_seq_matches_settlement as (
  select coalesce((select engine_seq from auction_row), 0) = coalesce(max(engine_seq) filter (where status = 'SETTLED'), 0) as pass
  from redis_engine_settlements
  where auction_id = :'auction_id'
    and engine_epoch = (select engine_epoch from auction_row)
),
accepted_settlement_coverage as (
  select count(*) as violations
  from redis_engine_settlements s
  left join bids b
    on b.auction_id = s.auction_id
   and b.engine_epoch = s.engine_epoch
   and b.engine_seq = s.engine_seq
   and b.status = 'ACCEPTED'
  left join auction_events e
    on e.auction_id = s.auction_id
   and e.engine_epoch = s.engine_epoch
   and e.engine_seq = s.engine_seq
   and e.event_type in ('bid_accepted','auction_sold')
  where s.auction_id = :'auction_id'
    and s.status = 'SETTLED'
    and s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
    and (b.id is null or e.id is null)
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
    ('P0', 'engine_not_paused', (select not engine_paused from auction_row), 'auction engine must not be paused after a valid PTS run'),
    ('P0', 'no_non_terminal_settlements', (select non_terminal = 0 from settlement), 'no PROCESSING/FAILED/DLQ settlement rows after the chosen settle window'),
    ('P0', 'redis_kafka_pg_accepted_match', (select accepted_or_sold = accepted from settlement, bid_counts), 'accepted/sold Kafka ledger rows must match PG accepted bids'),
    ('P0', 'auction_accepted_count_matches_pg', (select accepted_bid_count = accepted from auction_row, bid_counts), 'auction accepted counter must match PG accepted bids'),
    ('P0', 'redis_engine_seq_matches_settlement', (select pass from engine_seq_matches_settlement), 'auction engine_seq must equal the latest settled Redis/Kafka engine ledger seq'),
    ('P0', 'kafka_position_present', (select missing_kafka_position = 0 from settlement), 'every Kafka ledger settlement must record topic/partition/offset'),
    ('P0', 'accepted_settlement_has_public_event', (select violations = 0 from accepted_settlement_coverage), 'accepted/sold settlements must have matching bid and public auction_event rows'),
    ('P0', 'no_public_auction_event_seq_gap', (select missing_count = 0 from event_gap), 'public auction event seq must be continuous'),
    ('P0', 'no_duplicate_client_bid_id', (select violations = 0 from duplicate_client_bid), 'client_bid_id must not create duplicate bid rows'),
    ('P0', 'no_duplicate_engine_seq', (select violations = 0 from duplicate_engine_seq), 'engine_epoch/engine_seq must identify at most one accepted bid'),
    ('P0', 'engine_epoch_seq_monotonic', (select violations = 0 from epoch_seq_violations), 'engine_epoch/engine_seq must be monotonic'),
    ('P0', 'kafka_offset_matches_engine_order', (select violations = 0 from kafka_order_violations), 'Kafka offset order must preserve engine_seq order for the auction partition'),
    ('P0', 'no_created_at_seq_inversion', (select violations = 0 from created_at_inversions), 'created_at must not go backward relative to accepted seq'),
    ('P0', 'no_accepted_after_final_end', (select violations = 0 from accepted_after_end), 'no accepted bid may appear after final end_at'),
    ('P0', 'increment_grid_valid', (select violations = 0 from increment_violations), 'accepted bid deltas must follow auction increment_cents'),
    ('P0', 'at_most_one_order', (select orders <= 1 from orders_count), 'one auction can create at most one order'),
    ('P0', 'no_cross_auction_event_payload_leak', (select violations = 0 from cross_auction_mismatch), 'event payload bid_id/auction_id must belong to the same auction'),
    ('P0', 'outbox_drained', (select pending = 0 from outbox_not_published), 'outbox must drain: unpublished deliveries mean WebSocket push was incomplete and viewer state is stale')
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

\echo '## public auction_events sequence gap count'
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

\echo '## accepted/sold settlement public event coverage'
select s.engine_epoch, s.engine_seq, s.result,
       b.id as bid_id, b.seq as bid_seq,
       e.id as event_id, e.seq as event_seq, e.event_type
from redis_engine_settlements s
left join bids b
  on b.auction_id = s.auction_id
 and b.engine_epoch = s.engine_epoch
 and b.engine_seq = s.engine_seq
 and b.status = 'ACCEPTED'
left join auction_events e
  on e.auction_id = s.auction_id
 and e.engine_epoch = s.engine_epoch
 and e.engine_seq = s.engine_seq
 and e.event_type in ('bid_accepted','auction_sold')
where s.auction_id = :'auction_id'
  and s.status = 'SETTLED'
  and s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
  and (b.id is null or e.id is null)
order by s.engine_epoch, s.engine_seq
limit 50;

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

\echo '## auction winner/highest accepted consistency'
with bid_counts as (
  select count(*) filter (where status = 'ACCEPTED') as accepted
  from bids
  where auction_id = :'auction_id'
),
max_accepted as (
  select user_id, amount_cents, engine_seq
  from bids
  where auction_id = :'auction_id'
    and status = 'ACCEPTED'
  order by amount_cents desc, engine_seq desc
  limit 1
)
select a.current_winner_id, a.current_price_cents, a.accepted_bid_count,
       m.user_id as highest_accepted_user,
       m.amount_cents as highest_accepted_amount,
       m.engine_seq as highest_accepted_engine_seq,
       ((select accepted from bid_counts) = 0
         or (a.current_winner_id = m.user_id and a.current_price_cents = m.amount_cents)) as winner_matches_highest_accepted
from auctions a
left join max_accepted m on true
where a.id = :'auction_id';

\echo '## engine sequence completeness'
with auction_row as (
  select engine_epoch, engine_seq
  from auctions
  where id = :'auction_id'
),
settlement_bounds as (
  select min(engine_seq) as min_seq, max(engine_seq) as max_seq, count(*) as actual_count
  from redis_engine_settlements
  where auction_id = :'auction_id'
    and engine_epoch = (select engine_epoch from auction_row)
),
expected as (
  select generate_series(1, coalesce((select max_seq from settlement_bounds), 0)) as engine_seq
)
select (select min_seq from settlement_bounds) as min_seq,
       (select max_seq from settlement_bounds) as max_seq,
       (select actual_count from settlement_bounds) as actual_count,
       (select engine_seq from auction_row) as auction_engine_seq,
       count(expected.engine_seq) filter (where s.engine_seq is null) as missing_engine_seq
from expected
left join redis_engine_settlements s
  on s.auction_id = :'auction_id'
 and s.engine_epoch = (select engine_epoch from auction_row)
 and s.engine_seq = expected.engine_seq;

\echo '## BID_TOO_LOW justification violations'
select b.id, b.client_bid_id, b.user_id, b.amount_cents, b.engine_seq,
       b.reject_reason, prior.prior_price, a.start_price_cents, a.increment_cents
from bids b
join auctions a on a.id = b.auction_id
left join lateral (
  select max(prev.amount_cents) as prior_price
  from bids prev
  where prev.auction_id = b.auction_id
    and prev.engine_epoch = b.engine_epoch
    and prev.status = 'ACCEPTED'
    and prev.engine_seq < b.engine_seq
) prior on true
where b.auction_id = :'auction_id'
  and b.status = 'REJECTED'
  and b.reject_reason = 'BID_TOO_LOW'
  and b.amount_cents >= coalesce(prior.prior_price, a.start_price_cents) + a.increment_cents
order by b.engine_seq
limit 50;

\echo '## idempotency response consistency violations'
select b.engine_seq, b.status, b.client_bid_id, b.user_id,
       i.status as idem_status, i.http_status, i.result_code,
       i.response_json->>'result' as response_result,
       i.response_json->>'amount_cents' as response_amount,
       i.response_json->>'engine_seq' as response_engine_seq,
       i.response_json->>'reject_reason' as response_reject_reason
from bids b
left join idempotency_records i
  on i.scope_type = 'bid'
 and i.scope_id = b.auction_id
 and i.user_id = b.user_id
 and i.idempotency_key = b.client_bid_id
where b.auction_id = :'auction_id'
  and (
    i.idempotency_key is null
    or i.status <> 'COMPLETED'
    or i.http_status <> 200
    or i.result_code is distinct from b.status
    or i.response_json->>'result' is distinct from b.status
    or i.response_json->>'bid_id' is distinct from b.id
    or i.response_json->>'auction_id' is distinct from b.auction_id
    or (i.response_json->>'amount_cents')::bigint is distinct from b.amount_cents
    or (i.response_json->>'engine_seq')::bigint is distinct from b.engine_seq
    or (i.response_json->>'engine_epoch')::bigint is distinct from b.engine_epoch
    or i.response_json->>'reject_reason' is distinct from b.reject_reason
  )
order by b.engine_seq
limit 50;
SQL

  echo
  echo "## machine-readable invariant gates"
  docker exec -i "$DB_CONTAINER" psql -q -A -F $'\t' -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -v expected_unique_bids="$EXPECTED_UNIQUE_BIDS" -f - <<'SQL'
with auction_row as (
  select id, status, current_price_cents, current_winner_id, accepted_bid_count,
         seq, engine_seq, engine_epoch, end_at, cap_price_cents,
         engine_paused, engine_pause_reason
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
bid_identity as (
  select count(*) as total,
         count(distinct user_id) as unique_users,
         count(distinct client_bid_id) as unique_client_bid_ids
  from bids
  where auction_id = :'auction_id'
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
engine_seq_matches_settlement as (
  select coalesce((select engine_seq from auction_row), 0) = coalesce(max(engine_seq) filter (where status = 'SETTLED'), 0) as pass
  from redis_engine_settlements
  where auction_id = :'auction_id'
    and engine_epoch = (select engine_epoch from auction_row)
),
accepted_settlement_coverage as (
  select count(*) as violations
  from redis_engine_settlements s
  left join bids b
    on b.auction_id = s.auction_id
   and b.engine_epoch = s.engine_epoch
   and b.engine_seq = s.engine_seq
   and b.status = 'ACCEPTED'
  left join auction_events e
    on e.auction_id = s.auction_id
   and e.engine_epoch = s.engine_epoch
   and e.engine_seq = s.engine_seq
   and e.event_type in ('bid_accepted','auction_sold')
  where s.auction_id = :'auction_id'
    and s.status = 'SETTLED'
    and s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
    and (b.id is null or e.id is null)
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
    ('P0', 'engine_not_paused', (select not engine_paused from auction_row), 'auction engine must not be paused after a valid PTS run'),
    ('P0', 'no_non_terminal_settlements', (select non_terminal = 0 from settlement), 'no PROCESSING/FAILED/DLQ settlement rows after the chosen settle window'),
    ('P0', 'redis_kafka_pg_accepted_match', (select accepted_or_sold = accepted from settlement, bid_counts), 'accepted/sold Kafka ledger rows must match PG accepted bids'),
    ('P0', 'auction_accepted_count_matches_pg', (select accepted_bid_count = accepted from auction_row, bid_counts), 'auction accepted counter must match PG accepted bids'),
    ('P0', 'redis_engine_seq_matches_settlement', (select pass from engine_seq_matches_settlement), 'auction engine_seq must equal the latest settled Redis/Kafka engine ledger seq'),
    ('P0', 'kafka_position_present', (select missing_kafka_position = 0 from settlement), 'every Kafka ledger settlement must record topic/partition/offset'),
    ('P0', 'accepted_settlement_has_public_event', (select violations = 0 from accepted_settlement_coverage), 'accepted/sold settlements must have matching bid and public auction_event rows'),
    ('P0', 'no_public_auction_event_seq_gap', (select missing_count = 0 from event_gap), 'public auction event seq must be continuous'),
    ('P0', 'no_duplicate_client_bid_id', (select violations = 0 from duplicate_client_bid), 'client_bid_id must not create duplicate bid rows'),
    ('P0', 'pts_expected_unique_users',
      (case when nullif(trim(:'expected_unique_bids'), '') is null then true else (select unique_users = nullif(trim(:'expected_unique_bids'), '')::bigint from bid_identity) end),
      'PTS post-run gate: distinct user_id count must match expected_unique_bids; detects disabled Alibaba PTS CSV split'),
    ('P0', 'pts_expected_unique_client_bid_ids',
      (case when nullif(trim(:'expected_unique_bids'), '') is null then true else (select unique_client_bid_ids = nullif(trim(:'expected_unique_bids'), '')::bigint from bid_identity) end),
      'PTS post-run gate: distinct client_bid_id count must match expected_unique_bids; detects duplicated CSV rows/idempotency replay workload'),
    ('P0', 'pts_expected_total_bid_rows',
      (case when nullif(trim(:'expected_unique_bids'), '') is null then true else (select total = nullif(trim(:'expected_unique_bids'), '')::bigint from bid_identity) end),
      'PTS post-run gate: total persisted bid decisions must match expected_unique_bids'),
    ('P0', 'no_duplicate_engine_seq', (select violations = 0 from duplicate_engine_seq), 'engine_epoch/engine_seq must identify at most one accepted bid'),
    ('P0', 'engine_epoch_seq_monotonic', (select violations = 0 from epoch_seq_violations), 'engine_epoch/engine_seq must be monotonic'),
    ('P0', 'kafka_offset_matches_engine_order', (select violations = 0 from kafka_order_violations), 'Kafka offset order must preserve engine_seq order for the auction partition'),
    ('P0', 'no_created_at_seq_inversion', (select violations = 0 from created_at_inversions), 'created_at must not go backward relative to accepted seq'),
    ('P0', 'no_accepted_after_final_end', (select violations = 0 from accepted_after_end), 'no accepted bid may appear after final end_at'),
    ('P0', 'increment_grid_valid', (select violations = 0 from increment_violations), 'accepted bid deltas must follow auction increment_cents'),
    ('P0', 'at_most_one_order', (select orders <= 1 from orders_count), 'one auction can create at most one order'),
    ('P0', 'no_cross_auction_event_payload_leak', (select violations = 0 from cross_auction_mismatch), 'event payload bid_id/auction_id must belong to the same auction'),
    ('P0', 'outbox_drained', (select pending = 0 from outbox_not_published), 'outbox must drain: unpublished deliveries mean WebSocket push was incomplete and viewer state is stale')
) as gates(severity, name, pass, detail)
order by severity, name;
SQL

  docker exec -i "$DB_CONTAINER" psql -q -A -F $'\t' -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<'SQL'
with auction_row as (
  select id, current_price_cents, current_winner_id, accepted_bid_count,
         engine_seq, engine_epoch
  from auctions
  where id = :'auction_id'
),
bid_counts as (
  select count(*) as total,
         count(*) filter (where status = 'ACCEPTED') as accepted
  from bids
  where auction_id = :'auction_id'
),
max_accepted as (
  select user_id, amount_cents, engine_seq
  from bids
  where auction_id = :'auction_id'
    and status = 'ACCEPTED'
  order by amount_cents desc, engine_seq desc
  limit 1
),
engine_seq_completeness as (
  with bounds as (
    select min(engine_seq) as min_seq, max(engine_seq) as max_seq, count(*) as actual_count
    from redis_engine_settlements
    where auction_id = :'auction_id'
      and engine_epoch = (select engine_epoch from auction_row)
  ),
  expected as (
    select generate_series(1, coalesce((select max_seq from bounds), 0)) as engine_seq
  )
  select (select min_seq from bounds) as min_seq,
         (select max_seq from bounds) as max_seq,
         (select actual_count from bounds) as actual_count,
         coalesce(count(expected.engine_seq) filter (where s.engine_seq is null), 0) as missing_count
  from expected
  left join redis_engine_settlements s
    on s.auction_id = :'auction_id'
   and s.engine_epoch = (select engine_epoch from auction_row)
   and s.engine_seq = expected.engine_seq
),
low_price_reject_violations as (
  select count(*) as violations
  from bids b
  join auctions a on a.id = b.auction_id
  left join lateral (
    select max(prev.amount_cents) as prior_price
    from bids prev
    where prev.auction_id = b.auction_id
      and prev.engine_epoch = b.engine_epoch
      and prev.status = 'ACCEPTED'
      and prev.engine_seq < b.engine_seq
  ) prior on true
  where b.auction_id = :'auction_id'
    and b.status = 'REJECTED'
    and b.reject_reason = 'BID_TOO_LOW'
    and b.amount_cents >= coalesce(prior.prior_price, a.start_price_cents) + a.increment_cents
),
accepted_event_mismatch as (
  select count(*) as violations
  from bids b
  left join auction_events e
    on e.auction_id = b.auction_id
   and e.engine_epoch = b.engine_epoch
   and e.engine_seq = b.engine_seq
   and e.event_type in ('bid_accepted','auction_sold')
  where b.auction_id = :'auction_id'
    and b.status = 'ACCEPTED'
    and (
      e.id is null
      or e.payload_json->>'bid_id' is distinct from b.id
      or e.payload_json->>'user_id' is distinct from b.user_id
      or (e.payload_json->>'amount_cents')::bigint is distinct from b.amount_cents
    )
),
public_event_outbox_mismatch as (
  select count(*) as violations
  from auction_events e
  left join outbox_events o
    on o.auction_id = e.auction_id
   and o.seq = e.seq
   and o.event_type = e.event_type
  left join outbox_delivery d
    on d.outbox_id = o.id
   and d.status = 'PUBLISHED'
  where e.auction_id = :'auction_id'
    and (o.id is null or d.outbox_id is null)
),
settlement_mismatch as (
  select count(*) as violations
  from bids b
  left join redis_engine_settlements s
    on s.auction_id = b.auction_id
   and s.engine_epoch = b.engine_epoch
   and s.engine_seq = b.engine_seq
  where b.auction_id = :'auction_id'
    and (
      s.id is null
      or s.status <> 'SETTLED'
      or (b.status = 'ACCEPTED' and s.result not in ('ENGINE_ACCEPTED','ENGINE_SOLD'))
      or (b.status = 'REJECTED' and s.result <> 'ENGINE_REJECTED')
    )
),
idempotency_response_mismatch as (
  select count(*) as violations
  from bids b
  left join idempotency_records i
    on i.scope_type = 'bid'
   and i.scope_id = b.auction_id
   and i.user_id = b.user_id
   and i.idempotency_key = b.client_bid_id
  where b.auction_id = :'auction_id'
    and (
      i.idempotency_key is null
      or i.status <> 'COMPLETED'
      or i.http_status <> 200
      or i.result_code is distinct from b.status
      or i.response_json->>'result' is distinct from b.status
      or i.response_json->>'bid_id' is distinct from b.id
      or i.response_json->>'auction_id' is distinct from b.auction_id
      or (i.response_json->>'amount_cents')::bigint is distinct from b.amount_cents
      or (i.response_json->>'engine_seq')::bigint is distinct from b.engine_seq
      or (i.response_json->>'engine_epoch')::bigint is distinct from b.engine_epoch
      or i.response_json->>'reject_reason' is distinct from b.reject_reason
    )
),
cap_terminal_violations as (
  select count(*) as violations
  from auctions a
  where a.id = :'auction_id'
    and a.cap_price_cents is not null
    and exists (
      select 1
      from redis_engine_settlements s
      where s.auction_id = a.id
        and s.result = 'ENGINE_SOLD'
        and s.status = 'SETTLED'
    )
    and (
      (select count(*)
       from redis_engine_settlements s
       where s.auction_id = a.id
         and s.result = 'ENGINE_SOLD'
         and s.status = 'SETTLED') <> 1
      or exists (
        select 1
        from bids b
        where b.auction_id = a.id
          and b.status = 'REJECTED'
          and b.amount_cents = a.cap_price_cents
          and b.reject_reason = 'BID_TOO_LOW'
      )
      or (select count(*) from orders o where o.auction_id = a.id) <> 1
    )
),
soft_close_extension_violations as (
  select count(*) as violations
  from redis_engine_settlements s
  join bids b
    on b.auction_id = s.auction_id
   and b.engine_epoch = s.engine_epoch
   and b.engine_seq = s.engine_seq
  join auctions a on a.id = b.auction_id
  join auction_rules ar on ar.auction_id = a.id and ar.rule_version = a.rule_version
  where b.auction_id = :'auction_id'
    and s.status = 'SETTLED'
    and s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
    and (s.payload_json->>'end_at_ms') ~ '^[0-9]+$'
    and b.status = 'ACCEPTED'
    and b.engine_seq is not null
    and b.engine_seq > 1
    and exists (
      select 1
      from redis_engine_settlements prev_s
      join bids prev
        on prev.auction_id = prev_s.auction_id
       and prev.engine_epoch = prev_s.engine_epoch
       and prev.engine_seq = prev_s.engine_seq
      where prev_s.auction_id = s.auction_id
        and prev_s.status = 'SETTLED'
        and prev_s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
        and prev.status = 'ACCEPTED'
        and prev.engine_seq < b.engine_seq
        and prev_s.payload_json->>'end_at_ms' = s.payload_json->>'end_at_ms'
    )
    and exists (
      select 1
      from redis_engine_settlements later_s
      join bids later
        on later.auction_id = later_s.auction_id
       and later.engine_epoch = later_s.engine_epoch
       and later.engine_seq = later_s.engine_seq
      where later_s.auction_id = s.auction_id
        and later_s.status = 'SETTLED'
        and later_s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
        and (later_s.payload_json->>'end_at_ms') ~ '^[0-9]+$'
        and later.status = 'ACCEPTED'
        and later.engine_seq > b.engine_seq
        and to_timestamp((later_s.payload_json->>'end_at_ms')::double precision / 1000.0) > to_timestamp((s.payload_json->>'end_at_ms')::double precision / 1000.0)
        and to_timestamp((later_s.payload_json->>'end_at_ms')::double precision / 1000.0) < to_timestamp((s.payload_json->>'end_at_ms')::double precision / 1000.0) + make_interval(secs => ar.extend_by_seconds)
    )
)
select severity, name, case when pass then 'PASS' else 'FAIL' end as status, detail
from (
  values
    ('P0', 'auction_winner_matches_highest_accepted',
      ((select accepted from bid_counts) = 0
        or exists (
          select 1
          from auction_row a, max_accepted m
          where a.current_winner_id = m.user_id
            and a.current_price_cents = m.amount_cents
        )),
      'auction winner/current price must equal the highest accepted bid'),
    ('P0', 'engine_seq_complete',
      ((select total from bid_counts) = 0
        or exists (
          select 1
          from engine_seq_completeness c, auction_row a, bid_counts b
          where c.min_seq = 1
            and c.max_seq = b.total
            and c.actual_count = b.total
            and c.missing_count = 0
            and a.engine_seq = b.total
        )),
      'settled engine_seq must be complete from 1 through total bid decisions'),
    ('P0', 'bid_too_low_rejects_justified',
      (select violations = 0 from low_price_reject_violations),
      'each BID_TOO_LOW reject must be below the engine price floor at its engine_seq'),
    ('P0', 'accepted_public_event_exact_mapping',
      (select violations = 0 from accepted_event_mismatch),
      'each accepted bid must have an exact public event payload match'),
    ('P0', 'public_events_have_published_outbox',
      (select violations = 0 from public_event_outbox_mismatch),
      'each public auction_event must have a published outbox delivery'),
    ('P0', 'every_bid_has_settled_ledger',
      (select violations = 0 from settlement_mismatch),
      'each bid decision must have a matching terminal Redis/Kafka settlement row'),
    ('P0', 'idempotency_response_matches_bid',
      (select violations = 0 from idempotency_response_mismatch),
      'completed bid idempotency response_json must match the persisted bid decision'),
    ('P0', 'cap_terminal_single_sold_order',
      (select violations = 0 from cap_terminal_violations),
      'cap price is terminal: exactly one ENGINE_SOLD/order, no equal-cap loser misclassified as BID_TOO_LOW'),
    ('P0', 'soft_close_no_stacked_subwindow_extension',
      (select violations = 0 from soft_close_extension_violations),
      'soft close must not stack multiple extensions inside one old end_at window')
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

{
  echo "## reject reason distribution"
  docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<'SQL'
\pset pager off
select status, reject_reason, count(*) as count
from bids
where auction_id = :'auction_id'
group by status, reject_reason
order by status, count desc;
SQL
} >> "$OUT_DIR/l4b-correctness.txt"

docker exec -i "$DB_CONTAINER" psql -q -A -t -F $'\t' -U "$DB_USER" -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -v expected_unique_bids="$EXPECTED_UNIQUE_BIDS" -f - > "$OUT_DIR/l4b-invariant-gates.tsv" <<'SQL'
with auction_row as (
  select id, status, current_price_cents, current_winner_id, accepted_bid_count,
         seq, engine_seq, engine_epoch, end_at,
         engine_paused, engine_pause_reason
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
bid_identity as (
  select count(*) as total,
         count(distinct user_id) as unique_users,
         count(distinct client_bid_id) as unique_client_bid_ids
  from bids
  where auction_id = :'auction_id'
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
engine_seq_matches_settlement as (
  select coalesce((select engine_seq from auction_row), 0) = coalesce(max(engine_seq) filter (where status = 'SETTLED'), 0) as pass
  from redis_engine_settlements
  where auction_id = :'auction_id'
    and engine_epoch = (select engine_epoch from auction_row)
),
accepted_settlement_coverage as (
  select count(*) as violations
  from redis_engine_settlements s
  left join bids b
    on b.auction_id = s.auction_id
   and b.engine_epoch = s.engine_epoch
   and b.engine_seq = s.engine_seq
   and b.status = 'ACCEPTED'
  left join auction_events e
    on e.auction_id = s.auction_id
   and e.engine_epoch = s.engine_epoch
   and e.engine_seq = s.engine_seq
   and e.event_type in ('bid_accepted','auction_sold')
  where s.auction_id = :'auction_id'
    and s.status = 'SETTLED'
    and s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
    and (b.id is null or e.id is null)
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
    ('P0', 'engine_not_paused', (select not engine_paused from auction_row), 'auction engine must not be paused after a valid PTS run'),
    ('P0', 'no_non_terminal_settlements', (select non_terminal = 0 from settlement), 'no PROCESSING/FAILED/DLQ settlement rows after the chosen settle window'),
    ('P0', 'redis_kafka_pg_accepted_match', (select accepted_or_sold = accepted from settlement, bid_counts), 'accepted/sold Kafka ledger rows must match PG accepted bids'),
    ('P0', 'auction_accepted_count_matches_pg', (select accepted_bid_count = accepted from auction_row, bid_counts), 'auction accepted counter must match PG accepted bids'),
    ('P0', 'redis_engine_seq_matches_settlement', (select pass from engine_seq_matches_settlement), 'auction engine_seq must equal the latest settled Redis/Kafka engine ledger seq'),
    ('P0', 'kafka_position_present', (select missing_kafka_position = 0 from settlement), 'every Kafka ledger settlement must record topic/partition/offset'),
    ('P0', 'accepted_settlement_has_public_event', (select violations = 0 from accepted_settlement_coverage), 'accepted/sold settlements must have matching bid and public auction_event rows'),
    ('P0', 'no_public_auction_event_seq_gap', (select missing_count = 0 from event_gap), 'public auction event seq must be continuous'),
    ('P0', 'no_duplicate_client_bid_id', (select violations = 0 from duplicate_client_bid), 'client_bid_id must not create duplicate bid rows'),
    ('P0', 'pts_expected_unique_users',
      (case when nullif(trim(:'expected_unique_bids'), '') is null then true else (select unique_users = nullif(trim(:'expected_unique_bids'), '')::bigint from bid_identity) end),
      'PTS post-run gate: distinct user_id count must match expected_unique_bids; detects disabled Alibaba PTS CSV split'),
    ('P0', 'pts_expected_unique_client_bid_ids',
      (case when nullif(trim(:'expected_unique_bids'), '') is null then true else (select unique_client_bid_ids = nullif(trim(:'expected_unique_bids'), '')::bigint from bid_identity) end),
      'PTS post-run gate: distinct client_bid_id count must match expected_unique_bids; detects duplicated CSV rows/idempotency replay workload'),
    ('P0', 'pts_expected_total_bid_rows',
      (case when nullif(trim(:'expected_unique_bids'), '') is null then true else (select total = nullif(trim(:'expected_unique_bids'), '')::bigint from bid_identity) end),
      'PTS post-run gate: total persisted bid decisions must match expected_unique_bids'),
    ('P0', 'no_duplicate_engine_seq', (select violations = 0 from duplicate_engine_seq), 'engine_epoch/engine_seq must identify at most one accepted bid'),
    ('P0', 'engine_epoch_seq_monotonic', (select violations = 0 from epoch_seq_violations), 'engine_epoch/engine_seq must be monotonic'),
    ('P0', 'kafka_offset_matches_engine_order', (select violations = 0 from kafka_order_violations), 'Kafka offset order must preserve engine_seq order for the auction partition'),
    ('P0', 'no_created_at_seq_inversion', (select violations = 0 from created_at_inversions), 'created_at must not go backward relative to accepted seq'),
    ('P0', 'no_accepted_after_final_end', (select violations = 0 from accepted_after_end), 'no accepted bid may appear after final end_at'),
    ('P0', 'increment_grid_valid', (select violations = 0 from increment_violations), 'accepted bid deltas must follow auction increment_cents'),
    ('P0', 'at_most_one_order', (select orders <= 1 from orders_count), 'one auction can create at most one order'),
    ('P0', 'no_cross_auction_event_payload_leak', (select violations = 0 from cross_auction_mismatch), 'event payload bid_id/auction_id must belong to the same auction'),
    ('P0', 'outbox_drained', (select pending = 0 from outbox_not_published), 'outbox must drain: unpublished deliveries mean WebSocket push was incomplete and viewer state is stale')
) as gates(severity, name, pass, detail)
order by severity, name;
SQL

docker exec -i "$DB_CONTAINER" psql -q -A -t -F $'\t' -U "$DB_USER" -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - >> "$OUT_DIR/l4b-invariant-gates.tsv" <<'SQL'
with auction_row as (
  select id, current_price_cents, current_winner_id, accepted_bid_count,
         engine_seq, engine_epoch
  from auctions
  where id = :'auction_id'
),
bid_counts as (
  select count(*) as total,
         count(*) filter (where status = 'ACCEPTED') as accepted
  from bids
  where auction_id = :'auction_id'
),
max_accepted as (
  select user_id, amount_cents, engine_seq
  from bids
  where auction_id = :'auction_id'
    and status = 'ACCEPTED'
  order by amount_cents desc, engine_seq desc
  limit 1
),
engine_seq_completeness as (
  with bounds as (
    select min(engine_seq) as min_seq, max(engine_seq) as max_seq, count(*) as actual_count
    from redis_engine_settlements
    where auction_id = :'auction_id'
      and engine_epoch = (select engine_epoch from auction_row)
  ),
  expected as (
    select generate_series(1, coalesce((select max_seq from bounds), 0)) as engine_seq
  )
  select (select min_seq from bounds) as min_seq,
         (select max_seq from bounds) as max_seq,
         (select actual_count from bounds) as actual_count,
         coalesce(count(expected.engine_seq) filter (where s.engine_seq is null), 0) as missing_count
  from expected
  left join redis_engine_settlements s
    on s.auction_id = :'auction_id'
   and s.engine_epoch = (select engine_epoch from auction_row)
   and s.engine_seq = expected.engine_seq
),
low_price_reject_violations as (
  select count(*) as violations
  from bids b
  join auctions a on a.id = b.auction_id
  left join lateral (
    select max(prev.amount_cents) as prior_price
    from bids prev
    where prev.auction_id = b.auction_id
      and prev.engine_epoch = b.engine_epoch
      and prev.status = 'ACCEPTED'
      and prev.engine_seq < b.engine_seq
  ) prior on true
  where b.auction_id = :'auction_id'
    and b.status = 'REJECTED'
    and b.reject_reason = 'BID_TOO_LOW'
    and b.amount_cents >= coalesce(prior.prior_price, a.start_price_cents) + a.increment_cents
),
accepted_event_mismatch as (
  select count(*) as violations
  from bids b
  left join auction_events e
    on e.auction_id = b.auction_id
   and e.engine_epoch = b.engine_epoch
   and e.engine_seq = b.engine_seq
   and e.event_type in ('bid_accepted','auction_sold')
  where b.auction_id = :'auction_id'
    and b.status = 'ACCEPTED'
    and (
      e.id is null
      or e.payload_json->>'bid_id' is distinct from b.id
      or e.payload_json->>'user_id' is distinct from b.user_id
      or (e.payload_json->>'amount_cents')::bigint is distinct from b.amount_cents
    )
),
public_event_outbox_mismatch as (
  select count(*) as violations
  from auction_events e
  left join outbox_events o
    on o.auction_id = e.auction_id
   and o.seq = e.seq
   and o.event_type = e.event_type
  left join outbox_delivery d
    on d.outbox_id = o.id
   and d.status = 'PUBLISHED'
  where e.auction_id = :'auction_id'
    and (o.id is null or d.outbox_id is null)
),
settlement_mismatch as (
  select count(*) as violations
  from bids b
  left join redis_engine_settlements s
    on s.auction_id = b.auction_id
   and s.engine_epoch = b.engine_epoch
   and s.engine_seq = b.engine_seq
  where b.auction_id = :'auction_id'
    and (
      s.id is null
      or s.status <> 'SETTLED'
      or (b.status = 'ACCEPTED' and s.result not in ('ENGINE_ACCEPTED','ENGINE_SOLD'))
      or (b.status = 'REJECTED' and s.result <> 'ENGINE_REJECTED')
    )
),
idempotency_response_mismatch as (
  select count(*) as violations
  from bids b
  left join idempotency_records i
    on i.scope_type = 'bid'
   and i.scope_id = b.auction_id
   and i.user_id = b.user_id
   and i.idempotency_key = b.client_bid_id
  where b.auction_id = :'auction_id'
    and (
      i.idempotency_key is null
      or i.status <> 'COMPLETED'
      or i.http_status <> 200
      or i.result_code is distinct from b.status
      or i.response_json->>'result' is distinct from b.status
      or i.response_json->>'bid_id' is distinct from b.id
      or i.response_json->>'auction_id' is distinct from b.auction_id
      or (i.response_json->>'amount_cents')::bigint is distinct from b.amount_cents
      or (i.response_json->>'engine_seq')::bigint is distinct from b.engine_seq
      or (i.response_json->>'engine_epoch')::bigint is distinct from b.engine_epoch
      or i.response_json->>'reject_reason' is distinct from b.reject_reason
    )
),
cap_terminal_violations as (
  select count(*) as violations
  from auctions a
  where a.id = :'auction_id'
    and a.cap_price_cents is not null
    and exists (
      select 1
      from redis_engine_settlements s
      where s.auction_id = a.id
        and s.result = 'ENGINE_SOLD'
        and s.status = 'SETTLED'
    )
    and (
      (select count(*)
       from redis_engine_settlements s
       where s.auction_id = a.id
         and s.result = 'ENGINE_SOLD'
         and s.status = 'SETTLED') <> 1
      or exists (
        select 1
        from bids b
        where b.auction_id = a.id
          and b.status = 'REJECTED'
          and b.amount_cents = a.cap_price_cents
          and b.reject_reason = 'BID_TOO_LOW'
      )
      or (select count(*) from orders o where o.auction_id = a.id) <> 1
    )
),
soft_close_extension_violations as (
  select count(*) as violations
  from redis_engine_settlements s
  join bids b
    on b.auction_id = s.auction_id
   and b.engine_epoch = s.engine_epoch
   and b.engine_seq = s.engine_seq
  join auctions a on a.id = b.auction_id
  join auction_rules ar on ar.auction_id = a.id and ar.rule_version = a.rule_version
  where b.auction_id = :'auction_id'
    and s.status = 'SETTLED'
    and s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
    and (s.payload_json->>'end_at_ms') ~ '^[0-9]+$'
    and b.status = 'ACCEPTED'
    and b.engine_seq is not null
    and b.engine_seq > 1
    and exists (
      select 1
      from redis_engine_settlements prev_s
      join bids prev
        on prev.auction_id = prev_s.auction_id
       and prev.engine_epoch = prev_s.engine_epoch
       and prev.engine_seq = prev_s.engine_seq
      where prev_s.auction_id = s.auction_id
        and prev_s.status = 'SETTLED'
        and prev_s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
        and prev.status = 'ACCEPTED'
        and prev.engine_seq < b.engine_seq
        and prev_s.payload_json->>'end_at_ms' = s.payload_json->>'end_at_ms'
    )
    and exists (
      select 1
      from redis_engine_settlements later_s
      join bids later
        on later.auction_id = later_s.auction_id
       and later.engine_epoch = later_s.engine_epoch
       and later.engine_seq = later_s.engine_seq
      where later_s.auction_id = s.auction_id
        and later_s.status = 'SETTLED'
        and later_s.result in ('ENGINE_ACCEPTED','ENGINE_SOLD')
        and (later_s.payload_json->>'end_at_ms') ~ '^[0-9]+$'
        and later.status = 'ACCEPTED'
        and later.engine_seq > b.engine_seq
        and to_timestamp((later_s.payload_json->>'end_at_ms')::double precision / 1000.0) > to_timestamp((s.payload_json->>'end_at_ms')::double precision / 1000.0)
        and to_timestamp((later_s.payload_json->>'end_at_ms')::double precision / 1000.0) < to_timestamp((s.payload_json->>'end_at_ms')::double precision / 1000.0) + make_interval(secs => ar.extend_by_seconds)
    )
)
select severity, name, case when pass then 'PASS' else 'FAIL' end as status, detail
from (
  values
    ('P0', 'auction_winner_matches_highest_accepted',
      ((select accepted from bid_counts) = 0
        or exists (
          select 1
          from auction_row a, max_accepted m
          where a.current_winner_id = m.user_id
            and a.current_price_cents = m.amount_cents
        )),
      'auction winner/current price must equal the highest accepted bid'),
    ('P0', 'engine_seq_complete',
      ((select total from bid_counts) = 0
        or exists (
          select 1
          from engine_seq_completeness c, auction_row a, bid_counts b
          where c.min_seq = 1
            and c.max_seq = b.total
            and c.actual_count = b.total
            and c.missing_count = 0
            and a.engine_seq = b.total
        )),
      'settled engine_seq must be complete from 1 through total bid decisions'),
    ('P0', 'bid_too_low_rejects_justified',
      (select violations = 0 from low_price_reject_violations),
      'each BID_TOO_LOW reject must be below the engine price floor at its engine_seq'),
    ('P0', 'accepted_public_event_exact_mapping',
      (select violations = 0 from accepted_event_mismatch),
      'each accepted bid must have an exact public event payload match'),
    ('P0', 'public_events_have_published_outbox',
      (select violations = 0 from public_event_outbox_mismatch),
      'each public auction_event must have a published outbox delivery'),
    ('P0', 'every_bid_has_settled_ledger',
      (select violations = 0 from settlement_mismatch),
      'each bid decision must have a matching terminal Redis/Kafka settlement row'),
    ('P0', 'idempotency_response_matches_bid',
      (select violations = 0 from idempotency_response_mismatch),
      'completed bid idempotency response_json must match the persisted bid decision'),
    ('P0', 'cap_terminal_single_sold_order',
      (select violations = 0 from cap_terminal_violations),
      'cap price is terminal: exactly one ENGINE_SOLD/order, no equal-cap loser misclassified as BID_TOO_LOW'),
    ('P0', 'soft_close_no_stacked_subwindow_extension',
      (select violations = 0 from soft_close_extension_violations),
      'soft close must not stack multiple extensions inside one old end_at window')
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
  printf 'P0\tkafka_consumer_group_lag_zero\t%s\tSettlement consumer group lag must drain to zero: outstanding lag means some bid decisions were not settled in PostgreSQL\n' "$([ "${group_lag:-0}" = "0" ] && echo PASS || echo FAIL)"
} > "$kafka_gate_file"

cat "$kafka_gate_file" >> "$OUT_DIR/l4b-invariant-gates.tsv"

redis_pending_gate_file="$OUT_DIR/l4b-redis-pending-gates.tsv"
{
  # v3: primary check is the relay log stream cursor vs stream length.
  # The old pending hash is kept for backward compat but stream is authoritative.
  stream_len="$(docker exec "$REDIS_CONTAINER" redis-cli XLEN "bid:{$AUCTION_ID}:engine:log" 2>/dev/null | tr -d '\r' || echo 0)"
  relay_cursor="$(docker exec "$REDIS_CONTAINER" redis-cli GET "bid:{$AUCTION_ID}:engine:relay-cursor" 2>/dev/null | tr -d '\r' || echo '0-0')"
  pending_count="$(docker exec "$REDIS_CONTAINER" redis-cli HLEN "bid:{$AUCTION_ID}:engine:pending" | tr -d '\r')"
  settlement_total="$(docker exec -i "$DB_CONTAINER" psql -q -A -t -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<'SQL' | tr -d '[:space:]'
select count(*) from redis_engine_settlements where auction_id = :'auction_id';
SQL
)"

  if [ -n "${EXPECTED_UNIQUE_BIDS}" ]; then
    printf 'P0\tv3_relay_stream_complete\t%s\tDecision log stream must have exactly %s entries (one per PTS bid decision); got %s; relay cursor=%s\n' \
      "$([ "${stream_len:-0}" = "${EXPECTED_UNIQUE_BIDS}" ] && echo PASS || echo FAIL)" \
      "${EXPECTED_UNIQUE_BIDS}" "${stream_len}" "${relay_cursor}"
  else
    if [ "$REDIS_DATA_LOSS_OK" = "1" ]; then
      printf 'P0\tv3_relay_stream_complete\tPASS\tRedis data-loss profile: decision log stream may be empty after FLUSHALL; PG/Kafka settlement rows remain authoritative; settlements=%s stream_len=%s relay_cursor=%s\n' \
        "${settlement_total:-0}" "${stream_len:-0}" "${relay_cursor}"
    else
      printf 'P0\tv3_relay_stream_complete\t%s\tDecision log stream length must match PG settlement rows when exact PTS bid count is disabled; settlements=%s stream_len=%s relay_cursor=%s\n' \
        "$([ "${stream_len:-0}" = "${settlement_total:-0}" ] && echo PASS || echo FAIL)" \
        "${settlement_total:-0}" "${stream_len:-0}" "${relay_cursor}"
    fi
  fi
  printf 'P0\tredis_pending_decisions_empty\t%s\tRedis pending hash must be zero after full relay drain\n' \
    "$([ "${pending_count:-0}" = "0" ] && echo PASS || echo FAIL)"

  # v3 gate: relay cursor must not be "0-0" if stream has entries
  if [ "${stream_len:-0}" -gt 0 ] && [ "${relay_cursor:-0-0}" = "0-0" ]; then
    printf 'P0\tv3_relay_cursor_advanced\tFAIL\tStream has %s entries but relay cursor is still 0-0 — relay has not run\n' "$stream_len"
  else
    printf 'P0\tv3_relay_cursor_advanced\tPASS\tRelay cursor=%s stream_len=%s\n' "$relay_cursor" "$stream_len"
  fi
} > "$redis_pending_gate_file"

cat "$redis_pending_gate_file" >> "$OUT_DIR/l4b-invariant-gates.tsv"

reject_reason_gate_file="$OUT_DIR/l4b-reject-reason-gates.tsv"
{
  # In L1-C1 (ADMISSION_ENABLED=false), the only legitimate reject reasons are those
  # produced by the engine's own business rules. RATE_LIMITED / BID_AUCTION_TOO_HOT
  # indicate admission logic bled through despite the profile config; AUCTION_PAUSED
  # indicates the engine paused unexpectedly. Any of these corrupt the contention result
  # because they bypass fair price competition.
  docker exec -i "$DB_CONTAINER" psql -q -A -t -F $'\t' -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<'SQL'
select 'P0', 'rejected_bids_have_expected_reason',
       case when count(*) = 0 then 'PASS' else 'FAIL' end,
       'all rejected bids must carry a known engine business reason; unexpected codes indicate admission or rate-limiting contamination despite ADMISSION_ENABLED=false'
from bids
where auction_id = :'auction_id'
  and status = 'REJECTED'
  and coalesce(reject_reason, '') not in (
    'BID_TOO_LOW',
    'AUCTION_SOLD',
    'AUCTION_NOT_ACTIVE',
    'AUCTION_ENDED',
    'REJECTED_SELF_LEADING',
    'DUPLICATE_BID'
  );
SQL
} > "$reject_reason_gate_file"
cat "$reject_reason_gate_file" >> "$OUT_DIR/l4b-invariant-gates.tsv"

soft_close_gate_file="$OUT_DIR/l4b-soft-close-gates.tsv"
{
  # Positive assertion: whenever end_at_ms changes between two consecutive settled
  # decisions, the delta must equal exactly extend_by_seconds * 1000 ms.
  # The existing soft_close_no_stacked_subwindow_extension gate catches the negative
  # (stacking bug). This gate catches the positive failure mode: extension ran but
  # computed the wrong delta, meaning a bidder won more or less time than the rule allows.
  docker exec -i "$DB_CONTAINER" psql -q -A -t -F $'\t' -U "$DB_USER" -d "$DB_NAME" \
    -v ON_ERROR_STOP=1 -v auction_id="$AUCTION_ID" -f - <<'SQL'
with ordered as (
  select
    s.engine_seq,
    (s.payload_json->>'end_at_ms')::bigint                          as curr_end_at_ms,
    lag((s.payload_json->>'end_at_ms')::bigint) over (
      order by s.engine_seq
    )                                                                as prev_end_at_ms,
    ar.extend_by_seconds
  from redis_engine_settlements s
  join bids b
    on b.auction_id  = s.auction_id
   and b.engine_epoch = s.engine_epoch
   and b.engine_seq   = s.engine_seq
  join auctions a on a.id = b.auction_id
  join auction_rules ar
    on ar.auction_id    = a.id
   and ar.rule_version  = a.rule_version
  where s.auction_id = :'auction_id'
    and s.status = 'SETTLED'
    and s.result in ('ENGINE_ACCEPTED', 'ENGINE_SOLD')
    and (s.payload_json->>'end_at_ms') ~ '^[0-9]+$'
    and b.status = 'ACCEPTED'
)
select 'P1', 'soft_close_extension_delta_correct',
       case when count(*) = 0 then 'PASS' else 'FAIL' end,
       'every soft-close extension must advance end_at_ms by exactly extend_by_seconds * 1000 ms; wrong delta means the engine applied an incorrect window size'
from ordered
where prev_end_at_ms is not null
  and curr_end_at_ms <> prev_end_at_ms
  and (curr_end_at_ms - prev_end_at_ms) <> (extend_by_seconds * 1000);
SQL
} > "$soft_close_gate_file"
cat "$soft_close_gate_file" >> "$OUT_DIR/l4b-invariant-gates.tsv"

if awk -F '\t' '$1 == "P0" && $3 == "FAIL" { found=1 } END { exit found ? 0 : 1 }' "$OUT_DIR/l4b-invariant-gates.tsv"; then
  echo "[verify] P0 invariant violation; see $OUT_DIR/l4b-invariant-gates.tsv" >&2
  exit 1
fi

echo "[verify] done: $OUT_DIR/l4b-correctness.txt"
echo "[verify] gates: $OUT_DIR/l4b-invariant-gates.tsv"
