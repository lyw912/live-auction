package auction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	apierrors "live-auction/backend/internal/platform/errors"
)

const (
	defaultDepositBPS        int16 = 1000
	defaultDepositFloorCents int64 = 10_000
	defaultDepositCapCents   int64 = 100_000_000
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type CreateItemInput struct {
	Title       string  `json:"title"`
	ImageURL    *string `json:"image_url"`
	Description *string `json:"description"`
}

type CreateAuctionInput struct {
	RoomID          string `json:"room_id"`
	ItemID          string `json:"item_id"`
	StartPriceCents int64  `json:"start_price_cents"`
	IncrementCents  int64  `json:"increment_cents"`
	CapPriceCents   *int64 `json:"cap_price_cents"`
	Rule            Rule   `json:"rule"`
}

func (r *Repository) CreateItem(ctx context.Context, input CreateItemInput) (Item, error) {
	if input.Title == "" {
		return Item{}, apierrors.New(apierrors.CodeInvalidArgument, "title is required", 400)
	}
	item := Item{
		ID:          "item_" + uuid.NewString(),
		Title:       input.Title,
		ImageURL:    input.ImageURL,
		Description: input.Description,
		Status:      "READY",
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO items (id, title, image_url, description, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`, item.ID, item.Title, item.ImageURL, item.Description, item.Status).Scan(&item.CreatedAt)
	if err != nil {
		return Item{}, err
	}
	return item, nil
}

func (r *Repository) CreateAuction(ctx context.Context, input CreateAuctionInput, traceID string) (Auction, error) {
	if err := validateLifecycleRuleInput(input); err != nil {
		return Auction{}, err
	}

	tx, err := r.beginTx(ctx)
	if err != nil {
		return Auction{}, err
	}
	defer rollback(ctx, tx)

	auctionID := "auc_" + uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO auctions (
			id, room_id, item_id, status, current_price_cents,
			start_price_cents, increment_cents, cap_price_cents
		)
		VALUES ($1, $2, $3, 'DRAFT', $4, $4, $5, $6)
	`, auctionID, input.RoomID, input.ItemID, input.StartPriceCents, input.IncrementCents, input.CapPriceCents)
	if err != nil {
		return Auction{}, mapPGError(err)
	}

	if err := insertRule(ctx, tx, auctionID, 1, input.Rule, nil); err != nil {
		return Auction{}, err
	}
	if err := appendAuctionEvent(ctx, tx, auctionID, "auction_created", traceID, map[string]any{
		"room_id": input.RoomID,
		"item_id": input.ItemID,
		"status":  string(StatusDraft),
	}); err != nil {
		return Auction{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Auction{}, err
	}
	return r.GetAuction(ctx, auctionID)
}

func (r *Repository) UpdateRules(ctx context.Context, auctionID string, input UpdateRulesInput, traceID string) (Auction, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return Auction{}, err
	}
	defer rollback(ctx, tx)

	var status Status
	var ruleVersion int
	if err := tx.QueryRow(ctx, `
		SELECT status, rule_version
		FROM auctions
		WHERE id = $1
		FOR UPDATE
	`, auctionID).Scan(&status, &ruleVersion); err != nil {
		return Auction{}, mapNotFound(err)
	}
	if status != StatusDraft {
		return Auction{}, apierrors.New(apierrors.CodeRuleFrozenAfterScheduled, "rules can only be changed while auction is DRAFT", 409)
	}
	if err := validateRuleAgainstAuction(input.StartPriceCents, input.IncrementCents, input.CapPriceCents, input.Rule); err != nil {
		return Auction{}, err
	}

	nextVersion := ruleVersion + 1
	if err := insertRule(ctx, tx, auctionID, nextVersion, input.Rule, nil); err != nil {
		return Auction{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auctions
		SET start_price_cents = $2,
		    increment_cents = $3,
		    cap_price_cents = $4,
		    current_price_cents = $2,
		    rule_version = $5,
		    updated_at = now()
		WHERE id = $1
	`, auctionID, input.StartPriceCents, input.IncrementCents, input.CapPriceCents, nextVersion); err != nil {
		return Auction{}, err
	}
	if err := appendAuctionEvent(ctx, tx, auctionID, "rules_updated", traceID, map[string]any{
		"rule_version":      nextVersion,
		"start_price_cents": input.StartPriceCents,
		"increment_cents":   input.IncrementCents,
		"cap_price_cents":   input.CapPriceCents,
	}); err != nil {
		return Auction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Auction{}, err
	}
	return r.GetAuction(ctx, auctionID)
}

func (r *Repository) Schedule(ctx context.Context, auctionID string, startAt *time.Time, traceID string) (Auction, error) {
	return r.transition(ctx, auctionID, StatusDraft, StatusScheduled, traceID, "auction_scheduled", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE auction_rules
			SET frozen_at = now()
			WHERE auction_id = $1
			  AND rule_version = (SELECT rule_version FROM auctions WHERE id = $1)
		`, auctionID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE auctions SET start_at = $2 WHERE id = $1`, auctionID, startAt)
		return err
	})
}

func (r *Repository) Unschedule(ctx context.Context, auctionID string, traceID string) (Auction, error) {
	return r.transition(ctx, auctionID, StatusScheduled, StatusDraft, traceID, "auction_unscheduled", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE auction_rules
			SET frozen_at = NULL
			WHERE auction_id = $1
			  AND rule_version = (SELECT rule_version FROM auctions WHERE id = $1)
		`, auctionID)
		return err
	})
}

func (r *Repository) Start(ctx context.Context, auctionID string, traceID string) (Auction, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return Auction{}, err
	}
	defer rollback(ctx, tx)

	var status Status
	var durationSeconds int
	if err := tx.QueryRow(ctx, `
		SELECT a.status, ar.duration_seconds
		FROM auctions a
		JOIN auction_rules ar ON ar.auction_id = a.id AND ar.rule_version = a.rule_version
		WHERE a.id = $1
		FOR UPDATE OF a
	`, auctionID).Scan(&status, &durationSeconds); err != nil {
		return Auction{}, mapNotFound(err)
	}
	if status != StatusScheduled {
		return Auction{}, apierrors.New(apierrors.CodeInvalidArgument, "only SCHEDULED auction can start", 409)
	}

	now := time.Now().UTC()
	endAt := now.Add(time.Duration(durationSeconds) * time.Second)
	_, err = tx.Exec(ctx, `
		UPDATE auctions
		SET status = 'ACTIVE', start_at = $2, end_at = $3, updated_at = now()
		WHERE id = $1
	`, auctionID, now, endAt)
	if err != nil {
		return Auction{}, mapPGError(err)
	}
	if err := upsertSchedulerJob(ctx, tx, "END_AUCTION", "auction", auctionID, "end:"+auctionID, endAt); err != nil {
		return Auction{}, err
	}
	if err := appendAuctionEvent(ctx, tx, auctionID, "auction_started", traceID, map[string]any{
		"start_at": now,
		"end_at":   endAt,
	}); err != nil {
		return Auction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Auction{}, err
	}
	return r.GetAuction(ctx, auctionID)
}

