package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
)

const (
	rateLimitRedisDownAnomaly = "RATE_LIMIT_REDIS_DOWN"
	bidLimitRetryAfterSeconds = 1
)

type bidAdmission struct {
	cfg        config.Config
	db         *pgxpool.Pool
	redis      redis.Cmdable
	semaphores sync.Map
}

type bidAdmissionPermit struct {
	release func()
}

func newBidAdmission(cfg config.Config, db *pgxpool.Pool, redisClient redis.Cmdable) *bidAdmission {
	return &bidAdmission{cfg: cfg, db: db, redis: redisClient}
}

func (a *bidAdmission) admit(ctx context.Context, r *http.Request, user AuthUser, auctionID string, idempotencyKey string, input auction.BidInput, traceID string) (auction.BidResponse, *bidAdmissionPermit, bool, error) {
	if input.ClientBidID == "" || input.AmountCents <= 0 {
		return auction.BidResponse{}, nil, false, apierrors.New(apierrors.CodeInvalidArgument, "client_bid_id and positive amount_cents are required", http.StatusBadRequest)
	}
	if idempotencyKey == "" || idempotencyKey != input.ClientBidID {
		return auction.BidResponse{}, nil, false, apierrors.New(apierrors.CodeInvalidArgument, "Idempotency-Key must equal client_bid_id", http.StatusBadRequest)
	}
	requestHash := bidAdmissionRequestHash(auctionID, user.ID, input.ClientBidID, input.AmountCents)
	if replay, ok, err := a.completedBidReplay(ctx, auctionID, user.ID, idempotencyKey, requestHash); err != nil || ok {
		return replay, nil, ok, err
	}
	if err := a.checkRedisLimits(ctx, r, user, auctionID, traceID); err != nil {
		return auction.BidResponse{}, nil, false, err
	}
	permit, err := a.acquireLocalPermit(ctx, auctionID, user.ID, traceID)
	if err != nil {
		return auction.BidResponse{}, nil, false, err
	}
	return auction.BidResponse{}, permit, false, nil
}

func (a *bidAdmission) admitConfirm(ctx context.Context, r *http.Request, user AuthUser, auctionID string, idempotencyKey string, input auction.ConfirmBidInput, traceID string) (auction.BidResponse, *bidAdmissionPermit, bool, error) {
	if input.ConfirmToken == "" || input.IdempotencyKey == "" || idempotencyKey == "" || idempotencyKey != input.IdempotencyKey {
		return auction.BidResponse{}, nil, false, apierrors.New(apierrors.CodeInvalidArgument, "confirm_token and matching Idempotency-Key are required", http.StatusBadRequest)
	}
	if replay, ok, err := a.completedConfirmReplay(ctx, auctionID, user.ID, idempotencyKey); err != nil || ok {
		return replay, nil, ok, err
	}
	if err := a.checkRedisLimits(ctx, r, user, auctionID, traceID); err != nil {
		return auction.BidResponse{}, nil, false, err
	}
	permit, err := a.acquireLocalPermit(ctx, auctionID, user.ID, traceID)
	if err != nil {
		return auction.BidResponse{}, nil, false, err
	}
	return auction.BidResponse{}, permit, false, nil
}

func (a *bidAdmission) completedBidReplay(ctx context.Context, auctionID string, userID string, idempotencyKey string, requestHash string) (auction.BidResponse, bool, error) {
	if a.db == nil {
		return auction.BidResponse{}, false, nil
	}
	var storedHash string
	var status string
	var responseJSON []byte
	var lockedUntil *time.Time
	err := a.db.QueryRow(ctx, `
		SELECT request_hash, status, response_json, locked_until
		FROM idempotency_records
		WHERE scope_type = 'bid' AND scope_id = $1 AND user_id = $2 AND idempotency_key = $3
	`, auctionID, userID, idempotencyKey).Scan(&storedHash, &status, &responseJSON, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return auction.BidResponse{}, false, nil
	}
	if err != nil {
		return auction.BidResponse{}, false, err
	}
	if storedHash != requestHash {
		return auction.BidResponse{}, false, apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "idempotency key reused with different request", http.StatusConflict)
	}
	if status == auction.IdempotencyStatusProcessing && lockedUntil != nil && time.Now().UTC().After(*lockedUntil) {
		if err := a.markBidIdempotencyTimeout(ctx, auctionID, userID, idempotencyKey); err != nil {
			return auction.BidResponse{}, false, err
		}
		return auction.BidResponse{}, false, apierrors.New(apierrors.CodeIdempotencyTimeout, "previous idempotent operation timed out", http.StatusConflict)
	}
	if status != auction.IdempotencyStatusCompleted {
		return auction.BidResponse{}, false, apierrors.New(apierrors.CodeProcessingRetryLater, "same idempotency key is still processing", http.StatusConflict)
	}
	var resp auction.BidResponse
	if err := json.Unmarshal(responseJSON, &resp); err != nil {
		return auction.BidResponse{}, false, err
	}
	return resp, true, nil
}

