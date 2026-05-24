package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/redisx"
	"live-auction/backend/internal/storage"
)

func TestBidAdmissionCompletedReplayBypassesRedisLimiter(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	ctx := context.Background()
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidUserLimitPerSecond = 1
	cfg.BidIPLimitPerSecond = 1
	cfg.BidAuctionLimitPerSecond = 1
	router := NewRouter(cfg, &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())

	body := `{"client_bid_id":"admission-replay","amount_cents":15000,"client_seen_seq":0}`
	first := performBid(router, auctionRow.ID, body, "admission-replay", "user_1")
	if first.Code != http.StatusOK {
		t.Fatalf("first bid status = %d body=%s", first.Code, first.Body.String())
	}
	var firstResp auction.BidResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if firstResp.Result != auction.BidResultAccepted {
		t.Fatalf("first result = %s, want ACCEPTED body=%s", firstResp.Result, first.Body.String())
	}
	if _, err := rdb.Set(ctx, redisx.BidLimitUserKey(auctionRow.ID, "user_1"), 99, time.Second).Result(); err != nil {
		t.Fatalf("force user limit: %v", err)
	}
	if _, err := rdb.Set(ctx, redisx.BidLimitIPKey(auctionRow.ID, "192.0.2.1"), 99, time.Second).Result(); err != nil {
		t.Fatalf("force ip limit: %v", err)
	}
	if _, err := rdb.Set(ctx, redisx.BidLimitAuctionKey(auctionRow.ID), 99, time.Second).Result(); err != nil {
		t.Fatalf("force auction limit: %v", err)
	}

	replay := performBid(router, auctionRow.ID, body, "admission-replay", "user_1")
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status = %d body=%s", replay.Code, replay.Body.String())
	}
	var replayResp auction.BidResponse
	if err := json.Unmarshal(replay.Body.Bytes(), &replayResp); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if replayResp.BidID != firstResp.BidID || replayResp.Seq != firstResp.Seq {
		t.Fatalf("replay mismatch: got %#v want %#v", replayResp, firstResp)
	}
}

func TestBidAdmissionRedisDownFailsOpenAndRecordsAnomaly(t *testing.T) {
	db := openMonitorDB(t)
	badRedis := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 5 * time.Millisecond, ReadTimeout: 5 * time.Millisecond, WriteTimeout: 5 * time.Millisecond})
	t.Cleanup(func() { _ = badRedis.Close() })
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidLimitRedisTimeout = 10 * time.Millisecond
	router := NewRouter(cfg, &storage.Dependencies{Postgres: db, Redis: badRedis}, slog.Default())

	body := `{"client_bid_id":"admission-redis-down","amount_cents":15000,"client_seen_seq":0}`
	rec := performBid(router, auctionRow.ID, body, "admission-redis-down", "user_1")
	if rec.Code != http.StatusOK {
		t.Fatalf("bid status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp auction.BidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != auction.BidResultAccepted {
		t.Fatalf("result = %s, want ACCEPTED body=%s", resp.Result, rec.Body.String())
	}
	assertAdmissionAnomalyRecorded(t, db, auctionRow.ID, rateLimitRedisDownAnomaly)
}

func TestBidAdmissionUserLimitReturnsRateLimited(t *testing.T) {
	observability.Default = observability.NewRegistry()
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidUserLimitPerSecond = 1
	cfg.BidIPLimitPerSecond = 100
	cfg.BidAuctionLimitPerSecond = 100
	router := NewRouter(cfg, &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())

	firstBody := `{"client_bid_id":"admission-user-limit-1","amount_cents":15000,"client_seen_seq":0}`
	first := performBid(router, auctionRow.ID, firstBody, "admission-user-limit-1", "user_1")
	if first.Code != http.StatusOK {
		t.Fatalf("first bid status = %d body=%s", first.Code, first.Body.String())
	}
	limitedBody := `{"client_bid_id":"admission-user-limit-2","amount_cents":20000,"client_seen_seq":0}`
	limited := performBid(router, auctionRow.ID, limitedBody, "admission-user-limit-2", "user_1")
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status = %d, want 429 body=%s", limited.Code, limited.Body.String())
	}
	var payload apierrors.APIError
	if err := json.Unmarshal(limited.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode limited response: %v", err)
	}
	if payload.Code != apierrors.CodeRateLimited {
		t.Fatalf("code = %s, want RATE_LIMITED", payload.Code)
	}
	assertAdmissionAnomalyRecorded(t, db, auctionRow.ID, string(apierrors.CodeRateLimited))
	metrics := string(observability.Default.Render(context.Background()))
	for _, want := range []string{
		`redis_lua_script_total{outcome="allowed",script="` + redisx.ScriptBidAdmissionGCRA + `"}`,
		`redis_lua_script_total{outcome="rejected",script="` + redisx.ScriptBidAdmissionGCRA + `"}`,
		`redis_lua_script_latency_seconds_count{outcome="allowed",script="` + redisx.ScriptBidAdmissionGCRA + `"}`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics missing %q in:\n%s", want, metrics)
		}
	}
}

