package auction

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	apierrors "live-auction/backend/internal/platform/errors"
)

func TestPlaceBidAcceptedWritesTruthRowsAndIdempotency(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)

	input := BidInput{ClientBidID: "bid-accepted-1", AmountCents: 15_000}
	resp, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
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

	replay, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid replay: %v", err)
	}
	if replay.BidID != resp.BidID || replay.Seq != resp.Seq {
		t.Fatalf("idempotent replay mismatch: got %#v want %#v", replay, resp)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 1, 1, 1)

	changed := BidInput{ClientBidID: input.ClientBidID, AmountCents: 20_000}
	_, err = repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", changed.ClientBidID, changed, "tr_bid")
	if !hasCode(err, apierrors.CodeIdempotencyKeyReusedWithDifferentRequest) {
		t.Fatalf("PlaceBid changed idempotency err = %v, want key reused with different request", err)
	}
}

func TestPlaceBidOrdinaryRejectIsStoredForAuditWithoutRealtimeOutbox(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)

	input := BidInput{ClientBidID: "bid-low-1", AmountCents: 1_000}
	resp, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid reject: %v", err)
	}
	if resp.Result != BidResultRejected || resp.RejectReason == nil || *resp.RejectReason != "BID_TOO_LOW" {
		t.Fatalf("unexpected reject response: %#v", resp)
	}
	if resp.Seq != auction.Seq {
		t.Fatalf("ordinary reject seq = %d, want current auction seq %d", resp.Seq, auction.Seq)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 0, 0, 1)
	assertBidRealtimeRows(t, db, auction.ID, "bid_rejected", 0)
	assertRejectedBidAuditRow(t, db, auction.ID, input.ClientBidID, false)

	replay, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid reject replay: %v", err)
	}
	if replay.BidID != resp.BidID || replay.RejectReason == nil || *replay.RejectReason != *resp.RejectReason {
		t.Fatalf("idempotent reject replay mismatch: got %#v want %#v", replay, resp)
	}
	assertBidTruthRows(t, db, auction.ID, 1, 0, 0, 1)
	assertBidRealtimeRows(t, db, auction.ID, "bid_rejected", 0)
}

func TestPlaceBidPolicyRejectStillUsesDurableRealtime(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)

	first := BidInput{ClientBidID: "bid-leading-1", AmountCents: 15_000}
	accepted, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", first.ClientBidID, first, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid accepted: %v", err)
	}
	second := BidInput{ClientBidID: "bid-self-leading-1", AmountCents: 20_000}
	rejected, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", second.ClientBidID, second, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid self leading reject: %v", err)
	}
	if rejected.Result != BidResultRejected || rejected.RejectReason == nil || *rejected.RejectReason != "REJECTED_SELF_LEADING" {
		t.Fatalf("unexpected reject response: %#v", rejected)
	}
	if rejected.Seq != accepted.Seq+1 {
		t.Fatalf("policy reject seq = %d, want %d", rejected.Seq, accepted.Seq+1)
	}
	assertBidTruthRows(t, db, auction.ID, 2, 1, 2, 2)
	assertBidRealtimeRows(t, db, auction.ID, "bid_rejected", 1)
	assertRejectedBidAuditRow(t, db, auction.ID, second.ClientBidID, true)
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
	pending, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_fat")
	if err != nil {
		t.Fatalf("PlaceBid fat finger: %v", err)
	}
	if pending.Result != string(apierrors.CodeFatFingerConfirmRequired) || pending.ConfirmToken == "" || pending.AmountCents != input.AmountCents {
		t.Fatalf("unexpected pending confirm response: %#v", pending)
	}
	assertBidTruthRows(t, db, auction.ID, 0, 0, 0, 1)

	confirmed, err := repo.ConfirmBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, ConfirmBidInput{
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

	replay, err := repo.ConfirmBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, ConfirmBidInput{
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
	resp, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
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
	markAuctionOutboxPublished(t, db, auction.ID)

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
	var orderPaidEvents int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND event_type = 'order_paid'`, auction.ID).Scan(&orderPaidEvents); err != nil {
		t.Fatalf("count order_paid events: %v", err)
	}
	if orderPaidEvents != 1 {
		t.Fatalf("order_paid events = %d, want 1", orderPaidEvents)
	}
	var orderPaidOutbox int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND e.event_type = 'order_paid'
	`, auction.ID).Scan(&orderPaidOutbox); err != nil {
		t.Fatalf("count order_paid outbox: %v", err)
	}
	if orderPaidOutbox != 1 {
		t.Fatalf("order_paid outbox = %d, want 1", orderPaidOutbox)
	}
}

