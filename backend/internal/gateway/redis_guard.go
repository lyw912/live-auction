package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/redisx"
)

const (
	bidEngineModeRedisGuard  = "redis_guard"
	redisGuardProjectionTTL  = 30 * time.Minute
	redisGuardRefreshRetries = 2

	redisGuardOutcomeAllow       = "ALLOW"
	redisGuardOutcomeReject      = "REJECT"
	redisGuardOutcomeStale       = "STALE"
	redisGuardOutcomeMissing     = "MISSING"
	redisGuardOutcomeUnavailable = "UNAVAILABLE"
	redisGuardOutcomeError       = "ERROR"
)

var bidRedisGuardRunner = redisx.NewScriptRunner(redisx.ScriptBidRedisGuard, `
local projection = redis.call('HMGET', KEYS[1],
  'status',
  'current_price_cents',
  'start_price_cents',
  'increment_cents',
  'cap_price_cents',
  'end_at_ms',
  'seq',
  'accepted_bid_count',
  'current_winner_id',
  'projected_at_ms'
)

if projection[1] == false then
  return {'MISSING'}
end

local now_ms = tonumber(ARGV[1])
local amount = tonumber(ARGV[2])
local user_id = ARGV[3]
local max_staleness_ms = tonumber(ARGV[4])
local projected_at_ms = tonumber(projection[10])

local status = tostring(projection[1])
local current_price = tonumber(projection[2]) or 0
local start_price = tonumber(projection[3]) or 0
local increment = tonumber(projection[4]) or 0
local cap = tonumber(projection[5]) or 0
local end_at_ms = tonumber(projection[6]) or 0
local seq = tonumber(projection[7]) or 0
local accepted_bid_count = tonumber(projection[8]) or 0
local current_winner_id = projection[9]
local stale = projected_at_ms == nil or projected_at_ms <= 0 or (now_ms - projected_at_ms) > max_staleness_ms

if status ~= 'ACTIVE' then
  if status ~= 'SOLD' and status ~= 'ENDED' and status ~= 'CANCELLED' then
    return {'ALLOW'}
  end
  return {'REJECT', 'AUCTION_NOT_ACTIVE', tostring(seq), tostring(current_price), current_winner_id or '', tostring(end_at_ms)}
end
if current_winner_id ~= false and current_winner_id ~= '' and current_winner_id == user_id then
  return {'ALLOW'}
end
if increment <= 0 then
  return {'STALE'}
end
if cap > 0 and amount > cap then
  return {'REJECT', 'BID_ABOVE_CAP', tostring(seq), tostring(current_price), current_winner_id or '', tostring(end_at_ms)}
end

local base = current_price
if accepted_bid_count <= 0 then
  base = start_price
end
local minimum = base + increment
if stale and amount <= current_price then
  return {'REJECT', 'BID_TOO_LOW', tostring(seq), tostring(current_price), current_winner_id or '', tostring(end_at_ms)}
end
if amount < minimum then
  return {'REJECT', 'BID_TOO_LOW', tostring(seq), tostring(current_price), current_winner_id or '', tostring(end_at_ms)}
end
if stale then
  return {'STALE'}
end
if ((amount - base) % increment) ~= 0 then
  return {'REJECT', 'BID_INCREMENT_MISMATCH', tostring(seq), tostring(current_price), current_winner_id or '', tostring(end_at_ms)}
end

return {'ALLOW'}
`)

var bidRedisGuardRefreshRunner = redisx.NewScriptRunner(redisx.ScriptBidRedisGuardRefresh, `
local projection = redis.call('HMGET', KEYS[1], 'seq', 'accepted_bid_count')
if projection[1] == false then
  return {'missing'}
end

local old_seq = tonumber(projection[1]) or 0
local new_seq = tonumber(ARGV[1]) or 0
if new_seq <= old_seq then
  return {'stale'}
end

local old_count = tonumber(projection[2]) or 0
local new_count = old_count + 1
if new_count < 1 then
  new_count = 1
end

redis.call('HSET', KEYS[1],
  'status', ARGV[2],
  'current_price_cents', ARGV[3],
  'current_winner_id', ARGV[4],
  'end_at_ms', ARGV[5],
  'seq', ARGV[1],
  'accepted_bid_count', tostring(new_count),
  'projected_at_ms', ARGV[6]
)
redis.call('EXPIRE', KEYS[1], ARGV[7])
return {'updated'}
`)

