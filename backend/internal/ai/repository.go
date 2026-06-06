package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apierrors "live-auction/backend/internal/platform/errors"
)

type Repository struct {
	db *pgxpool.Pool
}

type Job struct {
	ID            string         `json:"id"`
	RoomID        string         `json:"room_id,omitempty"`
	AuctionID     string         `json:"auction_id,omitempty"`
	Kind          string         `json:"kind"`
	Status        string         `json:"status"`
	InputHash     string         `json:"input_hash"`
	PromptVersion string         `json:"prompt_version"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	Input         map[string]any `json:"input_json"`
	Output        map[string]any `json:"output_json"`
	Safety        map[string]any `json:"safety_json"`
	ErrorMessage  string         `json:"error_message,omitempty"`
	ReviewedBy    string         `json:"reviewed_by,omitempty"`
	AppliedAt     *time.Time     `json:"applied_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateListingDraft(ctx context.Context, hostID string, gen Generator, req ListingDraftRequest) (Job, error) {
	req.RoomID = strings.TrimSpace(req.RoomID)
	req.SellerNotes = cleanText(req.SellerNotes, 480)
	req.TargetCategory = cleanText(req.TargetCategory, 80)
	req.ImageURLs = limitStrings(req.ImageURLs, 5, 512)
	req.ImageDataURLs = limitStrings(req.ImageDataURLs, 3, 2_800_000)
	if req.RoomID == "" || (req.SellerNotes == "" && len(req.ImageURLs) == 0 && len(req.ImageDataURLs) == 0) {
		return Job{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id and seller_notes or image_urls are required", http.StatusBadRequest)
	}
	if err := r.ensureHostRoom(ctx, hostID, req.RoomID); err != nil {
		return Job{}, err
	}
	inputMap := structToMap(req)
	storedInputMap := structToMap(req)
	if len(req.ImageDataURLs) > 0 {
		storedInputMap["image_data_urls"] = []string{"local_image_data_url"}
	}
	inputHash := InputHash(map[string]any{
		"kind":           "listing_draft",
		"prompt_version": PromptVersionListingDraft,
		"input":          storedInputMap,
	})
	result, err := gen.GenerateStructured(ctx, StructuredRequest{
		Kind:          "listing_draft",
		PromptVersion: PromptVersionListingDraft,
		SchemaName:    "listing_draft",
		Input:         inputMap,
		Timeout:       8 * time.Second,
	})
	status := "SUCCEEDED"
	errorMessage := ""
	if err != nil {
		if errors.Is(err, ErrDisabled) {
			status = "DISABLED"
			errorMessage = "AI provider disabled"
		} else {
			status = "FAILED"
			errorMessage = cleanText(err.Error(), 240)
		}
		result = StructuredResult{Provider: "none", Model: "none", Output: structToMap(BuildFallbackListingDraft(req)), Safety: map[string]any{
			"fallback":              true,
			"human_review_required": true,
			"no_auto_publish":       true,
		}}
	}
	draft := NormalizeListingDraft(result.Output, req)
	result.Output = structToMap(draft)
	result.Safety = ensureMap(result.Safety)
	result.Safety["human_review_required"] = true
	result.Safety["no_auto_publish"] = true
	return r.insertJob(ctx, Job{
		ID:            "aijob_" + uuid.NewString(),
		RoomID:        req.RoomID,
		Kind:          "listing_draft",
		Status:        status,
		InputHash:     inputHash,
		PromptVersion: PromptVersionListingDraft,
		Provider:      result.Provider,
		Model:         result.Model,
		Input:         storedInputMap,
		Output:        result.Output,
		Safety:        result.Safety,
		ErrorMessage:  errorMessage,
		ReviewedBy:    hostID,
	})
}

func (r *Repository) GetJob(ctx context.Context, hostID string, jobID string) (Job, error) {
	var job Job
	var roomID *string
	var auctionID *string
	var inputRaw, outputRaw, safetyRaw []byte
	var reviewedBy *string
	var errorMessage *string
	err := r.db.QueryRow(ctx, `
		SELECT id, room_id, auction_id, kind, status, input_hash, prompt_version, provider, model,
		       input_json, output_json, safety_json, error_message, reviewed_by, applied_at, created_at, updated_at
		FROM ai_generation_jobs
		WHERE id = $1
	`, jobID).Scan(&job.ID, &roomID, &auctionID, &job.Kind, &job.Status, &job.InputHash, &job.PromptVersion, &job.Provider, &job.Model, &inputRaw, &outputRaw, &safetyRaw, &errorMessage, &reviewedBy, &job.AppliedAt, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, apierrors.New(apierrors.CodeAuctionNotFound, "ai job not found", http.StatusNotFound)
		}
		return Job{}, err
	}
	if roomID != nil {
		job.RoomID = *roomID
		if err := r.ensureHostRoom(ctx, hostID, job.RoomID); err != nil {
			return Job{}, err
		}
	}
	if auctionID != nil {
		job.AuctionID = *auctionID
	}
	if reviewedBy != nil {
		job.ReviewedBy = *reviewedBy
	}
	if errorMessage != nil {
		job.ErrorMessage = *errorMessage
	}
	_ = json.Unmarshal(inputRaw, &job.Input)
	_ = json.Unmarshal(outputRaw, &job.Output)
	_ = json.Unmarshal(safetyRaw, &job.Safety)
	return job, nil
}

