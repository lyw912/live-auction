package auction

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
)

const (
	DefaultFakePaymentWebhookSecret = "local_fake_payment_secret"

	BidResultAccepted         = "ACCEPTED"
	BidResultAcceptedExtended = "ACCEPTED_EXTENDED"
	BidResultAcceptedSold     = "ACCEPTED_SOLD"
	BidResultRejected         = "REJECTED"

	BidSourceManual     = "MANUAL"
	BidSourceAutoMaxBid = "AUTO_MAX_BID"

	IdempotencyStatusProcessing = "PROCESSING"
	IdempotencyStatusCompleted  = "COMPLETED"

	OrderStatusPending    = "ORDER_PENDING"
	OrderStatusInitiated  = "PAYMENT_INITIATED"
	OrderStatusSucceeded  = "PAYMENT_SUCCEEDED"
	OrderStatusPaid       = "PAID"
	OrderStatusExpired    = "ORDER_EXPIRED"
	DepositStatusHeld     = "HELD"
	DepositStatusRefunded = "REFUNDED"
	DepositStatusForfeit  = "FORFEITED"
)

type BidInput struct {
	ClientBidID   string `json:"client_bid_id"`
	AmountCents   int64  `json:"amount_cents"`
	ClientSeenSeq int64  `json:"client_seen_seq"`
}

type BidResponse struct {
	Result            string     `json:"result"`
	BidID             string     `json:"bid_id,omitempty"`
	AuctionID         string     `json:"auction_id"`
	Seq               int64      `json:"seq"`
	CurrentPriceCents int64      `json:"current_price_cents"`
	CurrentWinnerID   *string    `json:"current_winner_id,omitempty"`
	EndAt             *time.Time `json:"end_at,omitempty"`
	ServerTimeMS      int64      `json:"server_time_ms"`
	RejectReason      *string    `json:"reject_reason"`
	ConfirmToken      string     `json:"confirm_token,omitempty"`
	ExpiresInMS       int64      `json:"expires_in_ms,omitempty"`
	AmountCents       int64      `json:"amount_cents,omitempty"`
	ConfirmExpiresAt  *time.Time `json:"confirm_expires_at,omitempty"`
}

type ConfirmBidInput struct {
	ConfirmToken   string `json:"confirm_token"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PaymentInput struct {
	Confirm bool `json:"confirm"`
}

type ProviderPaymentWebhook struct {
	ProviderEventID   string `json:"provider_event_id"`
	ProviderPaymentID string `json:"provider_payment_id"`
	OrderID           string `json:"order_id"`
	EventType         string `json:"event_type"`
	Signature         string `json:"signature"`
}

type Order struct {
	ID            string     `json:"id"`
	AuctionID     string     `json:"auction_id"`
	WinnerID      string     `json:"winner_id"`
	AmountCents   int64      `json:"amount_cents"`
	Status        string     `json:"status"`
	DepositCents  int64      `json:"deposit_cents"`
	DepositStatus string     `json:"deposit_status"`
	ExpireAt      time.Time  `json:"expire_at"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	ProviderID    *string    `json:"provider_payment_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type PaymentResponse struct {
	OrderID           string    `json:"order_id"`
	OrderStatus       string    `json:"order_status"`
	PaidAt            time.Time `json:"paid_at"`
	DepositStatus     string    `json:"deposit_status"`
	ProviderPaymentID string    `json:"provider_payment_id,omitempty"`
}

type PaymentReconcileReport struct {
	Checked  int `json:"checked"`
	Repaired int `json:"repaired"`
	Anomaly  int `json:"anomaly"`
}

type BidHistoryRow struct {
	BidID       string    `json:"bid_id"`
	AuctionID   string    `json:"auction_id"`
	AmountCents int64     `json:"amount_cents"`
	Result      string    `json:"result"`
	CreatedAt   time.Time `json:"created_at"`
}

type OrderHistoryRow struct {
	OrderID     string    `json:"order_id"`
	AuctionID   string    `json:"auction_id"`
	AmountCents int64     `json:"amount_cents"`
	OrderStatus string    `json:"order_status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (r *Repository) PlaceBid(ctx context.Context, auctionID string, userID string, idempotencyKey string, input BidInput, traceID string) (BidResponse, error) {
	start := time.Now()
	resultLabel := "error"
	reasonLabel := ""
	defer func() {
		observability.Inc("auction_bid_request_total", map[string]string{"result": resultLabel, "reason": reasonLabel})
		observability.Observe("auction_bid_latency_seconds", time.Since(start).Seconds(), nil, observability.DefaultLatencyBuckets)
	}()
	if input.ClientBidID == "" || input.AmountCents <= 0 {
		resultLabel = "invalid"
		reasonLabel = string(apierrors.CodeInvalidArgument)
		return BidResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "client_bid_id and positive amount_cents are required", http.StatusBadRequest)
	}
	if idempotencyKey == "" || idempotencyKey != input.ClientBidID {
		resultLabel = "invalid"
		reasonLabel = string(apierrors.CodeInvalidArgument)
		return BidResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "Idempotency-Key must equal client_bid_id", http.StatusBadRequest)
	}
	requestHash := bidRequestHash(auctionID, userID, input.ClientBidID, input.AmountCents)
	if replay, ok, err := r.completedIdempotency(ctx, "bid", auctionID, userID, idempotencyKey, requestHash); err != nil || ok {
		if ok {
			resultLabel = replay.Result
			if replay.RejectReason != nil {
				reasonLabel = *replay.RejectReason
			} else {
				reasonLabel = "idempotent_replay"
			}
		}
		return replay, err
	}

	tx, err := r.beginTx(ctx)
	if err != nil {
		return BidResponse{}, err
	}
	defer rollback(ctx, tx)

	if err := upsertProcessing(ctx, tx, "bid", auctionID, userID, idempotencyKey, requestHash); err != nil {
		return BidResponse{}, err
	}

	locked, err := lockAuctionForBid(ctx, tx, auctionID)
	if err != nil {
		return BidResponse{}, mapPGError(err)
	}
	response, bidStatus, rejectCode, durableRealtime, lockedAfterBid, err := r.evaluateAndApplyBid(ctx, tx, locked, userID, input, traceID, false)
	if err != nil {
		return BidResponse{}, err
	}
	resultLabel = response.Result
	if rejectCode != nil {
		reasonLabel = *rejectCode
	}
	bidRowResponseJSON, err := json.Marshal(response)
	if err != nil {
		return BidResponse{}, err
	}
	if response.Result != string(apierrors.CodeFatFingerConfirmRequired) {
		if err := insertBidRow(ctx, tx, response.BidID, auctionID, userID, input, nullableSeq(durableRealtime, response.Seq), bidStatus, rejectCode, requestHash, bidRowResponseJSON, traceID, BidSourceManual); err != nil {
			return BidResponse{}, err
		}
	}
	finalResponse := response
	if bidStatus == "ACCEPTED" && response.Result != BidResultAcceptedSold {
		if autoResponse, ok, err := r.applyAutoMaxBid(ctx, tx, lockedAfterBid, traceID); err != nil {
			return BidResponse{}, err
		} else if ok {
			finalResponse.Result = autoResponse.Result
			finalResponse.Seq = autoResponse.Seq
			finalResponse.CurrentPriceCents = autoResponse.CurrentPriceCents
			finalResponse.CurrentWinnerID = autoResponse.CurrentWinnerID
			finalResponse.EndAt = autoResponse.EndAt
		}
	}
	responseJSON, err := json.Marshal(finalResponse)
	if err != nil {
		return BidResponse{}, err
	}
	if err := completeIdempotency(ctx, tx, "bid", auctionID, userID, idempotencyKey, requestHash, http.StatusOK, finalResponse.Result, responseJSON); err != nil {
		return BidResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BidResponse{}, err
	}
	return finalResponse, nil
}

