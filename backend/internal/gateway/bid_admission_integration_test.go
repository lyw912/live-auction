package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	blockUntilMS := time.Now().Add(time.Second).UnixMilli()
	if _, err := rdb.Set(ctx, redisx.BidLimitUserKey(auctionRow.ID, "user_1"), blockUntilMS, time.Second).Result(); err != nil {
		t.Fatalf("force user limit: %v", err)
	}
	if _, err := rdb.Set(ctx, redisx.BidLimitIPKey(auctionRow.ID, "192.0.2.1"), blockUntilMS, time.Second).Result(); err != nil {
		t.Fatalf("force ip limit: %v", err)
	}
	if _, err := rdb.Set(ctx, redisx.BidLimitAuctionKey(auctionRow.ID), blockUntilMS, time.Second).Result(); err != nil {
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
	if limited.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, want 1", limited.Header().Get("Retry-After"))
	}
	if payload.Details == nil || payload.Details["retry_after_ms"] == nil || payload.Details["retry_after_secs"] == nil {
		t.Fatalf("missing retry-after details in payload: %#v", payload.Details)
	}
	assertAdmissionAnomalyRecorded(t, db, auctionRow.ID, string(apierrors.CodeRateLimited))
	assertAdmissionAnomalyRetryAfterRecorded(t, db, auctionRow.ID, string(apierrors.CodeRateLimited))
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
	if payload.Details == nil || payload.Details["retry_after_ms"] == nil || payload.Details["retry_after_secs"] == nil {
		t.Fatalf("missing retry-after details in payload: %#v", payload.Details)
	}
	assertAdmissionAnomalyRecorded(t, db, auctionRow.ID, string(apierrors.CodeBidAuctionTooHot))
	assertAdmissionAnomalyRetryAfterRecorded(t, db, auctionRow.ID, string(apierrors.CodeBidAuctionTooHot))
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

func TestPostgresBidLaneFullReturnsRetryAfterAndRecordsAnomaly(t *testing.T) {
	observability.Default = observability.NewRegistry()
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidLaneQueueSize = 1
	cfg.BidLaneQueueTimeout = time.Second
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
		Lanes:  newBidLaneManager(cfg, db),
	}
	lane := &bidLane{auctionID: auctionRow.ID, tasks: make(chan *bidLaneTask, 1)}
	handler.Lanes.lanes.Store(auctionRow.ID, lane)
	lane.tasks <- &bidLaneTask{
		ctx:       context.Background(),
		queuedAt:  time.Now(),
		expiresAt: time.Now().Add(time.Second),
		started:   make(chan struct{}),
		done:      make(chan bidLaneResult, 1),
		run: func(context.Context) (auction.BidResponse, error) {
			return auction.BidResponse{}, nil
		},
	}
	lane.depth.Add(1)
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	body := `{"client_bid_id":"lane-full","amount_cents":15000,"client_seen_seq":0}`
	rec := performBid(router, auctionRow.ID, body, "lane-full", "user_1")
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
	assertAdmissionAnomalyRetryAfterRecorded(t, db, auctionRow.ID, string(apierrors.CodeBidAuctionTooHot))
	metrics := string(observability.Default.Render(context.Background()))
	for _, want := range []string{
		`auction_bid_queue_rejected_total{reason="BID_AUCTION_TOO_HOT"} 1`,
		`auction_bid_queue_depth{auction_id="` + auctionRow.ID + `"}`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics missing %q in:\n%s", want, metrics)
		}
	}
}

func TestPostgresBidLaneCompletedReplayBypassesFullQueue(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidLaneQueueSize = 1
	cfg.BidLaneQueueTimeout = time.Second
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
		Lanes:  newBidLaneManager(cfg, db),
	}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	body := `{"client_bid_id":"lane-replay","amount_cents":15000,"client_seen_seq":0}`
	first := performBid(router, auctionRow.ID, body, "lane-replay", "user_1")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}
	var firstResp auction.BidResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	lane := &bidLane{auctionID: auctionRow.ID, tasks: make(chan *bidLaneTask, 1)}
	handler.Lanes.lanes.Store(auctionRow.ID, lane)
	lane.tasks <- &bidLaneTask{
		ctx:       context.Background(),
		queuedAt:  time.Now(),
		expiresAt: time.Now().Add(time.Second),
		started:   make(chan struct{}),
		done:      make(chan bidLaneResult, 1),
		run: func(context.Context) (auction.BidResponse, error) {
			return auction.BidResponse{}, nil
		},
	}
	lane.depth.Add(1)

	replay := performBid(router, auctionRow.ID, body, "lane-replay", "user_1")
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