func TestPaymentWaitsForSettlementConvergence(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	capPrice := int64(20_000)
	auction := createActiveAuction(t, repo, db, &capPrice)

	input := BidInput{ClientBidID: "bid-cap-convergence-gate", AmountCents: 20_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid_convergence_gate"); err != nil {
		t.Fatalf("PlaceBid cap: %v", err)
	}
	var orderID string
	if err := db.QueryRow(ctx, `SELECT id FROM orders WHERE auction_id = $1`, auction.ID).Scan(&orderID); err != nil {
		t.Fatalf("select order: %v", err)
	}

	if _, err := repo.PayMock(ctx, orderID, "user_1", "pay-before-outbox-converged", PaymentInput{Confirm: true}, "tr_pay_blocked"); !hasCode(err, apierrors.CodeProcessingRetryLater) {
		t.Fatalf("PayMock before outbox convergence err = %v, want PROCESSING_RETRY_LATER", err)
	}

	markAuctionOutboxPublished(t, db, auction.ID)
	pay, err := repo.PayMock(ctx, orderID, "user_1", "pay-after-converged", PaymentInput{Confirm: true}, "tr_pay_converged")
	if err != nil {
		t.Fatalf("PayMock after convergence: %v", err)
	}
	if pay.OrderStatus != OrderStatusPaid {
		t.Fatalf("payment status = %s, want PAID", pay.OrderStatus)
	}
}

func TestPaymentWaitsForOpenRedisEngineSettlement(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	capPrice := int64(20_000)
	auction := createActiveAuction(t, repo, db, &capPrice)

	input := BidInput{ClientBidID: "bid-cap-open-settlement-gate", AmountCents: 20_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid_open_settlement_gate"); err != nil {
		t.Fatalf("PlaceBid cap: %v", err)
	}
	var orderID string
	if err := db.QueryRow(ctx, `SELECT id FROM orders WHERE auction_id = $1`, auction.ID).Scan(&orderID); err != nil {
		t.Fatalf("select order: %v", err)
	}
	markAuctionOutboxPublished(t, db, auction.ID)
	if _, err := db.Exec(ctx, `
		INSERT INTO redis_engine_settlements (
		  id, auction_id, stream_id, engine_epoch, engine_seq, result,
		  status, payload_json, payload_sha256, created_at, updated_at
		)
		VALUES (
		  $1, $2, $3, 1, 999, 'ENGINE_ACCEPTED',
		  'PROCESSING', '{"result":"ENGINE_ACCEPTED"}'::jsonb,
		  encode(digest(convert_to('{"result":"ENGINE_ACCEPTED"}', 'UTF8'), 'sha256'), 'hex'), now(), now()
		)
	`, time.Now().UnixNano(), auction.ID, "open-stream-"+auction.ID); err != nil {
		t.Fatalf("insert open settlement: %v", err)
	}

	if _, err := repo.PayMock(ctx, orderID, "user_1", "pay-before-settlement-converged", PaymentInput{Confirm: true}, "tr_pay_blocked_settlement"); !hasCode(err, apierrors.CodeProcessingRetryLater) {
		t.Fatalf("PayMock before settlement convergence err = %v, want PROCESSING_RETRY_LATER", err)
	}
}

func TestPlaceBidTriggersAutoMaxBidWithinSameTransaction(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)
	autoUser := createTestUser(t, db)

	intent, err := repo.UpsertMaxBidIntent(ctx, auction.ID, autoUser, MaxBidIntentInput{MaxAmountCents: 30_000})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	input := BidInput{ClientBidID: "bid-auto-trigger-1", AmountCents: 15_000}
	resp, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_auto")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != BidResultAccepted || resp.CurrentPriceCents != 20_000 || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != autoUser {
		t.Fatalf("manual response did not reflect final auto state: %#v", resp)
	}

	assertAutoBidRow(t, db, auction.ID, autoUser, 20_000, BidSourceAutoMaxBid)
	assertBidTruthRows(t, db, auction.ID, 2, 2, 2, 1)
	assertMaxBidIntentApplied(t, db, intent.ID)
	assertPublicAutoPayloadDoesNotLeakMax(t, db, auction.ID)

	replay, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_auto_replay")
	if err != nil {
		t.Fatalf("PlaceBid replay: %v", err)
	}
	if replay.Seq != resp.Seq || replay.CurrentPriceCents != resp.CurrentPriceCents || replay.CurrentWinnerID == nil || *replay.CurrentWinnerID != autoUser {
		t.Fatalf("idempotent replay mismatch: got %#v want %#v", replay, resp)
	}
	assertBidTruthRows(t, db, auction.ID, 2, 2, 2, 1)
}

