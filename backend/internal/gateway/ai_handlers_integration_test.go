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

	aicap "live-auction/backend/internal/ai"
	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/storage"
)

type fakeStructuredGenerator struct {
	results map[string]aicap.StructuredResult
	errs    map[string]error
	calls   []aicap.StructuredRequest
}

func (g *fakeStructuredGenerator) GenerateStructured(_ context.Context, req aicap.StructuredRequest) (aicap.StructuredResult, error) {
	g.calls = append(g.calls, req)
	if err := g.errs[req.Kind]; err != nil {
		return aicap.StructuredResult{}, err
	}
	if result, ok := g.results[req.Kind]; ok {
		return result, nil
	}
	return aicap.DeterministicGenerator{}.GenerateStructured(context.Background(), req)
}

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
	aiRepo := aicap.NewRepository(db)
	assertAPIStatus(t, router, http.MethodGet, "/api/host/auctions/"+row.ID+"/ai-settings", nil, userHeaders("host_1", "host"), http.StatusOK)
	settingsBody := bytes.NewBufferString(`{"auto_commentary_enabled":false}`)
	assertAPIStatus(t, router, http.MethodPatch, "/api/host/auctions/"+row.ID+"/ai-settings", settingsBody, userHeaders("host_1", "host"), http.StatusOK)
	_, _, err := aiRepo.CreateAutoCommentary(context.Background(), aicap.DeterministicGenerator{}, aicap.CommentaryRequest{
		AuctionID:         row.ID,
		SourceSeq:         41,
		EventType:         "auction_sold",
		CurrentPriceCents: 25000,
	})
	if err == nil {
		t.Fatalf("auto commentary should stop when auction AI settings disable it")
	}
	settingsBody = bytes.NewBufferString(`{"auto_commentary_enabled":true}`)
	assertAPIStatus(t, router, http.MethodPatch, "/api/host/auctions/"+row.ID+"/ai-settings", settingsBody, userHeaders("host_1", "host"), http.StatusOK)
	auto, _, err := aiRepo.CreateAutoCommentary(context.Background(), aicap.DeterministicGenerator{}, aicap.CommentaryRequest{
		AuctionID:         row.ID,
		SourceSeq:         42,
		EventType:         "auction_sold",
		CurrentPriceCents: 25000,
	})
	if err != nil {
		t.Fatalf("auto commentary: %v", err)
	}
	replay, _, err := aiRepo.CreateAutoCommentary(context.Background(), aicap.DeterministicGenerator{}, aicap.CommentaryRequest{
		AuctionID:         row.ID,
		SourceSeq:         42,
		EventType:         "auction_sold",
		CurrentPriceCents: 25000,
	})
	if err != nil {
		t.Fatalf("auto commentary replay: %v", err)
	}
	if auto.ID != replay.ID || auto.Source != "SYSTEM_AI" || auto.Safety["auto_generated"] != true {
		t.Fatalf("auto commentary not idempotent/safe: auto=%#v replay=%#v", auto, replay)
	}
	providerGen := &fakeStructuredGenerator{results: map[string]aicap.StructuredResult{
		"auction_commentary": {
			Provider: "test-provider",
			Model:    "commentary-model",
			Output: map[string]any{
				"auction_id":    row.ID,
				"source_seq":    float64(43),
				"style":         "heat",
				"body":          "成交价已刷新，按系统结果为准。",
				"facts_used":    []any{"auction_id", "source_seq", "current_price_cents"},
				"safety_labels": []any{},
			},
			Safety: map[string]any{"provider_mode": "test"},
		},
	}}
	providerAuto, providerJob, err := aiRepo.CreateAutoCommentary(context.Background(), providerGen, aicap.CommentaryRequest{
		AuctionID:         row.ID,
		SourceSeq:         43,
		EventType:         "auction_sold",
		CurrentPriceCents: 26000,
	})
	if err != nil {
		t.Fatalf("provider auto commentary: %v", err)
	}
	if providerJob.Provider != "test-provider" || providerAuto.Body != "成交价已刷新，按系统结果为准。" || providerAuto.Safety["auto_generated"] != true {
		t.Fatalf("provider auto commentary not used: msg=%#v job=%#v", providerAuto, providerJob)
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
		Job   struct {
			Kind string `json:"kind"`
		} `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &alerts); err != nil {
		t.Fatalf("decode alerts: %v", err)
	}
	if len(alerts.Items) == 0 {
		t.Fatalf("expected sentinel alert")
	}
	if alerts.Job.Kind != "sentinel_explanation" {
		t.Fatalf("expected sentinel job, got %#v", alerts.Job)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/host/auctions/"+row.ID+"/recap", nil)
	for key, values := range userHeaders("host_1", "host") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("recap status = %d body=%s", rec.Code, rec.Body.String())
	}
	var recapPayload struct {
		Recap struct {
			AuctionID string `json:"auction_id"`
		} `json:"recap"`
		HighlightAsset struct {
			ID            string `json:"id"`
			MediaType     string `json:"media_type"`
			AssetURL      string `json:"asset_url"`
			RenderProfile string `json:"render_profile"`
		} `json:"highlight_asset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &recapPayload); err != nil {
		t.Fatalf("decode recap: %v", err)
	}
	if recapPayload.Recap.AuctionID != row.ID || recapPayload.HighlightAsset.ID == "" || recapPayload.HighlightAsset.MediaType != "text/html" || !strings.HasPrefix(recapPayload.HighlightAsset.AssetURL, "data:text/html;base64,") {
		t.Fatalf("bad recap/highlight payload: %#v", recapPayload)
	}
	var storedAssets int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM auction_highlight_assets WHERE auction_id = $1 AND render_profile = 'server-html-reel-v1'`, row.ID).Scan(&storedAssets); err != nil {
		t.Fatalf("count highlight assets: %v", err)
	}
	if storedAssets == 0 {
		t.Fatalf("expected persisted highlight asset")
	}
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
		Answer struct {
			Answer    string   `json:"answer"`
			FactsUsed []string `json:"facts_used"`
		} `json:"answer"`
		Job struct {
			Kind string `json:"kind"`
		} `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode qa answer: %v", err)
	}
	if answer.Answer.Answer == "" || len(answer.Answer.FactsUsed) == 0 || answer.Job.Kind != "product_qa" {
		t.Fatalf("bad qa answer: %#v", answer)
	}
}

