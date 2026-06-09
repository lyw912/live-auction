package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	aicap "live-auction/backend/internal/ai"
	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/storage"
)

type fakeStructuredGenerator struct {
	results map[string]aicap.StructuredResult
	errs    map[string]error
	delays  map[string]time.Duration
	calls   []aicap.StructuredRequest
}

func (g *fakeStructuredGenerator) GenerateStructured(ctx context.Context, req aicap.StructuredRequest) (aicap.StructuredResult, error) {
	g.calls = append(g.calls, req)
	if delay := g.delays[req.Kind]; delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return aicap.StructuredResult{}, ctx.Err()
		case <-timer.C:
		}
	}
	if err := g.errs[req.Kind]; err != nil {
		return aicap.StructuredResult{}, err
	}
	if result, ok := g.results[req.Kind]; ok {
		return result, nil
	}
	return aicap.DeterministicGenerator{}.GenerateStructured(context.Background(), req)
}

func int64FromAny(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func stringSliceFromAny(value any) []string {
	raw, ok := value.([]string)
	if ok {
		return raw
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
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
	wrongRoomBody := bytes.NewBufferString(`{"room_id":"room_not_owned_by_payload","auction_id":"forged_auction","source_seq":77,"event_type":"rule_guardrail","item_title":"` + row.Item.Title + `","current_price_cents":15000,"active_bidders_30s":0,"accepted_bids_30s":0}`)
	req = httptest.NewRequest(http.MethodPost, "/api/host/auctions/"+row.ID+"/commentary", wrongRoomBody)
	for key, values := range userHeaders("host_1", "host") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("commentary should use auction room over body room, status = %d body=%s", rec.Code, rec.Body.String())
	}
	var normalized struct {
		Message struct {
			RoomID    string `json:"room_id"`
			AuctionID string `json:"auction_id"`
		} `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &normalized); err != nil {
		t.Fatalf("decode normalized commentary: %v", err)
	}
	if normalized.Message.RoomID != row.RoomID || normalized.Message.AuctionID != row.ID {
		t.Fatalf("commentary was not normalized to auction scope: %#v", normalized.Message)
	}
	manualTopics := []struct {
		eventType  string
		wantBody   string
		wantSource string
	}{
		{"product_evidence", "先看证据：证书、实物图和已披露瑕疵都在拍品详情里。喜欢的买家先确认品相，再按自己的预算出价。", "HOST_SCRIPT"},
		{"rule_guardrail", "这场按服务端规则走：起拍、加价、封顶和保证金都已写清，系统只接受有效价格，大额误触会先确认。", "HOST_SCRIPT"},
		{"extended", "末段有人出价会自动延时，这是给所有买家补反应时间，不是主播手动拖场；看清倒计时再决定是否跟。", "HOST_SCRIPT"},
	}
	for index, topic := range manualTopics {
		body := bytes.NewBufferString(`{"room_id":"` + row.RoomID + `","auction_id":"` + row.ID + `","source_seq":` + strconv.Itoa(9001+index) + `,"event_type":"` + topic.eventType + `","item_title":"` + row.Item.Title + `","current_price_cents":15000,"active_bidders_30s":0,"accepted_bids_30s":0}`)
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
			t.Fatalf("manual commentary %s status = %d body=%s", topic.eventType, rec.Code, rec.Body.String())
		}
		var manual struct {
			Message struct {
				Body   string `json:"body"`
				Source string `json:"source"`
			} `json:"message"`
			Job struct {
				Provider string         `json:"provider"`
				Safety   map[string]any `json:"safety_json"`
			} `json:"job"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &manual); err != nil {
			t.Fatalf("decode manual commentary: %v", err)
		}
		if manual.Message.Body != topic.wantBody {
			t.Fatalf("manual commentary %s body = %q, want %q", topic.eventType, manual.Message.Body, topic.wantBody)
		}
		if manual.Message.Source != topic.wantSource || manual.Job.Provider != "host_quick_template" || manual.Job.Safety["quick_template"] != true {
			t.Fatalf("manual commentary %s should use quick template source/job: %#v", topic.eventType, manual)
		}
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
	if providerJob.Output["auction_id"] != row.ID || int64FromAny(providerJob.Output["source_seq"]) != 43 || containsString(stringSliceFromAny(providerJob.Output["facts_used"]), "invented.viewer_count") {
		t.Fatalf("provider commentary facts were not normalized to server facts: %#v", providerJob.Output)
	}
	unsafeGen := &fakeStructuredGenerator{results: map[string]aicap.StructuredResult{
		"auction_commentary": {
			Provider: "test-provider",
			Model:    "commentary-model",
			Output: map[string]any{
				"auction_id":    "forged_auction",
				"source_seq":    float64(999),
				"style":         "heat",
				"body":          "这件一定保真升值，隐藏最高价已接近。",
				"facts_used":    []any{"invented.viewer_count", "hidden_max_bid"},
				"safety_labels": []any{},
			},
			Safety: map[string]any{"provider_mode": "test"},
		},
	}}
	guardedAuto, guardedJob, err := aiRepo.CreateAutoCommentary(context.Background(), unsafeGen, aicap.CommentaryRequest{
		AuctionID:         row.ID,
		SourceSeq:         44,
		EventType:         "bid_accepted",
		CurrentPriceCents: 27000,
	})
	if err != nil {
		t.Fatalf("guarded unsafe provider auto commentary: %v", err)
	}
	if guardedAuto.Body == "这件一定保真升值，隐藏最高价已接近。" || guardedJob.Status != "FAILED" || guardedJob.Provider != "deterministic" || guardedJob.Output["auction_id"] != row.ID || int64FromAny(guardedJob.Output["source_seq"]) != 44 {
		t.Fatalf("unsafe commentary was not guarded: msg=%#v job=%#v", guardedAuto, guardedJob)
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
		if _, err := repo.PlaceBidPostgresLegacyForTests(req.Context(), row.ID, "user_1", bidID, auction.BidInput{
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
	if recapPayload.Recap.AuctionID != row.ID || recapPayload.HighlightAsset.ID == "" || recapPayload.HighlightAsset.MediaType != "video/webm" || !strings.HasPrefix(recapPayload.HighlightAsset.AssetURL, "data:video/webm;base64,") {
		t.Fatalf("bad recap/highlight payload: %#v", recapPayload)
	}
	webmPayload := strings.TrimPrefix(recapPayload.HighlightAsset.AssetURL, "data:video/webm;base64,")
	webmBytes, err := base64.StdEncoding.DecodeString(webmPayload)
	if err != nil {
		t.Fatalf("decode webm asset: %v", err)
	}
	if len(webmBytes) < 4 || webmBytes[0] != 0x1a || webmBytes[1] != 0x45 || webmBytes[2] != 0xdf || webmBytes[3] != 0xa3 {
		t.Fatalf("expected webm EBML header, got %x", webmBytes[:4])
	}
	var storedAssets int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM auction_highlight_assets WHERE auction_id = $1 AND render_profile = 'server-webm-reel-v1' AND media_type = 'video/webm'`, row.ID).Scan(&storedAssets); err != nil {
		t.Fatalf("count highlight assets: %v", err)
	}
	if storedAssets == 0 {
		t.Fatalf("expected persisted highlight asset")
	}
	_, _, reusedAsset, err := aiRepo.BuildAuctionRecap(context.Background(), "host_1", row.ID)
	if err != nil {
		t.Fatalf("reuse recap asset: %v", err)
	}
	if reusedAsset.ID != recapPayload.HighlightAsset.ID || reusedAsset.MediaType != "video/webm" {
		t.Fatalf("expected recent webm asset reuse, got %#v", reusedAsset)
	}
	var storedAssetsAfterReuse int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM auction_highlight_assets WHERE auction_id = $1 AND render_profile = 'server-webm-reel-v1' AND media_type = 'video/webm'`, row.ID).Scan(&storedAssetsAfterReuse); err != nil {
		t.Fatalf("count highlight assets after reuse: %v", err)
	}
	if storedAssetsAfterReuse != storedAssets {
		t.Fatalf("expected highlight reuse without extra render, before=%d after=%d", storedAssets, storedAssetsAfterReuse)
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

func TestAutoCommentaryWorkerQueuePersistsAndBackfillsEvents(t *testing.T) {
	db := openMonitorDB(t)
	repo := auction.NewRepository(db)
	row := createMonitorAuction(t, repo, db)
	aiRepo := aicap.NewRepository(db)
	gen := &fakeStructuredGenerator{results: map[string]aicap.StructuredResult{
		"auction_commentary": {
			Provider: "test-provider",
			Model:    "worker-commentary-model",
			Output: map[string]any{
				"style": "heat",
				"body":  "刚刚有人加价，当前价格已刷新。",
			},
			Safety: map[string]any{"provider_mode": "test"},
		},
	}}
	opts := aicap.AutoCommentaryWorkerOptions{
		WorkerID:  "test-commentary-worker",
		AuctionID: row.ID,
		BatchSize: 4,
		Lease:     5 * time.Second,
	}

	job, inserted, err := aiRepo.EnqueueAutoCommentary(context.Background(), aicap.CommentaryRequest{
		AuctionID:         row.ID,
		SourceSeq:         70,
		EventType:         "bid_accepted",
		CurrentPriceCents: 20_000,
	})
	if err != nil {
		t.Fatalf("enqueue auto commentary: %v", err)
	}
	if !inserted || job.Status != "PENDING" || job.Safety["auto_requested"] != true {
		t.Fatalf("bad queued job: inserted=%v job=%#v", inserted, job)
	}
	_, inserted, err = aiRepo.EnqueueAutoCommentary(context.Background(), aicap.CommentaryRequest{
		AuctionID:         row.ID,
		SourceSeq:         70,
		EventType:         "bid_accepted",
		CurrentPriceCents: 20_000,
	})
	if err != nil {
		t.Fatalf("duplicate enqueue auto commentary: %v", err)
	}
	if inserted {
		t.Fatalf("duplicate auto commentary queue job should dedupe")
	}

	noBackfillOpts := opts
	noBackfillOpts.BackfillLookback = time.Nanosecond
	stats, err := aiRepo.ProcessAutoCommentaryQueue(context.Background(), gen, noBackfillOpts)
	if err != nil {
		t.Fatalf("process auto commentary queue: %v", err)
	}
	if stats.Processed != 1 {
		t.Fatalf("queued commentary not processed: %#v", stats)
	}
	var messages int
	if err := db.QueryRow(context.Background(), `
		SELECT count(*)
		FROM auction_system_messages
		WHERE auction_id = $1 AND source = 'SYSTEM_AI' AND source_seq = 70
	`, row.ID).Scan(&messages); err != nil {
		t.Fatalf("count system messages: %v", err)
	}
	if messages != 1 {
		t.Fatalf("system messages for queued commentary = %d, want 1", messages)
	}

	if _, err := db.Exec(context.Background(), `
		INSERT INTO auction_events (auction_id, seq, event_type, payload_json, server_time_ms, trace_id)
		VALUES ($1, 71, 'auction_sold', '{"state_version":71,"current_price_cents":31000,"user_id":"user_1"}', $2, 'tr_ai_commentary_backfill')
		ON CONFLICT (auction_id, seq) DO NOTHING
	`, row.ID, time.Now().UTC().UnixMilli()); err != nil {
		t.Fatalf("insert auction event for backfill: %v", err)
	}
	backfillOpts := opts
	backfillOpts.BackfillLookback = time.Hour
	stats, err = aiRepo.ProcessAutoCommentaryQueue(context.Background(), gen, backfillOpts)
	if err != nil {
		t.Fatalf("process backfilled commentary queue: %v", err)
	}
	if stats.Enqueued != 1 || stats.Processed != 1 {
		t.Fatalf("missing-event backfill not processed: %#v", stats)
	}
	if err := db.QueryRow(context.Background(), `
		SELECT count(*)
		FROM auction_system_messages
		WHERE auction_id = $1 AND source = 'SYSTEM_AI' AND source_seq IN (70, 71)
	`, row.ID).Scan(&messages); err != nil {
		t.Fatalf("count backfilled system messages: %v", err)
	}
	if messages != 2 {
		t.Fatalf("system messages after backfill = %d, want 2", messages)
	}
	if len(gen.calls) < 2 || gen.calls[len(gen.calls)-1].Input["current_price_cents"] != float64(31_000) {
		t.Fatalf("backfill should use event payload facts, calls=%#v", gen.calls)
	}

	slowGen := &fakeStructuredGenerator{delays: map[string]time.Duration{"auction_commentary": 200 * time.Millisecond}}
	_, inserted, err = aiRepo.EnqueueAutoCommentary(context.Background(), aicap.CommentaryRequest{
		AuctionID:         row.ID,
		SourceSeq:         72,
		EventType:         "bid_accepted",
		CurrentPriceCents: 32_000,
	})
	if err != nil || !inserted {
		t.Fatalf("enqueue slow auto commentary: inserted=%v err=%v", inserted, err)
	}
	timeoutOpts := opts
	timeoutOpts.BatchSize = 1
	timeoutOpts.TaskTimeout = 20 * time.Millisecond
	timeoutOpts.BackfillLookback = time.Nanosecond
	stats, err = aiRepo.ProcessAutoCommentaryQueue(context.Background(), slowGen, timeoutOpts)
	if err != nil {
		t.Fatalf("process timeout commentary queue: %v", err)
	}
	if stats.Failed != 1 || stats.Processed != 0 {
		t.Fatalf("timeout commentary should fail one job without processing: %#v", stats)
	}
	if err := db.QueryRow(context.Background(), `
		SELECT count(*)
		FROM auction_system_messages
		WHERE auction_id = $1 AND source = 'SYSTEM_AI' AND source_seq = 72
	`, row.ID).Scan(&messages); err != nil {
		t.Fatalf("count timeout system messages: %v", err)
	}
	if messages != 0 {
		t.Fatalf("timeout job must not publish a system message, got %d", messages)
	}
}

func TestSentinelAndProductQAUseProviderWithFactGuards(t *testing.T) {
	db := openMonitorDB(t)
	repo := auction.NewRepository(db)
	row := createMonitorAuction(t, repo, db)
	aiRepo := aicap.NewRepository(db)
	for i := 0; i < 5; i++ {
		bidID := "ai-provider-sentinel-" + uuid.NewString()
		if _, err := repo.PlaceBidPostgresLegacyForTests(context.Background(), row.ID, "user_1", bidID, auction.BidInput{
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