func (r *Repository) ConfirmBid(ctx context.Context, auctionID string, userID string, idempotencyKey string, input ConfirmBidInput, traceID string) (BidResponse, error) {
	if input.ConfirmToken == "" || input.IdempotencyKey == "" || idempotencyKey == "" || idempotencyKey != input.IdempotencyKey {
		return BidResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "confirm_token and matching Idempotency-Key are required", http.StatusBadRequest)
	}
	tx, err := r.beginTx(ctx)
	if err != nil {
		return BidResponse{}, err
	}
	defer rollback(ctx, tx)

	var storedHash string
	var status string
	var resultCode *string
	var pendingJSON []byte
	if err := tx.QueryRow(ctx, `
		SELECT request_hash, status, result_code, response_json
		FROM idempotency_records
		WHERE scope_type = 'bid' AND scope_id = $1 AND user_id = $2 AND idempotency_key = $3
		FOR UPDATE
	`, auctionID, userID, idempotencyKey).Scan(&storedHash, &status, &resultCode, &pendingJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BidResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "confirm token not found", http.StatusNotFound)
		}
		return BidResponse{}, err
	}
	if status == IdempotencyStatusCompleted && resultCode != nil && *resultCode != string(apierrors.CodeFatFingerConfirmRequired) {
		var completed BidResponse
		if err := json.Unmarshal(pendingJSON, &completed); err != nil {
			return BidResponse{}, err
		}
		return completed, nil
	}
	if status != IdempotencyStatusCompleted || resultCode == nil || *resultCode != string(apierrors.CodeFatFingerConfirmRequired) {
		return BidResponse{}, apierrors.New(apierrors.CodeConfirmUsed, "confirm token already used", http.StatusConflict)
	}
	var pending BidResponse
	if err := json.Unmarshal(pendingJSON, &pending); err != nil {
		return BidResponse{}, err
	}
	if pending.ConfirmToken != input.ConfirmToken {
		return BidResponse{}, apierrors.New(apierrors.CodeConfirmUsed, "confirm token mismatch", http.StatusConflict)
	}
	if pending.ConfirmExpiresAt != nil && time.Now().UTC().After(*pending.ConfirmExpiresAt) {
		return BidResponse{}, apierrors.New(apierrors.CodeConfirmUsed, "confirm token expired", http.StatusConflict)
	}
	bidInput := BidInput{
		ClientBidID:   idempotencyKey,
		AmountCents:   pending.AmountCents,
		ClientSeenSeq: pending.Seq,
	}
	if storedHash != bidRequestHash(auctionID, userID, bidInput.ClientBidID, bidInput.AmountCents) {
		return BidResponse{}, apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "confirm request hash mismatch", http.StatusConflict)
	}
	locked, err := lockAuctionForBid(ctx, tx, auctionID)
	if err != nil {
		return BidResponse{}, mapPGError(err)
	}
	response, bidStatus, rejectCode, durableRealtime, lockedAfterBid, err := r.evaluateAndApplyBid(ctx, tx, locked, userID, bidInput, traceID, true)
	if err != nil {
		return BidResponse{}, err
	}
	bidRowResponseJSON, err := json.Marshal(response)
	if err != nil {
		return BidResponse{}, err
	}
	if err := insertBidRow(ctx, tx, response.BidID, auctionID, userID, bidInput, nullableSeq(durableRealtime, response.Seq), bidStatus, rejectCode, storedHash, bidRowResponseJSON, traceID, BidSourceManual); err != nil {
		return BidResponse{}, err
	}
	finalResponse := response
	if bidStatus == "ACCEPTED" && response.Result != BidResultAcceptedSold {
		if autoResponse, ok, err := r.applyAutoMaxBid(ctx, tx, lockedAfterBid, traceID); err != nil {
			return BidResponse{}, err
		} else if ok {
			finalResponse.Result = autoResponse.Result
			finalResponse.Seq = autoResponse.Seq
			finalResponse.CurrentPriceCents = autoResponse.CurrentPriceCents
			finalResponse.CurrentWinnerID = autoResponse.CurrentWinnerID
			finalResponse.EndAt = autoResponse.EndAt
		}
	}
	responseJSON, err := json.Marshal(finalResponse)
	if err != nil {
		return BidResponse{}, err
	}
	if err := completeIdempotency(ctx, tx, "bid", auctionID, userID, idempotencyKey, storedHash, http.StatusOK, finalResponse.Result, responseJSON); err != nil {
		return BidResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BidResponse{}, err
	}
	return finalResponse, nil
}

func (r *Repository) PayMock(ctx context.Context, orderID string, userID string, idempotencyKey string, input PaymentInput, traceID string) (PaymentResponse, error) {
	return r.PayMockWithSecret(ctx, orderID, userID, idempotencyKey, input, DefaultFakePaymentWebhookSecret, traceID)
}