func (r *Repository) Cancel(ctx context.Context, auctionID string, input CancelInput, traceID string) (Auction, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return Auction{}, err
	}
	defer rollback(ctx, tx)

	var status Status
	if err := tx.QueryRow(ctx, `SELECT status FROM auctions WHERE id = $1 FOR UPDATE`, auctionID).Scan(&status); err != nil {
		return Auction{}, mapNotFound(err)
	}
	if status == StatusSold || status == StatusEnded || status == StatusCancelled {
		return Auction{}, apierrors.New(apierrors.CodeInvalidArgument, "terminal auction cannot be cancelled", 409)
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "host_cancelled"
	}
	_, err = tx.Exec(ctx, `
		UPDATE auctions
		SET status = 'CANCELLED', is_narrating = false, narrating_started_at = NULL,
		    updated_at = now()
		WHERE id = $1
	`, auctionID)
	if err != nil {
		return Auction{}, err
	}
	if err := appendAuctionEvent(ctx, tx, auctionID, "auction_cancelled", traceID, map[string]any{
		"previous_status": status,
		"reason":          reason,
	}); err != nil {
		return Auction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Auction{}, err
	}
	return r.GetAuction(ctx, auctionID)
}

func (r *Repository) NarrateStart(ctx context.Context, auctionID string, traceID string) (Auction, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return Auction{}, err
	}
	defer rollback(ctx, tx)

	var status Status
	var roomID string
	if err := tx.QueryRow(ctx, `SELECT status, room_id FROM auctions WHERE id = $1 FOR UPDATE`, auctionID).Scan(&status, &roomID); err != nil {
		return Auction{}, mapNotFound(err)
	}
	if status == StatusSold || status == StatusEnded || status == StatusCancelled {
		return Auction{}, apierrors.New(apierrors.CodeInvalidNarrateTarget, "terminal auction cannot narrate", 409)
	}
	var activeID string
	err = tx.QueryRow(ctx, `SELECT id FROM auctions WHERE room_id = $1 AND status = 'ACTIVE' LIMIT 1`, roomID).Scan(&activeID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Auction{}, err
	}
	if err == nil && activeID != auctionID {
		return Auction{}, apierrors.New(apierrors.CodeInvalidNarrateTarget, "room active auction must be narrating target", 409)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auctions
		SET is_narrating = true, narrating_started_at = now(), updated_at = now()
		WHERE id = $1
	`, auctionID); err != nil {
		return Auction{}, mapPGError(err)
	}
	if err := appendAuctionEvent(ctx, tx, auctionID, "narrate_started", traceID, map[string]any{
		"room_id": roomID,
	}); err != nil {
		return Auction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Auction{}, err
	}
	return r.GetAuction(ctx, auctionID)
}

func (r *Repository) NarrateStop(ctx context.Context, auctionID string, traceID string) (Auction, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return Auction{}, err
	}
	defer rollback(ctx, tx)

	var roomID string
	if err := tx.QueryRow(ctx, `SELECT room_id FROM auctions WHERE id = $1 FOR UPDATE`, auctionID).Scan(&roomID); err != nil {
		return Auction{}, mapNotFound(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auctions
		SET is_narrating = false, narrating_started_at = NULL, updated_at = now()
		WHERE id = $1
	`, auctionID); err != nil {
		return Auction{}, err
	}
	if err := appendAuctionEvent(ctx, tx, auctionID, "narrate_stopped", traceID, map[string]any{
		"room_id": roomID,
	}); err != nil {
		return Auction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Auction{}, err
	}
	return r.GetAuction(ctx, auctionID)
}

