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

	"live-auction/backend/internal/auction"
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

func TestDemoCompetingBidIsLocalHostScopedAndWritesRealBid(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	router := NewRouter(testConfig(), deps, slog.Default())
	row := createACLAuction(t, repo, db, "room_demo_bid_"+uuid.NewString(), "host_1", "user_2", "ACTIVE")

	body := bytes.NewBufferString(`{"bidder_id":"user_2","client_bid_id":"demo-bid-1","amount_cents":15000,"client_seen_seq":0}`)
	assertAPIStatus(t, router, http.MethodPost, "/api/demo/auctions/"+row.ID+"/competing-bid", body, userHeaders("user_1", "user"), http.StatusForbidden)

	prodCfg := testConfig()
	prodCfg.AppEnv = "prod"
	prodCfg.AllowMockAuth = true
	prodRouter := NewRouter(prodCfg, deps, slog.Default())
	body = bytes.NewBufferString(`{"bidder_id":"user_2","client_bid_id":"demo-bid-prod","amount_cents":15000,"client_seen_seq":0}`)
	assertAPIStatus(t, prodRouter, http.MethodPost, "/api/demo/auctions/"+row.ID+"/competing-bid", body, userHeaders("host_1", "host"), http.StatusForbidden)

	body = bytes.NewBufferString(`{"bidder_id":"user_2","client_bid_id":"demo-bid-2","amount_cents":15000,"client_seen_seq":0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/demo/auctions/"+row.ID+"/competing-bid", body)
	for key, values := range userHeaders("host_1", "host") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("demo bid status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload auction.BidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode demo bid: %v", err)
	}
	if payload.Result != auction.BidResultAccepted || payload.CurrentPriceCents != 15000 {
		t.Fatalf("demo bid payload = %#v", payload)
	}
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM bids WHERE auction_id = $1 AND user_id = 'user_2' AND status = 'ACCEPTED'`, row.ID).Scan(&count); err != nil {
		t.Fatalf("count demo bid: %v", err)
	}
	if count != 1 {
		t.Fatalf("accepted demo bids = %d, want 1", count)
	}
}
