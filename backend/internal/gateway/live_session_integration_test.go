package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/storage"
)

func TestLiveSessionReturnsMediaOnlyDescriptor(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	row := createACLAuction(t, repo, db, "room_live_session_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")
	if _, err := db.Exec(context.Background(), `UPDATE items SET image_url = $1 WHERE id = $2`, "items/live-session-poster.jpg", row.ItemID); err != nil {
		t.Fatalf("update item poster: %v", err)
	}
	cfg := testConfig()
	cfg.LiveDemoMediaProtocol = "ll-hls"
	cfg.LiveDemoMediaURL = "http://127.0.0.1:8888/auc_live/index.m3u8"
	cfg.LiveDemoMimeType = "application/vnd.apple.mpegurl"
	cfg.LiveDemoIsLive = true
	cfg.LiveDemoLatencyMs = 3000
	cfg.LiveFallbackMP4URL = "/demo/jade-live-loop.mp4"
	router := NewRouter(cfg, deps, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/live/sessions/"+row.ID, nil)
	for key, values := range userHeaders("user_1", "user") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("live session status = %d body=%s", rec.Code, rec.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw live session: %v", err)
	}
	allowedKeys := map[string]bool{
		"auctionId":       true,
		"isLive":          true,
		"posterURL":       true,
		"sources":         true,
		"latencyTargetMs": true,
		"capabilities":    true,
		"sessionEpoch":    true,
	}
	for key := range raw {
		if !allowedKeys[key] {
			t.Fatalf("live session leaked unsupported key %q in %#v", key, raw)
		}
	}
	forbiddenKeys := []string{
		"status",
		"current_price_cents",
		"currentWinnerId",
		"current_winner_id",
		"winner",
		"seq",
		"end_at",
		"endAt",
		"settlement_status",
		"rule",
	}
	for _, key := range forbiddenKeys {
		if _, ok := raw[key]; ok {
			t.Fatalf("live session leaked auction truth key %q in %#v", key, raw)
		}
	}

	var body struct {
		AuctionID       string `json:"auctionId"`
		IsLive          bool   `json:"isLive"`
		PosterURL       string `json:"posterURL"`
		LatencyTargetMS int    `json:"latencyTargetMs"`
		Sources         []struct {
			Protocol string `json:"protocol"`
			URL      string `json:"url"`
			MimeType string `json:"mimeType"`
			Priority int    `json:"priority"`
		} `json:"sources"`
		Capabilities struct {
			NativeHlsOnSafari bool `json:"nativeHlsOnSafari"`
			MSEHls            bool `json:"mseHls"`
			WebRTC            bool `json:"webrtc"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode live session body: %v", err)
	}
	if body.AuctionID != row.ID || !body.IsLive || body.LatencyTargetMS != 3000 {
		t.Fatalf("unexpected descriptor identity/live fields: %#v", body)
	}
	if body.PosterURL != "/api/media/items/live-session-poster.jpg" {
		t.Fatalf("posterURL = %q", body.PosterURL)
	}
	if len(body.Sources) != 2 || body.Sources[0].Protocol != "ll-hls" || body.Sources[0].URL != "http://127.0.0.1:8888/auc_live/index.m3u8" || body.Sources[0].MimeType != "application/vnd.apple.mpegurl" || body.Sources[0].Priority != 10 {
		t.Fatalf("unexpected sources: %#v", body.Sources)
	}
	if body.Sources[1].Protocol != "mp4" || body.Sources[1].URL != "/demo/jade-live-loop.mp4" || body.Sources[1].MimeType != "video/mp4" || body.Sources[1].Priority != 90 {
		t.Fatalf("unexpected sources: %#v", body.Sources)
	}
	if !body.Capabilities.NativeHlsOnSafari || !body.Capabilities.MSEHls || body.Capabilities.WebRTC {
		t.Fatalf("unexpected capabilities: %#v", body.Capabilities)
	}
}

func TestLiveSessionRequiresAuctionMembership(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	foreign := createACLAuction(t, repo, db, "room_live_session_foreign_"+uuid.NewString(), "host_1", "user_1", "BANNED")
	router := NewRouter(testConfig(), deps, slog.Default())

	assertAPIStatus(t, router, http.MethodGet, "/api/live/sessions/"+foreign.ID, nil, userHeaders("user_1", "user"), http.StatusForbidden)
}
