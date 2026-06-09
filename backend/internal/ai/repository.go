package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

var highlightRenderSlots = make(chan struct{}, 1)

const highlightAssetReuseWindow = 10 * time.Minute

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
		Timeout:       12 * time.Second,
	})
	status := "SUCCEEDED"
	errorMessage := ""
	if err != nil {
		status = "FAILED"
		errorMessage = cleanText(err.Error(), 240)
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
	body := cleanText(stringValue(result.Output["body"]), 160)
	style := cleanText(firstNonEmpty(stringValue(result.Output["style"]), "steady"), 20)
	body, moneyNormalized := normalizeCommentaryMoney(body, req.CurrentPriceCents)
	if unsafeCommentaryBody(body) {
		status = "FAILED"
		errorMessage = firstNonEmpty(errorMessage, "AI provider returned unsafe commentary body")
		body = ""
	}
	if body == "" {
		status = "FAILED"
		errorMessage = firstNonEmpty(errorMessage, "AI provider returned empty commentary body")
		body, style, result.Safety = BuildCommentary(req)
		result.Provider = "deterministic"
		result.Model = "fallback-template"
		moneyNormalized = false
	}
	result.Safety = ensureMap(result.Safety)
	factsUsed := normalizeCommentaryFacts(result.Output["facts_used"])
	if len(factsUsed) == 0 {
		_, _, fallbackSafety := BuildCommentary(req)
		factsUsed = stringSlice(fallbackSafety["facts_used"])
	}
	result.Output = normalizedCommentaryOutput(req, body, style, factsUsed, result.Output["safety_labels"])
	result.Safety["facts_used"] = factsUsed
	if moneyNormalized {
		result.Safety["money_normalized"] = true
	}
	if result.Provider == "deterministic" || result.Model == "fallback-template" || status == "FAILED" {
		result.Safety["fallback"] = true
	}
	result.Safety["provider"] = result.Provider
	result.Safety["model"] = result.Model
	result.Safety["generation_status"] = status
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
		ErrorMessage:  errorMessage,
		ReviewedBy:    hostID,
	})
	return msg, job, err
}

