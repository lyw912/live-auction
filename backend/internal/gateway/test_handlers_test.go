package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/redisengine"
	"live-auction/backend/internal/redisx"
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
	cfg := testConfig()
	cfg.BidEngineResponseDurability = "kafka_ack"
	ledger := redisengine.NewMemoryLedger()
	worker := redisengine.NewWorker(db, rdb, ledger, "gateway-demo-bid-"+uuid.NewString())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go worker.Run(ctx, 10*time.Millisecond)
	router := NewRouterWithRealtimeAndLedger(cfg, deps, slog.Default(), nil, ledger)
	row := createACLAuction(t, repo, db, "room_demo_bid_"+uuid.NewString(), "host_1", "user_2", "ACTIVE")
	seedAdmissionACL(t, rdb, row, "user_1")
	seedAdmissionACL(t, rdb, row, "user_2")

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
	if payload.Result != auction.BidResultEngineAccepted || payload.CurrentPriceCents != 15000 {
		t.Fatalf("demo bid payload = %#v", payload)
	}
	if state, err := rdb.HGetAll(context.Background(), redisx.BidEngineStateKey(row.ID)).Result(); err != nil {
		t.Fatalf("read redis engine state: %v", err)
	} else if state["current_winner_id"] != "user_2" || state["current_price_cents"] != "15000" {
		t.Fatalf("redis engine state = %#v, want user_2 at 15000", state)
	}
	waitForAcceptedBidCount(t, db, row.ID, "user_2", 1)
}

func waitForAcceptedBidCount(t *testing.T, db *pgxpool.Pool, auctionID string, userID string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var count int
		if err := db.QueryRow(context.Background(), `SELECT count(*) FROM bids WHERE auction_id = $1 AND user_id = $2 AND status = 'ACCEPTED'`, auctionID, userID).Scan(&count); err != nil {
			t.Fatalf("count accepted bid: %v", err)
		}
		if count == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("accepted bids = %d, want %d for auction=%s user=%s", count, want, auctionID, userID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
