package gateway

import (
	"net/http"
	"strings"

	apierrors "live-auction/backend/internal/platform/errors"
)

type testRoomSetupRequest struct {
	RoomID string   `json:"room_id"`
	HostID string   `json:"host_id"`
	Users  []string `json:"users"`
}

func (h AuctionHandler) TestSetupRoom(w http.ResponseWriter, r *http.Request) {
	if h.Config.AppEnv != "test" {
		writeError(w, r, apierrors.New(apierrors.CodeForbiddenRoom, "test setup disabled", http.StatusForbidden))
		return
	}
	user, ok := currentUser(r)
	if !ok || user.Role != "host" {
		writeError(w, r, apierrors.New(apierrors.CodeForbiddenRoom, "host role required", http.StatusForbidden))
		return
	}
	var req testRoomSetupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	req.RoomID = strings.TrimSpace(req.RoomID)
	req.HostID = strings.TrimSpace(req.HostID)
	if req.RoomID == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "room_id is required", http.StatusBadRequest))
		return
	}
	if req.HostID == "" {
		req.HostID = user.ID
	}
	if req.HostID != user.ID {
		writeError(w, r, apierrors.New(apierrors.CodeForbiddenRoom, "host can only create own test room", http.StatusForbidden))
		return
	}
	tx, err := h.Deps.Postgres.Begin(r.Context())
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO rooms (id, host_id, status)
		VALUES ($1, $2, 'OPEN')
		ON CONFLICT (id) DO UPDATE SET host_id = EXCLUDED.host_id, status = 'OPEN'
	`, req.RoomID, req.HostID); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, $2, 'host', 'ACTIVE')
		ON CONFLICT (room_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL
	`, req.RoomID, req.HostID); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	for _, rawUserID := range req.Users {
		userID := strings.TrimSpace(rawUserID)
		if userID == "" {
			continue
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO room_memberships (room_id, user_id, role, status)
			VALUES ($1, $2, 'viewer', 'ACTIVE')
			ON CONFLICT (room_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL
		`, req.RoomID, userID); err != nil {
			writeResult(w, r, http.StatusOK, nil, err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"room_id": req.RoomID,
		"host_id": req.HostID,
		"users":   req.Users,
	})
}