func (r *Repository) PayMockWithSecret(ctx context.Context, orderID string, userID string, idempotencyKey string, input PaymentInput, secret string, traceID string) (PaymentResponse, error) {
	if secret == "" {
		secret = DefaultFakePaymentWebhookSecret
	}
	if idempotencyKey == "" {
		return PaymentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "Idempotency-Key is required", http.StatusBadRequest)
	}
	if !input.Confirm {
		return PaymentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "confirm must be true", http.StatusBadRequest)
	}
	requestHash := paymentRequestHash(orderID, userID, idempotencyKey)
	if replay, ok, err := r.completedPaymentIdempotency(ctx, orderID, userID, idempotencyKey, requestHash); err != nil || ok {
		return replay, err
	}

	tx, err := r.beginTx(ctx)
	if err != nil {
		return PaymentResponse{}, err
	}
	defer rollback(ctx, tx)
	if err := upsertProcessing(ctx, tx, "payment", orderID, userID, idempotencyKey, requestHash); err != nil {
		return PaymentResponse{}, err
	}

	var winnerID string
	var auctionID string
	var status string
	var paidAt *time.Time
	var providerPaymentID *string
	if err := tx.QueryRow(ctx, `
		SELECT auction_id, winner_id, status, paid_at, provider_payment_id
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, orderID).Scan(&auctionID, &winnerID, &status, &paidAt, &providerPaymentID); err != nil {
		return PaymentResponse{}, mapOrderNotFound(err)
	}
	if winnerID != userID {
		return PaymentResponse{}, apierrors.New(apierrors.CodeForbiddenRoom, "only winner can pay order", http.StatusForbidden)
	}
	if status == OrderStatusExpired {
		return PaymentResponse{}, apierrors.New(apierrors.CodeOrderAlreadyExpired, "order already expired", http.StatusConflict)
	}
	now := time.Now().UTC()
	if providerPaymentID == nil {
		generated := "pay_" + uuid.NewString()
		providerPaymentID = &generated
	}
	if status != OrderStatusPaid {
		if err := tx.QueryRow(ctx, `
			UPDATE orders
			SET status = 'PAYMENT_INITIATED',
			    provider_payment_id = $2,
			    payment_initiated_at = COALESCE(payment_initiated_at, $3)
			WHERE id = $1
			RETURNING provider_payment_id
		`, orderID, *providerPaymentID, now).Scan(&providerPaymentID); err != nil {
			return PaymentResponse{}, err
		}
	}
	if err := insertPaymentEvent(ctx, tx, "evt_init_"+idempotencyKey, *providerPaymentID, orderID, "payment_initiated", true, traceID, map[string]any{
		"source":          "pay_mock",
		"idempotency_key": idempotencyKey,
		"user_id":         userID,
	}); err != nil {
		return PaymentResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentResponse{}, err
	}

	webhook := ProviderPaymentWebhook{
		ProviderEventID:   "evt_success_" + idempotencyKey,
		ProviderPaymentID: *providerPaymentID,
		OrderID:           orderID,
		EventType:         "payment_succeeded",
	}
	webhook.Signature = SignProviderWebhook(webhook, secret)
	resp, err := r.HandleProviderWebhook(ctx, webhook, secret, traceID)
	if err != nil {
		return PaymentResponse{}, err
	}
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		return PaymentResponse{}, err
	}
	tx, err = r.beginTx(ctx)
	if err != nil {
		return PaymentResponse{}, err
	}
	defer rollback(ctx, tx)
	if err := completeIdempotency(ctx, tx, "payment", orderID, userID, idempotencyKey, requestHash, http.StatusOK, OrderStatusPaid, responseJSON); err != nil {
		return PaymentResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentResponse{}, err
	}
	return resp, nil
}

func (r *Repository) HandleProviderWebhook(ctx context.Context, input ProviderPaymentWebhook, secret string, traceID string) (PaymentResponse, error) {
	if input.ProviderEventID == "" || input.ProviderPaymentID == "" || input.OrderID == "" || input.EventType == "" {
		return PaymentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "provider_event_id, provider_payment_id, order_id, and event_type are required", http.StatusBadRequest)
	}
	signatureValid := VerifyProviderWebhook(input, secret)
	tx, err := r.beginTx(ctx)
	if err != nil {
		return PaymentResponse{}, err
	}
	defer rollback(ctx, tx)

	payload, err := json.Marshal(map[string]any{
		"provider_event_id":   input.ProviderEventID,
		"provider_payment_id": input.ProviderPaymentID,
		"order_id":            input.OrderID,
		"event_type":          input.EventType,
	})
	if err != nil {
		return PaymentResponse{}, err
	}
	inserted := false
	if err := tx.QueryRow(ctx, `
		INSERT INTO payment_events (provider, provider_event_id, provider_payment_id, order_id, event_type, signature_valid, processed_at, payload_json, trace_id)
		VALUES ('local_fake', $1, $2, $3, $4, $5, CASE WHEN $5 THEN now() ELSE NULL END, $6, $7)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
		RETURNING true
	`, input.ProviderEventID, input.ProviderPaymentID, input.OrderID, input.EventType, signatureValid, payload, traceID).Scan(&inserted); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return PaymentResponse{}, err
	}
	if !signatureValid {
		if err := recordPaymentAnomaly(ctx, tx, input.OrderID, "PAYMENT_WEBHOOK_INVALID_SIGNATURE", "invalid fake provider webhook signature", map[string]any{
			"provider_event_id":   input.ProviderEventID,
			"provider_payment_id": input.ProviderPaymentID,
			"trace_id":            traceID,
		}); err != nil {
			return PaymentResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return PaymentResponse{}, err
		}
		return PaymentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "invalid provider signature", http.StatusUnauthorized)
	}

	var auctionID string
	var winnerID string
	var amountCents int64
	var status string
	var depositStatus string
	var paidAt *time.Time
	var existingProviderID *string
	if err := tx.QueryRow(ctx, `
		SELECT auction_id, winner_id, amount_cents, status, deposit_status, paid_at, provider_payment_id
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, input.OrderID).Scan(&auctionID, &winnerID, &amountCents, &status, &depositStatus, &paidAt, &existingProviderID); err != nil {
		return PaymentResponse{}, mapOrderNotFound(err)
	}
	if existingProviderID != nil && *existingProviderID != input.ProviderPaymentID {
		return PaymentResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "provider payment id does not match order", http.StatusConflict)
	}
	if status == OrderStatusExpired {
		if err := recordPaymentAnomaly(ctx, tx, input.OrderID, "PAYMENT_RECONCILE_MISMATCH", "late provider success for expired order", map[string]any{
			"provider_event_id": input.ProviderEventID,
			"status":            status,
			"trace_id":          traceID,
		}); err != nil {
			return PaymentResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return PaymentResponse{}, err
		}
		return PaymentResponse{}, apierrors.New(apierrors.CodeOrderAlreadyExpired, "order already expired", http.StatusConflict)
	}
	now := time.Now().UTC()
	if status != OrderStatusPaid {
		if err := tx.QueryRow(ctx, `
			UPDATE orders
			SET status = 'PAID',
			    deposit_status = 'REFUNDED',
			    paid_at = COALESCE(paid_at, $2),
			    payment_succeeded_at = COALESCE(payment_succeeded_at, $2),
			    provider_payment_id = COALESCE(provider_payment_id, $3)
			WHERE id = $1
			RETURNING paid_at, deposit_status
		`, input.OrderID, now, input.ProviderPaymentID).Scan(&paidAt, &depositStatus); err != nil {
			return PaymentResponse{}, err
		}
		if err := appendAuctionEvent(ctx, tx, auctionID, "order_paid", traceID, map[string]any{
			"order_id":            input.OrderID,
			"user_id":             winnerID,
			"amount_cents":        amountCents,
			"order_status":        OrderStatusPaid,
			"deposit_status":      DepositStatusRefunded,
			"provider_payment_id": input.ProviderPaymentID,
			"paid_at":             *paidAt,
		}); err != nil {
			return PaymentResponse{}, err
		}
	}
	if paidAt == nil {
		paidAt = &now
	}
	resp := PaymentResponse{OrderID: input.OrderID, OrderStatus: OrderStatusPaid, PaidAt: *paidAt, DepositStatus: DepositStatusRefunded, ProviderPaymentID: input.ProviderPaymentID}
	if err := tx.Commit(ctx); err != nil {
		return PaymentResponse{}, err
	}
	return resp, nil
}

