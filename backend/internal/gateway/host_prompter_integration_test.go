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

func TestHostPrompterPromptsRequireHostAndUseRealAuctionEvents(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	row := createMonitorAuction(t, repo, db)
	ctx := context.Background()

	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET end_at = now() + interval '8 seconds', extend_count = 1
		WHERE id = $1
	`, row.ID); err != nil {
		t.Fatalf("update active auction: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO auction_events (auction_id, seq, event_type, payload_json, server_time_ms, trace_id)
		VALUES ($1, 99, 'auction_extended', '{"old_end_at":"test","new_end_at":"test"}', 1779435630000, 'tr_prompt_extended')
	`, row.ID); err != nil {
		t.Fatalf("insert extension event: %v", err)
	}
	for i := 0; i < 3; i++ {
		clientBidID := "prompt-low-" + uuid.NewString()
		_, err := repo.PlaceBidPostgresLegacyForTests(ctx, row.ID, "user_1", clientBidID, auction.BidInput{
			ClientBidID:   clientBidID,
			AmountCents:   1,
			ClientSeenSeq: row.Seq,
		}, "tr_prompt_reject")
		if err != nil {
			t.Fatalf("PlaceBid reject %d: %v", i, err)
		}
	}

	router := NewRouter(testConfig(), deps, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/host/auctions/"+row.ID+"/prompts", nil)
	req.Header.Set("X-Mock-Role", "user")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("user prompter status = %d, want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/host/auctions/"+row.ID+"/prompts", nil)
	req.Header.Set("X-Mock-Role", "host")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host prompter status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		AuctionID string `json:"auction_id"`
		Prompts   []struct {
			Type                string `json:"type"`
			Severity            string `json:"severity"`
			Action              string `json:"action"`
			Source              string `json:"source"`
			ReferencePriceCents int64  `json:"reference_price_cents"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode prompts: %v", err)
	}
	if body.AuctionID != row.ID {
		t.Fatalf("auction_id = %q, want %q", body.AuctionID, row.ID)
	}
	seen := map[string]bool{}
	for _, prompt := range body.Prompts {
		seen[prompt.Type] = true
		if prompt.Action == "" || prompt.Source == "" || prompt.Severity == "" {
			t.Fatalf("prompt missing action/source/severity: %#v", prompt)
		}
		if prompt.Type == "last_10_seconds" && prompt.ReferencePriceCents == 0 {
			t.Fatalf("last_10_seconds missing next valid bid reference: %#v", prompt)
		}
	}
	for _, want := range []string{"last_10_seconds", "extension_triggered", "high_bid_frequency"} {
		if !seen[want] {
			t.Fatalf("missing prompt %s in %#v", want, body.Prompts)
		}
	}
}

func TestHostPrompterSoldUnpaidPrompt(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	ctx := context.Background()
	roomID := "room_prompt_sold_" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN')`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	item, err := repo.CreateItem(ctx, auction.CreateItemInput{Title: "Prompt Sold Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	capAmount := int64(15_000)
	row, err := repo.CreateAuction(ctx, auction.CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		CapPriceCents:   &capAmount,
		Rule: auction.Rule{
			DurationSeconds:     60,
			ExtendWindowSeconds: 10,
			ExtendBySeconds:     10,
			MaxExtendCount:      3,
			DepositBPS:          1000,
			DepositFloorCents:   1000,
			DepositCapCents:     100_000_000,
		},
	}, "tr_prompt_sold_create")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if _, err := repo.Schedule(ctx, row.ID, nil, "tr_prompt_sold_schedule"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	row, err = repo.Start(ctx, row.ID, "tr_prompt_sold_start")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	bidID := "prompt-sold-" + uuid.NewString()
	resp, err := repo.PlaceBidPostgresLegacyForTests(ctx, row.ID, "user_1", bidID, auction.BidInput{
		ClientBidID: bidID,
		AmountCents: capAmount,
	}, "tr_prompt_sold")
	if err != nil {
		t.Fatalf("place sold bid: %v", err)
	}
	if resp.Result != auction.BidResultAcceptedSold {
		t.Fatalf("sold bid result = %s, want %s", resp.Result, auction.BidResultAcceptedSold)
	}

	router := NewRouter(testConfig(), deps, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/host/auctions/"+row.ID+"/prompts", nil)
	req.Header.Set("X-Mock-Role", "host")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host prompter status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Prompts []struct {
			Type string `json:"type"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode sold prompts: %v", err)
	}
	for _, prompt := range body.Prompts {
		if prompt.Type == "sold_unpaid" {
			return
		}
	}
	t.Fatalf("missing sold_unpaid prompt: %#v", body.Prompts)
}
