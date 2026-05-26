package auction

import (
	"context"
	"testing"
)

func TestMaxBidIntentRepositoryUpsertCancelAndUserScope(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)
	userID := createTestUser(t, db)

	intent, err := repo.UpsertMaxBidIntent(ctx, auction.ID, userID, MaxBidIntentInput{
		MaxAmountCents: 25_000,
		ClientSeenSeq:  auction.Seq,
		Source:         MaxBidIntentSourceMaxBid,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent create: %v", err)
	}
	if intent.Status != MaxBidIntentStatusActive || intent.Source != MaxBidIntentSourceMaxBid || intent.MaxAmountCents != 25_000 {
		t.Fatalf("unexpected intent: %#v", intent)
	}
	if intent.Version != 0 {
		t.Fatalf("version = %d, want 0 on create", intent.Version)
	}

	updated, err := repo.UpsertMaxBidIntent(ctx, auction.ID, userID, MaxBidIntentInput{
		MaxAmountCents: 30_000,
		ClientSeenSeq:  auction.Seq,
		Source:         MaxBidIntentSourcePreBid,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent update: %v", err)
	}
	if updated.ID != intent.ID {
		t.Fatalf("updated id = %s, want same %s", updated.ID, intent.ID)
	}
	if updated.Version != intent.Version+1 || updated.MaxAmountCents != 30_000 || updated.Source != MaxBidIntentSourcePreBid {
		t.Fatalf("unexpected updated intent: %#v", updated)
	}

	got, err := repo.GetMaxBidIntent(ctx, auction.ID, userID)
	if err != nil {
		t.Fatalf("GetMaxBidIntent: %v", err)
	}
	if got.ID != intent.ID || got.UserID != userID {
		t.Fatalf("unexpected current-user intent: %#v", got)
	}

	cancelled, err := repo.CancelMaxBidIntent(ctx, auction.ID, userID)
	if err != nil {
		t.Fatalf("CancelMaxBidIntent: %v", err)
	}
	if cancelled.Status != MaxBidIntentStatusCancelled || cancelled.CancelledAt == nil {
		t.Fatalf("unexpected cancelled intent: %#v", cancelled)
	}

	reactivated, err := repo.UpsertMaxBidIntent(ctx, auction.ID, userID, MaxBidIntentInput{
		MaxAmountCents: 35_000,
		ClientSeenSeq:  auction.Seq,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent reactivate: %v", err)
	}
	if reactivated.Status != MaxBidIntentStatusActive || reactivated.CancelledAt != nil || reactivated.MaxAmountCents != 35_000 {
		t.Fatalf("unexpected reactivated intent: %#v", reactivated)
	}
}

func TestMaxBidIntentRepositoryValidatesExecutableGridAndTerminalState(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	capPrice := int64(30_000)
	auction := createActiveAuction(t, repo, db, &capPrice)
	userID := createTestUser(t, db)

	_, err := repo.UpsertMaxBidIntent(ctx, auction.ID, userID, MaxBidIntentInput{MaxAmountCents: 12_000})
	if !hasCode(err, "MAX_BID_TOO_LOW") {
		t.Fatalf("too-low err = %v, want MAX_BID_TOO_LOW", err)
	}

	_, err = repo.UpsertMaxBidIntent(ctx, auction.ID, userID, MaxBidIntentInput{MaxAmountCents: 17_000})
	if !hasCode(err, "MAX_BID_INCREMENT_MISMATCH") {
		t.Fatalf("increment err = %v, want MAX_BID_INCREMENT_MISMATCH", err)
	}

	_, err = repo.UpsertMaxBidIntent(ctx, auction.ID, userID, MaxBidIntentInput{MaxAmountCents: 35_000})
	if !hasCode(err, "MAX_BID_ABOVE_CAP") {
		t.Fatalf("above-cap err = %v, want MAX_BID_ABOVE_CAP", err)
	}

	if _, err := repo.Cancel(ctx, auction.ID, CancelInput{Reason: "terminal"}, "tr_max_bid_terminal"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	_, err = repo.UpsertMaxBidIntent(ctx, auction.ID, userID, MaxBidIntentInput{MaxAmountCents: 25_000})
	if !hasCode(err, "AUCTION_NOT_ACTIVE") {
		t.Fatalf("terminal err = %v, want AUCTION_NOT_ACTIVE", err)
	}
}

func TestListActiveMaxBidIntentsForAuctionOrdersPrivateCandidates(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)
	firstUser := createTestUser(t, db)
	secondUser := createTestUser(t, db)
	thirdUser := createTestUser(t, db)

	first, err := repo.UpsertMaxBidIntent(ctx, auction.ID, firstUser, MaxBidIntentInput{MaxAmountCents: 30_000})
	if err != nil {
		t.Fatalf("first intent: %v", err)
	}
	second, err := repo.UpsertMaxBidIntent(ctx, auction.ID, secondUser, MaxBidIntentInput{MaxAmountCents: 35_000})
	if err != nil {
		t.Fatalf("second intent: %v", err)
	}
	third, err := repo.UpsertMaxBidIntent(ctx, auction.ID, thirdUser, MaxBidIntentInput{MaxAmountCents: 30_000})
	if err != nil {
		t.Fatalf("third intent: %v", err)
	}
	if _, err := repo.CancelMaxBidIntent(ctx, auction.ID, thirdUser); err != nil {
		t.Fatalf("cancel third: %v", err)
	}

	tx, err := repo.beginTx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer rollback(ctx, tx)
	intents, err := repo.ListActiveMaxBidIntentsForAuction(ctx, tx, auction.ID, 10)
	if err != nil {
		t.Fatalf("ListActiveMaxBidIntentsForAuction: %v", err)
	}
	if len(intents) != 2 {
		t.Fatalf("intents len = %d, want 2: %#v", len(intents), intents)
	}
	if intents[0].ID != second.ID || intents[1].ID != first.ID {
		t.Fatalf("unexpected order: got %s,%s want %s,%s", intents[0].ID, intents[1].ID, second.ID, first.ID)
	}
	if intents[0].ID == third.ID || intents[1].ID == third.ID {
		t.Fatalf("cancelled intent leaked into active candidates: %#v", intents)
	}
}

func TestMaxBidIntentRepositoryIdempotency(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)
	userID := createTestUser(t, db)

	first, err := repo.PutMaxBidIntent(ctx, auction.ID, userID, "intent-idem-1", MaxBidIntentInput{MaxAmountCents: 25_000})
	if err != nil {
		t.Fatalf("PutMaxBidIntent first: %v", err)
	}
	replay, err := repo.PutMaxBidIntent(ctx, auction.ID, userID, "intent-idem-1", MaxBidIntentInput{MaxAmountCents: 25_000})
	if err != nil {
		t.Fatalf("PutMaxBidIntent replay: %v", err)
	}
	if replay.Intent.ID != first.Intent.ID || replay.Intent.Version != first.Intent.Version {
		t.Fatalf("put replay mutated intent: got %#v want %#v", replay.Intent, first.Intent)
	}

	_, err = repo.PutMaxBidIntent(ctx, auction.ID, userID, "intent-idem-1", MaxBidIntentInput{MaxAmountCents: 30_000})
	if !hasCode(err, "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST") {
		t.Fatalf("changed put err = %v, want idempotency hash mismatch", err)
	}

	cancel, err := repo.DeleteMaxBidIntent(ctx, auction.ID, userID, "intent-delete-idem-1")
	if err != nil {
		t.Fatalf("DeleteMaxBidIntent first: %v", err)
	}
	cancelReplay, err := repo.DeleteMaxBidIntent(ctx, auction.ID, userID, "intent-delete-idem-1")
	if err != nil {
		t.Fatalf("DeleteMaxBidIntent replay: %v", err)
	}
	if cancelReplay.Intent.ID != cancel.Intent.ID || cancelReplay.Intent.Version != cancel.Intent.Version {
		t.Fatalf("delete replay mutated intent: got %#v want %#v", cancelReplay.Intent, cancel.Intent)
	}
}
