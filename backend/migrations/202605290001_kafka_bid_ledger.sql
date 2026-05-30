-- +goose Up
ALTER TABLE redis_engine_settlements
  ADD COLUMN ledger_source text NOT NULL DEFAULT 'kafka'
    CHECK (ledger_source IN ('redis_stream','kafka')),
  ADD COLUMN ledger_topic text,
  ADD COLUMN ledger_partition int,
  ADD COLUMN ledger_offset bigint,
  ADD COLUMN ledger_key text,
  ADD COLUMN dlq_topic text,
  ADD COLUMN dlq_error text,
  ADD COLUMN dlq_at timestamptz;

CREATE UNIQUE INDEX ux_redis_engine_settlements_kafka_offset
  ON redis_engine_settlements(ledger_topic, ledger_partition, ledger_offset)
  WHERE ledger_source = 'kafka'
    AND ledger_topic IS NOT NULL
    AND ledger_partition IS NOT NULL
    AND ledger_offset IS NOT NULL;

CREATE INDEX ix_redis_engine_settlements_ledger_lag
  ON redis_engine_settlements(ledger_source, status, ledger_topic, ledger_partition, ledger_offset);

-- +goose Down
DROP INDEX IF EXISTS ix_redis_engine_settlements_ledger_lag;
DROP INDEX IF EXISTS ux_redis_engine_settlements_kafka_offset;

ALTER TABLE redis_engine_settlements
  DROP COLUMN IF EXISTS dlq_at,
  DROP COLUMN IF EXISTS dlq_error,
  DROP COLUMN IF EXISTS dlq_topic,
  DROP COLUMN IF EXISTS ledger_key,
  DROP COLUMN IF EXISTS ledger_offset,
  DROP COLUMN IF EXISTS ledger_partition,
  DROP COLUMN IF EXISTS ledger_topic,
  DROP COLUMN IF EXISTS ledger_source;
