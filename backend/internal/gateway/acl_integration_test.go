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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/storage"
)

func TestRoomMembershipACLRejectsForeignAndBannedViewer(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())
	repo := auction.NewRepository(db)
	owned := createACLAuction(t, repo, db, "room_acl_owned_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")
	foreign := createACLAuction(t, repo, db, "room_acl_foreign_"+uuid.NewString(), "host_1", "user_1", "BANNED")

	assertAPIStatus(t, router, http.MethodGet, "/api/rooms/"+owned.RoomID+"/auctions", nil, userHeaders("user_1", "user"), http.StatusOK)
	assertAPIStatus(t, router, http.MethodGet, "/api/rooms/"+foreign.RoomID+"/auctions", nil, userHeaders("user_1", "user"), http.StatusForbidden)
	assertAuctionListExcludes(t, router, userHeaders("user_1", "user"), foreign.ID)

	bidBody := `{"client_bid_id":"acl-bid-1","amount_cents":15000,"client_seen_seq":0}`
	headers := userHeaders("user_1", "user")
	headers.Set("Idempotency-Key", "acl-bid-1")
	assertAPIStatus(t, router, http.MethodPost, "/api/auctions/"+foreign.ID+"/bids", bytes.NewBufferString(bidBody), headers, http.StatusForbidden)

	ticketBody := `{"room_id":"` + foreign.RoomID + `","auction_id":"` + foreign.ID + `"}`
	assertAPIStatus(t, router, http.MethodPost, "/api/auth/ws-ticket", bytes.NewBufferString(ticketBody), userHeaders("user_1", "user"), http.StatusForbidden)
	assertACLAnomalyRecorded(t, db, foreign.ID, "user_1")
}

func assertAuctionListExcludes(t *testing.T, router http.Handler, headers http.Header, auctionID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/auctions", nil)
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list auctions status = %d body=%s", rec.Code, rec.Body.String())
	}
	var rows []auction.Auction
	if err := json.NewDecoder(rec.Body).Decode(&rows); err != nil {
		t.Fatalf("decode auction list: %v", err)
	}
	for _, row := range rows {
		if row.ID == auctionID {
			t.Fatalf("foreign auction %s leaked in list", auctionID)
		}
	}
}

func TestHostOwnershipACLRejectsForeignHostMutation(t *testing.T) {
	db := openMonitorDB(t)
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	t.Cleanup(func() { _ = rdb.Close() })
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())
	repo := auction.NewRepository(db)
	row := createACLAuction(t, repo, db, "room_acl_host_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")

	if _, err := db.Exec(context.Background(), `
		INSERT INTO users (id, role, display_name) VALUES ('host_acl_foreign', 'host', 'Foreign Host')
		ON CONFLICT DO NOTHING
	`); err != nil {
		t.Fatalf("insert foreign host: %v", err)
	}
	assertAPIStatus(t, router, http.MethodPost, "/api/auctions/"+row.ID+"/cancel", bytes.NewBufferString(`{"reason":"forged host"}`), userHeaders("host_acl_foreign", "host"), http.StatusForbidden)
	assertACLAnomalyRecorded(t, db, row.ID, "host_acl_foreign")
}

func createACLAuction(t *testing.T, repo *auction.Repository, db *pgxpool.Pool, roomID string, hostID string, viewerID string, viewerStatus string) auction.Auction {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO users (id, role, display_name) VALUES ($1, 'host', $1), ($2, 'user', $2)
		ON CONFLICT DO NOTHING
	`, hostID, viewerID); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO rooms (id, host_id, status) VALUES ($1, $2, 'OPEN')
		ON CONFLICT (id) DO UPDATE SET host_id = EXCLUDED.host_id, status = EXCLUDED.status
	`, roomID, hostID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	memberRole := "viewer"
	if viewerStatus == "BANNED" {
		memberRole = "blocked"
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, $2, 'host', 'ACTIVE'), ($1, $3, $4, $5)
		ON CONFLICT (room_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL
	`, roomID, hostID, viewerID, memberRole, viewerStatus); err != nil {
		t.Fatalf("insert memberships: %v", err)
	}
	item, err := repo.CreateItem(context.Background(), auction.CreateItemInput{Title: "ACL Item " + roomID})
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
	}, "tr_acl")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if _, err := repo.Schedule(context.Background(), row.ID, nil, "tr_acl"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	started, err := repo.Start(context.Background(), row.ID, "tr_acl")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return started
}

func userHeaders(userID string, role string) http.Header {
	headers := http.Header{}
	headers.Set("X-Mock-Role", role)
	headers.Set("X-Mock-User-Id", userID)
	headers.Set("Content-Type", "application/json")
	return headers
}

func assertAPIStatus(t *testing.T, router http.Handler, method string, path string, body *bytes.Buffer, headers http.Header, want int) {
	t.Helper()
	var reqBody *bytes.Buffer
	if body == nil {
		reqBody = bytes.NewBuffer(nil)
	} else {
		reqBody = body
	}
	req := httptest.NewRequest(method, path, reqBody)
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s status = %d, want %d body=%s", method, path, rec.Code, want, rec.Body.String())
	}
}

func assertACLAnomalyRecorded(t *testing.T, db *pgxpool.Pool, auctionID string, userID string) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `
		SELECT count(*)
		FROM system_anomaly_events
		WHERE type = 'ACL_FORBIDDEN'
		  AND payload_json->>'user_id' = $1
		  AND ($2 = '' OR auction_id = $2)
	`, userID, auctionID).Scan(&count); err != nil {
		t.Fatalf("query acl anomaly: %v", err)
	}
	if count == 0 {
		t.Fatalf("missing ACL_FORBIDDEN anomaly for user=%s auction=%s", userID, auctionID)
	}
}