func TestStartActivatesPreBidAutoMaxBid(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createScheduledAuction(t, repo, db, nil)
	autoUser := createTestUser(t, db)

	intent, err := repo.UpsertMaxBidIntent(ctx, auction.ID, autoUser, MaxBidIntentInput{
		MaxAmountCents: 25_000,
		Source:         MaxBidIntentSourcePreBid,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	started, err := repo.Start(ctx, auction.ID, "tr_prebid_start")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if started.Status != StatusActive || started.CurrentPriceCents != 15_000 || started.CurrentWinnerID == nil || *started.CurrentWinnerID != autoUser {
		t.Fatalf("started auction missing pre-bid auto state: %#v", started)
	}
	assertAutoBidRow(t, db, auction.ID, autoUser, 15_000, BidSourceAutoMaxBid)
	assertMaxBidIntentApplied(t, db, intent.ID)
	assertPublicAutoPayloadDoesNotLeakMax(t, db, auction.ID)
}

func TestAutoMaxBidDefendsEqualMaxByEarlierIntent(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	auction := createActiveAuction(t, repo, db, nil)
	firstUser := createTestUser(t, db)
	secondUser := createTestUser(t, db)

	first, err := repo.UpsertMaxBidIntent(ctx, auction.ID, firstUser, MaxBidIntentInput{MaxAmountCents: 25_000})
	if err != nil {
		t.Fatalf("first intent: %v", err)
	}
	if _, err := repo.UpsertMaxBidIntent(ctx, auction.ID, secondUser, MaxBidIntentInput{MaxAmountCents: 25_000}); err != nil {
		t.Fatalf("second intent: %v", err)
	}

	input := BidInput{ClientBidID: "bid-equal-max-1", AmountCents: 15_000}
	resp, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_equal_max")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.CurrentPriceCents != 25_000 || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != firstUser {
		t.Fatalf("equal max did not keep earlier intent as winner: %#v", resp)
	}
	assertMaxBidIntentApplied(t, db, first.ID)
}

func TestAutoMaxBidCapCreatesSingleSoldOrder(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	capPrice := int64(25_000)
	auction := createActiveAuction(t, repo, db, &capPrice)
	autoUser := createTestUser(t, db)

	if _, err := repo.UpsertMaxBidIntent(ctx, auction.ID, autoUser, MaxBidIntentInput{MaxAmountCents: 25_000}); err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	input := BidInput{ClientBidID: "bid-auto-cap-1", AmountCents: 20_000}
	resp, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_auto_cap")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != BidResultAcceptedSold || resp.CurrentPriceCents != 25_000 || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != autoUser {
		t.Fatalf("auto cap response mismatch: %#v", resp)
	}
	var status Status
	if err := db.QueryRow(ctx, `SELECT status FROM auctions WHERE id = $1`, auction.ID).Scan(&status); err != nil {
		t.Fatalf("select status: %v", err)
	}
	if status != StatusSold {
		t.Fatalf("status = %s, want SOLD", status)
	}
	var orders int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE auction_id = $1`, auction.ID).Scan(&orders); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orders != 1 {
		t.Fatalf("orders = %d, want 1", orders)
	}
	assertAutoBidRow(t, db, auction.ID, autoUser, 25_000, BidSourceAutoMaxBid)
}

func TestListBidAndOrderHistoryRows(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	capPrice := int64(20_000)
	auction := createActiveAuction(t, repo, db, &capPrice)
	userID := createTestUser(t, db)

	input := BidInput{ClientBidID: "bid-history-1", AmountCents: 20_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, userID, input.ClientBidID, input, "tr_history"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	bids, err := repo.ListBidHistory(ctx, userID)
	if err != nil {
		t.Fatalf("ListBidHistory: %v", err)
	}
	if len(bids) == 0 || bids[0].AuctionID != auction.ID || bids[0].AmountCents != 20_000 || bids[0].Result != BidResultAcceptedSold {
		t.Fatalf("unexpected bid history: %#v", bids)
	}

	orders, err := repo.ListOrders(ctx, userID, "user")
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
	resp, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_bid")
	if err != nil {
		t.Fatalf("PlaceBid after cancel: %v", err)
	}
	if resp.Result != BidResultRejected || resp.RejectReason == nil || *resp.RejectReason != "AUCTION_NOT_ACTIVE" {
		t.Fatalf("unexpected reject after cancel: %#v", resp)
	}
	assertBidRealtimeRows(t, db, auction.ID, "bid_rejected", 0)
	assertRejectedBidAuditRow(t, db, auction.ID, input.ClientBidID, false)
}

func TestProviderWebhookDuplicateCreatesOnePaidTransition(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	orderID, auctionID := createPendingOrder(t, repo, db)
	suffix := uuid.NewString()
	webhook := ProviderPaymentWebhook{
		ProviderEventID:   "evt_provider_dup_" + suffix,
		ProviderPaymentID: "pay_provider_dup_" + suffix,
		OrderID:           orderID,
		EventType:         "payment_succeeded",
	}
	webhook.Signature = SignProviderWebhook(webhook, DefaultFakePaymentWebhookSecret)
	first, err := repo.HandleProviderWebhook(ctx, webhook, DefaultFakePaymentWebhookSecret, "tr_provider")
	if err != nil {
		t.Fatalf("HandleProviderWebhook first: %v", err)
	}
	second, err := repo.HandleProviderWebhook(ctx, webhook, DefaultFakePaymentWebhookSecret, "tr_provider_replay")
	if err != nil {
		t.Fatalf("HandleProviderWebhook replay: %v", err)
	}
	if first.OrderStatus != OrderStatusPaid || second.OrderStatus != OrderStatusPaid || !second.PaidAt.Equal(first.PaidAt) {
		t.Fatalf("unexpected webhook responses: %#v %#v", first, second)
	}
	assertOrderPaidEventCount(t, db, auctionID, 1)
	assertPaymentEventCount(t, db, orderID, 1)
}

func TestProviderWebhookInvalidSignatureAuditedAndIgnored(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	orderID, auctionID := createPendingOrder(t, repo, db)
	suffix := uuid.NewString()
	webhook := ProviderPaymentWebhook{
		ProviderEventID:   "evt_bad_sig_" + suffix,
		ProviderPaymentID: "pay_bad_sig_" + suffix,
		OrderID:           orderID,
		EventType:         "payment_succeeded",
		Signature:         "bad",
	}
	if _, err := repo.HandleProviderWebhook(ctx, webhook, DefaultFakePaymentWebhookSecret, "tr_bad_sig"); !hasCode(err, apierrors.CodeInvalidArgument) {
		t.Fatalf("invalid signature err = %v, want INVALID_ARGUMENT", err)
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status); err != nil {
		t.Fatalf("read order status: %v", err)
	}
	if status != OrderStatusPending {
		t.Fatalf("status = %s, want ORDER_PENDING", status)
	}
	assertPaymentAnomalyCount(t, db, auctionID, "PAYMENT_WEBHOOK_INVALID_SIGNATURE", 1)
}

func TestProviderWebhookLateSuccessForExpiredOrderAudited(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	orderID, auctionID := createPendingOrder(t, repo, db)
	suffix := uuid.NewString()
	if _, err := db.Exec(ctx, `UPDATE orders SET status = 'ORDER_EXPIRED', deposit_status = 'FORFEITED' WHERE id = $1`, orderID); err != nil {
		t.Fatalf("expire order: %v", err)
	}
	webhook := ProviderPaymentWebhook{
		ProviderEventID:   "evt_late_success_" + suffix,
		ProviderPaymentID: "pay_late_success_" + suffix,
		OrderID:           orderID,
		EventType:         "payment_succeeded",
	}
	webhook.Signature = SignProviderWebhook(webhook, DefaultFakePaymentWebhookSecret)
	if _, err := repo.HandleProviderWebhook(ctx, webhook, DefaultFakePaymentWebhookSecret, "tr_late_success"); !hasCode(err, apierrors.CodeOrderAlreadyExpired) {
		t.Fatalf("late webhook err = %v, want ORDER_ALREADY_EXPIRED", err)
	}
	assertPaymentAnomalyCount(t, db, auctionID, "PAYMENT_RECONCILE_MISMATCH", 1)
	assertOrderPaidEventCount(t, db, auctionID, 0)
}

func TestProviderPaymentReconcileRepairsInitiatedOrderWithSuccessEvent(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	orderID, auctionID := createPendingOrder(t, repo, db)
	suffix := uuid.NewString()
	providerPaymentID := "pay_reconcile_success_" + suffix
	if _, err := db.Exec(ctx, `
		UPDATE orders
		SET status = 'PAYMENT_INITIATED', provider_payment_id = $2, payment_initiated_at = now() - interval '2 minutes'
		WHERE id = $1
	`, orderID, providerPaymentID); err != nil {
		t.Fatalf("mark initiated: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO payment_events (provider, provider_event_id, provider_payment_id, order_id, event_type, signature_valid, processed_at, payload_json, trace_id)
		VALUES ('local_fake', $1, $2, $3, 'payment_succeeded', true, now(), '{}', 'tr_reconcile')
	`, "evt_reconcile_success_"+suffix, providerPaymentID, orderID); err != nil {
		t.Fatalf("insert provider event: %v", err)
	}
	report, err := repo.ReconcileProviderPayments(ctx, 10, time.Second, "tr_reconcile")
	if err != nil {
		t.Fatalf("ReconcileProviderPayments: %v", err)
	}
	if report.Repaired < 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if status != OrderStatusPaid {
		t.Fatalf("status = %s, want PAID", status)
	}
	assertOrderPaidEventCount(t, db, auctionID, 1)
}

