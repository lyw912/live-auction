package auction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	apierrors "live-auction/backend/internal/platform/errors"
)

func TestRepositoryCreateScheduleStartLifecycle(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	roomID := createTestRoom(t, db)

	item, err := repo.CreateItem(ctx, CreateItemInput{Title: "Lifecycle Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	auction, err := repo.CreateAuction(ctx, CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		CapPriceCents:   ptrInt64(30_000),
		Rule:            validRule(),
	}, "tr_test")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if auction.Status != StatusDraft {
		t.Fatalf("status = %s, want DRAFT", auction.Status)
	}
	if auction.Seq != 1 {
		t.Fatalf("seq = %d, want 1 after create event", auction.Seq)
	}

	updatedRule := validRule()
	updatedRule.DurationSeconds = 120
	auction, err = repo.UpdateRules(ctx, auction.ID, UpdateRulesInput{
		StartPriceCents: 20_000,
		IncrementCents:  10_000,
		CapPriceCents:   ptrInt64(50_000),
		Rule:            updatedRule,
	}, "tr_test")
	if err != nil {
		t.Fatalf("UpdateRules: %v", err)
	}
	if auction.RuleVersion != 2 {
		t.Fatalf("rule_version = %d, want 2", auction.RuleVersion)
	}
	if auction.StartPriceCents != 20_000 || auction.IncrementCents != 10_000 || auction.CapPriceCents == nil || *auction.CapPriceCents != 50_000 || auction.CurrentPriceCents != 20_000 {
		t.Fatalf("rule money fields not updated: %#v", auction)
	}

	auction, err = repo.Schedule(ctx, auction.ID, nil, "tr_test")
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if auction.Status != StatusScheduled {
		t.Fatalf("status = %s, want SCHEDULED", auction.Status)
	}
	if auction.Rule.FrozenAt == nil {
		t.Fatalf("expected rule frozen_at after schedule")
	}

	_, err = repo.UpdateRules(ctx, auction.ID, UpdateRulesInput{
		StartPriceCents: auction.StartPriceCents,
		IncrementCents:  auction.IncrementCents,
		CapPriceCents:   auction.CapPriceCents,
		Rule:            validRule(),
	}, "tr_test")
	if !hasCode(err, apierrors.CodeRuleFrozenAfterScheduled) {
		t.Fatalf("UpdateRules after schedule err = %v, want RULE_FROZEN_AFTER_SCHEDULED", err)
	}

	auction, err = repo.Start(ctx, auction.ID, "tr_test")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if auction.Status != StatusActive {
		t.Fatalf("status = %s, want ACTIVE", auction.Status)
	}
	if auction.StartAt == nil || auction.EndAt == nil {
		t.Fatalf("start_at/end_at must be set after start")
	}
	if auction.ServerTimeMS <= 0 {
		t.Fatalf("server_time_ms = %d, want positive server timestamp", auction.ServerTimeMS)
	}

	assertEventOutboxCounts(t, db, auction.ID, 4)
}

func TestRepositoryRejectsUnreachableCap(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	roomID := createTestRoom(t, db)

	item, err := repo.CreateItem(ctx, CreateItemInput{Title: "Invalid Cap Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	_, err = repo.CreateAuction(ctx, CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  10_000,
		CapPriceCents:   ptrInt64(35_000),
		Rule:            validRule(),
	}, "tr_test")
	if !hasCode(err, apierrors.CodeInvalidAuctionRuleCapUnreachable) {
		t.Fatalf("CreateAuction err = %v, want INVALID_AUCTION_RULE_CAP_UNREACHABLE", err)
	}
}

func TestRepositoryCancelStoresReasonInEvent(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	roomID := createTestRoom(t, db)

	item, err := repo.CreateItem(ctx, CreateItemInput{Title: "Cancel Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	auction, err := repo.CreateAuction(ctx, CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		CapPriceCents:   ptrInt64(30_000),
		Rule:            validRule(),
	}, "tr_cancel_reason")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}

	cancelled, err := repo.Cancel(ctx, auction.ID, CancelInput{Reason: "主播临时下架"}, "tr_cancel_reason")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", cancelled.Status)
	}
	var reason string
	if err := db.QueryRow(ctx, `
		SELECT payload_json->>'reason'
		FROM auction_events
		WHERE auction_id = $1 AND event_type = 'auction_cancelled'
		ORDER BY seq DESC
		LIMIT 1
	`, auction.ID).Scan(&reason); err != nil {
		t.Fatalf("read cancel event reason: %v", err)
	}
	if reason != "主播临时下架" {
		t.Fatalf("cancel reason = %q", reason)
	}
}