func TestBidAdmissionLocalAuctionTooHotReturnsRetryAfter(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidAuctionMaxInFlight = 1
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
	}
	raw, _ := handler.Bids.semaphores.LoadOrStore(auctionRow.ID, make(chan struct{}, 1))
	raw.(chan struct{}) <- struct{}{}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	body := `{"client_bid_id":"admission-too-hot","amount_cents":15000,"client_seen_seq":0}`
	rec := performBid(router, auctionRow.ID, body, "admission-too-hot", "user_1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, want 1", rec.Header().Get("Retry-After"))
	}
	var payload apierrors.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if payload.Code != apierrors.CodeBidAuctionTooHot {
		t.Fatalf("code = %s, want BID_AUCTION_TOO_HOT", payload.Code)
	}
	assertAdmissionAnomalyRecorded(t, db, auctionRow.ID, string(apierrors.CodeBidAuctionTooHot))
}

func TestAdmissionDisabledBypassesBidRedisAndLocalLimits(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	ctx := context.Background()
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.AdmissionEnabled = false
	cfg.BidUserLimitPerSecond = 1
	cfg.BidIPLimitPerSecond = 1
	cfg.BidAuctionLimitPerSecond = 1
	cfg.BidAuctionMaxInFlight = 1
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
	}
	if _, err := rdb.Set(ctx, redisx.BidLimitUserKey(auctionRow.ID, "user_1"), 99, time.Second).Result(); err != nil {
		t.Fatalf("force user limit: %v", err)
	}
	raw, _ := handler.Bids.semaphores.LoadOrStore(auctionRow.ID, make(chan struct{}, 1))
	raw.(chan struct{}) <- struct{}{}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	body := `{"client_bid_id":"admission-disabled","amount_cents":15000,"client_seen_seq":0}`
	rec := performBid(router, auctionRow.ID, body, "admission-disabled", "user_1")
	if rec.Code == http.StatusTooManyRequests {
		t.Fatalf("admission disabled still returned 429 body=%s", rec.Body.String())
	}
	var count int
	if err := db.QueryRow(context.Background(), `
		SELECT count(*)
		FROM system_anomaly_events
		WHERE auction_id = $1 AND type IN ($2, $3)
	`, auctionRow.ID, string(apierrors.CodeRateLimited), string(apierrors.CodeBidAuctionTooHot)).Scan(&count); err != nil {
		t.Fatalf("count admission anomalies: %v", err)
	}
	if count != 0 {
		t.Fatalf("admission disabled recorded %d admission anomalies", count)
	}
}

func createAdmissionAuction(t *testing.T, db *pgxpool.Pool, viewerID string) auction.Auction {
	t.Helper()
	repo := auction.NewRepository(db)
	roomID := "room_admission_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO users (id, role, display_name) VALUES ('host_1', 'host', 'Host'), ($1, 'user', $1)
		ON CONFLICT DO NOTHING
	`, viewerID); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN')
	`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, 'host_1', 'host', 'ACTIVE'), ($1, $2, 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL
	`, roomID, viewerID); err != nil {
		t.Fatalf("insert memberships: %v", err)
	}
	item, err := repo.CreateItem(context.Background(), auction.CreateItemInput{Title: "Admission Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	row, err := repo.CreateAuction(context.Background(), auction.CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		Rule: auction.Rule{
			DurationSeconds:     60,
			ExtendWindowSeconds: 10,
			ExtendBySeconds:     10,
			MaxExtendCount:      3,
			DepositBPS:          1000,
			DepositFloorCents:   5_000,
			DepositCapCents:     50_000,
		},
	}, "tr_admission")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if _, err := repo.Schedule(context.Background(), row.ID, nil, "tr_admission"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	started, err := repo.Start(context.Background(), row.ID, "tr_admission")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return started
}

func performBid(router http.Handler, auctionID string, body string, key string, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auctions/"+auctionID+"/bids", bytes.NewBufferString(body))
	req.RemoteAddr = "192.0.2.1:12345"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("X-Mock-Role", "user")
	req.Header.Set("X-Mock-User-Id", userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func admissionTestConfig() config.Config {
	cfg := testConfig()
	cfg.BidUserLimitPerSecond = 100
	cfg.BidIPLimitPerSecond = 100
	cfg.BidAuctionLimitPerSecond = 100
	cfg.BidAuctionMaxInFlight = 64
	cfg.BidLimitWindow = time.Second
	cfg.BidLimitRedisTimeout = 50 * time.Millisecond
	return cfg
}

func assertAdmissionAnomalyRecorded(t *testing.T, db *pgxpool.Pool, auctionID string, anomalyType string) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `
		SELECT count(*)
		FROM system_anomaly_events
		WHERE auction_id = $1 AND type = $2
	`, auctionID, anomalyType).Scan(&count); err != nil {
		t.Fatalf("count anomaly: %v", err)
	}
	if count == 0 {
		t.Fatalf("missing anomaly %s for auction %s", anomalyType, auctionID)
	}
}
