package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/storage"
)

func TestChatRoutesPersistAndSeedRoomMessages(t *testing.T) {
	db := openMonitorDB(t)
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	t.Cleanup(func() { _ = rdb.Close() })

	roomID := "room_chat_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN')`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, 'user_1', 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL
	`, roomID); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())

	body := bytes.NewBufferString(`{"client_msg_id":"msg_1","body":"加一口看看"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+roomID+"/chat", body)
	req.Header.Set("X-Mock-Role", "user")
	req.Header.Set("X-Mock-User-Id", "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post chat status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/rooms/"+roomID+"/chat?limit=30", nil)
	req.Header.Set("X-Mock-Role", "user")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get chat status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chat: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0]["body"] != "加一口看看" {
		t.Fatalf("unexpected chat payload: %#v", payload.Items)
	}
}