func TestRepositoryLeaderboardActionMetrics(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)
	user2 := createTestUser(t, db)
	user3 := createTestUser(t, db)

	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, user2, "lb-bid-1", BidInput{ClientBidID: "lb-bid-1", AmountCents: 15_000}, "tr_lb"); err != nil {
		t.Fatalf("PlaceBid user2: %v", err)
	}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, user3, "lb-bid-2", BidInput{ClientBidID: "lb-bid-2", AmountCents: 20_000}, "tr_lb"); err != nil {
		t.Fatalf("PlaceBid user3: %v", err)
	}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", "lb-bid-3", BidInput{ClientBidID: "lb-bid-3", AmountCents: 25_000}, "tr_lb"); err != nil {
		t.Fatalf("PlaceBid user1: %v", err)
	}

	board, err := repo.GetLeaderboard(ctx, auction.ID, user2, 5)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if board.Seq < 6 {
		t.Fatalf("seq = %d, want latest auction seq after bids", board.Seq)
	}
	if board.ServerTimeMS <= 0 {
		t.Fatalf("server_time_ms = %d, want positive server timestamp", board.ServerTimeMS)
	}
	if board.NextValidBidCents != 30_000 {
		t.Fatalf("next_valid_bid_cents = %d, want 30000", board.NextValidBidCents)
	}
	if board.State != "OUTBID" {
		t.Fatalf("state = %s, want OUTBID", board.State)
	}
	if board.MyRank == nil || *board.MyRank != 3 {
		t.Fatalf("my_rank = %v, want 3", board.MyRank)
	}
	var myEntry *LeaderboardEntry
	for i := range board.Entries {
		if board.Entries[i].IsCurrent {
			myEntry = &board.Entries[i]
			break
		}
	}
	if myEntry == nil || myEntry.UserMasked != "匿名用户 1" {
		t.Fatalf("current entry = %#v, want stable anonymous label for first bidder", myEntry)
	}
	if board.GapToLeaderCents == nil || *board.GapToLeaderCents != 10_000 {
		t.Fatalf("gap_to_leader = %v, want 10000", board.GapToLeaderCents)
	}
	if board.GapToNextRankCents == nil || *board.GapToNextRankCents != 5_000 {
		t.Fatalf("gap_to_next_rank = %v, want 5000", board.GapToNextRankCents)
	}
	if board.AcceptedBidderCount != 3 || board.ActiveBidders30s != 3 || board.AcceptedBids30s != 3 {
		t.Fatalf("stats mismatch: %#v", board)
	}
	if board.PriceVelocityCPM != 20_000 {
		t.Fatalf("price_velocity_cents_per_min = %d, want 20000", board.PriceVelocityCPM)
	}

	leading, err := repo.GetLeaderboard(ctx, auction.ID, "user_1", 5)
	if err != nil {
		t.Fatalf("GetLeaderboard leading: %v", err)
	}
	if leading.State != "LEADING" {
		t.Fatalf("leading state = %s, want LEADING", leading.State)
	}
	if leading.GapToLeaderCents == nil || *leading.GapToLeaderCents != 0 {
		t.Fatalf("leading gap_to_leader = %v, want 0", leading.GapToLeaderCents)
	}
}

func TestRepositoryLeaderboardIncludesCurrentUserOutsideTopLimit(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)
	viewer := "user_1"
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, viewer, "lb-self-outside-top", BidInput{ClientBidID: "lb-self-outside-top", AmountCents: 15_000}, "tr_lb_self"); err != nil {
		t.Fatalf("PlaceBid viewer: %v", err)
	}
	for i, amount := range []int64{20_000, 25_000, 30_000} {
		userID := createTestUser(t, db)
		clientBidID := fmt.Sprintf("lb-other-%d", i)
		if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, userID, clientBidID, BidInput{ClientBidID: clientBidID, AmountCents: amount}, "tr_lb_self"); err != nil {
			t.Fatalf("PlaceBid other %d: %v", i, err)
		}
	}

	board, err := repo.GetLeaderboard(ctx, auction.ID, viewer, 2)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if board.MyRank == nil || *board.MyRank != 4 {
		t.Fatalf("my_rank = %v, want 4", board.MyRank)
	}
	var foundCurrent bool
	for _, entry := range board.Entries {
		if entry.IsCurrent {
			foundCurrent = true
			if entry.Rank != 4 || entry.UserMasked != "匿名用户 1" {
				t.Fatalf("current entry = %#v, want rank 4 stable anonymous user 1", entry)
			}
		}
	}
	if !foundCurrent {
		t.Fatalf("leaderboard entries did not include current user outside top limit: %#v", board.Entries)
	}
}

