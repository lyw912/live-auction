package auction

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	apierrors "live-auction/backend/internal/platform/errors"
)

func (r *Repository) UpsertMaxBidIntent(ctx context.Context, auctionID string, userID string, input MaxBidIntentInput) (MaxBidIntent, error) {
	if userID == "" || auctionID == "" || input.MaxAmountCents <= 0 {
		return MaxBidIntent{}, apierrors.New(apierrors.CodeInvalidArgument, "auction_id, user_id, and positive max_amount_cents are required", http.StatusBadRequest)
	}
	source := input.Source
	if source == "" {
		source = MaxBidIntentSourceMaxBid
	}
	if source != MaxBidIntentSourceMaxBid && source != MaxBidIntentSourcePreBid {
		return MaxBidIntent{}, apierrors.New(apierrors.CodeInvalidArgument, "source must be MAX_BID or PRE_BID", http.StatusBadRequest)
	}

	tx, err := r.beginTx(ctx)
	if err != nil {
		return MaxBidIntent{}, err
	}
	defer rollback(ctx, tx)

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
	if err := tx.Commit(ctx); err != nil {
		return MaxBidIntent{}, err
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

	if _, err := lockAuctionForMaxBidIntent(ctx, tx, auctionID); err != nil {
		return MaxBidIntent{}, err
	}

	var intent MaxBidIntent
	err = tx.QueryRow(ctx, `
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
	if err := tx.Commit(ctx); err != nil {
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
