package auction

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	apierrors "live-auction/backend/internal/platform/errors"
)

func TestPlaceBidAcceptedWritesTruthRowsAndIdempotency(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)

	input := BidInput{ClientBidID: "bid-accepted-1", AmountCents: 15_000}
	resp, err := repo.PlaceBid(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != BidResultAccepted {
		t.Fatalf("result = %s, want ACCEPTED", resp.Result)
	}
	if resp.CurrentPriceCents != 15_000 || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_1" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 1, 1, 1)

	replay, err := repo.PlaceBid(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid replay: %v", err)
	}
	if replay.BidID != resp.BidID || replay.Seq != resp.Seq {
		t.Fatalf("idempotent replay mismatch: got %#v want %#v", replay, resp)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 1, 1, 1)

	changed := BidInput{ClientBidID: input.ClientBidID, AmountCents: 20_000}
	_, err = repo.PlaceBid(ctx, auction.ID, "user_1", changed.ClientBidID, changed, "tr_bid")
	if !hasCode(err, apierrors.CodeIdempotencyKeyReusedWithDifferentRequest) {
		t.Fatalf("PlaceBid changed idempotency err = %v, want key reused with different request", err)
	}
}

func TestPlaceBidExecutableRejectIsStoredAndIdempotent(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)

	input := BidInput{ClientBidID: "bid-low-1", AmountCents: 1_000}
	resp, err := repo.PlaceBid(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid reject: %v", err)
	}
	if resp.Result != BidResultRejected || resp.RejectReason == nil || *resp.RejectReason != "BID_TOO_LOW" {
		t.Fatalf("unexpected reject response: %#v", resp)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 0, 1, 1)

	replay, err := repo.PlaceBid(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid reject replay: %v", err)
	}
	if replay.BidID != resp.BidID || replay.RejectReason == nil || *replay.RejectReason != *resp.RejectReason {
		t.Fatalf("idempotent reject replay mismatch: got %#v want %#v", replay, resp)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 0, 1, 1)
}

func TestFatFingerConfirmTokenThenConfirmBid(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	threshold := int64(20_000)
	auction := createActiveAuctionWithRule(t, repo, db, nil, func(rule Rule) Rule {
		rule.FatFingerThresholdCents = &threshold
		return rule
	})

	input := BidInput{ClientBidID: "bid-fat-finger-1", AmountCents: 50_000}
	pending, err := repo.PlaceBid(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_fat")
	if err != nil {
		t.Fatalf("PlaceBid fat finger: %v", err)
	}
	if pending.Result != string(apierrors.CodeFatFingerConfirmRequired) || pending.ConfirmToken == "" || pending.AmountCents != input.AmountCents {
		t.Fatalf("unexpected pending confirm response: %#v", pending)
	}
	assertBidTruthRows(t, db, auction.ID, 0, 0, 0, 1)

	confirmed, err := repo.ConfirmBid(ctx, auction.ID, "user_1", input.ClientBidID, ConfirmBidInput{
		ConfirmToken:   pending.ConfirmToken,
		IdempotencyKey: input.ClientBidID,
	}, "tr_fat_confirm")
	if err != nil {
		t.Fatalf("ConfirmBid: %v", err)
	}
	if confirmed.Result != BidResultAccepted || confirmed.CurrentPriceCents != input.AmountCents || confirmed.Seq <= pending.Seq {
		t.Fatalf("unexpected confirm response: %#v", confirmed)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 1, 1, 1)

	replay, err := repo.ConfirmBid(ctx, auction.ID, "user_1", input.ClientBidID, ConfirmBidInput{
		ConfirmToken:   pending.ConfirmToken,
		IdempotencyKey: input.ClientBidID,
	}, "tr_fat_confirm_reuse")
	if err != nil {
		t.Fatalf("ConfirmBid replay: %v", err)
	}
	if replay.BidID != confirmed.BidID || replay.Seq != confirmed.Seq || replay.CurrentPriceCents != confirmed.CurrentPriceCents {
		t.Fatalf("confirm replay mismatch: got %#v want %#v", replay, confirmed)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 1, 1, 1)
}

func TestPlaceBidCapSoldCreatesOrderAndPaymentIsIdempotent(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	capPrice := int64(20_000)
	auction := createActiveAuction(t, repo, db, &capPrice)

	input := BidInput{ClientBidID: "bid-cap-1", AmountCents: 20_000}
	resp, err := repo.PlaceBid(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid cap: %v", err)
	}
	if resp.Result != BidResultAcceptedSold {
		t.Fatalf("result = %s, want ACCEPTED_SOLD", resp.Result)
	}
	var status string
	var orderID string
	var depositStatus string
	if err := db.QueryRow(ctx, `SELECT id, status, deposit_status FROM orders WHERE auction_id = $1`, auction.ID).Scan(&orderID, &status, &depositStatus); err != nil {
		t.Fatalf("select order: %v", err)
	}
	if status != OrderStatusPending || depositStatus != DepositStatusHeld {
		t.Fatalf("order status = %s/%s, want pending/held", status, depositStatus)
	}

	_, err = repo.PayMock(ctx, orderID, "user_1", "pay-missing-confirm", PaymentInput{}, "tr_pay")
	if !hasCode(err, apierrors.CodeInvalidArgument) {
		t.Fatalf("PayMock missing confirm err = %v, want INVALID_ARGUMENT", err)
	}

	pay1, err := repo.PayMock(ctx, orderID, "user_1", "pay-1", PaymentInput{Confirm: true}, "tr_pay")
	if err != nil {
		t.Fatalf("PayMock: %v", err)
	}
	pay2, err := repo.PayMock(ctx, orderID, "user_1", "pay-1", PaymentInput{Confirm: true}, "tr_pay")
	if err != nil {
		t.Fatalf("PayMock replay: %v", err)
	}
	if pay1.OrderStatus != OrderStatusPaid || pay2.OrderStatus != pay1.OrderStatus || pay2.OrderID != pay1.OrderID {
		t.Fatalf("unexpected payment replay: %#v %#v", pay1, pay2)
	}
	pay3, err := repo.PayMock(ctx, orderID, "user_1", "pay-2", PaymentInput{Confirm: true}, "tr_pay")
	if err != nil {
		t.Fatalf("PayMock second key: %v", err)
	}
	if !pay3.PaidAt.Equal(pay1.PaidAt) {
		t.Fatalf("paid_at changed on second payment key: %s != %s", pay3.PaidAt, pay1.PaidAt)
	}
}

func TestListBidAndOrderHistoryRows(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	capPrice := int64(20_000)
	auction := createActiveAuction(t, repo, db, &capPrice)

	input := BidInput{ClientBidID: "bid-history-1", AmountCents: 20_000}
	if _, err := repo.PlaceBid(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_history"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	bids, err := repo.ListBidHistory(ctx, "user_1")
	if err != nil {
		t.Fatalf("ListBidHistory: %v", err)
	}
	if len(bids) == 0 || bids[0].AuctionID != auction.ID || bids[0].AmountCents != 20_000 || bids[0].Result != BidResultAcceptedSold {
		t.Fatalf("unexpected bid history: %#v", bids)
	}

	orders, err := repo.ListOrders(ctx, "user_1", "user")
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	history := ToOrderHistoryRows(orders)
	if len(history) == 0 || history[0].AuctionID != auction.ID || history[0].AmountCents != 20_000 || history[0].OrderStatus != OrderStatusPending {
		t.Fatalf("unexpected order history: %#v", history)
	}
}

func TestCancelActiveThenLaterBidRejects(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)

	cancelled, err := repo.Cancel(ctx, auction.ID, CancelInput{Reason: "test cancel"}, "tr_cancel")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", cancelled.Status)
	}

	input := BidInput{ClientBidID: "bid-after-cancel-1", AmountCents: 15_000}
	resp, err := repo.PlaceBid(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid after cancel: %v", err)
	}
	if resp.Result != BidResultRejected || resp.RejectReason == nil || *resp.RejectReason != "AUCTION_NOT_ACTIVE" {
		t.Fatalf("unexpected reject after cancel: %#v", resp)
	}
}