func TestRepositoryLeaderboardMarksInconsistentAuctionStateRecovering(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)

	disableAuctionProjectionTriggersForTest(t, db)
	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET current_price_cents = 35000,
		    current_winner_id = 'user_2',
		    accepted_bid_count = 1,
		    seq = 41
		WHERE id = $1
	`, auction.ID); err != nil {
		t.Fatalf("corrupt auction state: %v", err)
	}

	board, err := repo.GetLeaderboard(ctx, auction.ID, "user_1", 5)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if board.State != "RECOVERING" {
		t.Fatalf("state = %s, want RECOVERING for winner without accepted bid row", board.State)
	}
	if board.LeaderAmountCents != 35000 {
		t.Fatalf("leader_amount_cents = %d, want fallback current price", board.LeaderAmountCents)
	}
	if len(board.Entries) != 0 {
		t.Fatalf("entries = %#v, want no fabricated leaderboard rows", board.Entries)
	}
}

func TestRepositoryAuctionProjectionConstraintRejectsMissingAcceptedBidFact(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)

	_, err := db.Exec(ctx, `
		UPDATE auctions
		SET current_price_cents = 35000,
		    current_winner_id = 'user_2',
		    accepted_bid_count = 1,
		    seq = 41
		WHERE id = $1
	`, auction.ID)
	if err == nil {
		t.Fatalf("corrupt auction projection update succeeded, want constraint violation")
	}
}

func disableAuctionProjectionTriggersForTest(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`ALTER TABLE auctions DISABLE TRIGGER auctions_projection_matches_bid_facts`,
		`ALTER TABLE bids DISABLE TRIGGER bids_projection_matches_auction`,
	} {
		if _, err := db.Exec(ctx, stmt); err != nil {
			t.Fatalf("disable auction projection trigger: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, stmt := range []string{
			`ALTER TABLE bids ENABLE TRIGGER bids_projection_matches_auction`,
			`ALTER TABLE auctions ENABLE TRIGGER auctions_projection_matches_bid_facts`,
		} {
			if _, err := db.Exec(context.Background(), stmt); err != nil {
				t.Fatalf("enable auction projection trigger: %v", err)
			}
		}
	})
}

func openIntegrationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func createTestRoom(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	roomID := "room_test_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO rooms (id, host_id, status)
		VALUES ($1, 'host_1', 'OPEN')
	`, roomID); err != nil {
		t.Fatalf("insert test room: %v", err)
	}
	return roomID
}

func validRule() Rule {
	return Rule{
		DurationSeconds:     60,
		ExtendWindowSeconds: 10,
		ExtendBySeconds:     10,
		MaxExtendCount:      3,
		DepositBPS:          1000,
		DepositFloorCents:   10_000,
		DepositCapCents:     100_000_000,
	}
}

func ptrInt64(value int64) *int64 {
	return &value
}

func hasCode(err error, code apierrors.Code) bool {
	var apiErr apierrors.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == code
	}
	return false
}

func assertEventOutboxCounts(t *testing.T, db *pgxpool.Pool, auctionID string, want int) {
	t.Helper()
	ctx := context.Background()
	var eventCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auction_events WHERE auction_id = $1`, auctionID).Scan(&eventCount); err != nil {
		t.Fatalf("count auction_events: %v", err)
	}
	if eventCount != want {
		t.Fatalf("auction_events count = %d, want %d", eventCount, want)
	}
	var outboxCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE auction_id = $1`, auctionID).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox_events: %v", err)
	}
	if outboxCount != want {
		t.Fatalf("outbox_events count = %d, want %d", outboxCount, want)
	}
}