func (r *Repository) ReconcileProviderPayments(ctx context.Context, limit int, grace time.Duration, traceID string) (PaymentReconcileReport, error) {
	if limit <= 0 {
		limit = 100
	}
	if grace <= 0 {
		grace = time.Minute
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, auction_id, winner_id, provider_payment_id, payment_initiated_at
		FROM orders
		WHERE status = 'PAYMENT_INITIATED'
		  AND provider_payment_id IS NOT NULL
		ORDER BY payment_initiated_at DESC NULLS LAST, created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return PaymentReconcileReport{}, err
	}
	defer rows.Close()

	report := PaymentReconcileReport{}
	now := time.Now().UTC()
	for rows.Next() {
		report.Checked++
		var orderID string
		var auctionID string
		var winnerID string
		var providerPaymentID string
		var initiatedAt *time.Time
		if err := rows.Scan(&orderID, &auctionID, &winnerID, &providerPaymentID, &initiatedAt); err != nil {
			return report, err
		}
		var providerEventID string
		err := r.db.QueryRow(ctx, `
			SELECT provider_event_id
			FROM payment_events
			WHERE order_id = $1
			  AND provider_payment_id = $2
			  AND event_type = 'payment_succeeded'
			  AND signature_valid = true
			ORDER BY created_at DESC
			LIMIT 1
		`, orderID, providerPaymentID).Scan(&providerEventID)
		if err == nil {
			webhook := ProviderPaymentWebhook{
				ProviderEventID:   providerEventID + "_reconcile",
				ProviderPaymentID: providerPaymentID,
				OrderID:           orderID,
				EventType:         "payment_succeeded",
			}
			webhook.Signature = SignProviderWebhook(webhook, DefaultFakePaymentWebhookSecret)
			if _, err := r.HandleProviderWebhook(ctx, webhook, DefaultFakePaymentWebhookSecret, traceID); err != nil {
				return report, err
			}
			report.Repaired++
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return report, err
		}
		if initiatedAt == nil || now.Sub(*initiatedAt) < grace {
			continue
		}
		tx, err := r.beginTx(ctx)
		if err != nil {
			return report, err
		}
		if err := recordPaymentAnomaly(ctx, tx, orderID, "PAYMENT_RECONCILE_MISMATCH", "payment initiated without provider success event", map[string]any{
			"order_id":            orderID,
			"auction_id":          auctionID,
			"user_id":             winnerID,
			"provider_payment_id": providerPaymentID,
			"trace_id":            traceID,
		}); err != nil {
			_ = tx.Rollback(ctx)
			return report, err
		}
		if err := tx.Commit(ctx); err != nil {
			return report, err
		}
		report.Anomaly++
	}
	return report, rows.Err()
}

func SignProviderWebhook(input ProviderPaymentWebhook, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(providerWebhookSigningPayload(input)))
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyProviderWebhook(input ProviderPaymentWebhook, secret string) bool {
	expected := SignProviderWebhook(ProviderPaymentWebhook{
		ProviderEventID:   input.ProviderEventID,
		ProviderPaymentID: input.ProviderPaymentID,
		OrderID:           input.OrderID,
		EventType:         input.EventType,
	}, secret)
	provided, err := hex.DecodeString(input.Signature)
	if err != nil {
		return false
	}
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	return hmac.Equal(provided, expectedBytes)
}

func providerWebhookSigningPayload(input ProviderPaymentWebhook) string {
	return fmt.Sprintf("%s|%s|%s|%s", input.ProviderEventID, input.ProviderPaymentID, input.OrderID, input.EventType)
}

func insertPaymentEvent(ctx context.Context, tx pgx.Tx, providerEventID string, providerPaymentID string, orderID string, eventType string, signatureValid bool, traceID string, payload map[string]any) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO payment_events (provider, provider_event_id, provider_payment_id, order_id, event_type, signature_valid, processed_at, payload_json, trace_id)
		VALUES ('local_fake', $1, $2, $3, $4, $5, CASE WHEN $5 THEN now() ELSE NULL END, $6, $7)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
	`, providerEventID, providerPaymentID, orderID, eventType, signatureValid, payloadJSON, traceID)
	return err
}

func recordPaymentAnomaly(ctx context.Context, tx pgx.Tx, orderID string, anomalyType string, message string, payload map[string]any) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var auctionID *string
	_ = tx.QueryRow(ctx, `SELECT auction_id FROM orders WHERE id = $1`, orderID).Scan(&auctionID)
	_, err = tx.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('MED', $1, $2, $3, $4)
	`, anomalyType, auctionID, message, payloadJSON)
	return err
}

type lockedAuction struct {
	ID                  string
	Status              Status
	CurrentPriceCents   int64
	CurrentWinnerID     *string
	StartPriceCents     int64
	IncrementCents      int64
	CapPriceCents       *int64
	EndAt               time.Time
	Seq                 int64
	AcceptedBidCount    int64
	ExtendCount         int
	DurationSeconds     int
	ExtendWindowSeconds int
	ExtendBySeconds     int
	MaxExtendCount      int
	DepositBPS          int16
	DepositFloorCents   int64
	DepositCapCents     int64
	FatFingerThreshold  *int64
}