func (r *Repository) MarkJobApplied(ctx context.Context, hostID string, jobID string) (Job, error) {
	job, err := r.GetJob(ctx, hostID, jobID)
	if err != nil {
		return Job{}, err
	}
	if job.Kind != "listing_draft" || job.Status != "SUCCEEDED" {
		return Job{}, apierrors.New(apierrors.CodeInvalidArgument, "only succeeded listing drafts can be marked applied", http.StatusBadRequest)
	}
	_, err = r.db.Exec(ctx, `
		UPDATE ai_generation_jobs
		SET applied_at = now(), reviewed_by = $2, updated_at = now()
		WHERE id = $1
	`, jobID, hostID)
	if err != nil {
		return Job{}, err
	}
	return r.GetJob(ctx, hostID, jobID)
}

func (r *Repository) GetAuctionAISettings(ctx context.Context, hostID string, auctionID string) (AuctionAISettings, error) {
	state, err := r.auctionState(ctx, auctionID)
	if err != nil {
		return AuctionAISettings{}, err
	}
	if hostID != "" {
		if err := r.ensureHostRoom(ctx, hostID, state.RoomID); err != nil {
			return AuctionAISettings{}, err
		}
	}
	settings := AuctionAISettings{
		AuctionID:             auctionID,
		AutoCommentaryEnabled: true,
	}
	var updatedBy *string
	err = r.db.QueryRow(ctx, `
		SELECT auto_commentary_enabled, updated_by, updated_at
		FROM auction_ai_settings
		WHERE auction_id = $1
	`, auctionID).Scan(&settings.AutoCommentaryEnabled, &updatedBy, &settings.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			settings.UpdatedAt = time.Now().UTC()
			return settings, nil
		}
		return AuctionAISettings{}, err
	}
	if updatedBy != nil {
		settings.UpdatedBy = *updatedBy
	}
	return settings, nil
}

func (r *Repository) UpdateAuctionAISettings(ctx context.Context, hostID string, auctionID string, autoCommentaryEnabled bool) (AuctionAISettings, error) {
	state, err := r.auctionState(ctx, auctionID)
	if err != nil {
		return AuctionAISettings{}, err
	}
	if err := r.ensureHostRoom(ctx, hostID, state.RoomID); err != nil {
		return AuctionAISettings{}, err
	}
	settings := AuctionAISettings{AuctionID: auctionID}
	var updatedBy *string
	err = r.db.QueryRow(ctx, `
		INSERT INTO auction_ai_settings (auction_id, auto_commentary_enabled, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (auction_id)
		DO UPDATE SET auto_commentary_enabled = EXCLUDED.auto_commentary_enabled,
		              updated_by = EXCLUDED.updated_by,
		              updated_at = now()
		RETURNING auto_commentary_enabled, updated_by, updated_at
	`, auctionID, autoCommentaryEnabled, hostID).Scan(&settings.AutoCommentaryEnabled, &updatedBy, &settings.UpdatedAt)
	if err != nil {
		return AuctionAISettings{}, err
	}
	if updatedBy != nil {
		settings.UpdatedBy = *updatedBy
	}
	return settings, nil
}

func (r *Repository) CreateCommentary(ctx context.Context, hostID string, gen Generator, req CommentaryRequest) (SystemMessage, Job, error) {
	if req.AuctionID == "" || req.RoomID == "" {
		return SystemMessage{}, Job{}, apierrors.New(apierrors.CodeInvalidArgument, "auction_id and room_id are required", http.StatusBadRequest)
	}
	if err := r.ensureHostRoom(ctx, hostID, req.RoomID); err != nil {
		return SystemMessage{}, Job{}, err
	}
	inputMap := structToMap(req)
	result, err := gen.GenerateStructured(ctx, StructuredRequest{
		Kind:          "auction_commentary",
		PromptVersion: PromptVersionCommentary,
		SchemaName:    "auction_commentary",
		Input:         inputMap,
		Timeout:       3 * time.Second,
	})
	status := "SUCCEEDED"
	if err != nil {
		status = "FAILED"
		body, style, safety := BuildCommentary(req)
		result = StructuredResult{
			Provider: "deterministic",
			Model:    "fallback-template",
			Output: map[string]any{
				"auction_id": req.AuctionID,
				"source_seq": req.SourceSeq,
				"style":      style,
				"body":       body,
			},
			Safety: safety,
		}
	}
	body := cleanText(stringValue(result.Output["body"]), 40)
	style := cleanText(firstNonEmpty(stringValue(result.Output["style"]), "steady"), 20)
	if body == "" {
		body, style, result.Safety = BuildCommentary(req)
	}
	result.Safety = ensureMap(result.Safety)
	msg, err := r.InsertSystemMessage(ctx, req.RoomID, req.AuctionID, "SYSTEM_AI", req.SourceSeq, style, body, inputMap, result.Safety)
	if err != nil {
		return SystemMessage{}, Job{}, err
	}
	job, err := r.insertJob(ctx, Job{
		ID:            "aijob_" + uuid.NewString(),
		RoomID:        req.RoomID,
		AuctionID:     req.AuctionID,
		Kind:          "auction_commentary",
		Status:        status,
		InputHash:     InputHash(inputMap),
		PromptVersion: PromptVersionCommentary,
		Provider:      result.Provider,
		Model:         result.Model,
		Input:         inputMap,
		Output:        result.Output,
		Safety:        result.Safety,
		ReviewedBy:    hostID,
	})
	return msg, job, err
}

