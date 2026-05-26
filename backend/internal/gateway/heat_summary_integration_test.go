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

func TestHeatSummaryRequiresHostAndAggregatesRealThirtySecondSignals(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	row := createMonitorAuction(t, repo, db)
	ctx := context.Background()

	acceptedID := "heat-accepted-" + uuid.NewString()
	if _, err := repo.PlaceBid(ctx, row.ID, "user_1", acceptedID, auction.BidInput{
		ClientBidID:   acceptedID,
		AmountCents:   row.CurrentPriceCents + row.IncrementCents,
		ClientSeenSeq: row.Seq,
	}, "tr_heat_accepted"); err != nil {
		t.Fatalf("PlaceBid accepted: %v", err)
	}
	rejectedID := "heat-rejected-" + uuid.NewString()
	if _, err := repo.PlaceBid(ctx, row.ID, "user_2", rejectedID, auction.BidInput{
		ClientBidID:   rejectedID,
		AmountCents:   1,
		ClientSeenSeq: row.Seq,
	}, "tr_heat_rejected"); err != nil {
		t.Fatalf("PlaceBid rejected: %v", err)
	}
	if _, err := repo.CreateChatMessage(ctx, row.RoomID, "user_1", auction.CreateChatInput{
		ClientMsgID: "heat-chat-" + uuid.NewString(),
		Body:        "heat from real chat",
	}, "tr_heat_chat"); err != nil {
		t.Fatalf("CreateChatMessage: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_activity_events (room_id, auction_id, user_id, event_type, source, payload_json)
		VALUES
		  ($1, $2, 'user_1', 'ws_reconnect', 'ws', '{"last_seq": 2}'),
		  ($1, $2, 'user_1', 'ws_recovered', 'ws', '{"source": "db"}')
	`, row.RoomID, row.ID); err != nil {
		t.Fatalf("insert recovery activity: %v", err)
	}

	router := NewRouter(testConfig(), deps, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/host/auctions/"+row.ID+"/heat-summary", nil)
	req.Header.Set("X-Mock-Role", "user")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user heat summary status = %d, want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/host/auctions/"+row.ID+"/heat-summary", nil)
	req.Header.Set("X-Mock-Role", "host")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host heat summary status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		AuctionID             string `json:"auction_id"`
		RoomID                string `json:"room_id"`
		WindowSeconds         int    `json:"window_seconds"`
		ActiveBidders30s      int64  `json:"active_bidders_30s"`
		AcceptedBids30s       int64  `json:"accepted_bids_30s"`
		RejectedBids30s       int64  `json:"rejected_bids_30s"`
		ChatMessages30s       int64  `json:"chat_messages_30s"`
		RecoveryEvents30s     int64  `json:"recovery_events_30s"`
		WatcherCountAvailable bool   `json:"watcher_count_available"`
		Source                string `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode heat summary: %v", err)
	}
	if body.AuctionID != row.ID || body.RoomID != row.RoomID || body.WindowSeconds != 30 {
		t.Fatalf("unexpected summary identity/window: %#v", body)
	}
	if body.ActiveBidders30s < 2 || body.AcceptedBids30s < 1 || body.RejectedBids30s < 1 || body.ChatMessages30s < 1 || body.RecoveryEvents30s < 2 {
		t.Fatalf("summary did not aggregate real recent signals: %#v", body)
	}
	if body.WatcherCountAvailable {
		t.Fatalf("watcher count must remain unavailable until measured: %#v", body)
	}
	if body.Source == "" {
		t.Fatalf("summary missing source: %#v", body)
	}
}
