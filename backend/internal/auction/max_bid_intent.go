package auction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	apierrors "live-auction/backend/internal/platform/errors"
)

const (
	MaxBidIntentResultActive    = "ACTIVE"
	MaxBidIntentResultCancelled = "CANCELLED"
)

func (r *Repository) PutMaxBidIntent(ctx context.Context, auctionID string, userID string, idempotencyKey string, input MaxBidIntentInput) (MaxBidIntentResponse, error) {
	if idempotencyKey == "" {
		return MaxBidIntentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "Idempotency-Key is required", http.StatusBadRequest)
	}
	source, err := normalizeMaxBidIntentSource(input.Source)
	if err != nil {
		return MaxBidIntentResponse{}, err
	}
	requestHash := maxBidIntentRequestHash(auctionID, userID, idempotencyKey, input.MaxAmountCents, source)
	if replay, ok, err := r.completedMaxBidIntentIdempotency(ctx, auctionID, userID, idempotencyKey, requestHash); err != nil || ok {
		return replay, err
	}

	tx, err := r.beginTx(ctx)
	if err != nil {
		return MaxBidIntentResponse{}, err
	}
	defer rollback(ctx, tx)

	if err := upsertProcessing(ctx, tx, "max_bid_intent", auctionID, userID, idempotencyKey, requestHash); err != nil {
		return MaxBidIntentResponse{}, err
	}
	input.Source = source
	intent, err := r.upsertMaxBidIntentTx(ctx, tx, auctionID, userID, input)
	if err != nil {
		return MaxBidIntentResponse{}, err
	}
	resp := MaxBidIntentResponse{Result: MaxBidIntentResultActive, Intent: intent}
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		return MaxBidIntentResponse{}, err
	}
	if err := completeIdempotency(ctx, tx, "max_bid_intent", auctionID, userID, idempotencyKey, requestHash, http.StatusOK, resp.Result, responseJSON); err != nil {
		return MaxBidIntentResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MaxBidIntentResponse{}, err
	}
	return resp, nil
}

func (r *Repository) DeleteMaxBidIntent(ctx context.Context, auctionID string, userID string, idempotencyKey string) (MaxBidIntentResponse, error) {
	if idempotencyKey == "" {
		return MaxBidIntentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "Idempotency-Key is required", http.StatusBadRequest)
	}
	requestHash := maxBidIntentCancelRequestHash(auctionID, userID, idempotencyKey)
	if replay, ok, err := r.completedMaxBidIntentIdempotency(ctx, auctionID, userID, idempotencyKey, requestHash); err != nil || ok {
		return replay, err
	}

	tx, err := r.beginTx(ctx)
	if err != nil {
		return MaxBidIntentResponse{}, err
	}
	defer rollback(ctx, tx)

	if err := upsertProcessing(ctx, tx, "max_bid_intent", auctionID, userID, idempotencyKey, requestHash); err != nil {
		return MaxBidIntentResponse{}, err
	}
	intent, err := r.cancelMaxBidIntentTx(ctx, tx, auctionID, userID)
	if err != nil {
		return MaxBidIntentResponse{}, err
	}
	resp := MaxBidIntentResponse{Result: MaxBidIntentResultCancelled, Intent: intent}
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		return MaxBidIntentResponse{}, err
	}
	if err := completeIdempotency(ctx, tx, "max_bid_intent", auctionID, userID, idempotencyKey, requestHash, http.StatusOK, resp.Result, responseJSON); err != nil {
		return MaxBidIntentResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MaxBidIntentResponse{}, err
	}
	return resp, nil
}

func (r *Repository) UpsertMaxBidIntent(ctx context.Context, auctionID string, userID string, input MaxBidIntentInput) (MaxBidIntent, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return MaxBidIntent{}, err
	}
	defer rollback(ctx, tx)
	intent, err := r.upsertMaxBidIntentTx(ctx, tx, auctionID, userID, input)
	if err != nil {
		return MaxBidIntent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MaxBidIntent{}, err
	}
	return intent, nil
}

func (r *Repository) upsertMaxBidIntentTx(ctx context.Context, tx pgx.Tx, auctionID string, userID string, input MaxBidIntentInput) (MaxBidIntent, error) {
	if userID == "" || auctionID == "" || input.MaxAmountCents <= 0 {
		return MaxBidIntent{}, apierrors.New(apierrors.CodeInvalidArgument, "auction_id, user_id, and positive max_amount_cents are required", http.StatusBadRequest)
	}
	source, err := normalizeMaxBidIntentSource(input.Source)
	if err != nil {
		return MaxBidIntent{}, err
	}

	rule, err := lockAuctionForMaxBidIntent(ctx, tx, auctionID)
	if err != nil {
		return MaxBidIntent{}, err
	}
	if err := validateMaxBidIntentAmount(rule, input.MaxAmountCents); err != nil {
		return MaxBidIntent{}, err
	}

	var intent MaxBidIntent
	err = tx.QueryRow(ctx, `
		INSERT INTO max_bid_intents (id, auction_id, user_id, max_amount_cents, status, source)
		VALUES ($1, $2, $3, $4, 'ACTIVE', $5)
		ON CONFLICT (auction_id, user_id)
		DO UPDATE SET max_amount_cents = EXCLUDED.max_amount_cents,
		              status = 'ACTIVE',
		              source = EXCLUDED.source,
		              updated_at = now(),
		              cancelled_at = NULL,
		              exhausted_at = NULL,
		              version = max_bid_intents.version + 1
		RETURNING id, auction_id, user_id, max_amount_cents, status, source,
		          created_at, updated_at, cancelled_at, exhausted_at, last_applied_seq, version
	`, "mbi_"+uuid.NewString(), auctionID, userID, input.MaxAmountCents, source).Scan(
		&intent.ID, &intent.AuctionID, &intent.UserID, &intent.MaxAmountCents, &intent.Status, &intent.Source,
		&intent.CreatedAt, &intent.UpdatedAt, &intent.CancelledAt, &intent.ExhaustedAt, &intent.LastAppliedSeq, &intent.Version,
	)
	if err != nil {
		return MaxBidIntent{}, mapPGError(err)
	}
	return intent, nil
}