func (r *Repository) CreateAutoCommentary(ctx context.Context, gen Generator, req CommentaryRequest) (SystemMessage, Job, error) {
	if req.AuctionID == "" {
		return SystemMessage{}, Job{}, apierrors.New(apierrors.CodeInvalidArgument, "auction_id is required", http.StatusBadRequest)
	}
	state, err := r.auctionState(ctx, req.AuctionID)
	if err != nil {
		return SystemMessage{}, Job{}, err
	}
	req.RoomID = firstNonEmpty(req.RoomID, state.RoomID)
	req.ItemTitle = firstNonEmpty(req.ItemTitle, state.ItemTitle)
	settings, err := r.GetAuctionAISettings(ctx, "", req.AuctionID)
	if err != nil {
		return SystemMessage{}, Job{}, err
	}
	if !settings.AutoCommentaryEnabled {
		return SystemMessage{}, Job{}, apierrors.New(apierrors.CodeInvalidArgument, "auto commentary disabled for auction", http.StatusConflict)
	}
	if req.CurrentPriceCents <= 0 {
		req.CurrentPriceCents = state.CurrentPriceCents
	}
	if req.CurrentWinnerMasked == "" {
		req.CurrentWinnerMasked = maskUserID(state.CurrentWinnerID)
	}
	if req.SourceSeq <= 0 {
		req.SourceSeq = time.Now().UnixMilli()
	}
	inputMap := structToMap(req)
	result, err := gen.GenerateStructured(ctx, StructuredRequest{
		Kind:          "auction_commentary",
		PromptVersion: PromptVersionCommentary,
		SchemaName:    "auction_commentary",
		Input:         inputMap,
		Timeout:       6 * time.Second,
	})
	status := "SUCCEEDED"
	if err != nil {
		status = "FAILED"
		body, style, safety := BuildCommentary(req)
		result = StructuredResult{
			Provider: "deterministic",
			Model:    "fallback-template",
			Output: map[string]any{
				"auction_id":  req.AuctionID,
				"source_seq":  req.SourceSeq,
				"style":       style,
				"body":        body,
				"auto_source": true,
			},
			Safety: safety,
		}
	}
	body := cleanText(stringValue(result.Output["body"]), 40)
	style := cleanText(firstNonEmpty(stringValue(result.Output["style"]), "steady"), 20)
	if body == "" {
		body, style, result.Safety = BuildCommentary(req)
	}
	result.Safety = ensureMap(result.Safety)
	result.Safety["auto_generated"] = true
	msg, err := r.InsertSystemMessage(ctx, req.RoomID, req.AuctionID, "SYSTEM_AI", req.SourceSeq, style, body, inputMap, result.Safety)
	if err != nil {
		return SystemMessage{}, Job{}, err
	}
	job, err := r.insertJob(ctx, Job{
		ID:            "aijob_" + uuid.NewString(),
		RoomID:        req.RoomID,
		AuctionID:     req.AuctionID,
		Kind:          "auction_commentary",
		Status:        status,
		InputHash:     InputHash(inputMap),
		PromptVersion: PromptVersionCommentary,
		Provider:      result.Provider,
		Model:         result.Model,
		Input:         inputMap,
		Output:        result.Output,
		Safety:        result.Safety,
	})
	return msg, job, err
}

