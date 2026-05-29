package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/storage"
)

func TestMonitorRoutesReturnRealDBRowsAndRequireHost(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	row := createMonitorAuction(t, repo, db)
	insertMonitorAnomaly(t, db, row.ID)
	forceMonitorSchedulerJob(t, db, row.ID)
	seedMonitorDebeziumDiagnostics(t, db, row.ID)

	router := NewRouter(testConfig(), deps, slog.Default())
	assertMonitorForbiddenForUser(t, router, "/api/monitor/auctions")
	assertMonitorHasItems(t, router, "/api/monitor/auctions", "auction_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/anomalies", "type", "MONITOR_TEST")
	assertMonitorHasItems(t, router, "/api/monitor/outbox", "aggregate_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/outbox", "delivery_state", "READY")
	assertMonitorHasItems(t, router, "/api/monitor/outbox/watermarks", "shard_id", float64(0))
	assertMonitorHasItems(t, router, "/api/monitor/outbox/watermarks", "ack_pending_count", float64(1))
	assertMonitorHasItems(t, router, "/api/monitor/scheduler", "target_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/rejects", "auction_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/recovery", "room_id", row.RoomID)
	assertMonitorHasItems(t, router, "/api/monitor/recovery", "max_queue_bytes", float64(65536))
	assertMonitorHasItems(t, router, "/api/monitor/snapshots", "auction_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/signals", "target_id", row.ID)
	assertMonitorCreateSignal(t, router, row.ID)
	assertMonitorFlightRecorder(t, router, row.ID)
}

