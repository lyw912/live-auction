-- +goose Up
ALTER TABLE auction_system_messages
  DROP CONSTRAINT IF EXISTS auction_system_messages_source_check,
  ADD CONSTRAINT auction_system_messages_source_check CHECK (source IN ('SYSTEM_TEMPLATE','SYSTEM_AI','HOST','HOST_SCRIPT'));

ALTER TABLE auction_system_messages
  DROP CONSTRAINT IF EXISTS auction_system_messages_body_check,
  ADD CONSTRAINT auction_system_messages_body_check CHECK (char_length(body) BETWEEN 1 AND 240);

-- +goose Down
ALTER TABLE auction_system_messages
  DROP CONSTRAINT IF EXISTS auction_system_messages_source_check,
  ADD CONSTRAINT auction_system_messages_source_check CHECK (source IN ('SYSTEM_TEMPLATE','SYSTEM_AI','HOST'));

ALTER TABLE auction_system_messages
  DROP CONSTRAINT IF EXISTS auction_system_messages_body_check,
  ADD CONSTRAINT auction_system_messages_body_check CHECK (char_length(body) BETWEEN 1 AND 80);