func TestSentinelAndProductQAUseProviderWithFactGuards(t *testing.T) {
	db := openMonitorDB(t)
	repo := auction.NewRepository(db)
	row := createMonitorAuction(t, repo, db)
	aiRepo := aicap.NewRepository(db)
	for i := 0; i < 5; i++ {
		bidID := "ai-provider-sentinel-" + uuid.NewString()
		if _, err := repo.PlaceBid(context.Background(), row.ID, "user_1", bidID, auction.BidInput{
			ClientBidID:   bidID,
			AmountCents:   1,
			ClientSeenSeq: row.Seq,
		}, "tr_ai_provider_sentinel"); err != nil {
			t.Fatalf("seed provider sentinel rejected bid: %v", err)
		}
	}
	gen := &fakeStructuredGenerator{results: map[string]aicap.StructuredResult{
		"sentinel_explanation": {
			Provider: "test-provider",
			Model:    "sentinel-model",
			Output: map[string]any{
				"alerts": []any{map[string]any{
					"risk_type":          "bid_rule_probe",
					"severity":           "HIGH",
					"score":              float64(82),
					"explanation":        "连续低于规则的出价被拒，先面向全场说明加价规则。",
					"recommended_action": "提醒规则并观察后续有效出价",
					"facts_used":         []any{"rejected_bids"},
				}},
			},
			Safety: map[string]any{"provider_mode": "test"},
		},
		"product_qa": {
			Provider: "test-provider",
			Model:    "qa-model",
			Output: map[string]any{
				"answer":            "起拍价为 ¥100.00，每次加价 ¥10.00。",
				"facts_used":        []any{"auction.start_price_display", "auction.increment_display"},
				"safety_note":       "只基于本场已审核规则回答。",
				"follow_up_prompts": []any{"有瑕疵说明吗？", "封顶价是多少？"},
			},
			Safety: map[string]any{"provider_mode": "test"},
		},
	}}
	alerts, sentinelJob, err := aiRepo.EvaluateSentinel(context.Background(), "host_1", gen, row.ID)
	if err != nil {
		t.Fatalf("provider sentinel: %v", err)
	}
	if len(alerts) == 0 || alerts[0].Features["provider_reviewed"] != true || alerts[0].Severity != "HIGH" || sentinelJob.Provider != "test-provider" {
		t.Fatalf("provider sentinel not used: alerts=%#v job=%#v", alerts, sentinelJob)
	}
	answer, qaJob, err := aiRepo.AnswerProductQuestion(context.Background(), row.RoomID, gen, aicap.ProductQARequest{AuctionID: row.ID, Question: "起拍价和加价是多少"})
	if err != nil {
		t.Fatalf("provider qa: %v", err)
	}
	if answer.Answer != "起拍价为 ¥100.00，每次加价 ¥10.00。" || qaJob.Provider != "test-provider" || len(answer.FactsUsed) != 2 {
		t.Fatalf("provider qa not used: answer=%#v job=%#v", answer, qaJob)
	}
	gen.results["product_qa"] = aicap.StructuredResult{
		Provider: "test-provider",
		Model:    "qa-model",
		Output: map[string]any{
			"answer":            "封顶价是 ¥500.00。",
			"facts_used":        []any{"auction.cap_price_display"},
			"safety_note":       "只基于本场已审核规则回答。",
			"follow_up_prompts": []any{"有瑕疵说明吗？"},
		},
		Safety: map[string]any{"provider_mode": "test"},
	}
	followUp, followUpJob, err := aiRepo.AnswerProductQuestion(context.Background(), row.RoomID, gen, aicap.ProductQARequest{
		AuctionID: row.ID,
		ThreadID:  "buyer-thread-1",
		Question:  "那封顶呢",
		History: []aicap.ProductQATurn{{
			Question:  "起拍价和加价是多少",
			Answer:    answer.Answer,
			FactsUsed: answer.FactsUsed,
		}},
	})
	if err != nil {
		t.Fatalf("provider qa follow-up: %v", err)
	}
	lastCall := gen.calls[len(gen.calls)-1]
	recentTurns, _ := lastCall.Input["recent_turns"].([]aicap.ProductQATurn)
	if followUp.ThreadID != "buyer-thread-1" || followUp.ContextTurnCount == 0 || len(recentTurns) == 0 || followUpJob.Safety["context_turn_count"] == nil {
		t.Fatalf("qa follow-up did not carry context: answer=%#v job=%#v input=%#v", followUp, followUpJob, lastCall.Input)
	}
	gen.results["product_qa"] = aicap.StructuredResult{
		Provider: "test-provider",
		Model:    "qa-model",
		Output: map[string]any{
			"answer":            "这件一定保真并且未来会升值。",
			"facts_used":        []any{"unapproved.certification"},
			"safety_note":       "unsafe",
			"follow_up_prompts": []any{"能提现收益吗？"},
		},
	}
	guarded, guardedJob, err := aiRepo.AnswerProductQuestion(context.Background(), row.RoomID, gen, aicap.ProductQARequest{AuctionID: row.ID, Question: "能保真升值吗"})
	if err != nil {
		t.Fatalf("guarded qa: %v", err)
	}
	if guarded.Answer == "这件一定保真并且未来会升值。" || guardedJob.Provider != "test-provider" || guardedJob.Output["answer"] == "这件一定保真并且未来会升值。" {
		t.Fatalf("unsafe qa was not guarded: answer=%#v job=%#v", guarded, guardedJob)
	}
}