func TestProviderPaymentReconcileWritesMismatchForStaleInitiatedOrder(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)
	orderID, auctionID := createPendingOrder(t, repo, db)
	providerPaymentID := "pay_reconcile_missing_" + uuid.NewString()
	if _, err := db.Exec(ctx, `
		UPDATE orders
		SET status = 'PAYMENT_INITIATED', provider_payment_id = $2, payment_initiated_at = now() - interval '2 minutes'
		WHERE id = $1
	`, orderID, providerPaymentID); err != nil {
		t.Fatalf("mark initiated: %v", err)
	}
	report, err := repo.ReconcileProviderPayments(ctx, 10, time.Second, "tr_reconcile_missing")
	if err != nil {
		t.Fatalf("ReconcileProviderPayments: %v", err)
	}
	if report.Anomaly < 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	assertPaymentAnomalyCount(t, db, auctionID, "PAYMENT_RECONCILE_MISMATCH", 1)
}

func createActiveAuction(t *testing.T, repo *Repository, db *pgxpool.Pool, capPrice *int64) Auction {
	return createActiveAuctionWithRule(t, repo, db, capPrice, func(rule Rule) Rule { return rule })
}

func createScheduledAuction(t *testing.T, repo *Repository, db *pgxpool.Pool, capPrice *int64) Auction {
	t.Helper()
	roomID := createTestRoom(t, db)
	item, err := repo.CreateItem(context.Background(), CreateItemInput{Title: "Scheduled Bid Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	auction, err := repo.CreateAuction(context.Background(), CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		CapPriceCents:   capPrice,
		Rule:            validRule(),
	}, "tr_scheduled")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	auction, err = repo.Schedule(context.Background(), auction.ID, nil, "tr_scheduled")
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	return auction
}