func TestPostgresBidLaneCompletedConfirmReplayBypassesFullQueue(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidLaneQueueSize = 1
	cfg.BidLaneQueueTimeout = time.Second
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
		Lanes:  newBidLaneManager(cfg, db),
	}
	completed := auction.BidResponse{
		Result:            auction.BidResultAccepted,
		BidID:             "bid_confirm_replay",
		AuctionID:         auctionRow.ID,
		Seq:               7,
		CurrentPriceCents: 15_000,
		ServerTimeMS:      time.Now().UnixMilli(),
	}
	responseJSON, err := json.Marshal(completed)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO idempotency_records (scope_type, scope_id, user_id, idempotency_key, request_hash, status, http_status, result_code, response_json, completed_at)
		VALUES ('bid', $1, 'user_1', 'lane-confirm-replay', 'confirm-hash', 'COMPLETED', 200, $2, $3, now())
	`, auctionRow.ID, auction.BidResultAccepted, responseJSON); err != nil {
		t.Fatalf("insert completed confirm replay: %v", err)
	}
	lane := &bidLane{auctionID: auctionRow.ID, tasks: make(chan *bidLaneTask, 1)}
	handler.Lanes.lanes.Store(auctionRow.ID, lane)
	lane.tasks <- &bidLaneTask{
		ctx:       context.Background(),
		queuedAt:  time.Now(),
		expiresAt: time.Now().Add(time.Second),
		started:   make(chan struct{}),
		done:      make(chan bidLaneResult, 1),
		run: func(context.Context) (auction.BidResponse, error) {
			return auction.BidResponse{}, nil
		},
	}
	lane.depth.Add(1)
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids/confirm", handler.ConfirmBid)

	req := httptest.NewRequest(http.MethodPost, "/api/auctions/"+auctionRow.ID+"/bids/confirm", bytes.NewBufferString(`{"confirm_token":"already-settled","idempotency_key":"lane-confirm-replay"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "lane-confirm-replay")
	req.Header.Set("X-Mock-Role", "user")
	req.Header.Set("X-Mock-User-Id", "user_1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm replay status = %d body=%s", rec.Code, rec.Body.String())
	}
	var replay auction.BidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &replay); err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if replay.BidID != completed.BidID || replay.Seq != completed.Seq {
		t.Fatalf("confirm replay mismatch: got %#v want %#v", replay, completed)
	}
}

