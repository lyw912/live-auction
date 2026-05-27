package auction

import (
	"context"
	"errors"
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

	if _, err := repo.PlaceBid(ctx, auction.ID, user2, "lb-bid-1", BidInput{ClientBidID: "lb-bid-1", AmountCents: 15_000}, "tr_lb"); err != nil {
		t.Fatalf("PlaceBid user2: %v", err)
	}
	if _, err := repo.PlaceBid(ctx, auction.ID, user3, "lb-bid-2", BidInput{ClientBidID: "lb-bid-2", AmountCents: 20_000}, "tr_lb"); err != nil {
		t.Fatalf("PlaceBid user3: %v", err)
	}
	if _, err := repo.PlaceBid(ctx, auction.ID, "user_1", "lb-bid-3", BidInput{ClientBidID: "lb-bid-3", AmountCents: 25_000}, "tr_lb"); err != nil {
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
