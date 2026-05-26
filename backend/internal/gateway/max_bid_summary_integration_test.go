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

	"github.com/google/uuid"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/storage"
)

func TestMaxBidSummaryRequiresHostAndDoesNotExposePrivateAmounts(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	row := createACLAuction(t, repo, db, "room_max_bid_summary_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")
	ctx := context.Background()
	if _, err := db.Exec(ctx, `
		INSERT INTO users (id, role, display_name)
		VALUES ('user_summary_2', 'user', 'Max Bid Summary 2'),
		       ('user_summary_3', 'user', 'Max Bid Summary 3'),
		       ('user_summary_4', 'user', 'Max Bid Summary 4')
		ON CONFLICT DO NOTHING
	`); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES
		  ($1, 'user_summary_2', 'viewer', 'ACTIVE'),
		  ($1, 'user_summary_3', 'viewer', 'ACTIVE'),
		  ($1, 'user_summary_4', 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL
	`, row.RoomID); err != nil {
		t.Fatalf("insert memberships: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO max_bid_intents (id, auction_id, user_id, max_amount_cents, status, source, cancelled_at, exhausted_at, last_applied_seq)
		VALUES
		  ('mbi_summary_active_max', $1, 'user_1', 95000, 'ACTIVE', 'MAX_BID', NULL, NULL, 3),
		  ('mbi_summary_active_pre', $1, 'user_summary_2', 90000, 'ACTIVE', 'PRE_BID', NULL, NULL, NULL),
		  ('mbi_summary_cancelled', $1, 'user_summary_3', 85000, 'CANCELLED', 'MAX_BID', now(), NULL, NULL),
		  ('mbi_summary_exhausted', $1, 'user_summary_4', 80000, 'EXHAUSTED', 'MAX_BID', NULL, now(), 2)
	`, row.ID); err != nil {
		t.Fatalf("insert intents: %v", err)
	}

	router := NewRouter(testConfig(), deps, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/host/auctions/"+row.ID+"/max-bid-summary", nil)
	req.Header.Set("X-Mock-Role", "user")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user max bid summary status = %d, want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/host/auctions/"+row.ID+"/max-bid-summary", nil)
	req.Header.Set("X-Mock-Role", "host")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host max bid summary status = %d body=%s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("max_amount_cents")) || strings.Contains(rec.Body.String(), "95000") || strings.Contains(rec.Body.String(), "90000") {
		t.Fatalf("summary leaked private max amount: %s", rec.Body.String())
	}
	var body struct {
		AuctionID          string `json:"auction_id"`
		RoomID             string `json:"room_id"`
		ActiveIntentCount  int64  `json:"active_intent_count"`
		PreBidCount        int64  `json:"pre_bid_count"`
		MaxBidCount        int64  `json:"max_bid_count"`
		AppliedIntentCount int64  `json:"applied_intent_count"`
		ExhaustedCount     int64  `json:"exhausted_count"`
		CancelledCount     int64  `json:"cancelled_count"`
		HasPrivatePressure bool   `json:"has_private_pressure"`
		Source             string `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode max bid summary: %v", err)
	}
	if body.AuctionID != row.ID || body.RoomID != row.RoomID {
		t.Fatalf("unexpected summary identity: %#v", body)
	}
	if body.ActiveIntentCount != 2 || body.PreBidCount != 1 || body.MaxBidCount != 1 || body.AppliedIntentCount != 2 || body.ExhaustedCount != 1 || body.CancelledCount != 1 || !body.HasPrivatePressure {
		t.Fatalf("unexpected summary counts: %#v", body)
	}
	if body.Source != "postgres:max_bid_intents" {
		t.Fatalf("summary missing source: %#v", body)
	}
}
