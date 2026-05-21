package auction

import (
	"context"
	"testing"
	"time"

	apierrors "live-auction/backend/internal/platform/errors"
)

func TestBidDBLockTimeoutReturnsRetryLaterWithoutDuplicate(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	row := createActiveAuction(t, repo, db, nil)

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM auctions WHERE id = $1 FOR UPDATE`, row.ID); err != nil {
		t.Fatalf("lock auction: %v", err)
	}

	input := BidInput{ClientBidID: "lock-timeout-bid", AmountCents: 15_000}
	_, err = repo.PlaceBid(ctx, row.ID, "user_1", input.ClientBidID, input, "tr_lock_timeout")
	if !hasCode(err, apierrors.CodeBidRetryLater) {
		t.Fatalf("PlaceBid err = %v, want BID_RETRY_LATER", err)
	}
	var bids int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND client_bid_id = $2`, row.ID, input.ClientBidID).Scan(&bids); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if bids != 0 {
		t.Fatalf("bids = %d, want 0", bids)
	}
}

func TestExpiredProcessingIdempotencyReturnsTimeoutAndMarksFailed(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	row := createActiveAuction(t, repo, db, nil)
	input := BidInput{ClientBidID: "stuck-processing-bid", AmountCents: 15_000}
	requestHash := bidRequestHash(row.ID, "user_1", input.ClientBidID, input.AmountCents)
	if _, err := db.Exec(ctx, `
		INSERT INTO idempotency_records (scope_type, scope_id, user_id, idempotency_key, request_hash, status, attempts, locked_until)
		VALUES ('bid', $1, 'user_1', $2, $3, 'PROCESSING', 1, $4)
	`, row.ID, input.ClientBidID, requestHash, time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatalf("insert stuck idempotency: %v", err)
	}

	_, err := repo.PlaceBid(ctx, row.ID, "user_1", input.ClientBidID, input, "tr_idem_timeout")
	if !hasCode(err, apierrors.CodeIdempotencyTimeout) {
		t.Fatalf("PlaceBid err = %v, want IDEMPOTENCY_TIMEOUT", err)
	}
	var status string
	var resultCode string
	if err := db.QueryRow(ctx, `
		SELECT status, result_code
		FROM idempotency_records
		WHERE scope_type = 'bid' AND scope_id = $1 AND user_id = 'user_1' AND idempotency_key = $2
	`, row.ID, input.ClientBidID).Scan(&status, &resultCode); err != nil {
		t.Fatalf("select idempotency: %v", err)
	}
	if status != "FAILED" || resultCode != string(apierrors.CodeIdempotencyTimeout) {
		t.Fatalf("idempotency = %s/%s, want FAILED/IDEMPOTENCY_TIMEOUT", status, resultCode)
	}
}