func TestRedisGuardRejectsClearlyTooLowBidBeforePostgresLane(t *testing.T) {
	observability.Default = observability.NewRegistry()
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidEngineMode = bidEngineModeRedisGuard
	cfg.BidRedisGuardMaxStaleness = time.Second
	seedRedisGuardProjection(t, rdb, auctionRow.ID, map[string]any{
		"status":              "ACTIVE",
		"current_price_cents": 20_000,
		"start_price_cents":   10_000,
		"increment_cents":     5_000,
		"cap_price_cents":     0,
		"end_at_ms":           time.Now().Add(time.Minute).UnixMilli(),
		"seq":                 7,
		"accepted_bid_count":  2,
		"current_winner_id":   "user_2",
		"projected_at_ms":     time.Now().UnixMilli(),
	})
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
		Lanes:  newBidLaneManager(cfg, db),
		Guard:  newRedisGuard(cfg, db, rdb),
	}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	body := `{"client_bid_id":"guard-too-low","amount_cents":20000,"client_seen_seq":7}`
	rec := performBid(router, auctionRow.ID, body, "guard-too-low", "user_1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp auction.BidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != auction.BidResultRejected || resp.RejectReason == nil || *resp.RejectReason != string(apierrors.CodeBidTooLow) {
		t.Fatalf("unexpected guard response: %#v", resp)
	}
	var bidRows int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM bids WHERE auction_id = $1 AND client_bid_id = 'guard-too-low'`, auctionRow.ID).Scan(&bidRows); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if bidRows != 0 {
		t.Fatalf("guard reject wrote %d bid rows, want 0", bidRows)
	}
	assertAdmissionAnomalyRecorded(t, db, auctionRow.ID, string(apierrors.CodeBidTooLow))
	metrics := string(observability.Default.Render(context.Background()))
	if !strings.Contains(metrics, `auction_bid_redis_guard_total{outcome="REJECT",reason="BID_TOO_LOW"} 1`) {
		t.Fatalf("missing guard reject metric in:\n%s", metrics)
	}
}

func TestRedisGuardMissingProjectionFallsThroughToPostgresTruth(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidEngineMode = bidEngineModeRedisGuard
	cfg.BidRedisGuardMaxStaleness = 50 * time.Millisecond
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
		Lanes:  newBidLaneManager(cfg, db),
		Guard:  newRedisGuard(cfg, db, rdb),
	}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	missingBody := `{"client_bid_id":"guard-missing","amount_cents":15000,"client_seen_seq":0}`
	missing := performBid(router, auctionRow.ID, missingBody, "guard-missing", "user_1")
	if missing.Code != http.StatusOK {
		t.Fatalf("missing projection status = %d body=%s", missing.Code, missing.Body.String())
	}
	var missingResp auction.BidResponse
	if err := json.Unmarshal(missing.Body.Bytes(), &missingResp); err != nil {
		t.Fatalf("decode missing response: %v", err)
	}
	if missingResp.Result != auction.BidResultAccepted {
		t.Fatalf("missing projection did not fall through to PG: %#v", missingResp)
	}
}

func TestRedisGuardStaleProjectionRejectsAtOrBelowOldCurrentPrice(t *testing.T) {
	observability.Default = observability.NewRegistry()
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidEngineMode = bidEngineModeRedisGuard
	cfg.BidRedisGuardMaxStaleness = 50 * time.Millisecond
	repo := auction.NewRepository(db)
	first := auction.BidInput{ClientBidID: "guard-stale-reject-first", AmountCents: 15_000}
	if _, err := repo.PlaceBid(context.Background(), auctionRow.ID, "user_1", first.ClientBidID, first, "tr_guard_stale_reject_first"); err != nil {
		t.Fatalf("seed first bid: %v", err)
	}
	if _, err := db.Exec(context.Background(), `INSERT INTO users (id, role, display_name) VALUES ('user_2', 'user', 'Guard User 2') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert user_2: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		SELECT room_id, 'user_2', 'viewer', 'ACTIVE'
		FROM auctions WHERE id = $1
		ON CONFLICT (room_id, user_id) DO UPDATE SET status = 'ACTIVE', left_at = NULL
	`, auctionRow.ID); err != nil {
		t.Fatalf("insert user_2 membership: %v", err)
	}
	seedRedisGuardProjection(t, rdb, auctionRow.ID, map[string]any{
		"status":              "ACTIVE",
		"current_price_cents": 15_000,
		"start_price_cents":   10_000,
		"increment_cents":     5_000,
		"cap_price_cents":     0,
		"end_at_ms":           time.Now().Add(time.Minute).UnixMilli(),
		"seq":                 1,
		"accepted_bid_count":  1,
		"current_winner_id":   "user_1",
		"projected_at_ms":     time.Now().Add(-time.Minute).UnixMilli(),
	})
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   repo,
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
		Lanes:  newBidLaneManager(cfg, db),
		Guard:  newRedisGuard(cfg, db, rdb),
	}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	body := `{"client_bid_id":"guard-stale-too-low","amount_cents":15000,"client_seen_seq":0}`
	rec := performBid(router, auctionRow.ID, body, "guard-stale-too-low", "user_2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp auction.BidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != auction.BidResultRejected || resp.RejectReason == nil || *resp.RejectReason != string(apierrors.CodeBidTooLow) {
		t.Fatalf("unexpected stale guard response: %#v", resp)
	}
	var bidRows int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM bids WHERE auction_id = $1 AND client_bid_id = 'guard-stale-too-low'`, auctionRow.ID).Scan(&bidRows); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if bidRows != 0 {
		t.Fatalf("guard stale reject wrote %d bid rows, want 0", bidRows)
	}
	metrics := string(observability.Default.Render(context.Background()))
	if !strings.Contains(metrics, `auction_bid_redis_guard_total{outcome="REJECT",reason="BID_TOO_LOW"} 1`) {
		t.Fatalf("missing guard reject metric in:\n%s", metrics)
	}
}

