package gateway

import (
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

	router := NewRouter(testConfig(), deps, slog.Default())
	assertMonitorForbiddenForUser(t, router, "/api/monitor/auctions")
	assertMonitorHasItems(t, router, "/api/monitor/auctions", "auction_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/anomalies", "type", "MONITOR_TEST")
	assertMonitorHasItems(t, router, "/api/monitor/outbox", "aggregate_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/scheduler", "target_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/rejects", "auction_id", row.ID)
	assertMonitorHasItems(t, router, "/api/monitor/recovery", "room_id", row.RoomID)
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
		addr = "localhost:6379"
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
		  ($1, $2, 'user_1', 'ws_recovered', 'ws', '{"source": "db"}')
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
		SET status = 'FAILED', attempts = 1, last_error = 'monitor test', next_attempt_at = $2, updated_at = now()
		WHERE job_type = 'END_AUCTION' AND target_id = $1
	`, auctionID, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatalf("update scheduler job: %v", err)
	}
}