func lockAuctionForBid(ctx context.Context, tx pgx.Tx, auctionID string) (lockedAuction, error) {
	var a lockedAuction
	start := time.Now()
	err := tx.QueryRow(ctx, `
		SELECT
			a.id, a.status, a.current_price_cents, a.current_winner_id,
			a.start_price_cents, a.increment_cents, a.cap_price_cents,
			a.end_at, a.seq, a.accepted_bid_count, a.extend_count,
			ar.duration_seconds, ar.extend_window_seconds, ar.extend_by_seconds,
			ar.max_extend_count, ar.fat_finger_threshold_cents, COALESCE(ar.deposit_bps, $2),
			COALESCE(ar.deposit_floor_cents, $3), COALESCE(ar.deposit_cap_cents, $4)
		FROM auctions a
		JOIN auction_rules ar ON ar.auction_id = a.id AND ar.rule_version = a.rule_version
		WHERE a.id = $1
		FOR UPDATE OF a
	`, auctionID, defaultDepositBPS, defaultDepositFloorCents, defaultDepositCapCents).Scan(
		&a.ID, &a.Status, &a.CurrentPriceCents, &a.CurrentWinnerID,
		&a.StartPriceCents, &a.IncrementCents, &a.CapPriceCents,
		&a.EndAt, &a.Seq, &a.AcceptedBidCount, &a.ExtendCount,
		&a.DurationSeconds, &a.ExtendWindowSeconds, &a.ExtendBySeconds,
		&a.MaxExtendCount, &a.FatFingerThreshold, &a.DepositBPS, &a.DepositFloorCents, &a.DepositCapCents,
	)
	observability.Observe("auction_bid_lock_wait_seconds", time.Since(start).Seconds(), nil, observability.DefaultLatencyBuckets)
	observability.Observe("db_query_latency_seconds", time.Since(start).Seconds(), map[string]string{"query": "lock_auction_for_bid"}, observability.DefaultLatencyBuckets)
	if err != nil {
		return lockedAuction{}, mapPGError(mapNotFound(err))
	}
	return a, nil
}

func currentSeq(a lockedAuction) int64 {
	return a.Seq
}

func shouldBroadcastRejectedBid(code apierrors.Code) bool {
	switch code {
	case apierrors.CodeBidTooLow, apierrors.CodeAuctionEnded, apierrors.CodeAuctionNotActive:
		return false
	default:
		return true
	}
}

func (r *Repository) evaluateAndApplyBid(ctx context.Context, tx pgx.Tx, a lockedAuction, userID string, input BidInput, traceID string, skipFatFinger bool) (BidResponse, string, *string, bool, lockedAuction, error) {
	bidID := "bid_" + uuid.NewString()
	now := time.Now().UTC()
	serverTimeMS := now.UnixMilli()
	reject := func(code apierrors.Code) (BidResponse, string, *string, bool, lockedAuction, error) {
		reason := string(code)
		seq := currentSeq(a)
		durableRealtime := shouldBroadcastRejectedBid(code)
		if durableRealtime {
			var err error
			seq, err = appendAuctionEventWithSeq(ctx, tx, a.ID, "bid_rejected", traceID, map[string]any{
				"bid_id":       bidID,
				"user_id":      userID,
				"amount_cents": input.AmountCents,
				"reason":       reason,
			})
			if err != nil {
				return BidResponse{}, "", nil, false, a, err
			}
		}
		resp := BidResponse{
			Result:            BidResultRejected,
			BidID:             bidID,
			AuctionID:         a.ID,
			Seq:               seq,
			CurrentPriceCents: a.CurrentPriceCents,
			CurrentWinnerID:   a.CurrentWinnerID,
			EndAt:             &a.EndAt,
			ServerTimeMS:      serverTimeMS,
			RejectReason:      &reason,
		}
		return resp, "REJECTED", &reason, durableRealtime, a, nil
	}
	if a.Status != StatusActive {
		resp, status, reason, durableRealtime, locked, err := reject(apierrors.CodeAuctionNotActive)
		return resp, status, reason, durableRealtime, locked, err
	}
	if now.After(a.EndAt) {
		resp, status, reason, durableRealtime, locked, err := reject(apierrors.CodeAuctionEnded)
		return resp, status, reason, durableRealtime, locked, err
	}
	if a.CurrentWinnerID != nil && *a.CurrentWinnerID == userID {
		resp, status, reason, durableRealtime, locked, err := reject(apierrors.CodeRejectedSelfLeading)
		return resp, status, reason, durableRealtime, locked, err
	}

	class := ClassifyBidAmount(a.StartPriceCents, a.CurrentPriceCents, a.IncrementCents, a.CapPriceCents, input.AmountCents, a.AcceptedBidCount > 0)
	switch class {
	case BidClassTooLow:
		resp, status, reason, durableRealtime, locked, err := reject(apierrors.CodeBidTooLow)
		return resp, status, reason, durableRealtime, locked, err
	case BidClassIncrementMismatch:
		resp, status, reason, durableRealtime, locked, err := reject(apierrors.CodeBidIncrementMismatch)
		return resp, status, reason, durableRealtime, locked, err
	case BidClassAboveCap:
		resp, status, reason, durableRealtime, locked, err := reject(apierrors.CodeBidAboveCap)
		return resp, status, reason, durableRealtime, locked, err
	}
	if !skipFatFinger && a.FatFingerThreshold != nil && *a.FatFingerThreshold > 0 {
		basis := a.CurrentPriceCents
		if a.AcceptedBidCount == 0 {
			basis = a.StartPriceCents
		}
		if input.AmountCents-basis >= *a.FatFingerThreshold {
			var seq int64
			if err := tx.QueryRow(ctx, `SELECT seq FROM auctions WHERE id = $1`, a.ID).Scan(&seq); err != nil {
				return BidResponse{}, "", nil, false, a, err
			}
			expiresAt := now.Add(30 * time.Second)
			return BidResponse{
				Result:            string(apierrors.CodeFatFingerConfirmRequired),
				AuctionID:         a.ID,
				Seq:               seq,
				CurrentPriceCents: a.CurrentPriceCents,
				CurrentWinnerID:   a.CurrentWinnerID,
				EndAt:             &a.EndAt,
				ServerTimeMS:      serverTimeMS,
				RejectReason:      nil,
				ConfirmToken:      "ft_" + uuid.NewString(),
				ExpiresInMS:       30_000,
				AmountCents:       input.AmountCents,
				ConfirmExpiresAt:  &expiresAt,
			}, "", nil, false, a, nil
		}
	}

	updated, resp, err := applyAcceptedBid(ctx, tx, a, acceptedBidInput{
		BidID:       bidID,
		UserID:      userID,
		AmountCents: input.AmountCents,
		TraceID:     traceID,
		Source:      BidSourceManual,
		Now:         now,
	})
	if err != nil {
		return BidResponse{}, "", nil, false, a, err
	}
	resp.ServerTimeMS = serverTimeMS
	return resp, "ACCEPTED", nil, true, updated, nil
}

type acceptedBidInput struct {
	BidID       string
	UserID      string
	AmountCents int64
	TraceID     string
	Source      string
	Now         time.Time
}