func (r *Repository) InsertSystemMessage(ctx context.Context, roomID string, auctionID string, source string, sourceSeq int64, style string, body string, facts map[string]any, safety map[string]any) (SystemMessage, error) {
	factsRaw, _ := json.Marshal(facts)
	safetyRaw, _ := json.Marshal(safety)
	var msg SystemMessage
	var auctionIDPtr *string
	var seqPtr *int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO auction_system_messages (room_id, auction_id, source, source_seq, style, body, facts_json, safety_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (auction_id, source, source_seq)
		DO UPDATE SET body = auction_system_messages.body
		RETURNING id, room_id, auction_id, source, source_seq, style, body, facts_json, safety_json, created_at
	`, roomID, nullableString(auctionID), source, nullableSeq(sourceSeq), style, body, factsRaw, safetyRaw).
		Scan(&msg.ID, &msg.RoomID, &auctionIDPtr, &msg.Source, &seqPtr, &msg.Style, &msg.Body, &factsRaw, &safetyRaw, &msg.CreatedAt)
	if err != nil {
		return SystemMessage{}, err
	}
	if auctionIDPtr != nil {
		msg.AuctionID = *auctionIDPtr
	}
	msg.SourceSeq = seqPtr
	_ = json.Unmarshal(factsRaw, &msg.Facts)
	_ = json.Unmarshal(safetyRaw, &msg.Safety)
	return msg, nil
}

func (r *Repository) ListSystemMessages(ctx context.Context, roomID string, limit int) ([]SystemMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, room_id, auction_id, source, source_seq, style, body, facts_json, safety_json, created_at
		FROM auction_system_messages
		WHERE room_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, roomID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SystemMessage{}
	for rows.Next() {
		var msg SystemMessage
		var auctionID *string
		var seq *int64
		var factsRaw, safetyRaw []byte
		if err := rows.Scan(&msg.ID, &msg.RoomID, &auctionID, &msg.Source, &seq, &msg.Style, &msg.Body, &factsRaw, &safetyRaw, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if auctionID != nil {
			msg.AuctionID = *auctionID
		}
		msg.SourceSeq = seq
		_ = json.Unmarshal(factsRaw, &msg.Facts)
		_ = json.Unmarshal(safetyRaw, &msg.Safety)
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (r *Repository) EvaluateSentinel(ctx context.Context, hostID string, gen Generator, auctionID string) ([]SentinelAlert, Job, error) {
	state, err := r.auctionState(ctx, auctionID)
	if err != nil {
		return nil, Job{}, err
	}
	if err := r.ensureHostRoom(ctx, hostID, state.RoomID); err != nil {
		return nil, Job{}, err
	}
	var features struct {
		AcceptedBids     int64
		AcceptedBidders  int64
		RejectedBids     int64
		TopBidderBids    int64
		TopBidderAmount  int64
		OrderPendingMins float64
	}
	err = r.db.QueryRow(ctx, `
		WITH bid_counts AS (
		  SELECT
		    count(*) FILTER (WHERE status = 'ACCEPTED') AS accepted_bids,
		    count(DISTINCT user_id) FILTER (WHERE status = 'ACCEPTED') AS accepted_bidders,
		    count(*) FILTER (WHERE status = 'REJECTED') AS rejected_bids
		  FROM bids
		  WHERE auction_id = $1
		),
		top_bidder AS (
		  SELECT user_id, count(*) AS bid_count, max(amount_cents) AS amount_cents
		  FROM bids
		  WHERE auction_id = $1 AND status = 'ACCEPTED'
		  GROUP BY user_id
		  ORDER BY bid_count DESC, amount_cents DESC
		  LIMIT 1
		),
		pending_order AS (
		  SELECT COALESCE(max(extract(epoch from now() - created_at) / 60), 0) AS pending_mins
		  FROM orders
		  WHERE auction_id = $1 AND status = 'ORDER_PENDING'
		)
		SELECT coalesce(b.accepted_bids,0), coalesce(b.accepted_bidders,0), coalesce(b.rejected_bids,0),
		       coalesce(t.bid_count,0), coalesce(t.amount_cents,0), coalesce(p.pending_mins,0)
		FROM bid_counts b
		CROSS JOIN pending_order p
		LEFT JOIN top_bidder t ON true
	`, auctionID).Scan(&features.AcceptedBids, &features.AcceptedBidders, &features.RejectedBids, &features.TopBidderBids, &features.TopBidderAmount, &features.OrderPendingMins)
	if err != nil {
		return nil, Job{}, err
	}
	alerts := []SentinelAlert{}
	if features.AcceptedBids >= 4 && features.AcceptedBidders <= 1 {
		alerts = append(alerts, SentinelAlert{
			RoomID:            state.RoomID,
			AuctionID:         auctionID,
			Severity:          "MED",
			RiskType:          "single_bidder_price_push",
			Score:             66,
			Explanation:       "多次有效出价集中在同一买家，建议主播核验证据和支付意愿。",
			RecommendedAction: "观察并准备暂停或取消异常竞拍",
			Features: map[string]any{
				"accepted_bids":    features.AcceptedBids,
				"accepted_bidders": features.AcceptedBidders,
				"top_bidder_bids":  features.TopBidderBids,
			},
		})
	}
	if features.RejectedBids >= 5 {
		alerts = append(alerts, SentinelAlert{
			RoomID:            state.RoomID,
			AuctionID:         auctionID,
			Severity:          "MED",
			RiskType:          "bid_rule_probe",
			Score:             58,
			Explanation:       "短时间内出现多次非法出价，可能是误操作或规则探测。",
			RecommendedAction: "口播加价规则，必要时查看拒绝明细",
			Features: map[string]any{
				"rejected_bids": features.RejectedBids,
			},
		})
	}
	if features.OrderPendingMins >= 10 {
		alerts = append(alerts, SentinelAlert{
			RoomID:            state.RoomID,
			AuctionID:         auctionID,
			Severity:          "HIGH",
			RiskType:          "sold_unpaid_pressure",
			Score:             78,
			Explanation:       "高价成交后长时间未支付，需防止恶意抬价后放弃支付。",
			RecommendedAction: "提醒支付倒计时并准备订单过期后的复盘",
			Features: map[string]any{
				"order_pending_minutes": features.OrderPendingMins,
			},
		})
	}
	input := SentinelEvaluationInput{
		RoomID:    state.RoomID,
		AuctionID: auctionID,
		ItemTitle: state.ItemTitle,
		Status:    state.Status,
		Features: map[string]any{
			"accepted_bids":         features.AcceptedBids,
			"accepted_bidders":      features.AcceptedBidders,
			"rejected_bids":         features.RejectedBids,
			"top_bidder_bids":       features.TopBidderBids,
			"top_bidder_amount":     features.TopBidderAmount,
			"order_pending_minutes": features.OrderPendingMins,
		},
		Candidates: alerts,
	}
	inputMap := structToMap(input)
	result, err := gen.GenerateStructured(ctx, StructuredRequest{
		Kind:          "sentinel_explanation",
		PromptVersion: PromptVersionSentinel,
		SchemaName:    "sentinel_explanation",
		Input:         inputMap,
		Timeout:       8 * time.Second,
	})
	status := "SUCCEEDED"
	errorMessage := ""
	if err != nil {
		status = "FAILED"
		errorMessage = cleanText(err.Error(), 240)
		result = StructuredResult{
			Provider: "deterministic",
			Model:    "fallback-template",
			Output: map[string]any{
				"alerts": structToAnySlice(alerts),
			},
			Safety: map[string]any{
				"fallback":             true,
				"aggregate_facts_only": true,
				"no_auto_block":        true,
			},
		}
	}
	alerts = NormalizeSentinelAlerts(result.Output, input)
	result.Output = map[string]any{"alerts": structToAnySlice(alerts)}
	result.Safety = ensureMap(result.Safety)
	result.Safety["aggregate_facts_only"] = true
	result.Safety["no_auto_block"] = true
	job, err := r.insertJob(ctx, Job{
		ID:            "aijob_" + uuid.NewString(),
		RoomID:        state.RoomID,
		AuctionID:     auctionID,
		Kind:          "sentinel_explanation",
		Status:        status,
		InputHash:     InputHash(inputMap),
		PromptVersion: PromptVersionSentinel,
		Provider:      result.Provider,
		Model:         result.Model,
		Input:         inputMap,
		Output:        result.Output,
		Safety:        result.Safety,
		ErrorMessage:  errorMessage,
		ReviewedBy:    hostID,
	})
	if err != nil {
		return nil, Job{}, err
	}
	for i := range alerts {
		alerts[i].Features = ensureMap(alerts[i].Features)
		alerts[i].Features["ai_job_id"] = job.ID
		inserted, err := r.insertRiskAlert(ctx, alerts[i])
		if err != nil {
			return nil, Job{}, err
		}
		alerts[i] = inserted
	}
	return alerts, job, nil
}

func (r *Repository) ListRiskAlerts(ctx context.Context, hostID string, auctionID string) ([]SentinelAlert, error) {
	state, err := r.auctionState(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	if err := r.ensureHostRoom(ctx, hostID, state.RoomID); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, room_id, auction_id, severity, risk_type, score, explanation, recommended_action, features_json, status, created_at, updated_at
		FROM auction_risk_alerts
		WHERE auction_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 20
	`, auctionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SentinelAlert{}
	for rows.Next() {
		alert, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, alert)
	}
	return out, rows.Err()
}