func (r *Repository) CreateQuickCommentary(ctx context.Context, hostID string, req CommentaryRequest) (SystemMessage, Job, error) {
	if req.AuctionID == "" || req.RoomID == "" {
		return SystemMessage{}, Job{}, apierrors.New(apierrors.CodeInvalidArgument, "auction_id and room_id are required", http.StatusBadRequest)
	}
	if !IsQuickCommentaryEvent(req.EventType) {
		return SystemMessage{}, Job{}, apierrors.New(apierrors.CodeInvalidArgument, "unsupported quick commentary event", http.StatusBadRequest)
	}
	if err := r.ensureHostRoom(ctx, hostID, req.RoomID); err != nil {
		return SystemMessage{}, Job{}, err
	}
	if req.SourceSeq <= 0 {
		req.SourceSeq = time.Now().UnixMilli()
	}
	inputMap := structToMap(req)
	body, style, safety := BuildCommentary(req)
	safety = ensureMap(safety)
	safety["quick_template"] = true
	safety["provider"] = "host_quick_template"
	safety["model"] = "reviewed-live-script"
	safety["generation_status"] = "SUCCEEDED"
	factsUsed := stringSlice(safety["facts_used"])
	output := normalizedCommentaryOutput(req, body, style, factsUsed, []string{"buyer_facing", "host_reviewed"})
	msg, err := r.InsertSystemMessage(ctx, req.RoomID, req.AuctionID, "HOST_SCRIPT", req.SourceSeq, style, body, inputMap, safety)
	if err != nil {
		return SystemMessage{}, Job{}, err
	}
	job, err := r.insertJob(ctx, Job{
		ID:            "aijob_" + uuid.NewString(),
		RoomID:        req.RoomID,
		AuctionID:     req.AuctionID,
		Kind:          "auction_commentary",
		Status:        "SUCCEEDED",
		InputHash:     InputHash(inputMap),
		PromptVersion: PromptVersionCommentary,
		Provider:      "host_quick_template",
		Model:         "reviewed-live-script",
		Input:         inputMap,
		Output:        output,
		Safety:        safety,
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
	errorMessage := ""
	if err != nil {
		status = "FAILED"
		errorMessage = cleanText(err.Error(), 240)
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
	body := cleanText(stringValue(result.Output["body"]), 160)
	style := cleanText(firstNonEmpty(stringValue(result.Output["style"]), "steady"), 20)
	body, moneyNormalized := normalizeCommentaryMoney(body, req.CurrentPriceCents)
	if unsafeCommentaryBody(body) {
		status = "FAILED"
		errorMessage = firstNonEmpty(errorMessage, "AI provider returned unsafe commentary body")
		body = ""
	}
	if body == "" {
		status = "FAILED"
		errorMessage = firstNonEmpty(errorMessage, "AI provider returned empty commentary body")
		body, style, result.Safety = BuildCommentary(req)
		result.Provider = "deterministic"
		result.Model = "fallback-template"
		moneyNormalized = false
	}
	result.Safety = ensureMap(result.Safety)
	factsUsed := normalizeCommentaryFacts(result.Output["facts_used"])
	if len(factsUsed) == 0 {
		_, _, fallbackSafety := BuildCommentary(req)
		factsUsed = stringSlice(fallbackSafety["facts_used"])
	}
	result.Output = normalizedCommentaryOutput(req, body, style, factsUsed, result.Output["safety_labels"])
	result.Safety["facts_used"] = factsUsed
	if moneyNormalized {
		result.Safety["money_normalized"] = true
	}
	if result.Provider == "deterministic" || result.Model == "fallback-template" || status == "FAILED" {
		result.Safety["fallback"] = true
	}
	result.Safety["auto_generated"] = true
	result.Safety["provider"] = result.Provider
	result.Safety["model"] = result.Model
	result.Safety["generation_status"] = status
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
		ErrorMessage:  errorMessage,
	})
	return msg, job, err
}

func normalizedCommentaryOutput(req CommentaryRequest, body string, style string, factsUsed []string, safetyLabels any) map[string]any {
	return map[string]any{
		"auction_id":    req.AuctionID,
		"source_seq":    req.SourceSeq,
		"style":         style,
		"body":          body,
		"facts_used":    factsUsed,
		"safety_labels": limitStrings(stringSlice(safetyLabels), 8, 48),
	}
}

func normalizeCommentaryMoney(body string, currentPriceCents int64) (string, bool) {
	if body == "" || currentPriceCents <= 0 {
		return body, false
	}
	expected := FormatCents(currentPriceCents)
	changed := false
	wrongYuan := fmt.Sprintf("%d", currentPriceCents)
	if strings.Contains(body, wrongYuan+"元") {
		body = strings.ReplaceAll(body, wrongYuan+"元", expected)
		changed = true
	}
	wrongWan := currentPriceCents / 100
	if wrongWan > 0 && wrongWan%100 == 0 {
		unit := wrongWan / 100
		re := regexp.MustCompile(fmt.Sprintf(`%d\s*万元`, unit))
		if re.MatchString(body) {
			body = re.ReplaceAllString(body, expected)
			changed = true
		}
	}
	return body, changed
}

func normalizeCommentaryFacts(value any) []string {
	allowed := map[string]bool{
		"auction_id":            true,
		"source_seq":            true,
		"event_type":            true,
		"item_title":            true,
		"current_price_cents":   true,
		"current_winner_masked": true,
		"active_bidders_30s":    true,
		"accepted_bids_30s":     true,
	}
	out := []string{}
	for _, fact := range limitStrings(stringSlice(value), 8, 64) {
		if allowed[fact] {
			out = append(out, fact)
		}
	}
	return out
}

func unsafeCommentaryBody(body string) bool {
	for _, token := range []string{"保真", "升值", "稳赚", "隐藏", "最高价", "观众", "观看人数", "库存", "秒杀"} {
		if strings.Contains(body, token) {
			return true
		}
	}
	return false
}

func (r *Repository) EnqueueAutoCommentary(ctx context.Context, req CommentaryRequest) (Job, bool, error) {
	if req.AuctionID == "" {
		return Job{}, false, apierrors.New(apierrors.CodeInvalidArgument, "auction_id is required", http.StatusBadRequest)
	}
	state, err := r.auctionState(ctx, req.AuctionID)
	if err != nil {
		return Job{}, false, err
	}
	req.RoomID = firstNonEmpty(req.RoomID, state.RoomID)
	req.ItemTitle = firstNonEmpty(req.ItemTitle, state.ItemTitle)
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
	inputHash := InputHash(map[string]any{
		"kind":           "auction_commentary",
		"prompt_version": PromptVersionCommentary,
		"input":          inputMap,
		"auto_requested": true,
	})
	job := Job{
		ID:            "aijob_" + uuid.NewString(),
		RoomID:        req.RoomID,
		AuctionID:     req.AuctionID,
		Kind:          "auction_commentary",
		Status:        "PENDING",
		InputHash:     inputHash,
		PromptVersion: PromptVersionCommentary,
		Provider:      "queued",
		Model:         "auto-commentary-worker",
		Input:         inputMap,
		Output:        map[string]any{},
		Safety: map[string]any{
			"auto_requested":    true,
			"no_bid_decision":   true,
			"non_blocking_path": true,
		},
	}
	stored, inserted, err := r.insertAutoCommentaryQueueJob(ctx, job)
	return stored, inserted, err
}

func (r *Repository) RunAutoCommentaryWorker(ctx context.Context, gen Generator, opts AutoCommentaryWorkerOptions) {
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 4
	}
	if opts.Lease <= 0 {
		opts.Lease = 30 * time.Second
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = 20 * time.Second
	}
	if opts.BackfillLookback <= 0 {
		opts.BackfillLookback = 24 * time.Hour
	}
	opts.WorkerID = cleanText(firstNonEmpty(opts.WorkerID, "ai-commentary-worker"), 80)
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()
	for {
		stats, _ := r.ProcessAutoCommentaryQueue(ctx, gen, opts)
		if stats.Processed+stats.Failed < opts.BatchSize {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (r *Repository) ProcessAutoCommentaryQueue(ctx context.Context, gen Generator, opts AutoCommentaryWorkerOptions) (CommentaryQueueStats, error) {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 4
	}
	if opts.Lease <= 0 {
		opts.Lease = 30 * time.Second
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = 20 * time.Second
	}
	if opts.BackfillLookback <= 0 {
		opts.BackfillLookback = 24 * time.Hour
	}
	opts.WorkerID = cleanText(firstNonEmpty(opts.WorkerID, "ai-commentary-worker"), 80)
	stats := CommentaryQueueStats{}
	enqueued, err := r.EnqueueMissingAutoCommentary(ctx, opts.BatchSize, opts.BackfillLookback, opts.AuctionID)
	if err != nil {
		return stats, err
	}
	stats.Enqueued = enqueued
	for i := 0; i < opts.BatchSize; i++ {
		job, ok, err := r.claimAutoCommentaryQueueJob(ctx, opts.WorkerID, opts.Lease, opts.AuctionID)
		if err != nil {
			return stats, err
		}
		if !ok {
			return stats, nil
		}
		var req CommentaryRequest
		if err := mapToStruct(job.Input, &req); err != nil {
			stats.Failed++
			_ = r.failAutoCommentaryQueueJob(ctx, job.ID, opts.WorkerID, err, false)
			continue
		}
		taskCtx, cancel := context.WithTimeout(ctx, opts.TaskTimeout)
		_, doneJob, err := r.CreateAutoCommentary(taskCtx, gen, req)
		cancel()
		if err != nil {
			stats.Failed++
			_ = r.failAutoCommentaryQueueJob(ctx, job.ID, opts.WorkerID, err, true)
			continue
		}
		stats.Processed++
		_ = r.completeAutoCommentaryQueueJob(ctx, job.ID, opts.WorkerID, doneJob)
	}
	return stats, nil
}

func (r *Repository) EnqueueMissingAutoCommentary(ctx context.Context, limit int, lookback time.Duration, auctionID string) (int, error) {
	if limit <= 0 {
		limit = 4
	}
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}
	rows, err := r.db.Query(ctx, `
		SELECT ev.auction_id, ev.seq, ev.event_type, a.room_id, COALESCE(i.title, ''),
		       COALESCE(
		         CASE
		           WHEN ev.payload_json->>'current_price_cents' ~ '^[0-9]+$'
		           THEN (ev.payload_json->>'current_price_cents')::bigint
		           ELSE NULL
		         END,
		         a.current_price_cents
		       ),
		       COALESCE(ev.payload_json->>'user_id', a.current_winner_id, '')
		FROM auction_events ev
		JOIN auctions a ON a.id = ev.auction_id
		LEFT JOIN items i ON i.id = a.item_id
		LEFT JOIN auction_system_messages msg
		  ON msg.auction_id = ev.auction_id
		 AND msg.source = 'SYSTEM_AI'
		 AND msg.source_seq = ev.seq
		WHERE ev.event_type IN ('bid_accepted','auction_sold')
		  AND msg.id IS NULL
		  AND to_timestamp(ev.server_time_ms / 1000.0) >= now() - ($2::bigint * interval '1 millisecond')
		  AND ($3 = '' OR ev.auction_id = $3)
		ORDER BY ev.server_time_ms, ev.auction_id, ev.seq
		LIMIT $1
	`, limit, lookback.Milliseconds(), auctionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	enqueued := 0
	for rows.Next() {
		var req CommentaryRequest
		var winnerID string
		if err := rows.Scan(&req.AuctionID, &req.SourceSeq, &req.EventType, &req.RoomID, &req.ItemTitle, &req.CurrentPriceCents, &winnerID); err != nil {
			return enqueued, err
		}
		req.CurrentWinnerMasked = maskUserID(winnerID)
		_, inserted, err := r.EnqueueAutoCommentary(ctx, req)
		if err != nil {
			return enqueued, err
		}
		if inserted {
			enqueued++
		}
	}
	if err := rows.Err(); err != nil {
		return enqueued, err
	}
	return enqueued, nil
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
		RuleSuggestion:  recapRuleSuggestion(state, acceptedBidders),
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
	if asset, ok, err := r.latestReusableHighlightAsset(ctx, auctionID); err != nil {
		return AuctionRecap{}, Job{}, HighlightAsset{}, err
	} else if ok {
		return recap, job, asset, nil
	}
	asset, err := r.createHighlightAsset(ctx, recap, job)
	return recap, job, asset, err
}

func (r *Repository) latestReusableHighlightAsset(ctx context.Context, auctionID string) (HighlightAsset, bool, error) {
	asset := HighlightAsset{}
	err := r.db.QueryRow(ctx, `
		SELECT id, auction_id, room_id, job_id, status, media_type, title, asset_url,
		       render_profile, duration_ms, facts_json, risk_json, created_at, updated_at
		FROM auction_highlight_assets
		WHERE auction_id = $1
		  AND status = 'RENDERED'
		  AND media_type = 'video/webm'
		  AND render_profile = 'server-webm-reel-v1'
		  AND created_at >= now() - ($2::double precision * interval '1 second')
		ORDER BY created_at DESC
		LIMIT 1
	`, auctionID, highlightAssetReuseWindow.Seconds()).
		Scan(&asset.ID, &asset.AuctionID, &asset.RoomID, &asset.JobID, &asset.Status, &asset.MediaType, &asset.Title,
			&asset.AssetURL, &asset.RenderProfile, &asset.DurationMS, &asset.Facts, &asset.Risk, &asset.CreatedAt, &asset.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HighlightAsset{}, false, nil
		}
		return HighlightAsset{}, false, err
	}
	return asset, true, nil
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
		"async_pipeline":          true,
		"does_not_block_bid_path": true,
		"buyer_identities_masked": true,
		"server_ffmpeg_pipeline":  true,
		"format":                  "webm",
	}
	title := cleanText(recap.ItemTitle+" 高光复盘", 80)
	videoAsset, err := renderHighlightWebM(ctx, recap)
	if err != nil {
		return HighlightAsset{}, err
	}
	asset := HighlightAsset{
		ID:            "hl_" + uuid.NewString(),
		AuctionID:     recap.AuctionID,
		RoomID:        recap.RoomID,
		JobID:         job.ID,
		Status:        "RENDERED",
		MediaType:     "video/webm",
		Title:         title,
		AssetURL:      "data:video/webm;base64," + base64.StdEncoding.EncodeToString(videoAsset),
		RenderProfile: "server-webm-reel-v1",
		DurationMS:    12_000,
		Facts:         facts,
		Risk:          risk,
	}
	err = r.db.QueryRow(ctx, `
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

func renderHighlightWebM(ctx context.Context, recap AuctionRecap) ([]byte, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found for highlight video: %w", err)
	}
	select {
	case highlightRenderSlots <- struct{}{}:
		defer func() { <-highlightRenderSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	dir, err := os.MkdirTemp("", "live-auction-highlight-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	textFiles := map[string]string{
		"title.txt": cleanText(recap.ItemTitle, 36),
		"price.txt": FormatCents(recap.FinalPriceCents),
		"facts.txt": strings.Join([]string{
			"真实出价 " + int64Text(recap.AcceptedBids) + " 口",
			"参与买家 " + int64Text(recap.AcceptedBidders) + " 人",
			"末段延时 " + int64Text(int64(recap.ExtendCount)) + " 次",
		}, "  ·  "),
		"winner.txt": "成交买家 " + firstNonEmpty(recap.WinnerMasked, "待确认"),
		"next.txt":   firstNonEmpty(firstString(recap.NextActions), "提醒赢家完成支付"),
		"brand.txt":  "直播竞拍高光复盘",
	}
	for name, value := range textFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600); err != nil {
			return nil, err
		}
	}

	outPath := filepath.Join(dir, "highlight.webm")
	filter := strings.Join([]string{
		"drawbox=x=0:y=0:w=720:h=1280:color=0x101820@1:t=fill",
		"drawbox=x=0:y=0:w=720:h=360:color=0xE9B44C@0.95:t=fill",
		"drawbox=x=38:y=420:w=644:h=500:color=0xFFFFFF@0.10:t=fill",
		"drawbox=x=52:y=434:w=616:h=472:color=0x111827@0.72:t=fill",
		"drawbox=x=0:y=1030:w=720:h=250:color=0x2B5C7A@0.92:t=fill",
		drawText(filepath.Join(dir, "brand.txt"), 54, 78, 30, "0x101820", "NotoSansCJK-Bold.ttc"),
		drawText(filepath.Join(dir, "title.txt"), 54, 155, 48, "0x101820", "NotoSansCJK-Bold.ttc"),
		drawText(filepath.Join(dir, "price.txt"), 54, 282, 78, "0x101820", "NotoSansCJK-Bold.ttc"),
		drawText(filepath.Join(dir, "facts.txt"), 78, 508, 30, "0xF8FAFC", "NotoSansCJK-Regular.ttc"),
		drawText(filepath.Join(dir, "winner.txt"), 78, 610, 38, "0xFFD166", "NotoSansCJK-Bold.ttc"),
		drawText(filepath.Join(dir, "next.txt"), 78, 745, 34, "0xF8FAFC", "NotoSansCJK-Regular.ttc"),
		"drawbox=x=78:y=860:w=220:h=6:color=0xE9B44C@1:t=fill",
		drawText(filepath.Join(dir, "brand.txt"), 54, 1092, 32, "0xF8FAFC", "NotoSansCJK-Bold.ttc"),
	}, ",")

	renderCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(renderCtx, ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=0x101820:s=720x1280:d=12:r=24",
		"-vf", filter,
		"-an",
		"-c:v", "libvpx-vp9",
		"-pix_fmt", "yuv420p",
		"-b:v", "900k",
		"-deadline", "good",
		"-cpu-used", "4",
		outPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg highlight render failed: %w: %s", err, cleanText(string(output), 300))
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 || data[0] != 0x1a || data[1] != 0x45 || data[2] != 0xdf || data[3] != 0xa3 {
		return nil, fmt.Errorf("ffmpeg highlight render produced invalid webm asset")
	}
	return data, nil
}

func drawText(textFile string, x int, y int, size int, color string, fontName string) string {
	return fmt.Sprintf(
		"drawtext=fontfile='%s':textfile='%s':x=%d:y=%d:fontsize=%d:fontcolor=%s:line_spacing=10",
		"/usr/share/fonts/opentype/noto/"+fontName,
		textFile,
		x,
		y,
		size,
		color,
	)
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
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
			Answer:    cleanText(stringValue(outputMap["answer"]), 420),
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
		answer := cleanText(turn.Answer, 420)
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

func recapRuleSuggestion(state auctionState, acceptedBidders int64) *RecapRuleSuggestion {
	if state.IncrementCents <= 0 || state.StartPriceCents < 0 {
		return nil
	}
	start := state.StartPriceCents
	if state.Status == "SOLD" && state.CurrentPriceCents > state.StartPriceCents && acceptedBidders >= 2 {
		weighted := int64(math.Round(float64(state.StartPriceCents)*0.70 + float64(state.CurrentPriceCents)*0.30))
		start = alignToIncrement(weighted, state.StartPriceCents, state.IncrementCents)
	}
	if start < state.StartPriceCents {
		start = state.StartPriceCents
	}
	capPrice := state.CapPriceCents
	minCap := start + state.IncrementCents*5
	if state.Status == "SOLD" && state.CurrentPriceCents > capPrice {
		capPrice = state.CurrentPriceCents
	}
	if capPrice < minCap {
		capPrice = minCap
	}
	capPrice = alignToIncrement(capPrice, start, state.IncrementCents)
	if capPrice < minCap {
		capPrice += state.IncrementCents
	}
	return &RecapRuleSuggestion{
		StartPriceCents:     start,
		IncrementCents:      state.IncrementCents,
		CapPriceCents:       capPrice,
		Basis:               "基于本场起拍价、成交价、加价幅度、有效出价人数生成；仅供下一件人工采信",
		Source:              "auction_recap:server_facts",
		HumanReviewRequired: true,
	}
}

func alignToIncrement(value int64, start int64, increment int64) int64 {
	if increment <= 0 || value <= start {
		return start
	}
	steps := (value - start + increment - 1) / increment
	return start + steps*increment
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

func (r *Repository) insertAutoCommentaryQueueJob(ctx context.Context, job Job) (Job, bool, error) {
	inputRaw, _ := json.Marshal(job.Input)
	outputRaw, _ := json.Marshal(job.Output)
	safetyRaw, _ := json.Marshal(job.Safety)
	var stored Job
	var roomID *string
	var auctionID *string
	var inputStored []byte
	var outputStored []byte
	var safetyStored []byte
	inserted := false
	err := r.db.QueryRow(ctx, `
		WITH inserted AS (
		  INSERT INTO ai_generation_jobs (
		    id, room_id, auction_id, kind, status, input_hash, prompt_version, provider, model,
		    input_json, output_json, safety_json
		  )
		  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		  ON CONFLICT (kind, input_hash)
		    WHERE kind = 'auction_commentary'
		      AND (safety_json->>'auto_requested') = 'true'
		  DO NOTHING
		  RETURNING *, true AS inserted
		)
		SELECT id, room_id, auction_id, kind, status, input_hash, prompt_version, provider, model,
		       input_json, output_json, safety_json, created_at, updated_at, inserted
		FROM inserted
		UNION ALL
		SELECT id, room_id, auction_id, kind, status, input_hash, prompt_version, provider, model,
		       input_json, output_json, safety_json, created_at, updated_at, false AS inserted
		FROM ai_generation_jobs
		WHERE kind = 'auction_commentary'
		  AND input_hash = $6
		  AND (safety_json->>'auto_requested') = 'true'
		LIMIT 1
	`, job.ID, nullableString(job.RoomID), nullableString(job.AuctionID), job.Kind, job.Status, job.InputHash, job.PromptVersion, job.Provider, job.Model, inputRaw, outputRaw, safetyRaw).
		Scan(&stored.ID, &roomID, &auctionID, &stored.Kind, &stored.Status, &stored.InputHash, &stored.PromptVersion, &stored.Provider, &stored.Model, &inputStored, &outputStored, &safetyStored, &stored.CreatedAt, &stored.UpdatedAt, &inserted)
	if err != nil {
		return Job{}, false, err
	}
	if roomID != nil {
		stored.RoomID = *roomID
	}
	if auctionID != nil {
		stored.AuctionID = *auctionID
	}
	_ = json.Unmarshal(inputStored, &stored.Input)
	_ = json.Unmarshal(outputStored, &stored.Output)
	_ = json.Unmarshal(safetyStored, &stored.Safety)
	return stored, inserted, nil
}

func (r *Repository) claimAutoCommentaryQueueJob(ctx context.Context, workerID string, lease time.Duration, auctionIDFilter string) (Job, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Job{}, false, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	var job Job
	var roomID *string
	var auctionID *string
	var inputRaw []byte
	var outputRaw []byte
	var safetyRaw []byte
	err = tx.QueryRow(ctx, `
		SELECT id, room_id, auction_id, kind, status, input_hash, prompt_version, provider, model,
		       input_json, output_json, safety_json, created_at, updated_at
		FROM ai_generation_jobs
		WHERE kind = 'auction_commentary'
		  AND status = 'PENDING'
		  AND (safety_json->>'auto_requested') = 'true'
		  AND (locked_until IS NULL OR locked_until < now())
		  AND attempts < 3
		  AND ($1 = '' OR auction_id = $1)
		ORDER BY created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, auctionIDFilter).Scan(&job.ID, &roomID, &auctionID, &job.Kind, &job.Status, &job.InputHash, &job.PromptVersion, &job.Provider, &job.Model, &inputRaw, &outputRaw, &safetyRaw, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_generation_jobs
		SET worker_id = $2,
		    locked_until = now() + ($3::bigint * interval '1 millisecond'),
		    attempts = attempts + 1,
		    updated_at = now()
		WHERE id = $1
	`, job.ID, workerID, lease.Milliseconds()); err != nil {
		return Job{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, false, err
	}
	if roomID != nil {
		job.RoomID = *roomID
	}
	if auctionID != nil {
		job.AuctionID = *auctionID
	}
	_ = json.Unmarshal(inputRaw, &job.Input)
	_ = json.Unmarshal(outputRaw, &job.Output)
	_ = json.Unmarshal(safetyRaw, &job.Safety)
	return job, true, nil
}

func (r *Repository) completeAutoCommentaryQueueJob(ctx context.Context, jobID string, workerID string, generated Job) error {
	outputRaw, _ := json.Marshal(generated.Output)
	safety := ensureMap(generated.Safety)
	safety["auto_requested"] = true
	safety["generated_job_id"] = generated.ID
	safetyRaw, _ := json.Marshal(safety)
	_, err := r.db.Exec(ctx, `
		UPDATE ai_generation_jobs
		SET status = 'SUCCEEDED',
		    provider = $2,
		    model = $3,
		    output_json = $4,
		    safety_json = $5,
		    worker_id = NULL,
		    locked_until = NULL,
		    error_message = NULL,
		    updated_at = now()
		WHERE id = $1
		  AND worker_id = $6
	`, jobID, generated.Provider, generated.Model, outputRaw, safetyRaw, workerID)
	return err
}

func (r *Repository) failAutoCommentaryQueueJob(ctx context.Context, jobID string, workerID string, cause error, retryable bool) error {
	message := ""
	if cause != nil {
		message = cleanText(cause.Error(), 240)
	}
	statusExpr := "CASE WHEN $3::boolean AND attempts < 3 THEN 'PENDING' ELSE 'FAILED' END"
	_, err := r.db.Exec(ctx, `
		UPDATE ai_generation_jobs
		SET status = `+statusExpr+`,
		    provider = 'worker',
		    model = 'auto-commentary-worker',
		    error_message = $2,
		    worker_id = NULL,
		    locked_until = NULL,
		    updated_at = now()
		WHERE id = $1
		  AND worker_id = $4
	`, jobID, message, retryable, workerID)
	return err
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
