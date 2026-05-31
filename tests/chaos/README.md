# Chaos / Fault-Injection Test Scripts

Each script tests one fault scenario from the v3 correctness matrix.

Business scenario context (what you tell the judges):
  A live jewellery auction is in its final 30 seconds.
  We inject each failure mode and prove the system either recovers safely
  or fails closed — never producing a false winner, duplicate charge,
  or hidden data loss.

Scripts:
  01-redis-unavailable.sh      Redis down before decision → ENGINE_PAUSED fail-closed
  02-redis-state-flush.sh      Redis FLUSHDB mid-auction → rebuild from Kafka+PG
  03-kafka-relay-down.sh       Kafka down during relay → relay fails; hot path unaffected
  04-settlement-crash.sh       Settlement worker killed → replay idempotently from Kafka
  05-pg-unavailable.sh         PostgreSQL down → live engine continues; orders wait
  06-reconnect-storm.sh        WS disconnect/reconnect storm → snapshot+diff recovery
  07-relay-backpressure.sh     Stream > relay batch ceiling → alert; drains; no silent queue
  08-settlement-idempotency.sh Kafka redelivers 3× → exactly one settlement row

Running:
  export PTS_HOST=172.16.179.112:18080
  export AUCTION_ID=auc_live
  export REDIS_ADDR=localhost:6380
  export REDIS_CLI="redis-cli -p 6380"
  bash tests/chaos/01-redis-unavailable.sh

Pass criteria: each script prints PASS or FAIL with reason; exit 0 = pass.