func TestRedisGuardStaleProjectionFallsThroughWhenBidMightStillWin(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidEngineMode = bidEngineModeRedisGuard
	cfg.BidRedisGuardMaxStaleness = 50 * time.Millisecond
	repo := auction.NewRepository(db)
	first := auction.BidInput{ClientBidID: "guard-stale-first", AmountCents: 15_000}
	if _, err := repo.PlaceBid(context.Background(), auctionRow.ID, "user_1", first.ClientBidID, first, "tr_guard_stale_first"); err != nil {
		t.Fatalf("seed first bid: %v", err)
	}

	seedRedisGuardProjection(t, rdb, auctionRow.ID, map[string]any{
		"status":              "ACTIVE",
		"current_price_cents": 10_000,
		"start_price_cents":   10_000,
		"increment_cents":     5_000,
		"cap_price_cents":     0,
		"end_at_ms":           time.Now().Add(time.Minute).UnixMilli(),
		"seq":                 0,
		"accepted_bid_count":  0,
		"current_winner_id":   "",
		"projected_at_ms":     time.Now().Add(-time.Minute).UnixMilli(),
	})
	if _, err := db.Exec(context.Background(), `INSERT INTO users (id, role, display_name) VALUES ('user_2', 'user', 'Guard User 2') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert user_2: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		SELECT room_id, 'user_2', 'viewer', 'ACTIVE'
		FROM auctions WHERE id = $1
		ON CONFLICT (room_id, user_id) DO UPDATE SET status = 'ACTIVE', left_at = NULL
	`, auctionRow.ID); err != nil {
		t.Fatalf("insert user_2 membership: %v", err)
	}
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   repo,
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
		Lanes:  newBidLaneManager(cfg, db),
		Guard:  newRedisGuard(cfg, db, rdb),
	}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	staleBody := `{"client_bid_id":"guard-stale","amount_cents":20000,"client_seen_seq":0}`
	stale := performBid(router, auctionRow.ID, staleBody, "guard-stale", "user_2")
	if stale.Code != http.StatusOK {
		t.Fatalf("stale projection status = %d body=%s", stale.Code, stale.Body.String())
	}
	var staleResp auction.BidResponse
	if err := json.Unmarshal(stale.Body.Bytes(), &staleResp); err != nil {
		t.Fatalf("decode stale response: %v", err)
	}
	if staleResp.Result != auction.BidResultAccepted {
		t.Fatalf("stale projection blocked PG truth: %#v", staleResp)
	}
}

func TestRedisGuardRefreshesProjectionAfterAcceptedBid(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidEngineMode = bidEngineModeRedisGuard
	cfg.BidRedisGuardMaxStaleness = 50 * time.Millisecond
	seedRedisGuardProjection(t, rdb, auctionRow.ID, map[string]any{
		"status":              "ACTIVE",
		"current_price_cents": 10_000,
		"start_price_cents":   10_000,
		"increment_cents":     5_000,
		"cap_price_cents":     0,
		"end_at_ms":           time.Now().Add(time.Minute).UnixMilli(),
		"seq":                 0,
		"accepted_bid_count":  0,
		"current_winner_id":   "",
		"projected_at_ms":     time.Now().Add(-time.Minute).UnixMilli(),
	})
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: rdb},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, rdb),
		Lanes:  newBidLaneManager(cfg, db),
		Guard:  newRedisGuard(cfg, db, rdb),
	}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	body := `{"client_bid_id":"guard-refresh-accepted","amount_cents":15000,"client_seen_seq":0}`
	rec := performBid(router, auctionRow.ID, body, "guard-refresh-accepted", "user_1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp auction.BidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != auction.BidResultAccepted {
		t.Fatalf("bid not accepted: %#v", resp)
	}
	projection, err := rdb.HGetAll(context.Background(), redisx.BidGuardProjectionKey(auctionRow.ID)).Result()
	if err != nil {
		t.Fatalf("read projection: %v", err)
	}
	if projection["current_price_cents"] != "15000" || projection["seq"] != strconv.FormatInt(resp.Seq, 10) || projection["current_winner_id"] != "user_1" {
		t.Fatalf("projection not refreshed from accepted bid: %#v", projection)
	}
	if ttl := rdb.TTL(context.Background(), redisx.BidGuardProjectionKey(auctionRow.ID)).Val(); ttl <= 0 {
		t.Fatalf("projection ttl = %s, want positive", ttl)
	}
}