type redisGuard struct {
	cfg   config.Config
	db    *pgxpool.Pool
	redis *redis.Client
}

type redisGuardDecision struct {
	Outcome           string
	RejectReason      *apierrors.Code
	Seq               int64
	CurrentPriceCents int64
	CurrentWinnerID   *string
	EndAt             *time.Time
	ProjectionAgeMS   int64
}

func newRedisGuard(cfg config.Config, db *pgxpool.Pool, redisClient *redis.Client) *redisGuard {
	cfg = normalizeRedisGuardConfig(cfg)
	if cfg.BidEngineMode != bidEngineModeRedisGuard || redisClient == nil {
		return nil
	}
	return &redisGuard{cfg: cfg, db: db, redis: redisClient}
}

func normalizeRedisGuardConfig(cfg config.Config) config.Config {
	if cfg.BidRedisGuardMaxStaleness <= 0 {
		cfg.BidRedisGuardMaxStaleness = 1500 * time.Millisecond
	}
	if cfg.BidRedisGuardTimeout <= 0 {
		cfg.BidRedisGuardTimeout = 30 * time.Millisecond
	}
	return cfg
}

func (g *redisGuard) Check(ctx context.Context, auctionID string, userID string, input auction.BidInput) redisGuardDecision {
	if g == nil || g.redis == nil {
		return redisGuardDecision{Outcome: redisGuardOutcomeAllow}
	}
	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, g.cfg.BidRedisGuardTimeout)
	defer cancel()

	now := time.Now().UTC()
	cmd := bidRedisGuardRunner.Run(
		timeoutCtx,
		g.redis,
		[]string{redisx.BidGuardProjectionKey(auctionID)},
		now.UnixMilli(),
		input.AmountCents,
		userID,
		g.cfg.BidRedisGuardMaxStaleness.Milliseconds(),
	)
	values, err := cmd.Slice()
	if err != nil {
		outcome := redisGuardOutcomeError
		if errors.Is(err, redis.Nil) {
			outcome = redisGuardOutcomeMissing
		} else {
			switch redisx.ClassifyScriptError(err) {
			case redisx.OutcomeTimeout, redisx.OutcomeUnavailable, redisx.OutcomeBusy:
				outcome = redisGuardOutcomeUnavailable
			}
		}
		recordRedisGuardDecision(outcome, "", time.Since(start))
		return redisGuardDecision{Outcome: outcome}
	}
	if len(values) == 0 {
		recordRedisGuardDecision(redisGuardOutcomeError, "", time.Since(start))
		return redisGuardDecision{Outcome: redisGuardOutcomeError}
	}
	outcome := stringValue(values[0])
	decision := redisGuardDecision{Outcome: outcome}
	if outcome == redisGuardOutcomeReject && len(values) >= 6 {
		reason := apierrors.Code(stringValue(values[1]))
		decision.RejectReason = &reason
		decision.Seq = int64Value(values[2])
		decision.CurrentPriceCents = int64Value(values[3])
		if winner := stringValue(values[4]); winner != "" {
			decision.CurrentWinnerID = &winner
		}
		if endAtMS := int64Value(values[5]); endAtMS > 0 {
			endAt := time.UnixMilli(endAtMS).UTC()
			decision.EndAt = &endAt
		}
	}
	recordRedisGuardDecision(outcome, rejectReasonLabel(decision.RejectReason), time.Since(start))
	return decision
}

func (g *redisGuard) Response(ctx context.Context, auctionID string, userID string, traceID string, decision redisGuardDecision) (auction.BidResponse, error) {
	if decision.Outcome != redisGuardOutcomeReject || decision.RejectReason == nil {
		return auction.BidResponse{}, nil
	}
	resp := auction.BidResponse{
		Result:            auction.BidResultRejected,
		BidID:             "guard_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		AuctionID:         auctionID,
		Seq:               decision.Seq,
		CurrentPriceCents: decision.CurrentPriceCents,
		CurrentWinnerID:   decision.CurrentWinnerID,
		EndAt:             decision.EndAt,
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
	}
	reason := string(*decision.RejectReason)
	resp.RejectReason = &reason
	if err := g.recordReject(ctx, auctionID, userID, traceID, *decision.RejectReason, decision); err != nil {
		return auction.BidResponse{}, err
	}
	return resp, nil
}

