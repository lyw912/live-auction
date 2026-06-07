-- Add index to support room-scoped host order queries (ListOrders host path).
-- +goose Up
CREATE INDEX IF NOT EXISTS ix_rooms_host_id ON rooms(host_id);

-- +goose Down
DROP INDEX IF EXISTS ix_rooms_host_id;