func createPendingOrder(t *testing.T, repo *Repository, db *pgxpool.Pool) (string, string) {
	t.Helper()
	ctx := context.Background()
	capPrice := int64(20_000)
	auction := createActiveAuction(t, repo, db, &capPrice)
	input := BidInput{ClientBidID: "bid-order-" + uuid.NewString(), AmountCents: 20_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auction.ID, "user_1", input.ClientBidID, input, "tr_order"); err != nil {
		t.Fatalf("PlaceBid cap: %v", err)
	}
	var orderID string
	if err := db.QueryRow(ctx, `SELECT id FROM orders WHERE auction_id = $1`, auction.ID).Scan(&orderID); err != nil {
		t.Fatalf("select order: %v", err)
	}
	return orderID, auction.ID
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

func createTestUser(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	userID := "user_test_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO users (id, role, display_name) VALUES ($1, 'user', 'Test User')`, userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	return userID
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

func markAuctionOutboxPublished(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		UPDATE outbox_delivery d
		SET status = 'PUBLISHED',
		    published_at = COALESCE(published_at, now())
		FROM outbox_events e
		WHERE e.id = d.outbox_id
		  AND e.auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("mark auction outbox published: %v", err)
	}
}

func assertBidRealtimeRows(t *testing.T, db *pgxpool.Pool, auctionID string, eventType string, want int) {
	t.Helper()
	var eventCount int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND event_type = $2`, auctionID, eventType).Scan(&eventCount); err != nil {
		t.Fatalf("count auction event %s: %v", eventType, err)
	}
	if eventCount != want {
		t.Fatalf("%s auction events = %d, want %d", eventType, eventCount, want)
	}
	var outboxCount int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE auction_id = $1 AND event_type = $2`, auctionID, eventType).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox event %s: %v", eventType, err)
	}
	if outboxCount != want {
		t.Fatalf("%s outbox events = %d, want %d", eventType, outboxCount, want)
	}
}

