package auction

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	apierrors "live-auction/backend/internal/platform/errors"
)

func TestActiveRaceOnlyOneScheduledAuctionStarts(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	roomID := createTestRoom(t, db)
	first := createScheduledAuctionInRoom(t, repo, roomID)
	second := createScheduledAuctionInRoom(t, repo, roomID)

	errs := runConcurrently(2, func(i int) error {
		id := first.ID
		if i == 1 {
			id = second.ID
		}
		_, err := repo.Start(ctx, id, fmt.Sprintf("tr_active_%d", i))
		return err
	})

	successes := 0
	conflicts := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if hasCode(err, apierrors.CodeConflictRoomHasActiveAuction) || hasCode(err, apierrors.CodeBidRetryLater) {
			conflicts++
			continue
		}
		t.Fatalf("unexpected start race err: %v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("start race successes/conflicts = %d/%d, want 1/1", successes, conflicts)
	}
	var activeCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auctions WHERE room_id = $1 AND status = 'ACTIVE'`, roomID).Scan(&activeCount); err != nil {
		t.Fatalf("count active: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active count = %d, want 1", activeCount)
	}
}

func TestCancelCapRaceProducesExactlyOneTerminalState(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	capPrice := int64(20_000)
	row := createActiveAuction(t, repo, db, &capPrice)

	errs := runConcurrently(2, func(i int) error {
		if i == 0 {
			_, err := repo.Cancel(ctx, row.ID, "tr_cancel_cap")
			return err
		}
		input := BidInput{ClientBidID: "cap-race-bid", AmountCents: capPrice}
		resp, err := repo.PlaceBid(ctx, row.ID, "user_1", input.ClientBidID, input, "tr_cancel_cap")
		if err != nil {
			return err
		}
		if resp.Result != BidResultAcceptedSold && resp.Result != BidResultRejected {
			return fmt.Errorf("unexpected bid result %s", resp.Result)
		}
		return nil
	})
	for _, err := range errs {
		if err != nil && !hasCode(err, apierrors.CodeBidRetryLater) && !hasCode(err, apierrors.CodeInvalidArgument) {
			t.Fatalf("cancel-cap err = %v", err)
		}
	}
	got, err := repo.GetAuction(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetAuction: %v", err)
	}
	if got.Status != StatusCancelled && got.Status != StatusSold {
		t.Fatalf("status = %s, want CANCELLED or SOLD", got.Status)
	}
	var terminalEvents int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND event_type IN ('auction_cancelled','auction_sold')`, row.ID).Scan(&terminalEvents); err != nil {
		t.Fatalf("count terminal events: %v", err)
	}
	if terminalEvents != 1 {
		t.Fatalf("terminal events = %d, want 1", terminalEvents)
	}
	var orders int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE auction_id = $1`, row.ID).Scan(&orders); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if got.Status == StatusSold && orders != 1 {
		t.Fatalf("sold orders = %d, want 1", orders)
	}
	if got.Status == StatusCancelled && orders != 0 {
		t.Fatalf("cancelled orders = %d, want 0", orders)
	}
}

func TestConcurrentFinalSecondBidsKeepContinuousSeqAndOneWinner(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	row := createActiveAuction(t, repo, db, nil)
	if _, err := db.Exec(ctx, `UPDATE auctions SET end_at = now() + interval '2 seconds' WHERE id = $1`, row.ID); err != nil {
		t.Fatalf("force final second: %v", err)
	}
	for i := 0; i < 8; i++ {
		userID := fmt.Sprintf("user_final_%d", i)
		if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ($1, 'user', $2) ON CONFLICT DO NOTHING`, userID, userID); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	errs := runConcurrently(8, func(i int) error {
		userID := fmt.Sprintf("user_final_%d", i)
		amount := int64(15_000 + i*5_000)
		input := BidInput{ClientBidID: fmt.Sprintf("final-bid-%d", i), AmountCents: amount}
		_, err := repo.PlaceBid(ctx, row.ID, userID, input.ClientBidID, input, "tr_final_second")
		return err
	})
	for _, err := range errs {
		if err != nil && !hasCode(err, apierrors.CodeBidRetryLater) {
			t.Fatalf("final-second err = %v", err)
		}
	}
	assertAcceptedBidSeqContinuous(t, db, row.ID)
	got, err := repo.GetAuction(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetAuction: %v", err)
	}
	if got.CurrentWinnerID == nil {
		t.Fatalf("expected one current winner")
	}
}

func TestNarrateRaceOnlyOneAuctionNarrating(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	roomID := createTestRoom(t, db)
	first := createScheduledAuctionInRoom(t, repo, roomID)
	second := createScheduledAuctionInRoom(t, repo, roomID)

	errs := runConcurrently(2, func(i int) error {
		id := first.ID
		if i == 1 {
			id = second.ID
		}
		_, err := repo.NarrateStart(ctx, id, fmt.Sprintf("tr_narrate_%d", i))
		return err
	})
	successes := 0
	conflicts := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if hasCode(err, apierrors.CodeConflictRoomHasNarration) || hasCode(err, apierrors.CodeBidRetryLater) {
			conflicts++
			continue
		}
		t.Fatalf("unexpected narrate race err: %v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("narrate successes/conflicts = %d/%d, want 1/1", successes, conflicts)
	}
	var narrating int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auctions WHERE room_id = $1 AND is_narrating`, roomID).Scan(&narrating); err != nil {
		t.Fatalf("count narrating: %v", err)
	}
	if narrating != 1 {
		t.Fatalf("narrating count = %d, want 1", narrating)
	}
}

func createScheduledAuctionInRoom(t *testing.T, repo *Repository, roomID string) Auction {
	t.Helper()
	item, err := repo.CreateItem(context.Background(), CreateItemInput{Title: "Concurrency Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	row, err := repo.CreateAuction(context.Background(), CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		Rule:            validRule(),
	}, "tr_concurrency")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	row, err = repo.Schedule(context.Background(), row.ID, nil, "tr_concurrency")
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	return row
}

func runConcurrently(count int, fn func(int) error) []error {
	errs := make([]error, count)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			errs[i] = fn(i)
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

func assertAcceptedBidSeqContinuous(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT seq FROM bids WHERE auction_id = $1 AND status = 'ACCEPTED' ORDER BY seq`, auctionID)
	if err != nil {
		t.Fatalf("select accepted seq: %v", err)
	}
	defer rows.Close()
	var previous int64
	seen := 0
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			t.Fatalf("scan seq: %v", err)
		}
		if previous != 0 && seq <= previous {
			t.Fatalf("seq not increasing: %d after %d", seq, previous)
		}
		previous = seq
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if seen == 0 {
		t.Fatalf("expected at least one accepted bid")
	}
}