func (g *redisGuard) RefreshAfterAcceptedBid(ctx context.Context, response auction.BidResponse) {
	if g == nil || g.redis == nil || !isAcceptedBidResult(response.Result) || response.Seq <= 0 {
		return
	}
	outcome := g.refreshAcceptedBidProjection(context.WithoutCancel(ctx), response)
	if outcome == "error" {
		go g.retryAcceptedBidProjection(response)
	}
}

func (g *redisGuard) retryAcceptedBidProjection(response auction.BidResponse) {
	for attempt := 0; attempt < redisGuardRefreshRetries; attempt++ {
		time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
		outcome := g.refreshAcceptedBidProjection(context.Background(), response)
		if outcome != "error" {
			return
		}
	}
}

func (g *redisGuard) refreshAcceptedBidProjection(ctx context.Context, response auction.BidResponse) string {
	winnerID := ""
	if response.CurrentWinnerID != nil {
		winnerID = *response.CurrentWinnerID
	}
	endAtMS := int64(0)
	if response.EndAt != nil {
		endAtMS = response.EndAt.UTC().UnixMilli()
	}
	status := "ACTIVE"
	if response.Result == auction.BidResultAcceptedSold {
		status = "SOLD"
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, g.cfg.BidRedisGuardTimeout)
	defer cancel()
	values, err := bidRedisGuardRefreshRunner.Run(
		timeoutCtx,
		g.redis,
		[]string{redisx.BidGuardProjectionKey(response.AuctionID)},
		response.Seq,
		status,
		response.CurrentPriceCents,
		winnerID,
		endAtMS,
		time.Now().UTC().UnixMilli(),
		int64(redisGuardProjectionTTL/time.Second),
	).Slice()
	if err != nil || len(values) == 0 {
		observability.Inc("auction_bid_redis_guard_projection_update_total", map[string]string{"outcome": "error"})
		return "error"
	}
	outcome := stringValue(values[0])
	observability.Inc("auction_bid_redis_guard_projection_update_total", map[string]string{"outcome": outcome})
	return outcome
}

func isAcceptedBidResult(result string) bool {
	switch result {
	case auction.BidResultAccepted, auction.BidResultAcceptedExtended, auction.BidResultAcceptedSold:
		return true
	default:
		return false
	}
}

func (g *redisGuard) recordReject(ctx context.Context, auctionID string, userID string, traceID string, code apierrors.Code, decision redisGuardDecision) error {
	if g == nil || g.db == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"auction_id":          auctionID,
		"user_id":             userID,
		"trace_id":            traceID,
		"engine_mode":         g.cfg.BidEngineMode,
		"code":                code,
		"reason":              "redis guard conservative reject",
		"seq":                 decision.Seq,
		"current_price_cents": decision.CurrentPriceCents,
	})
	if err != nil {
		return err
	}
	_, err = g.db.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('LOW', $1, $2, 'redis guard rejected bid before PostgreSQL lane', $3)
	`, string(code), auctionID, payload)
	return err
}

func recordRedisGuardDecision(outcome string, reason string, elapsed time.Duration) {
	if outcome == "" {
		outcome = redisGuardOutcomeError
	}
	observability.Inc("auction_bid_redis_guard_total", map[string]string{"outcome": outcome, "reason": reason})
	observability.Observe("auction_bid_redis_guard_seconds", elapsed.Seconds(), map[string]string{"outcome": outcome}, observability.DefaultLatencyBuckets)
}

func rejectReasonLabel(reason *apierrors.Code) string {
	if reason == nil {
		return ""
	}
	return string(*reason)
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		parsed, _ := strconv.ParseInt(v, 10, 64)
		return parsed
	case []byte:
		parsed, _ := strconv.ParseInt(string(v), 10, 64)
		return parsed
	default:
		return 0
	}
}