func assertRejectedBidAuditRow(t *testing.T, db *pgxpool.Pool, auctionID string, clientBidID string, wantSeq bool) {
	t.Helper()
	var status string
	var rejectReason *string
	var seq *int64
	if err := db.QueryRow(context.Background(), `
		SELECT status, reject_reason, seq
		FROM bids
		WHERE auction_id = $1 AND client_bid_id = $2
	`, auctionID, clientBidID).Scan(&status, &rejectReason, &seq); err != nil {
		t.Fatalf("select rejected bid audit row: %v", err)
	}
	if status != "REJECTED" || rejectReason == nil || *rejectReason == "" {
		t.Fatalf("unexpected rejected bid row status=%s reason=%v", status, rejectReason)
	}
	if wantSeq && seq == nil {
		t.Fatalf("rejected bid seq is nil, want durable realtime seq")
	}
	if !wantSeq && seq != nil {
		t.Fatalf("rejected bid seq = %d, want nil for non-realtime reject", *seq)
	}
}

func assertOrderPaidEventCount(t *testing.T, db *pgxpool.Pool, auctionID string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND event_type = 'order_paid'`, auctionID).Scan(&count); err != nil {
		t.Fatalf("count order_paid: %v", err)
	}
	if count != want {
		t.Fatalf("order_paid events = %d, want %d", count, want)
	}
}

func assertPaymentEventCount(t *testing.T, db *pgxpool.Pool, orderID string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM payment_events WHERE order_id = $1`, orderID).Scan(&count); err != nil {
		t.Fatalf("count payment_events: %v", err)
	}
	if count != want {
		t.Fatalf("payment_events = %d, want %d", count, want)
	}
}

func assertPaymentAnomalyCount(t *testing.T, db *pgxpool.Pool, auctionID string, anomalyType string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM system_anomaly_events WHERE auction_id = $1 AND type = $2`, auctionID, anomalyType).Scan(&count); err != nil {
		t.Fatalf("count payment anomaly: %v", err)
	}
	if count != want {
		t.Fatalf("%s anomalies = %d, want %d", anomalyType, count, want)
	}
}

func assertAutoBidRow(t *testing.T, db *pgxpool.Pool, auctionID string, userID string, amountCents int64, source string) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `
		SELECT count(*)
		FROM bids
		WHERE auction_id = $1 AND user_id = $2 AND amount_cents = $3 AND source = $4 AND status = 'ACCEPTED'
	`, auctionID, userID, amountCents, source).Scan(&count); err != nil {
		t.Fatalf("count auto bid row: %v", err)
	}
	if count != 1 {
		t.Fatalf("auto bid rows = %d, want 1", count)
	}
}

func assertMaxBidIntentApplied(t *testing.T, db *pgxpool.Pool, intentID string) {
	t.Helper()
	var lastApplied *int64
	if err := db.QueryRow(context.Background(), `SELECT last_applied_seq FROM max_bid_intents WHERE id = $1`, intentID).Scan(&lastApplied); err != nil {
		t.Fatalf("select intent: %v", err)
	}
	if lastApplied == nil || *lastApplied <= 0 {
		t.Fatalf("last_applied_seq = %v, want positive", lastApplied)
	}
}

func assertPublicAutoPayloadDoesNotLeakMax(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	rows, err := db.Query(context.Background(), `
		SELECT payload_json
		FROM auction_events
		WHERE auction_id = $1 AND payload_json->>'bid_source' = 'AUTO_MAX_BID'
	`, auctionID)
	if err != nil {
		t.Fatalf("select auto payloads: %v", err)
	}
	defer rows.Close()
	seen := false
	for rows.Next() {
		seen = true
		var payload map[string]any
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan payload: %v", err)
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		if _, ok := payload["max_amount_cents"]; ok {
			t.Fatalf("auto public event leaked max_amount_cents: %s", raw)
		}
		if _, ok := payload["intent_id"]; ok {
			t.Fatalf("auto public event leaked intent_id: %s", raw)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate payloads: %v", err)
	}
	if !seen {
		t.Fatalf("no AUTO_MAX_BID public payload found")
	}
}