func applyAcceptedBid(ctx context.Context, tx pgx.Tx, a lockedAuction, input acceptedBidInput) (lockedAuction, BidResponse, error) {
	class := ClassifyBidAmount(a.StartPriceCents, a.CurrentPriceCents, a.IncrementCents, a.CapPriceCents, input.AmountCents, a.AcceptedBidCount > 0)
	if class != BidClassAccepted && class != BidClassAcceptedSold {
		return a, BidResponse{}, fmt.Errorf("accepted bid amount %d classified as %s", input.AmountCents, class)
	}
	result := BidResultAccepted
	newStatus := a.Status
	newEndAt := a.EndAt
	newExtendCount := a.ExtendCount
	if class == BidClassAcceptedSold {
		result = BidResultAcceptedSold
		newStatus = StatusSold
	} else {
		extended := CalculateExtension(a.EndAt.Unix(), input.Now.Unix(), a.ExtendWindowSeconds, a.ExtendBySeconds, a.ExtendCount, a.MaxExtendCount)
		if extended > a.EndAt.Unix() {
			result = BidResultAcceptedExtended
			newEndAt = time.Unix(extended, 0).UTC()
			newExtendCount++
		}
	}

	_, err := tx.Exec(ctx, `
		UPDATE auctions
		SET status = $2, current_price_cents = $3, current_winner_id = $4,
		    end_at = $5, extend_count = $6, accepted_bid_count = accepted_bid_count + 1,
		    updated_at = now()
		WHERE id = $1
	`, a.ID, newStatus, input.AmountCents, input.UserID, newEndAt, newExtendCount)
	if err != nil {
		return a, BidResponse{}, err
	}
	eventType := "bid_accepted"
	if result == BidResultAcceptedSold {
		eventType = "auction_sold"
	}
	eventPayload := map[string]any{
		"bid_id":              input.BidID,
		"user_id":             input.UserID,
		"amount_cents":        input.AmountCents,
		"result":              result,
		"current_price_cents": input.AmountCents,
	}
	if input.Source == BidSourceAutoMaxBid {
		eventPayload["bid_source"] = BidSourceAutoMaxBid
	}
	if result == BidResultAcceptedSold {
		orderID, err := createOrderForSoldAuction(ctx, tx, a, input.UserID, input.AmountCents)
		if err != nil {
			return a, BidResponse{}, err
		}
		eventPayload["order_id"] = orderID
	}
	seq, err := appendAuctionEventWithSeq(ctx, tx, a.ID, eventType, input.TraceID, eventPayload)
	if err != nil {
		return a, BidResponse{}, err
	}
	winnerID := input.UserID
	updated := a
	updated.Status = newStatus
	updated.CurrentPriceCents = input.AmountCents
	updated.CurrentWinnerID = &winnerID
	updated.EndAt = newEndAt
	updated.ExtendCount = newExtendCount
	updated.Seq = seq
	updated.AcceptedBidCount++
	resp := BidResponse{
		Result:            result,
		BidID:             input.BidID,
		AuctionID:         a.ID,
		Seq:               seq,
		CurrentPriceCents: input.AmountCents,
		CurrentWinnerID:   &winnerID,
		EndAt:             &newEndAt,
		ServerTimeMS:      input.Now.UnixMilli(),
	}
	return updated, resp, nil
}

func (r *Repository) applyAutoMaxBid(ctx context.Context, tx pgx.Tx, a lockedAuction, traceID string) (BidResponse, bool, error) {
	var last BidResponse
	applied := false
	current := a
	for attempts := 0; attempts < 100; attempts++ {
		if current.Status != StatusActive {
			if err := markActiveMaxBidIntentsTerminal(ctx, tx, current.ID); err != nil {
				return BidResponse{}, false, err
			}
			return last, applied, nil
		}
		step, ok, err := r.nextAutoMaxBidStep(ctx, tx, current)
		if err != nil {
			return BidResponse{}, false, err
		}
		if !ok {
			if err := exhaustUnaffordableMaxBidIntents(ctx, tx, current); err != nil {
				return BidResponse{}, false, err
			}
			return last, applied, nil
		}

		amount := step.AmountCents
		bidID := "bid_" + uuid.NewString()
		now := time.Now().UTC()
		updated, resp, err := applyAcceptedBid(ctx, tx, current, acceptedBidInput{
			BidID:       bidID,
			UserID:      step.Intent.UserID,
			AmountCents: amount,
			TraceID:     traceID,
			Source:      BidSourceAutoMaxBid,
			Now:         now,
		})
		if err != nil {
			return BidResponse{}, false, err
		}
		resp.ServerTimeMS = now.UnixMilli()
		clientBidID := fmt.Sprintf("auto:%s:%d", step.Intent.ID, resp.Seq)
		requestHash := hashString(fmt.Sprintf("auto-max-bid:v1|%s|%s|%s|%d|%d", current.ID, step.Intent.ID, clientBidID, amount, resp.Seq))
		responseJSON, err := json.Marshal(resp)
		if err != nil {
			return BidResponse{}, false, err
		}
		if err := insertBidRow(ctx, tx, resp.BidID, current.ID, step.Intent.UserID, BidInput{
			ClientBidID:   clientBidID,
			AmountCents:   amount,
			ClientSeenSeq: resp.Seq - 1,
		}, nullableSeq(true, resp.Seq), "ACCEPTED", nil, requestHash, responseJSON, traceID, BidSourceAutoMaxBid); err != nil {
			return BidResponse{}, false, err
		}
		if err := markMaxBidIntentApplied(ctx, tx, step.Intent.ID, resp.Seq); err != nil {
			return BidResponse{}, false, err
		}
		if step.ExhaustedIntentID != "" {
			if err := markMaxBidIntentExhausted(ctx, tx, step.ExhaustedIntentID); err != nil {
				return BidResponse{}, false, err
			}
		}
		current = updated
		last = resp
		applied = true
	}
	return BidResponse{}, false, apierrors.New(apierrors.CodeBidRetryLater, "max bid settlement exceeded bounded iteration limit", http.StatusConflict)
}

type autoMaxBidStep struct {
	Intent            MaxBidIntent
	AmountCents       int64
	ExhaustedIntentID string
}

func (r *Repository) nextAutoMaxBidStep(ctx context.Context, tx pgx.Tx, a lockedAuction) (autoMaxBidStep, bool, error) {
	intents, err := r.ListActiveMaxBidIntentsForAuction(ctx, tx, a.ID, 100)
	if err != nil {
		return autoMaxBidStep{}, false, err
	}
	if len(intents) == 0 {
		return autoMaxBidStep{}, false, nil
	}
	minimum := nextExecutableBidAmount(a)
	var defender *MaxBidIntent
	for i := range intents {
		if a.CurrentWinnerID != nil && intents[i].UserID == *a.CurrentWinnerID {
			defender = &intents[i]
			break
		}
	}
	for _, intent := range intents {
		if a.CurrentWinnerID != nil && intent.UserID == *a.CurrentWinnerID {
			continue
		}
		if intent.MaxAmountCents < minimum {
			if err := markMaxBidIntentExhausted(ctx, tx, intent.ID); err != nil {
				return autoMaxBidStep{}, false, err
			}
			continue
		}
		if defender != nil && defenderBeatsOrTies(*defender, intent) {
			amount := defendingBidAmount(*defender, intent, a)
			if amount < minimum {
				if err := markMaxBidIntentExhausted(ctx, tx, intent.ID); err != nil {
					return autoMaxBidStep{}, false, err
				}
				continue
			}
			return autoMaxBidStep{Intent: *defender, AmountCents: amount, ExhaustedIntentID: intent.ID}, true, nil
		}
		return autoMaxBidStep{Intent: intent, AmountCents: minimum}, true, nil
	}
	return autoMaxBidStep{}, false, nil
}