func (r *Repository) ListAuctions(ctx context.Context, roomID string) ([]Auction, error) {
	if roomID == "" {
		return r.ListAuctionsForRooms(ctx, nil)
	}
	return r.ListAuctionsForRooms(ctx, []string{roomID})
}

func (r *Repository) ListAuctionsForRooms(ctx context.Context, roomIDs []string) ([]Auction, error) {
	if len(roomIDs) == 0 {
		return []Auction{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT a.id
		FROM auctions a
		WHERE a.room_id = ANY($1)
		ORDER BY a.created_at DESC
	`, roomIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var auctions []Auction
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		auction, err := r.GetAuction(ctx, id)
		if err != nil {
			return nil, err
		}
		auctions = append(auctions, auction)
	}
	return auctions, rows.Err()
}

func (r *Repository) GetAuction(ctx context.Context, auctionID string) (Auction, error) {
	var a Auction
	err := r.db.QueryRow(ctx, `
		SELECT
			a.id, a.room_id, a.item_id, a.status, a.is_narrating,
			a.current_price_cents, a.current_winner_id,
			a.start_price_cents, a.increment_cents, a.cap_price_cents,
			a.start_at, a.end_at, a.version, a.seq, a.accepted_bid_count,
			a.extend_count, a.rule_version, a.created_at, a.updated_at,
			i.id, i.title, i.image_url, i.description, i.status, i.created_at,
			ar.duration_seconds, ar.extend_window_seconds, ar.extend_by_seconds,
			ar.max_extend_count, ar.fat_finger_threshold_cents,
			COALESCE(ar.deposit_bps, $2),
			COALESCE(ar.deposit_floor_cents, $3),
			COALESCE(ar.deposit_cap_cents, $4),
			ar.frozen_at
		FROM auctions a
		JOIN items i ON i.id = a.item_id
		JOIN auction_rules ar ON ar.auction_id = a.id AND ar.rule_version = a.rule_version
		WHERE a.id = $1
	`, auctionID, defaultDepositBPS, defaultDepositFloorCents, defaultDepositCapCents).Scan(
		&a.ID, &a.RoomID, &a.ItemID, &a.Status, &a.IsNarrating,
		&a.CurrentPriceCents, &a.CurrentWinnerID,
		&a.StartPriceCents, &a.IncrementCents, &a.CapPriceCents,
		&a.StartAt, &a.EndAt, &a.Version, &a.Seq, &a.AcceptedBidCount,
		&a.ExtendCount, &a.RuleVersion, &a.CreatedAt, &a.UpdatedAt,
		&a.Item.ID, &a.Item.Title, &a.Item.ImageURL, &a.Item.Description, &a.Item.Status, &a.Item.CreatedAt,
		&a.Rule.DurationSeconds, &a.Rule.ExtendWindowSeconds, &a.Rule.ExtendBySeconds,
		&a.Rule.MaxExtendCount, &a.Rule.FatFingerThresholdCents,
		&a.Rule.DepositBPS, &a.Rule.DepositFloorCents, &a.Rule.DepositCapCents,
		&a.Rule.FrozenAt,
	)
	if err != nil {
		return Auction{}, mapNotFound(err)
	}
	return a, nil
}

func (r *Repository) transition(ctx context.Context, auctionID string, from Status, to Status, traceID string, eventType string, extra func(context.Context, pgx.Tx) error) (Auction, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return Auction{}, err
	}
	defer rollback(ctx, tx)

	var status Status
	if err := tx.QueryRow(ctx, `SELECT status FROM auctions WHERE id = $1 FOR UPDATE`, auctionID).Scan(&status); err != nil {
		return Auction{}, mapNotFound(err)
	}
	if status != from {
		if from == StatusDraft && to == StatusScheduled {
			return Auction{}, apierrors.New(apierrors.CodeRuleFrozenAfterScheduled, "auction must be DRAFT", 409)
		}
		return Auction{}, apierrors.New(apierrors.CodeInvalidArgument, fmt.Sprintf("auction must be %s", from), 409)
	}
	if extra != nil {
		if err := extra(ctx, tx); err != nil {
			return Auction{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auctions
		SET status = $2, updated_at = now()
		WHERE id = $1
	`, auctionID, to); err != nil {
		return Auction{}, err
	}
	if err := appendAuctionEvent(ctx, tx, auctionID, eventType, traceID, map[string]any{
		"from": status,
		"to":   to,
	}); err != nil {
		return Auction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Auction{}, err
	}
	return r.GetAuction(ctx, auctionID)
}

func (r *Repository) beginTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '500ms'`); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '3s'`); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func appendAuctionEvent(ctx context.Context, tx pgx.Tx, auctionID string, eventType string, traceID string, payload map[string]any) error {
	var seq int64
	var version int64
	if err := tx.QueryRow(ctx, `
		UPDATE auctions
		SET seq = seq + 1, version = version + 1, updated_at = now()
		WHERE id = $1
		RETURNING seq, version
	`, auctionID).Scan(&seq, &version); err != nil {
		return err
	}
	payload["state_version"] = version
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var outboxID int64
	serverTimeMS := time.Now().UTC().UnixMilli()
	if _, err := tx.Exec(ctx, `
		INSERT INTO auction_events (auction_id, seq, event_type, payload_json, server_time_ms, trace_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, auctionID, seq, eventType, payloadJSON, serverTimeMS, traceID); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, auction_id, seq, event_type, payload_json)
		VALUES ('auction', $1, $1, $2, $3, $4)
		RETURNING id
	`, auctionID, seq, eventType, payloadJSON).Scan(&outboxID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_delivery (outbox_id, status)
		VALUES ($1, 'PENDING')
	`, outboxID)
	return err
}

func upsertSchedulerJob(ctx context.Context, tx pgx.Tx, jobType string, targetType string, targetID string, idempotencyKey string, runAt time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO scheduler_jobs (id, job_type, target_type, target_id, idempotency_key, run_at, status, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', $6)
		ON CONFLICT (job_type, target_type, target_id, idempotency_key)
		DO UPDATE SET run_at = EXCLUDED.run_at,
		              status = CASE WHEN scheduler_jobs.status IN ('SUCCEEDED','DEAD') THEN scheduler_jobs.status ELSE 'PENDING' END,
		              next_attempt_at = EXCLUDED.next_attempt_at,
		              locked_by = NULL,
		              locked_until = NULL,
		              updated_at = now()
	`, "job_"+uuid.NewString(), jobType, targetType, targetID, idempotencyKey, runAt)
	return err
}