func TestMonitorAnomaliesFilterByTypeUserAuctionAndTrace(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	auctionID := "auc_filter_" + uuid.NewString()
	traceID := "tr_filter_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES
		  ('MED', 'RATE_LIMITED', $1, 'filter target', jsonb_build_object('user_id','user_filter','room_id','room_filter','trace_id',$2::text)),
		  ('MED', 'RATE_LIMITED', $1, 'other trace', jsonb_build_object('user_id','user_filter','room_id','room_filter','trace_id','tr_other')),
		  ('MED', 'PAYMENT_RECONCILE_MISMATCH', $1, 'other type', jsonb_build_object('user_id','user_filter','room_id','room_filter','trace_id',$2::text))
	`, auctionID, traceID); err != nil {
		t.Fatalf("insert filter anomalies: %v", err)
	}
	router := NewRouter(testConfig(), deps, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/monitor/anomalies?type=RATE_LIMITED&auction_id="+auctionID+"&user_id=user_filter&room_id=room_filter&trace_id="+traceID, nil)
	req.Header.Set("X-Mock-Role", "host")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter anomalies status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode filter anomalies: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0]["type"] != "RATE_LIMITED" {
		t.Fatalf("unexpected filtered anomalies: %#v", body.Items)
	}
}

func assertMonitorForbiddenForUser(t *testing.T, router http.Handler, path string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Mock-Role", "user")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("%s user status = %d, want 403", path, rec.Code)
	}
}

func assertMonitorHasItems(t *testing.T, router http.Handler, path string, field string, want any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path+"?limit=20", nil)
	req.Header.Set("X-Mock-Role", "host")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	for _, item := range body.Items {
		if item[field] == want {
			return
		}
	}
	t.Fatalf("%s missing %s=%v in %#v", path, field, want, body.Items)
}

func assertMonitorCreateSignal(t *testing.T, router http.Handler, auctionID string) {
	t.Helper()
	body := bytes.NewBufferString(`{"signal_type":"force_snapshot_rebuild","target_type":"auction","target_id":"` + auctionID + `","reason":"monitor integration test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/monitor/signals", body)
	req.Header.Set("X-Mock-Role", "host")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create signal status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create signal: %v", err)
	}
	if payload["status"] != "PENDING" {
		t.Fatalf("create signal payload = %#v", payload)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/monitor/signals", bytes.NewBufferString(`{"signal_type":"retry_dead_outbox","target_type":"auction","target_id":"bad","reason":"bad"}`))
	req.Header.Set("X-Mock-Role", "host")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid signal status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func assertMonitorFlightRecorder(t *testing.T, router http.Handler, auctionID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/monitor/auctions/"+auctionID+"/flight-recorder?limit=20&timeline_limit=80", nil)
	req.Header.Set("X-Mock-Role", "host")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("flight recorder status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Summary struct {
			AuctionID string `json:"auction_id"`
			RoomID    string `json:"room_id"`
			Status    string `json:"status"`
		} `json:"summary"`
		Rules     []map[string]any `json:"rules"`
		Anomalies []map[string]any `json:"anomalies"`
		Timeline  []map[string]any `json:"timeline"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode flight recorder: %v", err)
	}
	if body.Summary.AuctionID != auctionID {
		t.Fatalf("flight recorder summary auction_id = %q, want %q", body.Summary.AuctionID, auctionID)
	}
	if len(body.Rules) == 0 {
		t.Fatalf("flight recorder missing rules: %#v", body)
	}
	if len(body.Anomalies) == 0 {
		t.Fatalf("flight recorder missing anomalies: %#v", body)
	}
	kinds := map[string]bool{}
	for _, row := range body.Timeline {
		if kind, ok := row["kind"].(string); ok {
			kinds[kind] = true
			if kind == "bid" {
				payload, ok := row["payload"].(map[string]any)
				if !ok || payload["source"] != "MANUAL" {
					t.Fatalf("flight recorder bid row missing source: %#v", row)
				}
				if _, leaked := payload["max_amount_cents"]; leaked {
					t.Fatalf("flight recorder bid row leaked private max amount: %#v", row)
				}
			}
		}
	}
	for _, want := range []string{"auction_event", "bid", "outbox", "anomaly", "snapshot_rebuild"} {
		if !kinds[want] {
			t.Fatalf("flight recorder missing timeline kind %s in %#v", want, body.Timeline)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/monitor/auctions/"+auctionID+"/flight-recorder", nil)
	req.Header.Set("X-Mock-Role", "user")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("flight recorder user status = %d, want 403", rec.Code)
	}
}

func openMonitorDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func openMonitorRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6380"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createMonitorAuction(t *testing.T, repo *auction.Repository, db *pgxpool.Pool) auction.Auction {
	t.Helper()
	roomID := "room_monitor_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN')`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	item, err := repo.CreateItem(context.Background(), auction.CreateItemInput{Title: "Monitor Item"})
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
			DepositFloorCents:   10_000,
			DepositCapCents:     100_000_000,
		},
	}, "tr_monitor")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if _, err := repo.Schedule(context.Background(), row.ID, nil, "tr_monitor"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	started, err := repo.Start(context.Background(), row.ID, "tr_monitor")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := repo.PlaceBid(context.Background(), started.ID, "user_1", "monitor-low", auction.BidInput{
		ClientBidID:   "monitor-low",
		AmountCents:   1,
		ClientSeenSeq: started.Seq,
	}, "tr_monitor_reject"); err != nil {
		t.Fatalf("PlaceBid reject path: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO user_activity_events (room_id, auction_id, user_id, event_type, source, payload_json)
		VALUES
		  ($1, $2, 'user_1', 'ws_reconnect', 'ws', '{"last_seq": 1}'),
		  ($1, $2, 'user_1', 'ws_recovered', 'ws', '{"source": "db"}'),
		  ($1, $2, 'user_1', 'ws_slow_consumer_closed', 'ws', '{"phase":"backpressure","reason":"pending_bytes","queue_depth":256,"queue_bytes":65536,"queue_messages_limit":256,"queue_bytes_limit":1048576}')
	`, started.RoomID, started.ID); err != nil {
		t.Fatalf("insert recovery activity: %v", err)
	}
	return started
}

func insertMonitorAnomaly(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('LOW', 'MONITOR_TEST', $1, 'monitor test anomaly', '{"source":"test"}')
	`, auctionID); err != nil {
		t.Fatalf("insert anomaly: %v", err)
	}
}

func forceMonitorSchedulerJob(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		UPDATE scheduler_jobs
		SET status = 'FAILED', attempts = 1, last_error = 'monitor test', run_at = $2, next_attempt_at = $2, updated_at = now()
		WHERE job_type = 'END_AUCTION' AND target_id = $1
	`, auctionID, time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatalf("update scheduler job: %v", err)
	}
}

func seedMonitorDebeziumDiagnostics(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(ctx, `
		INSERT INTO outbox_relay_watermarks (
		  shard_id, owner_id, last_published_outbox_id, last_published_auction_id,
		  last_published_seq, last_published_at, oldest_ready_age_ms,
		  ready_count, publishing_count, dead_count
		)
		VALUES (0, 'monitor-worker', 1, $1, 1, now(), 0, 1, 1, 0)
		ON CONFLICT (shard_id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id,
		    last_published_auction_id = EXCLUDED.last_published_auction_id,
		    ready_count = EXCLUDED.ready_count,
		    publishing_count = EXCLUDED.publishing_count,
		    updated_at = now()
	`, auctionID); err != nil {
		t.Fatalf("seed watermarks: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHING',
		    attempts = 2,
		    max_attempts = 5,
		    locked_by = 'monitor-worker',
		    locked_until = now() + interval '30 seconds',
		    last_error_at = now() - interval '2 seconds'
		WHERE outbox_id = (
		  SELECT d.outbox_id
		  FROM outbox_delivery d
		  JOIN outbox_events e ON e.id = d.outbox_id
		  WHERE e.auction_id = $1
		  ORDER BY d.outbox_id
		  LIMIT 1
		)
	`, auctionID); err != nil {
		t.Fatalf("seed ack pending outbox: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO snapshot_rebuild_events (auction_id, request_id, source, status, stale, duration_ms)
		VALUES ($1, $2, 'db', 'COMPLETED', false, 12)
	`, auctionID, "monitor_snapshot_"+uuid.NewString()); err != nil {
		t.Fatalf("seed snapshot events: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO system_control_signals (signal_type, target_type, target_id, requested_by, reason, status, result_json)
		VALUES ('force_snapshot_rebuild', 'auction', $1, 'host_1', 'monitor seed', 'SUCCEEDED', '{"snapshot_bytes":128}')
	`, auctionID); err != nil {
		t.Fatalf("seed signals: %v", err)
	}
}