func nextExecutableBidAmount(a lockedAuction) int64 {
	return a.CurrentPriceCents + a.IncrementCents
}

func defenderBeatsOrTies(defender MaxBidIntent, challenger MaxBidIntent) bool {
	if defender.MaxAmountCents != challenger.MaxAmountCents {
		return defender.MaxAmountCents > challenger.MaxAmountCents
	}
	if !defender.CreatedAt.Equal(challenger.CreatedAt) {
		return defender.CreatedAt.Before(challenger.CreatedAt)
	}
	return defender.ID < challenger.ID
}

func defendingBidAmount(defender MaxBidIntent, challenger MaxBidIntent, a lockedAuction) int64 {
	amount := challenger.MaxAmountCents
	if defender.MaxAmountCents > challenger.MaxAmountCents {
		amount = challenger.MaxAmountCents + a.IncrementCents
	}
	if amount > defender.MaxAmountCents {
		amount = defender.MaxAmountCents
	}
	if a.CapPriceCents != nil && amount > *a.CapPriceCents {
		amount = *a.CapPriceCents
	}
	return amount
}

func markMaxBidIntentApplied(ctx context.Context, tx pgx.Tx, intentID string, seq int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE max_bid_intents
		SET last_applied_seq = $2,
		    updated_at = now(),
		    version = version + 1
		WHERE id = $1 AND status = 'ACTIVE'
	`, intentID, seq)
	return err
}

func markMaxBidIntentExhausted(ctx context.Context, tx pgx.Tx, intentID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE max_bid_intents
		SET status = 'EXHAUSTED',
		    exhausted_at = COALESCE(exhausted_at, now()),
		    updated_at = now(),
		    version = version + 1
		WHERE id = $1 AND status = 'ACTIVE'
	`, intentID)
	return err
}

func exhaustUnaffordableMaxBidIntents(ctx context.Context, tx pgx.Tx, a lockedAuction) error {
	_, err := tx.Exec(ctx, `
		UPDATE max_bid_intents
		SET status = 'EXHAUSTED',
		    exhausted_at = COALESCE(exhausted_at, now()),
		    updated_at = now(),
		    version = version + 1
		WHERE auction_id = $1 AND status = 'ACTIVE' AND max_amount_cents < $2
	`, a.ID, nextExecutableBidAmount(a))
	return err
}

func markActiveMaxBidIntentsTerminal(ctx context.Context, tx pgx.Tx, auctionID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE max_bid_intents
		SET status = 'TERMINAL',
		    updated_at = now(),
		    version = version + 1
		WHERE auction_id = $1 AND status = 'ACTIVE'
	`, auctionID)
	return err
}

func createOrderForSoldAuction(ctx context.Context, tx pgx.Tx, a lockedAuction, winnerID string, amountCents int64) (string, error) {
	deposit := CalculateDeposit(amountCents, int64(a.DepositBPS), a.DepositFloorCents, a.DepositCapCents)
	orderID := "ord_" + uuid.NewString()
	expireAt := time.Now().UTC().Add(15 * time.Minute)
	if _, err := tx.Exec(ctx, `
		INSERT INTO orders (id, auction_id, winner_id, amount_cents, status, deposit_cents, deposit_status, expire_at)
		VALUES ($1, $2, $3, $4, 'ORDER_PENDING', $5, 'HELD', $6)
	`, orderID, a.ID, winnerID, amountCents, deposit, expireAt); err != nil {
		return "", err
	}
	if err := upsertSchedulerJob(ctx, tx, "EXPIRE_ORDER", "order", orderID, "expire:"+orderID, expireAt); err != nil {
		return "", err
	}
	return orderID, nil
}

func nullableSeq(durableRealtime bool, seq int64) *int64 {
	if !durableRealtime {
		return nil
	}
	return &seq
}

func insertBidRow(ctx context.Context, tx pgx.Tx, bidID string, auctionID string, userID string, input BidInput, seq *int64, status string, rejectReason *string, requestHash string, responseJSON []byte, traceID string, source string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO bids (id, auction_id, user_id, client_bid_id, amount_cents, seq, status, reject_reason, request_hash, response_json, trace_id, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, bidID, auctionID, userID, input.ClientBidID, input.AmountCents, seq, status, rejectReason, requestHash, responseJSON, traceID, source)
	return err
}

func upsertProcessing(ctx context.Context, tx pgx.Tx, scopeType string, scopeID string, userID string, idempotencyKey string, requestHash string) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO idempotency_records (scope_type, scope_id, user_id, idempotency_key, request_hash, status, attempts, locked_until)
		VALUES ($1, $2, $3, $4, $5, 'PROCESSING', 1, now() + interval '10 seconds')
		ON CONFLICT (scope_type, scope_id, user_id, idempotency_key)
		DO NOTHING
	`, scopeType, scopeID, userID, idempotencyKey, requestHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	var storedHash string
	var status string
	var lockedUntil *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT request_hash, status, locked_until
		FROM idempotency_records
		WHERE scope_type = $1 AND scope_id = $2 AND user_id = $3 AND idempotency_key = $4
		FOR UPDATE
	`, scopeType, scopeID, userID, idempotencyKey).Scan(&storedHash, &status, &lockedUntil); err != nil {
		return err
	}
	if storedHash != requestHash {
		return apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "idempotency key reused with different request", http.StatusConflict)
	}
	if status == IdempotencyStatusCompleted {
		return apierrors.New(apierrors.CodeProcessingRetryLater, "idempotency completed after replay probe; retry to fetch result", http.StatusConflict)
	}
	if status == IdempotencyStatusProcessing && lockedUntil != nil && time.Now().UTC().After(*lockedUntil) {
		if _, err := tx.Exec(ctx, `
			UPDATE idempotency_records
			SET status = 'FAILED', locked_until = NULL, completed_at = now(), result_code = $5
			WHERE scope_type = $1 AND scope_id = $2 AND user_id = $3 AND idempotency_key = $4
		`, scopeType, scopeID, userID, idempotencyKey, apierrors.CodeIdempotencyTimeout); err != nil {
			return err
		}
		return apierrors.New(apierrors.CodeIdempotencyTimeout, "previous idempotent operation timed out", http.StatusConflict)
	}
	return apierrors.New(apierrors.CodeProcessingRetryLater, "same idempotency key is still processing", http.StatusConflict)
}

func completeIdempotency(ctx context.Context, tx pgx.Tx, scopeType string, scopeID string, userID string, idempotencyKey string, requestHash string, httpStatus int, resultCode string, responseJSON []byte) error {
	_, err := tx.Exec(ctx, `
		UPDATE idempotency_records
		SET status = 'COMPLETED', http_status = $6, result_code = $7,
		    response_json = $8, completed_at = now(), locked_until = NULL
		WHERE scope_type = $1 AND scope_id = $2 AND user_id = $3
		  AND idempotency_key = $4 AND request_hash = $5
	`, scopeType, scopeID, userID, idempotencyKey, requestHash, httpStatus, resultCode, responseJSON)
	return err
}

func (r *Repository) completedIdempotency(ctx context.Context, scopeType string, scopeID string, userID string, idempotencyKey string, requestHash string) (BidResponse, bool, error) {
	var storedHash string
	var status string
	var responseJSON []byte
	var lockedUntil *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT request_hash, status, response_json, locked_until
		FROM idempotency_records
		WHERE scope_type = $1 AND scope_id = $2 AND user_id = $3 AND idempotency_key = $4
	`, scopeType, scopeID, userID, idempotencyKey).Scan(&storedHash, &status, &responseJSON, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return BidResponse{}, false, nil
	}
	if err != nil {
		return BidResponse{}, false, err
	}
	if storedHash != requestHash {
		return BidResponse{}, false, apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "idempotency key reused with different request", http.StatusConflict)
	}
	if status == IdempotencyStatusProcessing && lockedUntil != nil && time.Now().UTC().After(*lockedUntil) {
		return BidResponse{}, false, r.markIdempotencyTimeout(ctx, scopeType, scopeID, userID, idempotencyKey)
	}
	if status != IdempotencyStatusCompleted {
		return BidResponse{}, false, apierrors.New(apierrors.CodeProcessingRetryLater, "same idempotency key is still processing", http.StatusConflict)
	}
	var resp BidResponse
	if err := json.Unmarshal(responseJSON, &resp); err != nil {
		return BidResponse{}, false, err
	}
	return resp, true, nil
}