func TestRedisGuardRefreshDoesNotOverwriteNewerProjection(t *testing.T) {
	observability.Default = observability.NewRegistry()
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidEngineMode = bidEngineModeRedisGuard
	cfg.BidRedisGuardMaxStaleness = 50 * time.Millisecond
	seedRedisGuardProjection(t, rdb, auctionRow.ID, map[string]any{
		"status":              "ACTIVE",
		"current_price_cents": 30_000,
		"start_price_cents":   10_000,
		"increment_cents":     5_000,
		"cap_price_cents":     0,
		"end_at_ms":           time.Now().Add(time.Minute).UnixMilli(),
		"seq":                 3,
		"accepted_bid_count":  3,
		"current_winner_id":   "user_3",
		"projected_at_ms":     time.Now().UnixMilli(),
	})
	guard := newRedisGuard(cfg, db, rdb)
	guard.RefreshAfterAcceptedBid(context.Background(), auction.BidResponse{
		Result:            auction.BidResultAccepted,
		AuctionID:         auctionRow.ID,
		Seq:               2,
		CurrentPriceCents: 20_000,
		CurrentWinnerID:   ptrString("user_2"),
		EndAt:             auctionRow.EndAt,
	})
	projection, err := rdb.HGetAll(context.Background(), redisx.BidGuardProjectionKey(auctionRow.ID)).Result()
	if err != nil {
		t.Fatalf("read projection: %v", err)
	}
	if projection["seq"] != "3" || projection["current_price_cents"] != "30000" || projection["current_winner_id"] != "user_3" {
		t.Fatalf("older refresh overwrote newer projection: %#v", projection)
	}
	metrics := string(observability.Default.Render(context.Background()))
	if !strings.Contains(metrics, `auction_bid_redis_guard_projection_update_total{outcome="stale"} 1`) {
		t.Fatalf("missing stale projection refresh metric in:\n%s", metrics)
	}
}

func TestRedisGuardUnavailableFallsThroughToPostgresTruth(t *testing.T) {
	db := openMonitorDB(t)
	badRedis := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 5 * time.Millisecond, ReadTimeout: 5 * time.Millisecond, WriteTimeout: 5 * time.Millisecond})
	t.Cleanup(func() { _ = badRedis.Close() })
	auctionRow := createAdmissionAuction(t, db, "user_1")
	cfg := admissionTestConfig()
	cfg.BidEngineMode = bidEngineModeRedisGuard
	cfg.BidLimitRedisTimeout = 5 * time.Millisecond
	cfg.BidRedisGuardTimeout = 5 * time.Millisecond
	handler := AuctionHandler{
		Config: cfg,
		Deps:   &storage.Dependencies{Postgres: db, Redis: badRedis},
		Repo:   auction.NewRepository(db),
		ACL:    newRoomACL(db),
		Bids:   newBidAdmission(cfg, db, badRedis),
		Lanes:  newBidLaneManager(cfg, db),
		Guard:  newRedisGuard(cfg, db, badRedis),
	}
	router := chi.NewRouter()
	router.Use(traceMiddleware)
	router.Use(mockAuthMiddleware(cfg))
	router.Post("/api/auctions/{id}/bids", handler.PlaceBid)

	body := `{"client_bid_id":"guard-unavailable","amount_cents":15000,"client_seen_seq":0}`
	rec := performBid(router, auctionRow.ID, body, "guard-unavailable", "user_1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp auction.BidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != auction.BidResultAccepted {
		t.Fatalf("redis unavailable did not fall through to PG: %#v", resp)
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

func seedRedisGuardProjection(t *testing.T, rdb *redis.Client, auctionID string, values map[string]any) {
	t.Helper()
	key := redisx.BidGuardProjectionKey(auctionID)
	if err := rdb.Del(context.Background(), key).Err(); err != nil {
		t.Fatalf("clear guard projection: %v", err)
	}
	pipe := rdb.TxPipeline()
	for field, value := range values {
		pipe.HSet(context.Background(), key, field, value)
	}
	if _, err := pipe.Exec(context.Background()); err != nil {
		t.Fatalf("seed guard projection: %v", err)
	}
	if err := rdb.Expire(context.Background(), key, time.Minute).Err(); err != nil {
		t.Fatalf("expire guard projection: %v", err)
	}
}

func ptrString(value string) *string {
	return &value
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
	cfg.BidEngineMode = bidEngineModePostgresLane
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

func assertAdmissionAnomalyRetryAfterRecorded(t *testing.T, db *pgxpool.Pool, auctionID string, anomalyType string) {
	t.Helper()
	var retryAfterMS int64
	var retryAfterSecs int
	if err := db.QueryRow(context.Background(), `
		SELECT (payload_json->>'retry_after_ms')::bigint, (payload_json->>'retry_after_secs')::int
		FROM system_anomaly_events
		WHERE auction_id = $1 AND type = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, auctionID, anomalyType).Scan(&retryAfterMS, &retryAfterSecs); err != nil {
		t.Fatalf("read anomaly retry-after payload: %v", err)
	}
	if retryAfterMS <= 0 || retryAfterSecs <= 0 {
		t.Fatalf("invalid retry-after payload for %s: ms=%d secs=%d", anomalyType, retryAfterMS, retryAfterSecs)
	}
}
