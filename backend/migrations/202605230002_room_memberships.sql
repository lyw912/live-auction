-- +goose Up
CREATE TABLE room_memberships (
  room_id text NOT NULL REFERENCES rooms(id),
  user_id text NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('host','viewer','blocked')),
  status text NOT NULL CHECK (status IN ('ACTIVE','LEFT','BANNED')),
  joined_at timestamptz NOT NULL DEFAULT now(),
  left_at timestamptz,
  PRIMARY KEY (room_id, user_id)
);

CREATE INDEX ix_room_memberships_user_status
  ON room_memberships(user_id, status, room_id);

INSERT INTO room_memberships (room_id, user_id, role, status)
SELECT r.id, r.host_id, 'host', 'ACTIVE'
FROM rooms r
ON CONFLICT (room_id, user_id) DO NOTHING;

INSERT INTO room_memberships (room_id, user_id, role, status)
VALUES ('room_1', 'user_1', 'viewer', 'ACTIVE')
ON CONFLICT (room_id, user_id) DO NOTHING;

INSERT INTO room_memberships (room_id, user_id, role, status)
VALUES ('room_main', 'user_1', 'viewer', 'ACTIVE')
ON CONFLICT (room_id, user_id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS room_memberships;
