package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"live-auction/backend/internal/config"
)

func TestRequireHostRejectsUserRole(t *testing.T) {
	handler := mockAuthMiddleware(testConfig())(requireHost(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	req.Header.Set("X-Mock-Role", "user")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireHostAllowsHostRole(t *testing.T) {
	handler := mockAuthMiddleware(testConfig())(requireHost(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	req.Header.Set("X-Mock-Role", "host")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func testConfig() config.Config {
	return config.Config{MockHostUserID: "host_1", MockUserID: "user_1"}
}