func (r *Repository) BuildAuctionRecap(ctx context.Context, hostID string, auctionID string) (AuctionRecap, Job, HighlightAsset, error) {
	state, err := r.auctionState(ctx, auctionID)
	if err != nil {
		return AuctionRecap{}, Job{}, HighlightAsset{}, err
	}
	if err := r.ensureHostRoom(ctx, hostID, state.RoomID); err != nil {
		return AuctionRecap{}, Job{}, HighlightAsset{}, err
	}
	var acceptedBidders int64
	if err := r.db.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM bids WHERE auction_id = $1 AND status = 'ACCEPTED'`, auctionID).Scan(&acceptedBidders); err != nil {
		return AuctionRecap{}, Job{}, HighlightAsset{}, err
	}
	recap := BuildRecap(AuctionRecap{
		AuctionID:       auctionID,
		RoomID:          state.RoomID,
		ItemTitle:       state.ItemTitle,
		Status:          state.Status,
		FinalPriceCents: state.CurrentPriceCents,
		WinnerMasked:    maskUserID(state.CurrentWinnerID),
		AcceptedBids:    state.AcceptedBidCount,
		AcceptedBidders: acceptedBidders,
		ExtendCount:     state.ExtendCount,
	})
	input := structToMap(map[string]any{"auction_id": auctionID, "kind": "auction_recap"})
	job, err := r.insertJob(ctx, Job{
		ID:            "aijob_" + uuid.NewString(),
		RoomID:        state.RoomID,
		AuctionID:     auctionID,
		Kind:          "auction_recap",
		Status:        "SUCCEEDED",
		InputHash:     InputHash(input),
		PromptVersion: PromptVersionRecap,
		Provider:      "deterministic",
		Model:         "local-template",
		Input:         input,
		Output:        structToMap(recap),
		Safety: map[string]any{
			"buyer_identities_masked": true,
			"no_capacity_claim":       true,
		},
		ReviewedBy: hostID,
	})
	if err != nil {
		return AuctionRecap{}, Job{}, HighlightAsset{}, err
	}
	asset, err := r.createHighlightAsset(ctx, recap, job)
	return recap, job, asset, err
}

func (r *Repository) createHighlightAsset(ctx context.Context, recap AuctionRecap, job Job) (HighlightAsset, error) {
	facts := map[string]any{
		"auction_id":        recap.AuctionID,
		"item_title":        recap.ItemTitle,
		"status":            recap.Status,
		"final_price_cents": recap.FinalPriceCents,
		"winner_masked":     recap.WinnerMasked,
		"accepted_bids":     recap.AcceptedBids,
		"accepted_bidders":  recap.AcceptedBidders,
		"extend_count":      recap.ExtendCount,
		"highlights":        recap.Highlights,
	}
	risk := map[string]any{
		"async_pipeline":             true,
		"does_not_block_bid_path":    true,
		"buyer_identities_masked":    true,
		"internal_demo_html_profile": true,
		"replacement_ready":          "asset_url can be replaced by MinIO mp4/webm from a worker",
	}
	title := cleanText(recap.ItemTitle+" 高光复盘", 80)
	htmlAsset := renderHighlightHTML(recap)
	asset := HighlightAsset{
		ID:            "hl_" + uuid.NewString(),
		AuctionID:     recap.AuctionID,
		RoomID:        recap.RoomID,
		JobID:         job.ID,
		Status:        "RENDERED",
		MediaType:     "text/html",
		Title:         title,
		AssetURL:      "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(htmlAsset)),
		RenderProfile: "server-html-reel-v1",
		DurationMS:    12_000,
		Facts:         facts,
		Risk:          risk,
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO auction_highlight_assets (
			id, auction_id, room_id, job_id, status, media_type, title,
			asset_url, render_profile, duration_ms, facts_json, risk_json
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at
	`, asset.ID, asset.AuctionID, asset.RoomID, asset.JobID, asset.Status, asset.MediaType, asset.Title,
		asset.AssetURL, asset.RenderProfile, asset.DurationMS, asset.Facts, asset.Risk).
		Scan(&asset.CreatedAt, &asset.UpdatedAt)
	return asset, err
}

