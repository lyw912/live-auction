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

func TestAIListingDraftIsHostOnlyStructuredAndApplyIsAuditOnly(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	router := NewRouter(testConfig(), deps, slog.Default())

	body := bytes.NewBufferString(`{"room_id":"room_main","seller_notes":"清代风格瓷杯 有证书 轻微磕碰","target_category":"collectibles"}`)
	assertAPIStatus(t, router, http.MethodPost, "/api/host/ai/listing-drafts", body, userHeaders("user_1", "user"), http.StatusForbidden)

	body = bytes.NewBufferString(`{"room_id":"room_main","seller_notes":"清代风格瓷杯 有证书 轻微磕碰","target_category":"collectibles"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/host/ai/listing-drafts", body)
	for key, values := range userHeaders("host_1", "host") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("listing draft status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Output struct {
			TitleCandidates []string `json:"title_candidates"`
			RuleSuggestion  struct {
				StartPriceCents int64 `json:"start_price_cents"`
				IncrementCents  int64 `json:"increment_cents"`
				CapPriceCents   int64 `json:"cap_price_cents"`
			} `json:"rule_suggestion"`
		} `json:"output_json"`
		Safety map[string]any `json:"safety_json"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode listing draft: %v", err)
	}
	if payload.ID == "" || payload.Status != "SUCCEEDED" || len(payload.Output.TitleCandidates) == 0 {
		t.Fatalf("bad listing draft payload: %#v", payload)
	}
	if payload.Output.RuleSuggestion.StartPriceCents <= 0 || payload.Output.RuleSuggestion.IncrementCents <= 0 || payload.Output.RuleSuggestion.CapPriceCents <= payload.Output.RuleSuggestion.StartPriceCents {
		t.Fatalf("unsafe rule suggestion: %#v", payload.Output.RuleSuggestion)
	}
	if payload.Safety["no_auto_publish"] != true {
		t.Fatalf("draft missing no_auto_publish safety: %#v", payload.Safety)
	}

	assertAPIStatus(t, router, http.MethodPost, "/api/host/ai/listing-drafts/"+payload.ID+"/apply", nil, userHeaders("host_1", "host"), http.StatusOK)
}

func TestAICommentarySystemMessagesSentinelRecapAndProductQA(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	deps := &storage.Dependencies{Postgres: db, Redis: rdb}
	repo := auction.NewRepository(db)
	row := createMonitorAuction(t, repo, db)
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, 'user_1', 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id) DO UPDATE SET status = 'ACTIVE'
	`, row.RoomID); err != nil {
		t.Fatalf("seed room membership: %v", err)
	}
	router := NewRouter(testConfig(), deps, slog.Default())

	body := bytes.NewBufferString(`{"room_id":"` + row.RoomID + `","auction_id":"` + row.ID + `","source_seq":1,"event_type":"bid_accepted","item_title":"` + row.Item.Title + `","current_price_cents":15000,"active_bidders_30s":1,"accepted_bids_30s":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/host/auctions/"+row.ID+"/commentary", body)
	for key, values := range userHeaders("host_1", "host") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("commentary status = %d body=%s", rec.Code, rec.Body.String())
	}
	var commentary struct {
		Message struct {
			Body      string         `json:"body"`
			SourceSeq *int64         `json:"source_seq"`
			Safety    map[string]any `json:"safety_json"`
		} `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &commentary); err != nil {
		t.Fatalf("decode commentary: %v", err)
	}
	if commentary.Message.Body == "" || commentary.Message.SourceSeq == nil || commentary.Message.Safety["no_hidden_max_bid"] != true {
		t.Fatalf("bad commentary payload: %#v", commentary.Message)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/rooms/"+row.RoomID+"/system-messages", nil)
	for key, values := range userHeaders("user_1", "user") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("system messages status = %d body=%s", rec.Code, rec.Body.String())
	}

	for i := 0; i < 5; i++ {
		bidID := "ai-sentinel-" + uuid.NewString()
		if _, err := repo.PlaceBid(req.Context(), row.ID, "user_1", bidID, auction.BidInput{
			ClientBidID:   bidID,
			AmountCents:   1,
			ClientSeenSeq: row.Seq,
		}, "tr_ai_sentinel"); err != nil {
			t.Fatalf("seed sentinel rejected bid: %v", err)
		}
	}
	req = httptest.NewRequest(http.MethodPost, "/api/host/auctions/"+row.ID+"/sentinel-evaluate", nil)
	for key, values := range userHeaders("host_1", "host") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sentinel status = %d body=%s", rec.Code, rec.Body.String())
	}
	var alerts struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &alerts); err != nil {
		t.Fatalf("decode alerts: %v", err)
	}
	if len(alerts.Items) == 0 {
		t.Fatalf("expected sentinel alert")
	}

	assertAPIStatus(t, router, http.MethodPost, "/api/host/auctions/"+row.ID+"/recap", nil, userHeaders("host_1", "host"), http.StatusOK)
	qaBody := bytes.NewBufferString(`{"auction_id":"` + row.ID + `","question":"起拍价是多少"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/rooms/"+row.RoomID+"/product-qa", qaBody)
	for key, values := range userHeaders("user_1", "user") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("product qa status = %d body=%s", rec.Code, rec.Body.String())
	}
	var answer struct {
		Answer    string   `json:"answer"`
		FactsUsed []string `json:"facts_used"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode qa answer: %v", err)
	}
	if answer.Answer == "" || len(answer.FactsUsed) == 0 {
		t.Fatalf("bad qa answer: %#v", answer)
	}
}