func insertRule(ctx context.Context, tx pgx.Tx, auctionID string, version int, rule Rule, frozenAt *time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO auction_rules (
			auction_id, rule_version, duration_seconds, extend_window_seconds,
			extend_by_seconds, max_extend_count, fat_finger_threshold_cents,
			deposit_bps, deposit_floor_cents, deposit_cap_cents, frozen_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, auctionID, version, rule.DurationSeconds, rule.ExtendWindowSeconds,
		rule.ExtendBySeconds, rule.MaxExtendCount, rule.FatFingerThresholdCents,
		coalesceInt16(rule.DepositBPS, defaultDepositBPS),
		coalesceInt64(rule.DepositFloorCents, defaultDepositFloorCents),
		coalesceInt64(rule.DepositCapCents, defaultDepositCapCents),
		frozenAt,
	)
	return err
}

func validateLifecycleRuleInput(input CreateAuctionInput) error {
	return validateRuleAgainstAuction(input.StartPriceCents, input.IncrementCents, input.CapPriceCents, input.Rule)
}

func validateRuleAgainstAuction(startPriceCents int64, incrementCents int64, capPriceCents *int64, rule Rule) error {
	violations := ValidateRule(RuleInput{
		StartPriceCents:         startPriceCents,
		IncrementCents:          incrementCents,
		CapPriceCents:           capPriceCents,
		DurationSeconds:         rule.DurationSeconds,
		ExtendWindowSeconds:     rule.ExtendWindowSeconds,
		ExtendBySeconds:         rule.ExtendBySeconds,
		MaxExtendCount:          rule.MaxExtendCount,
		FatFingerThresholdCents: rule.FatFingerThresholdCents,
	})
	if len(violations) == 0 {
		return nil
	}
	status := 400
	code := violations[0].Code
	return apierrors.APIError{
		Code:    code,
		Message: violations[0].Message,
		Status:  status,
		Details: map[string]any{
			"violations":     violations,
			"suggested_caps": violations[0].SuggestedCaps,
		},
	}
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", 404)
	}
	return err
}

func mapPGError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "55P03" {
			return apierrors.New(apierrors.CodeBidRetryLater, "database row is busy; retry later", 409)
		}
		switch pgErr.ConstraintName {
		case "ux_auctions_room_active":
			return apierrors.New(apierrors.CodeConflictRoomHasActiveAuction, "room already has active auction", 409)
		case "ux_auctions_room_narrating":
			return apierrors.New(apierrors.CodeConflictRoomHasNarration, "room already has narrating auction", 409)
		case "ck_auctions_cap_reachable":
			return apierrors.New(apierrors.CodeInvalidAuctionRuleCapUnreachable, "cap price is unreachable", 400)
		}
	}
	return err
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func coalesceInt16(value int16, fallback int16) int16 {
	if value == 0 {
		return fallback
	}
	return value
}

func coalesceInt64(value int64, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	return value
}
