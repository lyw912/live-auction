package gateway

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"live-auction/backend/internal/config"
	"live-auction/backend/internal/storage"
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

func TestAuthMiddlewareRejectsMockHeadersWhenDisabled(t *testing.T) {
	handler := authMiddleware(config.Config{AppEnv: "local", MockHostUserID: "host_1", MockUserID: "user_1"}, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("X-Mock-Role", "host")
	req.Header.Set("X-Mock-User-Id", "host_1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLoginMeLogoutAndRevokedSession(t *testing.T) {
	db := openMonitorDB(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: openMonitorRedis(t)}, nilLogger())

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"account":"host"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, loginReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	cookie := findSessionCookie(t, rec.Result().Cookies())

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, meReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", rec.Code, rec.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, logoutReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d body=%s", rec.Code, rec.Body.String())
	}

	meReq = httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, meReq)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked me status = %d, want 401 body=%s", rec.Code, rec.Body.String())
	}
}

func TestLoginRejectsArbitraryUserIDShortcut(t *testing.T) {
	db := openMonitorDB(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: openMonitorRedis(t)}, nilLogger())

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"user_id":"host_1"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("login arbitrary user_id status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestExpiredSessionRejects(t *testing.T) {
	db := openMonitorDB(t)
	cfg := testConfig()
	cfg.SessionTTL = time.Second
	handler := authMiddleware(cfg, db)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	token, tokenHash, err := newSessionToken()
	if err != nil {
		t.Fatalf("newSessionToken: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO auth_sessions (id, user_id, role, token_hash, expires_at)
		VALUES ('sess_test_expired', 'host_1', 'host', $1, now() - interval '1 second')
		ON CONFLICT (id) DO UPDATE SET token_hash = excluded.token_hash, expires_at = excluded.expires_at, revoked_at = NULL
	`, tokenHash); err != nil {
		t.Fatalf("insert expired session: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session status = %d, want 401", rec.Code)
	}
}

func findSessionCookie(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatalf("missing %s cookie in %#v", sessionCookieName, cookies)
	return nil
}

func testConfig() config.Config {
	return config.Config{AppEnv: "test", MockHostUserID: "host_1", MockUserID: "user_1", SessionTTL: 12 * time.Hour}
}

func nilLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
