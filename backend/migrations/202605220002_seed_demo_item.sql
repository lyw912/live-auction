-- +goose Up
INSERT INTO items (id, title, image_url, description, status)
VALUES (
  'item_1',
  'Demo Auction Item',
  NULL,
  'Seed item for local P0 development flow.',
  'READY'
)
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM items WHERE id = 'item_1';
