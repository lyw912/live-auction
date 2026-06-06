package ai

import (
	"context"
	"encoding/json"
	"errors"
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
	if req.RoomID == "" || (req.SellerNotes == "" && len(req.ImageURLs) == 0) {
		return Job{}, apierrors.New(apierrors.CodeInvalidArgument, "room_id and seller_notes or image_urls are required", http.StatusBadRequest)
	}
	if err := r.ensureHostRoom(ctx, hostID, req.RoomID); err != nil {
		return Job{}, err
	}
	inputMap := structToMap(req)
	inputHash := InputHash(map[string]any{
		"kind":           "listing_draft",
		"prompt_version": PromptVersionListingDraft,
		"input":          inputMap,
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
		Input:         inputMap,
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
		Timeout:       2 * time.Second,
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

func (r *Repository) EvaluateSentinel(ctx context.Context, hostID string, auctionID string) ([]SentinelAlert, error) {
	state, err := r.auctionState(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	if err := r.ensureHostRoom(ctx, hostID, state.RoomID); err != nil {
		return nil, err
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
		return nil, err
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
	for i := range alerts {
		inserted, err := r.insertRiskAlert(ctx, alerts[i])
		if err != nil {
			return nil, err
		}
		alerts[i] = inserted
	}
	return alerts, nil
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

func (r *Repository) BuildAuctionRecap(ctx context.Context, hostID string, auctionID string) (AuctionRecap, Job, error) {
	state, err := r.auctionState(ctx, auctionID)
	if err != nil {
		return AuctionRecap{}, Job{}, err
	}
	if err := r.ensureHostRoom(ctx, hostID, state.RoomID); err != nil {
		return AuctionRecap{}, Job{}, err
	}
	var acceptedBidders int64
	if err := r.db.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM bids WHERE auction_id = $1 AND status = 'ACCEPTED'`, auctionID).Scan(&acceptedBidders); err != nil {
		return AuctionRecap{}, Job{}, err
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
	return recap, job, err
}

func (r *Repository) AnswerProductQuestion(ctx context.Context, roomID string, req ProductQARequest) (ProductQAAnswer, error) {
	req.Question = cleanText(req.Question, 120)
	if req.AuctionID == "" || req.Question == "" {
		return ProductQAAnswer{}, apierrors.New(apierrors.CodeInvalidArgument, "auction_id and question are required", http.StatusBadRequest)
	}
	state, err := r.auctionState(ctx, req.AuctionID)
	if err != nil {
		return ProductQAAnswer{}, err
	}
	if roomID != "" && state.RoomID != roomID {
		return ProductQAAnswer{}, apierrors.New(apierrors.CodeForbiddenRoom, "auction does not belong to room", http.StatusForbidden)
	}
	rules := map[string]any{
		"start_price_cents": state.StartPriceCents,
		"increment_cents":   state.IncrementCents,
		"cap_price_cents":   state.CapPriceCents,
	}
	return AnswerFromFacts(req.AuctionID, req.Question, state.ItemTitle, state.ItemDescription, rules), nil
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