func (r *Repository) GetMaxBidIntent(ctx context.Context, auctionID string, userID string) (MaxBidIntent, error) {
	var intent MaxBidIntent
	err := r.db.QueryRow(ctx, `
		SELECT id, auction_id, user_id, max_amount_cents, status, source,
		       created_at, updated_at, cancelled_at, exhausted_at, last_applied_seq, version
		FROM max_bid_intents
		WHERE auction_id = $1 AND user_id = $2
	`, auctionID, userID).Scan(
		&intent.ID, &intent.AuctionID, &intent.UserID, &intent.MaxAmountCents, &intent.Status, &intent.Source,
		&intent.CreatedAt, &intent.UpdatedAt, &intent.CancelledAt, &intent.ExhaustedAt, &intent.LastAppliedSeq, &intent.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MaxBidIntent{}, apierrors.New(apierrors.CodeAuctionNotFound, "max bid intent not found", http.StatusNotFound)
	}
	if err != nil {
		return MaxBidIntent{}, err
	}
	return intent, nil
}

func (r *Repository) CancelMaxBidIntent(ctx context.Context, auctionID string, userID string) (MaxBidIntent, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return MaxBidIntent{}, err
	}
	defer rollback(ctx, tx)

	intent, err := r.cancelMaxBidIntentTx(ctx, tx, auctionID, userID)
	if err != nil {
		return MaxBidIntent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MaxBidIntent{}, err
	}
	return intent, nil
}

func (r *Repository) cancelMaxBidIntentTx(ctx context.Context, tx pgx.Tx, auctionID string, userID string) (MaxBidIntent, error) {
	if _, err := lockAuctionForMaxBidIntent(ctx, tx, auctionID); err != nil {
		return MaxBidIntent{}, err
	}

	var intent MaxBidIntent
	err := tx.QueryRow(ctx, `
		UPDATE max_bid_intents
		SET status = 'CANCELLED',
		    cancelled_at = COALESCE(cancelled_at, now()),
		    exhausted_at = NULL,
		    updated_at = now(),
		    version = version + 1
		WHERE auction_id = $1 AND user_id = $2 AND status = 'ACTIVE'
		RETURNING id, auction_id, user_id, max_amount_cents, status, source,
		          created_at, updated_at, cancelled_at, exhausted_at, last_applied_seq, version
	`, auctionID, userID).Scan(
		&intent.ID, &intent.AuctionID, &intent.UserID, &intent.MaxAmountCents, &intent.Status, &intent.Source,
		&intent.CreatedAt, &intent.UpdatedAt, &intent.CancelledAt, &intent.ExhaustedAt, &intent.LastAppliedSeq, &intent.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MaxBidIntent{}, apierrors.New(apierrors.CodeAuctionNotFound, "active max bid intent not found", http.StatusNotFound)
	}
	if err != nil {
		return MaxBidIntent{}, err
	}
	return intent, nil
}