func renderHighlightHTML(recap AuctionRecap) string {
	lines := []string{}
	for _, item := range recap.Highlights {
		lines = append(lines, "<li>"+html.EscapeString(item)+"</li>")
	}
	nextActions := []string{}
	for _, item := range recap.NextActions {
		nextActions = append(nextActions, "<span>"+html.EscapeString(item)+"</span>")
	}
	return `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(recap.ItemTitle) + ` 高光复盘</title>
<style>
body{margin:0;background:#101820;color:#f8fafc;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
.reel{min-height:100vh;display:grid;place-items:center;padding:32px;background:radial-gradient(circle at 20% 15%,#e6b85c55,transparent 28%),linear-gradient(135deg,#101820,#27333f)}
.panel{width:min(760px,92vw);border:1px solid #ffffff24;border-radius:18px;padding:30px;background:#111827cc;box-shadow:0 28px 80px #0008}
h1{font-size:clamp(32px,6vw,64px);margin:0 0 10px;letter-spacing:0}.price{font-size:clamp(40px,8vw,82px);font-weight:900;color:#ffd166;margin:10px 0}
.meta{display:flex;gap:10px;flex-wrap:wrap}.meta span,.actions span{border:1px solid #ffffff2b;border-radius:999px;padding:8px 12px;background:#ffffff12}
li{margin:10px 0;font-size:20px}.actions{display:flex;gap:10px;flex-wrap:wrap;margin-top:22px}
</style><body><main class="reel"><section class="panel">
<h1>` + html.EscapeString(recap.ItemTitle) + `</h1>
<div class="price">` + html.EscapeString(FormatCents(recap.FinalPriceCents)) + `</div>
<div class="meta"><span>` + html.EscapeString(recap.Status) + `</span><span>` + html.EscapeString(recap.WinnerMasked) + `</span><span>` + html.EscapeString(time.Now().Format("15:04:05")) + `</span></div>
<ul>` + strings.Join(lines, "") + `</ul>
<div class="actions">` + strings.Join(nextActions, "") + `</div>
</section></main></body></html>`
}

func (r *Repository) AnswerProductQuestion(ctx context.Context, roomID string, gen Generator, req ProductQARequest) (ProductQAAnswer, Job, error) {
	req.ThreadID = cleanText(req.ThreadID, 80)
	req.Question = cleanText(req.Question, 120)
	if req.AuctionID == "" || req.Question == "" {
		return ProductQAAnswer{}, Job{}, apierrors.New(apierrors.CodeInvalidArgument, "auction_id and question are required", http.StatusBadRequest)
	}
	state, err := r.auctionState(ctx, req.AuctionID)
	if err != nil {
		return ProductQAAnswer{}, Job{}, err
	}
	if roomID != "" && state.RoomID != roomID {
		return ProductQAAnswer{}, Job{}, apierrors.New(apierrors.CodeForbiddenRoom, "auction does not belong to room", http.StatusForbidden)
	}
	rules := map[string]any{
		"start_price_cents": state.StartPriceCents,
		"increment_cents":   state.IncrementCents,
		"cap_price_cents":   state.CapPriceCents,
	}
	facts := map[string]any{
		"item.title":                  state.ItemTitle,
		"auction.start_price_cents":   state.StartPriceCents,
		"auction.increment_cents":     state.IncrementCents,
		"auction.cap_price_cents":     state.CapPriceCents,
		"auction.start_price_display": FormatCents(state.StartPriceCents),
		"auction.increment_display":   FormatCents(state.IncrementCents),
	}
	if strings.TrimSpace(state.ItemDescription) != "" {
		facts["item.description"] = state.ItemDescription
	}
	if state.CapPriceCents > 0 {
		facts["auction.cap_price_display"] = FormatCents(state.CapPriceCents)
	}
	allowedFacts := map[string]string{}
	for key := range facts {
		allowedFacts[key] = key
	}
	contextTurns, err := r.productQAContext(ctx, state.RoomID, req.AuctionID, req.ThreadID, req.History)
	if err != nil {
		return ProductQAAnswer{}, Job{}, err
	}
	fallback := AnswerFromFacts(req.AuctionID, req.Question, state.ItemTitle, state.ItemDescription, rules)
	fallback.ThreadID = req.ThreadID
	fallback.ContextTurnCount = len(contextTurns)
	inputMap := map[string]any{
		"room_id":      state.RoomID,
		"auction_id":   req.AuctionID,
		"thread_id":    req.ThreadID,
		"question":     req.Question,
		"recent_turns": contextTurns,
		"facts":        facts,
	}
	result, err := gen.GenerateStructured(ctx, StructuredRequest{
		Kind:          "product_qa",
		PromptVersion: PromptVersionProductQA,
		SchemaName:    "product_qa",
		Input:         inputMap,
		Timeout:       12 * time.Second,
	})
	status := "SUCCEEDED"
	errorMessage := ""
	if err != nil {
		status = "FAILED"
		errorMessage = cleanText(err.Error(), 240)
		result = StructuredResult{
			Provider: "deterministic",
			Model:    "fallback-template",
			Output:   structToMap(fallback),
			Safety: map[string]any{
				"fallback":               true,
				"approved_facts_only":    true,
				"no_private_bid_data":    true,
				"no_authenticity_claims": true,
			},
		}
	}
	answer := NormalizeProductQAAnswer(result.Output, fallback, allowedFacts)
	result.Output = structToMap(answer)
	result.Safety = ensureMap(result.Safety)
	result.Safety["approved_facts_only"] = true
	result.Safety["no_private_bid_data"] = true
	result.Safety["no_authenticity_claims"] = true
	result.Safety["context_turn_count"] = len(contextTurns)
	job, err := r.insertJob(ctx, Job{
		ID:            "aijob_" + uuid.NewString(),
		RoomID:        state.RoomID,
		AuctionID:     req.AuctionID,
		Kind:          "product_qa",
		Status:        status,
		InputHash:     InputHash(inputMap),
		PromptVersion: PromptVersionProductQA,
		Provider:      result.Provider,
		Model:         result.Model,
		Input:         inputMap,
		Output:        result.Output,
		Safety:        result.Safety,
		ErrorMessage:  errorMessage,
	})
	if err != nil {
		return ProductQAAnswer{}, Job{}, err
	}
	return answer, job, nil
}

func (r *Repository) productQAContext(ctx context.Context, roomID string, auctionID string, threadID string, clientHistory []ProductQATurn) ([]ProductQATurn, error) {
	turns := normalizeProductQATurns(clientHistory)
	if len(turns) >= 4 {
		return turns, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT input_json, output_json
		FROM ai_generation_jobs
		WHERE room_id = $1
		  AND auction_id = $2
		  AND kind = 'product_qa'
		  AND status = 'SUCCEEDED'
		  AND ($3 = '' OR input_json->>'thread_id' = $3)
		ORDER BY created_at DESC
		LIMIT 4
	`, roomID, auctionID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	persisted := []ProductQATurn{}
	for rows.Next() {
		var inputRaw, outputRaw []byte
		if err := rows.Scan(&inputRaw, &outputRaw); err != nil {
			return nil, err
		}
		var inputMap map[string]any
		var outputMap map[string]any
		_ = json.Unmarshal(inputRaw, &inputMap)
		_ = json.Unmarshal(outputRaw, &outputMap)
		turn := ProductQATurn{
			Question:  cleanText(stringValue(inputMap["question"]), 120),
			Answer:    cleanText(stringValue(outputMap["answer"]), 180),
			FactsUsed: limitStrings(stringSlice(outputMap["facts_used"]), 8, 64),
		}
		if turn.Question != "" && turn.Answer != "" {
			persisted = append(persisted, turn)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := len(persisted) - 1; i >= 0 && len(turns) < 4; i-- {
		turns = append(turns, persisted[i])
	}
	return normalizeProductQATurns(turns), nil
}

func normalizeProductQATurns(turns []ProductQATurn) []ProductQATurn {
	out := []ProductQATurn{}
	seen := map[string]bool{}
	start := 0
	if len(turns) > 4 {
		start = len(turns) - 4
	}
	for _, turn := range turns[start:] {
		question := cleanText(turn.Question, 120)
		answer := cleanText(turn.Answer, 180)
		if question == "" || answer == "" {
			continue
		}
		key := question + "\n" + answer
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ProductQATurn{
			Question:  question,
			Answer:    answer,
			FactsUsed: limitStrings(turn.FactsUsed, 8, 64),
		})
	}
	return out
}

type auctionState struct {
	RoomID            string
	ItemTitle         string
	ItemDescription   string
	Status            string
	CurrentPriceCents int64
	CurrentWinnerID   string
	StartPriceCents   int64
	IncrementCents    int64
	CapPriceCents     int64
	AcceptedBidCount  int64
	ExtendCount       int
}

func (r *Repository) auctionState(ctx context.Context, auctionID string) (auctionState, error) {
	var state auctionState
	var winner *string
	var description *string
	var capValue *int64
	err := r.db.QueryRow(ctx, `
		SELECT a.room_id, i.title, i.description, a.status, a.current_price_cents, a.current_winner_id,
		       a.start_price_cents, a.increment_cents, a.cap_price_cents, a.accepted_bid_count, a.extend_count
		FROM auctions a
		JOIN items i ON i.id = a.item_id
		WHERE a.id = $1
	`, auctionID).Scan(&state.RoomID, &state.ItemTitle, &description, &state.Status, &state.CurrentPriceCents, &winner, &state.StartPriceCents, &state.IncrementCents, &capValue, &state.AcceptedBidCount, &state.ExtendCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auctionState{}, apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound)
		}
		return auctionState{}, err
	}
	if winner != nil {
		state.CurrentWinnerID = *winner
	}
	if description != nil {
		state.ItemDescription = *description
	}
	if capValue != nil {
		state.CapPriceCents = *capValue
	}
	return state, nil
}

func (r *Repository) ensureHostRoom(ctx context.Context, hostID string, roomID string) error {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rooms WHERE id = $1 AND host_id = $2)`, roomID, hostID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return apierrors.New(apierrors.CodeForbiddenRoom, "host cannot access room", http.StatusForbidden)
	}
	return nil
}

func (r *Repository) insertJob(ctx context.Context, job Job) (Job, error) {
	inputRaw, _ := json.Marshal(job.Input)
	outputRaw, _ := json.Marshal(job.Output)
	safetyRaw, _ := json.Marshal(job.Safety)
	var roomID any
	if job.RoomID != "" {
		roomID = job.RoomID
	}
	var auctionID any
	if job.AuctionID != "" {
		auctionID = job.AuctionID
	}
	var errorMessage any
	if job.ErrorMessage != "" {
		errorMessage = job.ErrorMessage
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO ai_generation_jobs (
		  id, room_id, auction_id, kind, status, input_hash, prompt_version, provider, model,
		  input_json, output_json, safety_json, error_message, reviewed_by
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING created_at, updated_at
	`, job.ID, roomID, auctionID, job.Kind, job.Status, job.InputHash, job.PromptVersion, job.Provider, job.Model, inputRaw, outputRaw, safetyRaw, errorMessage, nullableString(job.ReviewedBy)).
		Scan(&job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	return job, nil
}

func (r *Repository) insertRiskAlert(ctx context.Context, alert SentinelAlert) (SentinelAlert, error) {
	featuresRaw, _ := json.Marshal(alert.Features)
	err := r.db.QueryRow(ctx, `
		INSERT INTO auction_risk_alerts (room_id, auction_id, severity, risk_type, score, explanation, recommended_action, features_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, status, created_at, updated_at
	`, alert.RoomID, alert.AuctionID, alert.Severity, alert.RiskType, alert.Score, alert.Explanation, alert.RecommendedAction, featuresRaw).
		Scan(&alert.ID, &alert.Status, &alert.CreatedAt, &alert.UpdatedAt)
	return alert, err
}

type alertScanner interface {
	Scan(dest ...any) error
}

func scanAlert(row alertScanner) (SentinelAlert, error) {
	var alert SentinelAlert
	var featuresRaw []byte
	err := row.Scan(&alert.ID, &alert.RoomID, &alert.AuctionID, &alert.Severity, &alert.RiskType, &alert.Score, &alert.Explanation, &alert.RecommendedAction, &featuresRaw, &alert.Status, &alert.CreatedAt, &alert.UpdatedAt)
	if err != nil {
		return SentinelAlert{}, err
	}
	_ = json.Unmarshal(featuresRaw, &alert.Features)
	return alert, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableSeq(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func ensureMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func maskUserID(userID string) string {
	if userID == "" {
		return ""
	}
	runes := []rune(userID)
	if len(runes) <= 2 {
		return userID + "**"
	}
	return string(runes[:2]) + "**"
}
