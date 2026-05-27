package gateway

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"live-auction/backend/internal/storage"
)

func TestSetupRoomIsTestOnlyAndHostScoped(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	router := NewRouter(testConfig(), deps, slog.Default())

	roomID := "room_test_setup"
	body := bytes.NewBufferString(`{"room_id":"` + roomID + `","host_id":"host_1","users":["user_1","user_2"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/test/rooms", body)
	req.Header.Set("X-Mock-Role", "host")
	req.Header.Set("X-Mock-User-Id", "host_1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup room status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	if payload["room_id"] != roomID {
		t.Fatalf("room_id = %v, want %s", payload["room_id"], roomID)
	}
	assertAPIStatus(t, router, http.MethodGet, "/api/rooms/"+roomID+"/auctions", nil, userHeaders("user_2", "user"), http.StatusOK)

	prodCfg := testConfig()
	prodCfg.AppEnv = "local"
	prodCfg.AllowMockAuth = true
	prodRouter := NewRouter(prodCfg, deps, slog.Default())
	assertAPIStatus(t, prodRouter, http.MethodPost, "/api/test/rooms", bytes.NewBufferString(`{"room_id":"room_denied"}`), userHeaders("host_1", "host"), http.StatusForbidden)
	assertAPIStatus(t, router, http.MethodPost, "/api/test/rooms", bytes.NewBufferString(`{"room_id":"room_user_denied"}`), userHeaders("user_1", "user"), http.StatusForbidden)
	assertAPIStatus(t, router, http.MethodPost, "/api/test/rooms", bytes.NewBufferString(`{"room_id":"room_foreign","host_id":"host_other"}`), userHeaders("host_1", "host"), http.StatusForbidden)
}