func (r *Repository) ListActiveMaxBidIntentsForAuction(ctx context.Context, tx pgx.Tx, auctionID string, limit int) ([]MaxBidIntent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := tx.Query(ctx, `
		SELECT id, auction_id, user_id, max_amount_cents, status, source,
		       created_at, updated_at, cancelled_at, exhausted_at, last_applied_seq, version
		FROM max_bid_intents
		WHERE auction_id = $1 AND status = 'ACTIVE'
		ORDER BY max_amount_cents DESC, created_at ASC, id ASC
		LIMIT $2
		FOR UPDATE
	`, auctionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	intents := make([]MaxBidIntent, 0, limit)
	for rows.Next() {
		var intent MaxBidIntent
		if err := rows.Scan(
			&intent.ID, &intent.AuctionID, &intent.UserID, &intent.MaxAmountCents, &intent.Status, &intent.Source,
			&intent.CreatedAt, &intent.UpdatedAt, &intent.CancelledAt, &intent.ExhaustedAt, &intent.LastAppliedSeq, &intent.Version,
		); err != nil {
			return nil, err
		}
		intents = append(intents, intent)
	}
	return intents, rows.Err()
}

type maxBidIntentAuctionRule struct {
	Status            Status
	StartPriceCents   int64
	CurrentPriceCents int64
	IncrementCents    int64
	CapPriceCents     *int64
	AcceptedBidCount  int64
}

func lockAuctionForMaxBidIntent(ctx context.Context, tx pgx.Tx, auctionID string) (maxBidIntentAuctionRule, error) {
	var rule maxBidIntentAuctionRule
	err := tx.QueryRow(ctx, `
		SELECT status, start_price_cents, current_price_cents, increment_cents, cap_price_cents, accepted_bid_count
		FROM auctions
		WHERE id = $1
		FOR UPDATE
	`, auctionID).Scan(
		&rule.Status, &rule.StartPriceCents, &rule.CurrentPriceCents, &rule.IncrementCents, &rule.CapPriceCents, &rule.AcceptedBidCount,
	)
	if err != nil {
		return maxBidIntentAuctionRule{}, mapNotFound(err)
	}
	if rule.Status == StatusSold || rule.Status == StatusEnded || rule.Status == StatusCancelled {
		return maxBidIntentAuctionRule{}, apierrors.New(apierrors.CodeAuctionNotActive, "terminal auction cannot accept max bid intent", http.StatusConflict)
	}
	if rule.Status != StatusScheduled && rule.Status != StatusActive {
		return maxBidIntentAuctionRule{}, apierrors.New(apierrors.CodeAuctionNotActive, "max bid intent requires SCHEDULED or ACTIVE auction", http.StatusConflict)
	}
	return rule, nil
}

func validateMaxBidIntentAmount(rule maxBidIntentAuctionRule, amountCents int64) error {
	class := ClassifyBidAmount(rule.StartPriceCents, rule.CurrentPriceCents, rule.IncrementCents, rule.CapPriceCents, amountCents, rule.AcceptedBidCount > 0)
	switch class {
	case BidClassTooLow:
		return apierrors.New(apierrors.CodeMaxBidTooLow, "max_amount_cents is below the current executable minimum", http.StatusBadRequest)
	case BidClassIncrementMismatch:
		return apierrors.New(apierrors.CodeMaxBidIncrementMismatch, "max_amount_cents must align to the auction increment grid", http.StatusBadRequest)
	case BidClassAboveCap:
		return apierrors.New(apierrors.CodeMaxBidAboveCap, "max_amount_cents exceeds cap_price_cents", http.StatusBadRequest)
	default:
		return nil
	}
}

func (r *Repository) completedMaxBidIntentIdempotency(ctx context.Context, auctionID string, userID string, idempotencyKey string, requestHash string) (MaxBidIntentResponse, bool, error) {
	var storedHash string
	var status string
	var responseJSON []byte
	var lockedUntil *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT request_hash, status, response_json, locked_until
		FROM idempotency_records
		WHERE scope_type = 'max_bid_intent' AND scope_id = $1 AND user_id = $2 AND idempotency_key = $3
	`, auctionID, userID, idempotencyKey).Scan(&storedHash, &status, &responseJSON, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return MaxBidIntentResponse{}, false, nil
	}
	if err != nil {
		return MaxBidIntentResponse{}, false, err
	}
	if storedHash != requestHash {
		return MaxBidIntentResponse{}, false, apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "idempotency key reused with different request", http.StatusConflict)
	}
	if status == IdempotencyStatusProcessing && lockedUntil != nil && time.Now().UTC().After(*lockedUntil) {
		return MaxBidIntentResponse{}, false, r.markIdempotencyTimeout(ctx, "max_bid_intent", auctionID, userID, idempotencyKey)
	}
	if status != IdempotencyStatusCompleted {
		return MaxBidIntentResponse{}, false, apierrors.New(apierrors.CodeProcessingRetryLater, "same idempotency key is still processing", http.StatusConflict)
	}
	var resp MaxBidIntentResponse
	if err := json.Unmarshal(responseJSON, &resp); err != nil {
		return MaxBidIntentResponse{}, false, err
	}
	return resp, true, nil
}

func normalizeMaxBidIntentSource(source MaxBidIntentSource) (MaxBidIntentSource, error) {
	if source == "" {
		return MaxBidIntentSourceMaxBid, nil
	}
	if source != MaxBidIntentSourceMaxBid && source != MaxBidIntentSourcePreBid {
		return "", apierrors.New(apierrors.CodeInvalidArgument, "source must be MAX_BID or PRE_BID", http.StatusBadRequest)
	}
	return source, nil
}

func maxBidIntentRequestHash(auctionID string, userID string, idempotencyKey string, maxAmountCents int64, source MaxBidIntentSource) string {
	return maxBidHashString(fmt.Sprintf("max-bid-intent:v2|%s|%s|%s|%d|%s", auctionID, userID, idempotencyKey, maxAmountCents, source))
}

func maxBidIntentCancelRequestHash(auctionID string, userID string, idempotencyKey string) string {
	return maxBidHashString(fmt.Sprintf("max-bid-intent-cancel:v1|%s|%s|%s", auctionID, userID, idempotencyKey))
}

func maxBidHashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