func (r *Repository) completedPaymentIdempotency(ctx context.Context, scopeID string, userID string, idempotencyKey string, requestHash string) (PaymentResponse, bool, error) {
	var storedHash string
	var status string
	var responseJSON []byte
	var lockedUntil *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT request_hash, status, response_json, locked_until
		FROM idempotency_records
		WHERE scope_type = 'payment' AND scope_id = $1 AND user_id = $2 AND idempotency_key = $3
	`, scopeID, userID, idempotencyKey).Scan(&storedHash, &status, &responseJSON, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentResponse{}, false, nil
	}
	if err != nil {
		return PaymentResponse{}, false, err
	}
	if storedHash != requestHash {
		return PaymentResponse{}, false, apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "idempotency key reused with different request", http.StatusConflict)
	}
	if status == IdempotencyStatusProcessing && lockedUntil != nil && time.Now().UTC().After(*lockedUntil) {
		return PaymentResponse{}, false, r.markIdempotencyTimeout(ctx, "payment", scopeID, userID, idempotencyKey)
	}
	if status != IdempotencyStatusCompleted {
		return PaymentResponse{}, false, apierrors.New(apierrors.CodeProcessingRetryLater, "same idempotency key is still processing", http.StatusConflict)
	}
	var resp PaymentResponse
	if err := json.Unmarshal(responseJSON, &resp); err != nil {
		return PaymentResponse{}, false, err
	}
	return resp, true, nil
}

func (r *Repository) markIdempotencyTimeout(ctx context.Context, scopeType string, scopeID string, userID string, idempotencyKey string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE idempotency_records
		SET status = 'FAILED', locked_until = NULL, completed_at = now(), result_code = $5
		WHERE scope_type = $1 AND scope_id = $2 AND user_id = $3 AND idempotency_key = $4
		  AND status = 'PROCESSING'
	`, scopeType, scopeID, userID, idempotencyKey, apierrors.CodeIdempotencyTimeout)
	if err != nil {
		return err
	}
	return apierrors.New(apierrors.CodeIdempotencyTimeout, "previous idempotent operation timed out", http.StatusConflict)
}

func bidRequestHash(auctionID string, userID string, clientBidID string, amountCents int64) string {
	return hashString(fmt.Sprintf("bid:v1|%s|%s|%s|%d", auctionID, userID, clientBidID, amountCents))
}

func paymentRequestHash(orderID string, userID string, idempotencyKey string) string {
	return hashString(fmt.Sprintf("payment:v1|%s|%s|%s", orderID, userID, idempotencyKey))
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mapOrderNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apierrors.New(apierrors.CodeInvalidArgument, "order not found", http.StatusNotFound)
	}
	return err
}

func (r *Repository) ListOrders(ctx context.Context, userID string, role string) ([]Order, error) {
	query := `
		SELECT id, auction_id, winner_id, amount_cents, status, deposit_cents, deposit_status, expire_at, paid_at, provider_payment_id, created_at
		FROM orders
	`
	args := []any{}
	if role != "host" {
		query += ` WHERE winner_id = $1`
		args = append(args, userID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := []Order{}
	for rows.Next() {
		var order Order
		if err := rows.Scan(&order.ID, &order.AuctionID, &order.WinnerID, &order.AmountCents, &order.Status, &order.DepositCents, &order.DepositStatus, &order.ExpireAt, &order.PaidAt, &order.ProviderID, &order.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

func (r *Repository) ListBidHistory(ctx context.Context, userID string) ([]BidHistoryRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, auction_id, amount_cents, COALESCE(response_json->>'result', status), created_at
		FROM bids
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bids := []BidHistoryRow{}
	for rows.Next() {
		var bid BidHistoryRow
		if err := rows.Scan(&bid.BidID, &bid.AuctionID, &bid.AmountCents, &bid.Result, &bid.CreatedAt); err != nil {
			return nil, err
		}
		bids = append(bids, bid)
	}
	return bids, rows.Err()
}

func ToOrderHistoryRows(orders []Order) []OrderHistoryRow {
	rows := make([]OrderHistoryRow, 0, len(orders))
	for _, order := range orders {
		rows = append(rows, OrderHistoryRow{
			OrderID:     order.ID,
			AuctionID:   order.AuctionID,
			AmountCents: order.AmountCents,
			OrderStatus: order.Status,
			CreatedAt:   order.CreatedAt,
		})
	}
	return rows
}
