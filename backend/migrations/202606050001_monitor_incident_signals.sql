-- +goose Up
ALTER TABLE system_control_signals
  DROP CONSTRAINT system_control_signals_signal_type_check,
  ADD CONSTRAINT system_control_signals_signal_type_check CHECK (signal_type IN (
    'force_snapshot_rebuild',
    'retry_dead_outbox',
    'pause_relay_shard',
    'resume_relay_shard',
    'pause_redis_engine',
    'resume_redis_engine',
    'reconcile_redis_engine',
    'merchant_incident_note',
    'ack_alert',
    'mute_alert_10m'
  ));

ALTER TABLE system_control_signals
  DROP CONSTRAINT system_control_signals_target_type_check,
  ADD CONSTRAINT system_control_signals_target_type_check CHECK (target_type IN (
    'auction',
    'outbox',
    'relay_shard',
    'alert',
    'room'
  ));

-- +goose Down
ALTER TABLE system_control_signals
  DROP CONSTRAINT system_control_signals_signal_type_check,
  ADD CONSTRAINT system_control_signals_signal_type_check CHECK (signal_type IN (
    'force_snapshot_rebuild',
    'retry_dead_outbox',
    'pause_relay_shard',
    'resume_relay_shard',
    'pause_redis_engine',
    'resume_redis_engine',
    'reconcile_redis_engine'
  ));

ALTER TABLE system_control_signals
  DROP CONSTRAINT system_control_signals_target_type_check,
  ADD CONSTRAINT system_control_signals_target_type_check CHECK (target_type IN (
    'auction',
    'outbox',
    'relay_shard'
  ));