func (a *bidAdmission) completedConfirmReplay(ctx context.Context, auctionID string, userID string, idempotencyKey string) (auction.BidResponse, bool, error) {
	if a.db == nil {
		return auction.BidResponse{}, false, nil
	}
	var status string
	var resultCode *string
	var responseJSON []byte
	err := a.db.QueryRow(ctx, `
		SELECT status, result_code, response_json
		FROM idempotency_records
		WHERE scope_type = 'bid' AND scope_id = $1 AND user_id = $2 AND idempotency_key = $3
	`, auctionID, userID, idempotencyKey).Scan(&status, &resultCode, &responseJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return auction.BidResponse{}, false, nil
	}
	if err != nil {
		return auction.BidResponse{}, false, err
	}
	if status == auction.IdempotencyStatusCompleted && resultCode != nil && *resultCode != string(apierrors.CodeFatFingerConfirmRequired) {
		var resp auction.BidResponse
		if err := json.Unmarshal(responseJSON, &resp); err != nil {
			return auction.BidResponse{}, false, err
		}
		return resp, true, nil
	}
	return auction.BidResponse{}, false, nil
}

func (a *bidAdmission) markBidIdempotencyTimeout(ctx context.Context, auctionID string, userID string, idempotencyKey string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE idempotency_records
		SET status = 'FAILED', locked_until = NULL, completed_at = now(), result_code = $4
		WHERE scope_type = 'bid' AND scope_id = $1 AND user_id = $2 AND idempotency_key = $3
		  AND status = 'PROCESSING'
	`, auctionID, userID, idempotencyKey, apierrors.CodeIdempotencyTimeout)
	return err
}

func (a *bidAdmission) checkRedisLimits(ctx context.Context, r *http.Request, user AuthUser, auctionID string, traceID string) error {
	if !a.cfg.AdmissionEnabled {
		return nil
	}
	if a.redis == nil {
		return nil
	}
	window := a.cfg.BidLimitWindow
	if window <= 0 {
		window = time.Second
	}
	redisCtx := ctx
	cancel := func() {}
	if a.cfg.BidLimitRedisTimeout > 0 {
		redisCtx, cancel = context.WithTimeout(ctx, a.cfg.BidLimitRedisTimeout)
	}
	defer cancel()

	checks := []struct {
		key   string
		limit int
		code  apierrors.Code
	}{
		{key: fmt.Sprintf("bid:limit:user:%s:%s", auctionID, user.ID), limit: a.cfg.BidUserLimitPerSecond, code: apierrors.CodeRateLimited},
		{key: fmt.Sprintf("bid:limit:ip:%s:%s", auctionID, clientIP(r)), limit: a.cfg.BidIPLimitPerSecond, code: apierrors.CodeRateLimited},
		{key: fmt.Sprintf("bid:limit:auction:%s", auctionID), limit: a.cfg.BidAuctionLimitPerSecond, code: apierrors.CodeBidAuctionTooHot},
	}
	nowMS := time.Now().UnixMilli()
	windowMS := window.Milliseconds()
	for _, check := range checks {
		if check.limit <= 0 {
			continue
		}
		allowed, err := evalGCRALimit(redisCtx, a.redis, check.key, check.limit, windowMS, nowMS)
		if err != nil {
			_ = a.recordRedisDown(ctx, auctionID, user.ID, traceID, err)
			return nil
		}
		if !allowed {
			if check.code == apierrors.CodeBidAuctionTooHot {
				recordAdmissionMetric(apierrors.CodeBidAuctionTooHot)
				_ = a.recordAdmissionReject(ctx, auctionID, user.ID, traceID, apierrors.CodeBidAuctionTooHot, "redis auction global limit")
				return apierrors.New(apierrors.CodeBidAuctionTooHot, "auction is temporarily too hot", http.StatusTooManyRequests)
			}
			recordAdmissionMetric(apierrors.CodeRateLimited)
			_ = a.recordAdmissionReject(ctx, auctionID, user.ID, traceID, apierrors.CodeRateLimited, "redis user or ip limit")
			return apierrors.New(apierrors.CodeRateLimited, "too many bid attempts", http.StatusTooManyRequests)
		}
	}
	return nil
}

func evalGCRALimit(ctx context.Context, redisClient redis.Cmdable, key string, limit int, windowMS int64, nowMS int64) (bool, error) {
	emissionMS := windowMS / int64(limit)
	if emissionMS <= 0 {
		emissionMS = 1
	}
	ttlMS := windowMS + emissionMS*int64(limit)
	result, err := redisClient.Eval(ctx, `
local now = tonumber(ARGV[1])
local emission = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local tat = tonumber(redis.call("GET", KEYS[1]) or "0")
local allow_at = tat - ((burst - 1) * emission)
if now < allow_at then
  return 0
end
local new_tat = math.max(tat, now) + emission
redis.call("SET", KEYS[1], new_tat, "PX", ttl)
return 1
`, []string{key}, nowMS, emissionMS, limit, ttlMS).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (a *bidAdmission) acquireLocalPermit(ctx context.Context, auctionID string, userID string, traceID string) (*bidAdmissionPermit, error) {
	if !a.cfg.AdmissionEnabled {
		return &bidAdmissionPermit{}, nil
	}
	limit := a.cfg.BidAuctionMaxInFlight
	if limit <= 0 {
		limit = 64
	}
	raw, _ := a.semaphores.LoadOrStore(auctionID, make(chan struct{}, limit))
	sem := raw.(chan struct{})
	select {
	case sem <- struct{}{}:
		return &bidAdmissionPermit{release: func() { <-sem }}, nil
	default:
		recordAdmissionMetric(apierrors.CodeBidAuctionTooHot)
		_ = a.recordAdmissionReject(ctx, auctionID, userID, traceID, apierrors.CodeBidAuctionTooHot, "local auction semaphore full")
		return nil, apierrors.New(apierrors.CodeBidAuctionTooHot, "auction local admission queue is full", http.StatusTooManyRequests)
	}
}

func (p *bidAdmissionPermit) Release() {
	if p != nil && p.release != nil {
		p.release()
	}
}

func (a *bidAdmission) recordRedisDown(ctx context.Context, auctionID string, userID string, traceID string, cause error) error {
	if a.db == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"auction_id": auctionID,
		"user_id":    userID,
		"trace_id":   traceID,
		"error":      cause.Error(),
	})
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('MED', $1, $2, 'bid rate-limit Redis unavailable; fail open', $3)
	`, rateLimitRedisDownAnomaly, auctionID, payload)
	return err
}

func (a *bidAdmission) recordAdmissionReject(ctx context.Context, auctionID string, userID string, traceID string, code apierrors.Code, reason string) error {
	if a.db == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"auction_id": auctionID,
		"user_id":    userID,
		"trace_id":   traceID,
		"code":       code,
		"reason":     reason,
	})
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('MED', $1, $2, 'bid admission rejected request', $3)
	`, string(code), auctionID, payload)
	return err
}

func recordAdmissionMetric(code apierrors.Code) {
	observability.Inc("auction_bid_request_total", map[string]string{"result": "admission_rejected", "reason": string(code)})
}

func bidAdmissionRequestHash(auctionID string, userID string, clientBidID string, amountCents int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("bid:v1|%s|%s|%s|%d", auctionID, userID, clientBidID, amountCents)))
	return hex.EncodeToString(sum[:])
}