func createActiveAuction(t *testing.T, repo *Repository, db *pgxpool.Pool, capPrice *int64) Auction {
	return createActiveAuctionWithRule(t, repo, db, capPrice, func(rule Rule) Rule { return rule })
}

func createActiveAuctionWithRule(t *testing.T, repo *Repository, db *pgxpool.Pool, capPrice *int64, mutateRule func(Rule) Rule) Auction {
	t.Helper()
	roomID := createTestRoom(t, db)
	item, err := repo.CreateItem(context.Background(), CreateItemInput{Title: "Bid Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	auction, err := repo.CreateAuction(context.Background(), CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		CapPriceCents:   capPrice,
		Rule:            mutateRule(validRule()),
	}, "tr_lifecycle")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	auction, err = repo.Schedule(context.Background(), auction.ID, nil, "tr_lifecycle")
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	auction, err = repo.Start(context.Background(), auction.ID, "tr_lifecycle")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return auction
}

func assertBidTruthRows(t *testing.T, db *pgxpool.Pool, auctionID string, bidCount int, acceptedCount int, newEvents int, idemCount int) {
	t.Helper()
	ctx := context.Background()
	var bids int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1`, auctionID).Scan(&bids); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if bids != bidCount {
		t.Fatalf("bids = %d, want %d", bids, bidCount)
	}
	var accepted int
	if err := db.QueryRow(ctx, `SELECT accepted_bid_count FROM auctions WHERE id = $1`, auctionID).Scan(&accepted); err != nil {
		t.Fatalf("accepted count: %v", err)
	}
	if accepted != acceptedCount {
		t.Fatalf("accepted_bid_count = %d, want %d", accepted, acceptedCount)
	}
	var events int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND event_type IN ('bid_accepted','auction_sold','bid_rejected')`, auctionID).Scan(&events); err != nil {
		t.Fatalf("count bid events: %v", err)
	}
	if events != newEvents {
		t.Fatalf("bid events = %d, want %d", events, newEvents)
	}
	var idem int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM idempotency_records WHERE scope_id = $1`, auctionID).Scan(&idem); err != nil {
		t.Fatalf("count idempotency: %v", err)
	}
	if idem != idemCount {
		t.Fatalf("idempotency records = %d, want %d", idem, idemCount)
	}
}
