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

	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
)

const (
	BidResultAccepted         = "ACCEPTED"
	BidResultAcceptedExtended = "ACCEPTED_EXTENDED"
	BidResultAcceptedSold     = "ACCEPTED_SOLD"
	BidResultRejected         = "REJECTED"

	IdempotencyStatusProcessing = "PROCESSING"
	IdempotencyStatusCompleted  = "COMPLETED"

	OrderStatusPending    = "ORDER_PENDING"
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
	CreatedAt     time.Time  `json:"created_at"`
}

type PaymentResponse struct {
	OrderID       string    `json:"order_id"`
	OrderStatus   string    `json:"order_status"`
	PaidAt        time.Time `json:"paid_at"`
	DepositStatus string    `json:"deposit_status"`
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
	response, bidStatus, rejectCode, err := r.evaluateAndApplyBid(ctx, tx, locked, userID, input, traceID, false)
	if err != nil {
		return BidResponse{}, err
	}
	resultLabel = response.Result
	if rejectCode != nil {
		reasonLabel = *rejectCode
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return BidResponse{}, err
	}
	if err := completeIdempotency(ctx, tx, "bid", auctionID, userID, idempotencyKey, requestHash, http.StatusOK, response.Result, responseJSON); err != nil {
		return BidResponse{}, err
	}
	if response.Result != string(apierrors.CodeFatFingerConfirmRequired) {
		if err := insertBidRow(ctx, tx, response.BidID, auctionID, userID, input, response.Seq, bidStatus, rejectCode, requestHash, responseJSON, traceID); err != nil {
			return BidResponse{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BidResponse{}, err
	}
	return response, nil
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
	response, bidStatus, rejectCode, err := r.evaluateAndApplyBid(ctx, tx, locked, userID, bidInput, traceID, true)
	if err != nil {
		return BidResponse{}, err
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return BidResponse{}, err
	}
	if err := completeIdempotency(ctx, tx, "bid", auctionID, userID, idempotencyKey, storedHash, http.StatusOK, response.Result, responseJSON); err != nil {
		return BidResponse{}, err
	}
	if err := insertBidRow(ctx, tx, response.BidID, auctionID, userID, bidInput, response.Seq, bidStatus, rejectCode, storedHash, responseJSON, traceID); err != nil {
		return BidResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BidResponse{}, err
	}
	return response, nil
}

func (r *Repository) PayMock(ctx context.Context, orderID string, userID string, idempotencyKey string, input PaymentInput, traceID string) (PaymentResponse, error) {
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
	if err := tx.QueryRow(ctx, `SELECT auction_id, winner_id, status, paid_at FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&auctionID, &winnerID, &status, &paidAt); err != nil {
		return PaymentResponse{}, mapOrderNotFound(err)
	}
	if winnerID != userID {
		return PaymentResponse{}, apierrors.New(apierrors.CodeForbiddenRoom, "only winner can pay order", http.StatusForbidden)
	}
	if status == OrderStatusExpired {
		return PaymentResponse{}, apierrors.New(apierrors.CodeOrderAlreadyExpired, "order already expired", http.StatusConflict)
	}
	now := time.Now().UTC()
	if status != OrderStatusPaid {
		if err := tx.QueryRow(ctx, `
			UPDATE orders
			SET status = 'PAID', deposit_status = 'REFUNDED', paid_at = $2
			WHERE id = $1
			RETURNING paid_at
		`, orderID, now).Scan(&paidAt); err != nil {
			return PaymentResponse{}, err
		}
	}
	if paidAt == nil {
		paidAt = &now
	}
	resp := PaymentResponse{OrderID: orderID, OrderStatus: OrderStatusPaid, PaidAt: *paidAt, DepositStatus: DepositStatusRefunded}
	if status != OrderStatusPaid {
		if err := appendAuctionEvent(ctx, tx, auctionID, "order_paid", traceID, map[string]any{
			"order_id":       orderID,
			"user_id":        userID,
			"order_status":   OrderStatusPaid,
			"deposit_status": DepositStatusRefunded,
			"paid_at":        *paidAt,
		}); err != nil {
			return PaymentResponse{}, err
		}
	}
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		return PaymentResponse{}, err
	}
	if err := completeIdempotency(ctx, tx, "payment", orderID, userID, idempotencyKey, requestHash, http.StatusOK, OrderStatusPaid, responseJSON); err != nil {
		return PaymentResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentResponse{}, err
	}
	return resp, nil
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
			a.end_at, a.accepted_bid_count, a.extend_count,
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
		&a.EndAt, &a.AcceptedBidCount, &a.ExtendCount,
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

func (r *Repository) evaluateAndApplyBid(ctx context.Context, tx pgx.Tx, a lockedAuction, userID string, input BidInput, traceID string, skipFatFinger bool) (BidResponse, string, *string, error) {
	bidID := "bid_" + uuid.NewString()
	now := time.Now().UTC()
	serverTimeMS := now.UnixMilli()
	reject := func(code apierrors.Code) (BidResponse, string, *string, error) {
		reason := string(code)
		if err := appendAuctionEvent(ctx, tx, a.ID, "bid_rejected", traceID, map[string]any{
			"bid_id":       bidID,
			"user_id":      userID,
			"amount_cents": input.AmountCents,
			"reason":       reason,
		}); err != nil {
			return BidResponse{}, "", nil, err
		}
		var seq int64
		if err := tx.QueryRow(ctx, `SELECT seq FROM auctions WHERE id = $1`, a.ID).Scan(&seq); err != nil {
			return BidResponse{}, "", nil, err
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
		return resp, "REJECTED", &reason, nil
	}
	if a.Status != StatusActive {
		resp, status, reason, err := reject(apierrors.CodeAuctionNotActive)
		return resp, status, reason, err
	}
	if now.After(a.EndAt) {
		resp, status, reason, err := reject(apierrors.CodeAuctionEnded)
		return resp, status, reason, err
	}
	if a.CurrentWinnerID != nil && *a.CurrentWinnerID == userID {
		resp, status, reason, err := reject(apierrors.CodeRejectedSelfLeading)
		return resp, status, reason, err
	}

	class := ClassifyBidAmount(a.StartPriceCents, a.CurrentPriceCents, a.IncrementCents, a.CapPriceCents, input.AmountCents, a.AcceptedBidCount > 0)
	switch class {
	case BidClassTooLow:
		resp, status, reason, err := reject(apierrors.CodeBidTooLow)
		return resp, status, reason, err
	case BidClassIncrementMismatch:
		resp, status, reason, err := reject(apierrors.CodeBidIncrementMismatch)
		return resp, status, reason, err
	case BidClassAboveCap:
		resp, status, reason, err := reject(apierrors.CodeBidAboveCap)
		return resp, status, reason, err
	}
	if !skipFatFinger && a.FatFingerThreshold != nil && *a.FatFingerThreshold > 0 {
		basis := a.CurrentPriceCents
		if a.AcceptedBidCount == 0 {
			basis = a.StartPriceCents
		}
		if input.AmountCents-basis >= *a.FatFingerThreshold {
			var seq int64
			if err := tx.QueryRow(ctx, `SELECT seq FROM auctions WHERE id = $1`, a.ID).Scan(&seq); err != nil {
				return BidResponse{}, "", nil, err
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
			}, "", nil, nil
		}
	}

	result := BidResultAccepted
	newStatus := a.Status
	newEndAt := a.EndAt
	newExtendCount := a.ExtendCount
	if class == BidClassAcceptedSold {
		result = BidResultAcceptedSold
		newStatus = StatusSold
	} else {
		extended := CalculateExtension(a.EndAt.Unix(), now.Unix(), a.ExtendWindowSeconds, a.ExtendBySeconds, a.ExtendCount, a.MaxExtendCount)
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
	`, a.ID, newStatus, input.AmountCents, userID, newEndAt, newExtendCount)
	if err != nil {
		return BidResponse{}, "", nil, err
	}
	eventType := "bid_accepted"
	if result == BidResultAcceptedSold {
		eventType = "auction_sold"
	}
	eventPayload := map[string]any{
		"bid_id":              bidID,
		"user_id":             userID,
		"amount_cents":        input.AmountCents,
		"result":              result,
		"current_price_cents": input.AmountCents,
	}
	var seq int64
	if result == BidResultAcceptedSold {
		orderID, err := createOrderForSoldAuction(ctx, tx, a, userID, input.AmountCents)
		if err != nil {
			return BidResponse{}, "", nil, err
		}
		eventPayload["order_id"] = orderID
	}
	if err := appendAuctionEvent(ctx, tx, a.ID, eventType, traceID, eventPayload); err != nil {
		return BidResponse{}, "", nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT seq FROM auctions WHERE id = $1`, a.ID).Scan(&seq); err != nil {
		return BidResponse{}, "", nil, err
	}
	resp := BidResponse{
		Result:            result,
		BidID:             bidID,
		AuctionID:         a.ID,
		Seq:               seq,
		CurrentPriceCents: input.AmountCents,
		CurrentWinnerID:   &userID,
		EndAt:             &newEndAt,
		ServerTimeMS:      serverTimeMS,
	}
	return resp, "ACCEPTED", nil, nil
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

func insertBidRow(ctx context.Context, tx pgx.Tx, bidID string, auctionID string, userID string, input BidInput, seq int64, status string, rejectReason *string, requestHash string, responseJSON []byte, traceID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO bids (id, auction_id, user_id, client_bid_id, amount_cents, seq, status, reject_reason, request_hash, response_json, trace_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, bidID, auctionID, userID, input.ClientBidID, input.AmountCents, seq, status, rejectReason, requestHash, responseJSON, traceID)
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
		SELECT id, auction_id, winner_id, amount_cents, status, deposit_cents, deposit_status, expire_at, paid_at, created_at
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
		if err := rows.Scan(&order.ID, &order.AuctionID, &order.WinnerID, &order.AmountCents, &order.Status, &order.DepositCents, &order.DepositStatus, &order.ExpireAt, &order.PaidAt, &order.CreatedAt); err != nil {
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
