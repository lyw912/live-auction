package redisengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/redisx"
)

const (
	engineStateTTL    = 30 * time.Minute
	idempotencyTTL    = 24 * time.Hour
	maxSettleAttempts = 3
	resultAccepted    = "ENGINE_ACCEPTED"
	resultRejected    = "ENGINE_REJECTED"
	resultSold        = "ENGINE_SOLD"
	resultReconciling = "RECONCILING"
)

var ledgerRunner = redisx.NewScriptRunner(redisx.ScriptBidRedisLedger, `
local state_key = KEYS[1]
local idem_key = KEYS[2]
local pending_key = KEYS[3]

local now_ms = tonumber(ARGV[1])
local auction_id = ARGV[2]
local user_id = ARGV[3]
local client_bid_id = ARGV[4]
local amount = tonumber(ARGV[5])
local request_hash = ARGV[6]
local trace_id = ARGV[7]
local bid_id = ARGV[8]
local state_json = ARGV[9]
local state_ttl_ms = tonumber(ARGV[10])
local idem_ttl_ms = tonumber(ARGV[11])

local existing = redis.call('HMGET', idem_key, 'request_hash', 'result_json')
if existing[1] ~= false then
  if existing[1] ~= request_hash then
    return {'ERROR', 'IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST'}
  end
  return {'REPLAY', existing[2]}
end

if redis.call('EXISTS', state_key) == 0 then
  local decoded = cjson.decode(state_json)
  local decoded_winner = decoded['current_winner_id']
  if decoded_winner == nil then decoded_winner = '' end
  redis.call('HSET', state_key, 'status', tostring(decoded['status'] or ''))
  redis.call('HSET', state_key, 'current_price_cents', tostring(decoded['current_price_cents'] or 0))
  redis.call('HSET', state_key, 'current_winner_id', tostring(decoded_winner))
  redis.call('HSET', state_key, 'start_price_cents', tostring(decoded['start_price_cents'] or 0))
  redis.call('HSET', state_key, 'increment_cents', tostring(decoded['increment_cents'] or 0))
  redis.call('HSET', state_key, 'cap_price_cents', tostring(decoded['cap_price_cents'] or 0))
  redis.call('HSET', state_key, 'end_at_ms', tostring(decoded['end_at_ms'] or 0))
  redis.call('HSET', state_key, 'extend_window_ms', tostring(decoded['extend_window_ms'] or 0))
  redis.call('HSET', state_key, 'extend_by_ms', tostring(decoded['extend_by_ms'] or 0))
  redis.call('HSET', state_key, 'max_extend_count', tostring(decoded['max_extend_count'] or 0))
  redis.call('HSET', state_key, 'extend_count', tostring(decoded['extend_count'] or 0))
  redis.call('HSET', state_key, 'accepted_bid_count', tostring(decoded['accepted_bid_count'] or 0))
  redis.call('HSET', state_key, 'engine_seq', tostring(decoded['engine_seq'] or 0))
  redis.call('HSET', state_key, 'engine_epoch', tostring(decoded['engine_epoch'] or 1))
  redis.call('HSET', state_key, 'paused', tostring(decoded['paused'] or false))
  redis.call('HSET', state_key, 'pause_reason', tostring(decoded['pause_reason'] or ''))
  redis.call('PEXPIRE', state_key, state_ttl_ms)
end

local s = redis.call('HMGET', state_key,
  'status', 'current_price_cents', 'current_winner_id', 'start_price_cents',
  'increment_cents', 'cap_price_cents', 'end_at_ms', 'extend_window_ms',
  'extend_by_ms', 'max_extend_count', 'extend_count', 'accepted_bid_count',
  'engine_seq', 'engine_epoch', 'paused', 'pause_reason')

local status = tostring(s[1])
local current_price = tonumber(s[2]) or 0
local current_winner = s[3]
  if current_winner == false then current_winner = '' end
if current_winner == nil then current_winner = '' end
local start_price = tonumber(s[4]) or 0
local increment = tonumber(s[5]) or 0
local cap = tonumber(s[6]) or 0
local end_at_ms = tonumber(s[7]) or 0
local extend_window_ms = tonumber(s[8]) or 0
local extend_by_ms = tonumber(s[9]) or 0
local max_extend_count = tonumber(s[10]) or 0
local extend_count = tonumber(s[11]) or 0
local accepted_bid_count = tonumber(s[12]) or 0
local engine_seq = tonumber(s[13]) or 0
local engine_epoch = tonumber(s[14]) or 1
local paused = tostring(s[15])
local pause_reason = tostring(s[16] or '')

local function store_result(result)
  local encoded = cjson.encode(result)
  redis.call('HSET', idem_key, 'request_hash', request_hash)
  redis.call('HSET', idem_key, 'result_json', encoded)
  redis.call('HSET', idem_key, 'engine_seq', tostring(result['engine_seq'] or 0))
  redis.call('PEXPIRE', idem_key, idem_ttl_ms)
  return encoded
end

local function store_decision(result)
  local encoded = store_result(result)
  redis.call('HSET', pending_key, tostring(result['engine_seq']), encoded)
  redis.call('PEXPIRE', pending_key, state_ttl_ms)
  return encoded
end

local function reject(reason)
  engine_seq = engine_seq + 1
  redis.call('HSET', state_key, 'engine_seq', engine_seq)
  redis.call('PEXPIRE', state_key, state_ttl_ms)
  local result = {
    result = 'ENGINE_REJECTED',
    bid_id = bid_id,
    auction_id = auction_id,
    user_id = user_id,
    client_bid_id = client_bid_id,
    amount_cents = amount,
    engine_seq = engine_seq,
    engine_epoch = engine_epoch,
    settlement_status = 'PENDING',
    reject_reason = reason,
    current_price_cents = current_price,
    current_winner_id = current_winner,
    end_at_ms = end_at_ms,
    server_time_ms = now_ms,
    trace_id = trace_id,
    request_hash = request_hash
  }
  return {'OK', store_decision(result)}
end

if paused == '1' then
  return {'ERROR', 'ENGINE_PAUSED', pause_reason}
end
if status == 'RECONCILING' then
  return {'ERROR', 'RECONCILING', pause_reason}
end
if status ~= 'ACTIVE' then
  return reject('AUCTION_NOT_ACTIVE')
end
if now_ms > end_at_ms then
  return reject('AUCTION_ENDED')
end
if current_winner ~= '' and current_winner == user_id then
  return reject('REJECTED_SELF_LEADING')
end
if increment <= 0 then
  return {'ERROR', 'INVALID_AUCTION_RULE'}
end
if cap > 0 and amount > cap then
  return reject('BID_ABOVE_CAP')
end

local base = current_price
if accepted_bid_count <= 0 then
  base = start_price
end
if amount < (base + increment) then
  return reject('BID_TOO_LOW')
end
if ((amount - base) % increment) ~= 0 then
  return reject('BID_INCREMENT_MISMATCH')
end

engine_seq = engine_seq + 1
local result_code = 'ENGINE_ACCEPTED'
local new_status = 'ACTIVE'
local new_end_at_ms = end_at_ms
local new_extend_count = extend_count
if cap > 0 and amount == cap then
  result_code = 'ENGINE_SOLD'
  new_status = 'SOLD'
else
  if extend_count < max_extend_count and (end_at_ms - now_ms) <= extend_window_ms then
    local candidate = now_ms + extend_by_ms
    if candidate > end_at_ms then
      new_end_at_ms = candidate
      new_extend_count = extend_count + 1
    end
  end
end

redis.call('HSET', state_key, 'status', new_status)
redis.call('HSET', state_key, 'current_price_cents', tostring(amount))
redis.call('HSET', state_key, 'current_winner_id', user_id)
redis.call('HSET', state_key, 'end_at_ms', tostring(new_end_at_ms))
redis.call('HSET', state_key, 'extend_count', tostring(new_extend_count))
redis.call('HSET', state_key, 'accepted_bid_count', tostring(accepted_bid_count + 1))
redis.call('HSET', state_key, 'engine_seq', tostring(engine_seq))
redis.call('HSET', state_key, 'engine_epoch', tostring(engine_epoch))
redis.call('HSET', state_key, 'paused', '0')
redis.call('HSET', state_key, 'pause_reason', '')
redis.call('PEXPIRE', state_key, state_ttl_ms)

local result = {
  result = result_code,
  bid_id = bid_id,
  auction_id = auction_id,
  user_id = user_id,
  client_bid_id = client_bid_id,
  amount_cents = amount,
  engine_seq = engine_seq,
  engine_epoch = engine_epoch,
  settlement_status = 'PENDING',
  current_price_cents = amount,
  current_winner_id = user_id,
  end_at_ms = new_end_at_ms,
  server_time_ms = now_ms,
  trace_id = trace_id,
  request_hash = request_hash
}
return {'OK', store_decision(result)}
`)

type Engine struct {
	db     *pgxpool.Pool
	redis  *redis.Client
	ledger BidLedger
}

type snapshot struct {
	Status            string `json:"status"`
	CurrentPriceCents int64  `json:"current_price_cents"`
	CurrentWinnerID   string `json:"current_winner_id,omitempty"`
	StartPriceCents   int64  `json:"start_price_cents"`
	IncrementCents    int64  `json:"increment_cents"`
	CapPriceCents     int64  `json:"cap_price_cents,omitempty"`
	EndAtMS           int64  `json:"end_at_ms"`
	ExtendWindowMS    int64  `json:"extend_window_ms"`
	ExtendByMS        int64  `json:"extend_by_ms"`
	MaxExtendCount    int    `json:"max_extend_count"`
	ExtendCount       int    `json:"extend_count"`
	AcceptedBidCount  int64  `json:"accepted_bid_count"`
	EngineSeq         int64  `json:"engine_seq"`
	EngineEpoch       int64  `json:"engine_epoch"`
	Paused            bool   `json:"paused"`
	PauseReason       string `json:"pause_reason,omitempty"`
	RequiresPostgres  string `json:"requires_postgres,omitempty"`
}

type engineResult struct {
	Result            string  `json:"result"`
	BidID             string  `json:"bid_id"`
	AuctionID         string  `json:"auction_id"`
	UserID            string  `json:"user_id"`
	ClientBidID       string  `json:"client_bid_id"`
	AmountCents       int64   `json:"amount_cents"`
	EngineSeq         int64   `json:"engine_seq"`
	EngineEpoch       int64   `json:"engine_epoch"`
	SettlementStatus  string  `json:"settlement_status"`
	RejectReason      *string `json:"reject_reason,omitempty"`
	CurrentPriceCents int64   `json:"current_price_cents"`
	CurrentWinnerID   string  `json:"current_winner_id,omitempty"`
	EndAtMS           int64   `json:"end_at_ms"`
	ServerTimeMS      int64   `json:"server_time_ms"`
	TraceID           string  `json:"trace_id"`
	RequestHash       string  `json:"request_hash"`
}

func New(db *pgxpool.Pool, redisClient *redis.Client, ledger BidLedger) *Engine {
	return &Engine{db: db, redis: redisClient, ledger: ledger}
}

func (e *Engine) PlaceBid(ctx context.Context, auctionID string, userID string, idempotencyKey string, input auction.BidInput, traceID string) (auction.BidResponse, error) {
	if e == nil || e.db == nil || e.redis == nil || e.ledger == nil {
		return auction.BidResponse{}, apierrors.New(apierrors.CodeEnginePaused, "redis/kafka ledger engine is unavailable", http.StatusServiceUnavailable)
	}
	if input.ClientBidID == "" || input.AmountCents <= 0 {
		return auction.BidResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "client_bid_id and positive amount_cents are required", http.StatusBadRequest)
	}
	if idempotencyKey == "" || idempotencyKey != input.ClientBidID {
		return auction.BidResponse{}, apierrors.New(apierrors.CodeInvalidArgument, "Idempotency-Key must equal client_bid_id", http.StatusBadRequest)
	}
	requestHash := requestHash(auctionID, userID, input.ClientBidID, input.AmountCents)
	if replay, ok, err := e.completedReplay(ctx, auctionID, userID, idempotencyKey, requestHash); err != nil || ok {
		return replay, err
	}

	snap, err := e.loadSnapshot(ctx, auctionID)
	if err != nil {
		return auction.BidResponse{}, err
	}
	nowMS := time.Now().UTC().UnixMilli()
	bidID := "bid_" + uuid.NewString()
	stateJSON, err := json.Marshal(snap)
	if err != nil {
		return auction.BidResponse{}, err
	}
	start := time.Now()
	cmd := ledgerRunner.Run(ctx, e.redis, []string{
		redisx.BidEngineStateKey(auctionID),
		redisx.BidEngineIdempotencyKey(auctionID, input.ClientBidID),
		redisx.BidEnginePendingKey(auctionID),
	}, nowMS, auctionID, userID, input.ClientBidID, input.AmountCents, requestHash, traceID, bidID, string(stateJSON), engineStateTTL.Milliseconds(), idempotencyTTL.Milliseconds())
	values, err := cmd.Slice()
	if err != nil {
		ledgerRunner.Record(redisx.ClassifyScriptError(err), time.Since(start))
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_SCRIPT_ERROR", err.Error(), traceID)
		return auction.BidResponse{}, apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine paused after script failure: "+err.Error(), http.StatusServiceUnavailable)
	}
	if len(values) < 2 {
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_BAD_SCRIPT_RESULT", fmt.Sprintf("%v", values), traceID)
		return auction.BidResponse{}, apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine returned invalid result", http.StatusServiceUnavailable)
	}
	status := stringValue(values[0])
	if status == "ERROR" {
		code := apierrors.Code(stringValue(values[1]))
		if code == apierrors.CodeEngineReconciling {
			return auction.BidResponse{}, apierrors.New(apierrors.CodeEngineReconciling, "auction is reconciling", http.StatusConflict)
		}
		if code == apierrors.CodeEnginePaused {
			return auction.BidResponse{}, apierrors.New(apierrors.CodeEnginePaused, "auction engine is paused", http.StatusConflict)
		}
		return auction.BidResponse{}, apierrors.New(code, "redis ledger engine rejected request", http.StatusConflict)
	}
	var result engineResult
	if err := json.Unmarshal([]byte(stringValue(values[1])), &result); err != nil {
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_RESULT_DECODE_FAILED", err.Error(), traceID)
		return auction.BidResponse{}, err
	}
	if _, err := e.ledger.Append(ctx, result); err != nil {
		_ = e.pause(ctx, auctionID, "KAFKA_LEDGER_APPEND_FAILED", err.Error(), traceID)
		return auction.BidResponse{}, apierrors.New(apierrors.CodeEnginePaused, "kafka bid ledger append failed; auction engine paused", http.StatusServiceUnavailable)
	}
	_ = e.redis.HDel(ctx, redisx.BidEnginePendingKey(auctionID), strconv.FormatInt(result.EngineSeq, 10)).Err()
	ledgerRunner.Record("ok", time.Since(start))
	recordDecision(result.Result, time.Since(start))
	return result.response(), nil
}

func (e *Engine) completedReplay(ctx context.Context, auctionID string, userID string, idempotencyKey string, requestHash string) (auction.BidResponse, bool, error) {
	var storedHash string
	var status string
	var responseJSON []byte
	err := e.db.QueryRow(ctx, `
		SELECT request_hash, status, response_json
		FROM idempotency_records
		WHERE scope_type = 'bid' AND scope_id = $1 AND user_id = $2 AND idempotency_key = $3
	`, auctionID, userID, idempotencyKey).Scan(&storedHash, &status, &responseJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return auction.BidResponse{}, false, nil
	}
	if err != nil {
		return auction.BidResponse{}, false, err
	}
	if storedHash != requestHash {
		return auction.BidResponse{}, false, apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "idempotency key reused with different request", http.StatusConflict)
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

func (e *Engine) loadSnapshot(ctx context.Context, auctionID string) (snapshot, error) {
	var s snapshot
	var winner *string
	var capPrice *int64
	var endAt time.Time
	err := e.db.QueryRow(ctx, `
		SELECT a.status, a.current_price_cents, a.current_winner_id,
		       a.start_price_cents, a.increment_cents, a.cap_price_cents,
		       a.end_at, ar.extend_window_seconds, ar.extend_by_seconds,
		       ar.max_extend_count, a.extend_count, a.accepted_bid_count,
		       a.engine_seq, a.engine_epoch, a.engine_paused,
		       COALESCE(a.engine_pause_reason, ''),
		       CASE
		         WHEN ar.fat_finger_threshold_cents IS NOT NULL THEN 'fat_finger_confirm'
		         WHEN EXISTS (
		           SELECT 1
		           FROM max_bid_intents mbi
		           WHERE mbi.auction_id = a.id AND mbi.status = 'ACTIVE'
		         ) THEN 'active_max_bid_intent'
		         ELSE ''
		       END
		FROM auctions a
		JOIN auction_rules ar ON ar.auction_id = a.id AND ar.rule_version = a.rule_version
		WHERE a.id = $1
	`, auctionID).Scan(&s.Status, &s.CurrentPriceCents, &winner, &s.StartPriceCents, &s.IncrementCents, &capPrice, &endAt, &s.ExtendWindowMS, &s.ExtendByMS, &s.MaxExtendCount, &s.ExtendCount, &s.AcceptedBidCount, &s.EngineSeq, &s.EngineEpoch, &s.Paused, &s.PauseReason, &s.RequiresPostgres)
	if errors.Is(err, pgx.ErrNoRows) {
		return snapshot{}, apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound)
	}
	if err != nil {
		return snapshot{}, err
	}
	if winner != nil {
		s.CurrentWinnerID = *winner
	}
	if capPrice != nil {
		s.CapPriceCents = *capPrice
	}
	s.EndAtMS = endAt.UTC().UnixMilli()
	s.ExtendWindowMS *= int64(time.Second / time.Millisecond)
	s.ExtendByMS *= int64(time.Second / time.Millisecond)
	if s.Paused {
		return snapshot{}, apierrors.New(apierrors.CodeEnginePaused, "auction engine is paused", http.StatusConflict)
	}
	if s.Status == resultReconciling {
		return snapshot{}, apierrors.New(apierrors.CodeEngineReconciling, "auction is reconciling", http.StatusConflict)
	}
	if s.RequiresPostgres != "" {
		reason := "REDIS_ENGINE_UNSUPPORTED_RULE_" + strings.ToUpper(s.RequiresPostgres)
		_ = e.pause(ctx, auctionID, reason, "redis ledger engine cannot safely process auction requiring "+s.RequiresPostgres, "")
		return snapshot{}, apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine paused: "+s.RequiresPostgres+" requires PostgreSQL rule path", http.StatusConflict)
	}
	return s, nil
}

func (e *Engine) pause(ctx context.Context, auctionID string, reason string, message string, traceID string) error {
	payload, _ := json.Marshal(map[string]any{"reason": reason, "message": message, "trace_id": traceID})
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = true, engine_pause_reason = $2, engine_paused_at = now(), updated_at = now()
		WHERE id = $1
	`, auctionID, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('HIGH', $2, $1, $3, $4)
	`, auctionID, reason, message, payload); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if e.redis != nil {
		_ = e.redis.HSet(ctx, redisx.BidEngineStateKey(auctionID), "paused", 1, "pause_reason", reason).Err()
	}
	return nil
}

func (r engineResult) response() auction.BidResponse {
	result := r.Result
	if result == resultAccepted {
		result = auction.BidResultEngineAccepted
	}
	if result == resultSold {
		result = auction.BidResultEngineSold
	}
	if result == resultRejected {
		result = auction.BidResultEngineRejected
	}
	var winner *string
	if r.CurrentWinnerID != "" {
		winner = &r.CurrentWinnerID
	}
	var endAt *time.Time
	if r.EndAtMS > 0 {
		t := time.UnixMilli(r.EndAtMS).UTC()
		endAt = &t
	}
	return auction.BidResponse{
		Result:            result,
		BidID:             r.BidID,
		AuctionID:         r.AuctionID,
		Seq:               r.EngineSeq,
		EngineSeq:         r.EngineSeq,
		EngineEpoch:       r.EngineEpoch,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: r.CurrentPriceCents,
		CurrentWinnerID:   winner,
		EndAt:             endAt,
		ServerTimeMS:      r.ServerTimeMS,
		RejectReason:      r.RejectReason,
		AmountCents:       r.AmountCents,
	}
}

func requestHash(auctionID string, userID string, clientBidID string, amountCents int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("bid:v1|%s|%s|%s|%d", auctionID, userID, clientBidID, amountCents)))
	return hex.EncodeToString(sum[:])
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func recordDecision(result string, elapsed time.Duration) {
	observability.Inc("auction_bid_redis_ledger_total", map[string]string{"result": result})
	observability.Observe("auction_bid_redis_ledger_seconds", elapsed.Seconds(), map[string]string{"result": result}, observability.DefaultLatencyBuckets)
}

type Worker struct {
	db         *pgxpool.Pool
	redis      *redis.Client
	ledger     BidLedger
	dlqTopic   string
	consumerID string
	batchSize  int64
	block      time.Duration
}

type Report struct {
	CheckedAt          time.Time `json:"checked_at"`
	AuctionID          string    `json:"auction_id"`
	Status             string    `json:"status"`
	RedisSeq           int64     `json:"redis_engine_seq"`
	DBSeq              int64     `json:"db_engine_seq"`
	PendingSettlements int64     `json:"pending_settlements"`
	FailedSettlements  int64     `json:"failed_settlements"`
	DLQSettlements     int64     `json:"dlq_settlements"`
	Paused             bool      `json:"paused"`
	DriftCount         int       `json:"drift_count"`
	Message            string    `json:"message,omitempty"`
}

func NewWorker(db *pgxpool.Pool, redisClient *redis.Client, ledger BidLedger, consumerID string) *Worker {
	if consumerID == "" {
		consumerID = "settlement-" + uuid.NewString()
	}
	return &Worker{db: db, redis: redisClient, ledger: ledger, dlqTopic: ledgerDLQTopic(ledger), consumerID: consumerID, batchSize: 32, block: time.Second}
}

func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	reconcileTicker := time.NewTicker(5 * time.Second)
	defer reconcileTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = w.ProcessSignals(ctx, 16)
		if w.ledger != nil {
			_, _ = w.ProcessKafka(ctx, 100)
		}
		select {
		case <-ctx.Done():
			return
		case <-reconcileTicker.C:
			_, _ = w.ProcessReconcile(ctx, 100)
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessKafka(ctx context.Context, limit int) (int, error) {
	if w == nil || w.ledger == nil || w.db == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 1
	}
	processed := 0
	for processed < limit {
		fetchCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		message, err := w.ledger.Fetch(fetchCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return processed, nil
			}
			return processed, err
		}
		if err := w.settleLedgerMessage(ctx, message); err != nil {
			if err := w.retryOrDLQ(ctx, message, err); err != nil {
				return processed, err
			}
			processed++
			continue
		}
		if err := w.ledger.Commit(ctx, message); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (w *Worker) retryOrDLQ(ctx context.Context, message LedgerMessage, settleErr error) error {
	err := settleErr
	for {
		attempts := w.settlementAttempts(ctx, message)
		if isPermanentSettlementError(err) || attempts <= 0 || attempts >= maxSettleAttempts {
			_ = w.ledger.WriteDLQ(ctx, message, err)
			_ = w.markDLQ(ctx, message, err)
			auctionID := string(message.Key)
			if auctionID == "" {
				auctionID = "-"
			}
			_ = w.pause(ctx, auctionID, "KAFKA_LEDGER_SETTLEMENT_POISON", err.Error(), "", map[string]any{
				"ledger_topic":     message.Topic,
				"ledger_partition": message.Partition,
				"ledger_offset":    message.Offset,
				"attempts":         attempts,
			})
			return w.ledger.Commit(ctx, message)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		err = w.settleLedgerMessage(ctx, message)
		if err == nil {
			return w.ledger.Commit(ctx, message)
		}
	}
}

func (w *Worker) ProcessReconcile(ctx context.Context, limit int) (int, error) {
	if w == nil || w.db == nil || w.redis == nil {
		return 0, nil
	}
	auctionIDs, err := w.activeAuctionIDs(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, auctionID := range auctionIDs {
		if _, err := w.Reconcile(ctx, auctionID); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (w *Worker) ProcessSignals(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 16
	}
	rows, err := w.db.Query(ctx, `
		WITH claimed AS (
		  SELECT id
		  FROM system_control_signals
		  WHERE signal_type IN ('pause_redis_engine','resume_redis_engine','reconcile_redis_engine')
		    AND status IN ('PENDING','PROCESSING')
		    AND (locked_until IS NULL OR locked_until < now())
		  ORDER BY created_at, id
		  LIMIT $1
		  FOR UPDATE SKIP LOCKED
		)
		UPDATE system_control_signals s
		SET status = 'PROCESSING',
		    locked_by = $2,
		    locked_until = now() + interval '30 seconds'
		FROM claimed
		WHERE s.id = claimed.id
		RETURNING s.id, s.signal_type, s.target_id
	`, limit, w.consumerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type signal struct {
		id         int64
		signalType string
		auctionID  string
	}
	var signals []signal
	for rows.Next() {
		var s signal
		if err := rows.Scan(&s.id, &s.signalType, &s.auctionID); err != nil {
			return err
		}
		signals = append(signals, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, signal := range signals {
		result, err := w.processSignal(ctx, signal.signalType, signal.auctionID)
		status := "SUCCEEDED"
		var errMsg *string
		if err != nil {
			status = "FAILED"
			msg := err.Error()
			errMsg = &msg
		}
		resultJSON, _ := json.Marshal(result)
		_, updateErr := w.db.Exec(ctx, `
			UPDATE system_control_signals
			SET status = $2, processed_at = now(), result_json = $3, error_message = $4,
			    locked_by = NULL, locked_until = NULL
			WHERE id = $1
		`, signal.id, status, resultJSON, errMsg)
		if updateErr != nil {
			return updateErr
		}
	}
	return nil
}

func (w *Worker) processSignal(ctx context.Context, signalType string, auctionID string) (map[string]any, error) {
	switch signalType {
	case "pause_redis_engine":
		err := w.pause(ctx, auctionID, "REDIS_ENGINE_MANUAL_PAUSE", "redis engine manually paused", "", nil)
		return map[string]any{"auction_id": auctionID, "paused": true}, err
	case "resume_redis_engine":
		_, err := w.db.Exec(ctx, `
			UPDATE auctions
			SET engine_paused = false,
			    engine_pause_reason = NULL,
			    engine_paused_at = NULL,
			    engine_epoch = engine_epoch + 1,
			    updated_at = now()
			WHERE id = $1
		`, auctionID)
		if err == nil && w.redis != nil {
			_ = w.redis.Del(ctx, redisx.BidEngineStateKey(auctionID)).Err()
		}
		return map[string]any{"auction_id": auctionID, "resumed": err == nil}, err
	case "reconcile_redis_engine":
		report, err := w.Reconcile(ctx, auctionID)
		if err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(report)
		var out map[string]any
		_ = json.Unmarshal(payload, &out)
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported redis engine signal %s", signalType)
	}
}

func (w *Worker) activeAuctionIDs(ctx context.Context, limit int) ([]string, error) {
	rows, err := w.db.Query(ctx, `
		SELECT id
		FROM auctions
		WHERE engine_seq > 0 OR status = 'ACTIVE'
		ORDER BY updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (w *Worker) settleLedgerMessage(ctx context.Context, message LedgerMessage) error {
	var envelope struct {
		AuctionID string `json:"auction_id"`
	}
	if err := json.Unmarshal(message.Value, &envelope); err != nil {
		return permanentSettlementError{err: err}
	}
	if envelope.AuctionID == "" {
		return permanentSettlementError{err: fmt.Errorf("kafka ledger message %s has no auction_id", message.ID)}
	}
	return w.settlePayload(ctx, envelope.AuctionID, message.ID, string(message.Value), message)
}

func (w *Worker) settlePayload(ctx context.Context, auctionID string, ledgerID string, payload string, message LedgerMessage) error {
	var result engineResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return permanentSettlementError{err: err}
	}
	if result.AuctionID != auctionID {
		return permanentSettlementError{err: fmt.Errorf("ledger auction mismatch stream=%s payload=%s", auctionID, result.AuctionID)}
	}
	attempt, err := w.recordSettlementAttempt(ctx, auctionID, ledgerID, result, message)
	if err != nil {
		return err
	}
	if attempt.status == "SETTLED" || attempt.status == "SKIPPED" {
		return nil
	}
	tx, err := w.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var dbEpoch int64
	var dbSeq int64
	var paused bool
	if err := tx.QueryRow(ctx, `
		SELECT engine_epoch, engine_seq, engine_paused
		FROM auctions
		WHERE id = $1
		FOR UPDATE
	`, auctionID).Scan(&dbEpoch, &dbSeq, &paused); err != nil {
		return err
	}
	if paused {
		return fmt.Errorf("auction engine is paused")
	}
	if result.EngineEpoch != dbEpoch {
		_ = markSettlementFailed(ctx, tx, auctionID, ledgerID, fmt.Sprintf("stale epoch redis=%d db=%d", result.EngineEpoch, dbEpoch))
		_ = pauseTx(ctx, tx, auctionID, "REDIS_ENGINE_STALE_EPOCH", "settlement rejected stale engine epoch", map[string]any{
			"redis_engine_epoch": result.EngineEpoch,
			"db_engine_epoch":    dbEpoch,
			"engine_seq":         result.EngineSeq,
			"ledger_id":          ledgerID,
		})
		return tx.Commit(ctx)
	}
	if result.EngineSeq != dbSeq+1 {
		_ = markSettlementFailed(ctx, tx, auctionID, ledgerID, fmt.Sprintf("engine seq gap redis=%d db_next=%d", result.EngineSeq, dbSeq+1))
		_ = pauseTx(ctx, tx, auctionID, "REDIS_ENGINE_LEDGER_GAP", "settlement detected engine seq gap", map[string]any{
			"redis_engine_seq": result.EngineSeq,
			"db_engine_seq":    dbSeq,
			"ledger_id":        ledgerID,
		})
		return tx.Commit(ctx)
	}

	switch result.Result {
	case resultAccepted, resultSold:
		if err := settleAccepted(ctx, tx, result); err != nil {
			_ = markSettlementFailed(ctx, tx, auctionID, ledgerID, err.Error())
			return err
		}
	case resultRejected:
		if err := settleRejected(ctx, tx, result); err != nil {
			_ = markSettlementFailed(ctx, tx, auctionID, ledgerID, err.Error())
			return err
		}
	default:
		_ = markSettlementFailed(ctx, tx, auctionID, ledgerID, "unknown result "+result.Result)
		_ = tx.Commit(ctx)
		return permanentSettlementError{err: fmt.Errorf("unknown redis engine result %s", result.Result)}
	}
	if err := markSettlementSettled(ctx, tx, auctionID, ledgerID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	observability.Inc("auction_bid_redis_settlement_total", map[string]string{"result": result.Result, "status": "settled"})
	return nil
}

type settlementAttempt struct {
	attempts int
	status   string
}

func (w *Worker) recordSettlementAttempt(ctx context.Context, auctionID string, streamID string, result engineResult, message LedgerMessage) (settlementAttempt, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return settlementAttempt{}, err
	}
	ledgerSource := "redis_stream"
	if message.Topic != "" && strings.HasPrefix(message.ID, "kafka:") {
		ledgerSource = "kafka"
	}
	var attempt settlementAttempt
	err = w.db.QueryRow(ctx, `
		INSERT INTO redis_engine_settlements (
		  auction_id, stream_id, engine_epoch, engine_seq, result, status, attempts, payload_json,
		  ledger_source, ledger_topic, ledger_partition, ledger_offset, ledger_key
		)
		VALUES ($1, $2, $3, $4, $5, 'PROCESSING', 1, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (auction_id, stream_id) DO UPDATE
		SET attempts = CASE
		      WHEN redis_engine_settlements.status IN ('SETTLED','SKIPPED') THEN redis_engine_settlements.attempts
		      ELSE redis_engine_settlements.attempts + 1
		    END,
		    status = CASE
		      WHEN redis_engine_settlements.status IN ('SETTLED','SKIPPED') THEN redis_engine_settlements.status
		      ELSE 'PROCESSING'
		    END,
		    updated_at = now()
		RETURNING attempts, status
	`, auctionID, streamID, result.EngineEpoch, result.EngineSeq, result.Result, payload, ledgerSource, message.Topic, message.Partition, message.Offset, message.Key).Scan(&attempt.attempts, &attempt.status)
	if isUniqueViolation(err) {
		return w.existingSettlementAttempt(ctx, auctionID, streamID, result)
	}
	return attempt, err
}

func (w *Worker) existingSettlementAttempt(ctx context.Context, auctionID string, streamID string, result engineResult) (settlementAttempt, error) {
	var attempt settlementAttempt
	var existingStreamID string
	var existingRequestHash string
	err := w.db.QueryRow(ctx, `
		SELECT stream_id, attempts, status, COALESCE(payload_json->>'request_hash', '')
		FROM redis_engine_settlements
		WHERE auction_id = $1 AND engine_epoch = $2 AND engine_seq = $3
	`, auctionID, result.EngineEpoch, result.EngineSeq).Scan(&existingStreamID, &attempt.attempts, &attempt.status, &existingRequestHash)
	if err != nil {
		return settlementAttempt{}, err
	}
	if existingRequestHash != result.RequestHash {
		return settlementAttempt{}, permanentSettlementError{err: fmt.Errorf("engine seq conflict auction=%s epoch=%d seq=%d existing_stream=%s new_stream=%s", auctionID, result.EngineEpoch, result.EngineSeq, existingStreamID, streamID)}
	}
	return attempt, nil
}

func settleAccepted(ctx context.Context, tx pgx.Tx, result engineResult) error {
	newStatus := "ACTIVE"
	eventType := "bid_accepted"
	responseResult := auction.BidResultAccepted
	if result.Result == resultSold {
		newStatus = "SOLD"
		eventType = "auction_sold"
		responseResult = auction.BidResultAcceptedSold
	}
	var endAt *time.Time
	if result.EndAtMS > 0 {
		t := time.UnixMilli(result.EndAtMS).UTC()
		endAt = &t
	}
	tag, err := tx.Exec(ctx, `
		UPDATE auctions
		SET status = $2,
		    current_price_cents = $3,
		    current_winner_id = $4,
		    end_at = COALESCE($5, end_at),
		    accepted_bid_count = accepted_bid_count + 1,
		    seq = $6,
		    engine_seq = $6,
		    engine_epoch = $7,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1 AND engine_epoch = $7 AND engine_seq = $6 - 1
	`, result.AuctionID, newStatus, result.AmountCents, result.UserID, endAt, result.EngineSeq, result.EngineEpoch)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("accepted settlement fenced out auction=%s epoch=%d seq=%d", result.AuctionID, result.EngineEpoch, result.EngineSeq)
	}
	reason := (*string)(nil)
	resp := result.response()
	resp.Result = responseResult
	resp.SettlementStatus = auction.SettlementStatusSettled
	resp.Seq = result.EngineSeq
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if err := insertBid(ctx, tx, result, "ACCEPTED", reason, respJSON); err != nil {
		return err
	}
	payload := map[string]any{
		"bid_id":              result.BidID,
		"user_id":             result.UserID,
		"amount_cents":        result.AmountCents,
		"result":              responseResult,
		"current_price_cents": result.AmountCents,
		"engine_epoch":        result.EngineEpoch,
		"engine_seq":          result.EngineSeq,
		"settlement_status":   auction.SettlementStatusSettled,
	}
	if result.Result == resultSold {
		orderID, err := createOrder(ctx, tx, result.AuctionID, result.UserID, result.AmountCents)
		if err != nil {
			return err
		}
		payload["order_id"] = orderID
	}
	if err := appendEvent(ctx, tx, result.AuctionID, result.EngineSeq, result.EngineEpoch, eventType, result.TraceID, payload); err != nil {
		return err
	}
	return completeIdem(ctx, tx, result, responseResult, respJSON)
}

func settleRejected(ctx context.Context, tx pgx.Tx, result engineResult) error {
	broadcastReject := shouldBroadcastReject(result.RejectReason)
	tag, err := tx.Exec(ctx, `
		UPDATE auctions
		SET engine_seq = $2,
		    engine_epoch = $3,
		    seq = CASE WHEN $4 THEN $2 ELSE seq END,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1 AND engine_epoch = $3 AND engine_seq = $2 - 1
	`, result.AuctionID, result.EngineSeq, result.EngineEpoch, broadcastReject)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("rejected settlement fenced out auction=%s epoch=%d seq=%d", result.AuctionID, result.EngineEpoch, result.EngineSeq)
	}
	resp := result.response()
	resp.Result = auction.BidResultRejected
	resp.SettlementStatus = auction.SettlementStatusSettled
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if err := insertBid(ctx, tx, result, "REJECTED", result.RejectReason, respJSON); err != nil {
		return err
	}
	if broadcastReject {
		payload := map[string]any{
			"bid_id":            result.BidID,
			"user_id":           result.UserID,
			"amount_cents":      result.AmountCents,
			"reason":            stringPtrValue(result.RejectReason),
			"engine_epoch":      result.EngineEpoch,
			"engine_seq":        result.EngineSeq,
			"settlement_status": auction.SettlementStatusSettled,
		}
		if err := appendEvent(ctx, tx, result.AuctionID, result.EngineSeq, result.EngineEpoch, "bid_rejected", result.TraceID, payload); err != nil {
			return err
		}
	}
	return completeIdem(ctx, tx, result, auction.BidResultRejected, respJSON)
}

func insertBid(ctx context.Context, tx pgx.Tx, result engineResult, status string, rejectReason *string, responseJSON []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO bids (
		  id, auction_id, user_id, client_bid_id, amount_cents, seq, status,
		  reject_reason, request_hash, response_json, trace_id, source,
		  engine_epoch, engine_seq, settlement_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'SETTLED')
		ON CONFLICT (auction_id, user_id, client_bid_id) DO NOTHING
	`, result.BidID, result.AuctionID, result.UserID, result.ClientBidID, result.AmountCents, result.EngineSeq, status, rejectReason, result.RequestHash, responseJSON, result.TraceID, auction.BidSourceManual, result.EngineEpoch, result.EngineSeq)
	return err
}

func completeIdem(ctx context.Context, tx pgx.Tx, result engineResult, resultCode string, responseJSON []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO idempotency_records (
		  scope_type, scope_id, user_id, idempotency_key, request_hash, status,
		  attempts, http_status, result_code, response_json, completed_at
		)
		VALUES ('bid', $1, $2, $3, $4, 'COMPLETED', 1, 200, $5, $6, now())
		ON CONFLICT (scope_type, scope_id, user_id, idempotency_key) DO UPDATE
		SET status = 'COMPLETED',
		    http_status = EXCLUDED.http_status,
		    result_code = EXCLUDED.result_code,
		    response_json = EXCLUDED.response_json,
		    completed_at = now(),
		    locked_until = NULL
		WHERE idempotency_records.request_hash = EXCLUDED.request_hash
	`, result.AuctionID, result.UserID, result.ClientBidID, result.RequestHash, resultCode, responseJSON)
	return err
}

func appendEvent(ctx context.Context, tx pgx.Tx, auctionID string, seq int64, epoch int64, eventType string, traceID string, payload map[string]any) error {
	payload["state_version"] = seq
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	serverTimeMS := time.Now().UTC().UnixMilli()
	if _, err := tx.Exec(ctx, `
		INSERT INTO auction_events (auction_id, seq, event_type, payload_json, server_time_ms, trace_id, engine_epoch, engine_seq)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $2)
		ON CONFLICT (auction_id, seq) DO NOTHING
	`, auctionID, seq, eventType, payloadJSON, serverTimeMS, traceID, epoch); err != nil {
		return err
	}
	var outboxID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO outbox_events (
		  aggregate_type, aggregate_id, auction_id, seq, event_type,
		  event_schema_version, event_key, payload_json, payload_sha256
		)
		VALUES (
		  'auction', $1, $1, $2, $3, 1, $1, $4,
		  encode(digest(convert_to($4::jsonb::text, 'UTF8'), 'sha256'), 'hex')
		)
		RETURNING id
	`, auctionID, seq, eventType, payloadJSON).Scan(&outboxID)
	if isUniqueViolation(err) {
		err = tx.QueryRow(ctx, `
			SELECT id
			FROM outbox_events
			WHERE aggregate_type = 'auction'
			  AND aggregate_id = $1
			  AND event_type = $2
			  AND seq = $3
			LIMIT 1
		`, auctionID, eventType, seq).Scan(&outboxID)
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_delivery (outbox_id, status)
		VALUES ($1, 'PENDING')
		ON CONFLICT (outbox_id) DO NOTHING
	`, outboxID)
	return err
}

func createOrder(ctx context.Context, tx pgx.Tx, auctionID string, winnerID string, amountCents int64) (string, error) {
	orderID := "ord_" + uuid.NewString()
	expireAt := time.Now().UTC().Add(15 * time.Minute)
	var existing string
	err := tx.QueryRow(ctx, `
		INSERT INTO orders (id, auction_id, winner_id, amount_cents, status, deposit_cents, deposit_status, expire_at)
		VALUES ($1, $2, $3, $4, 'ORDER_PENDING', $5, 'HELD', $6)
		ON CONFLICT (auction_id) DO UPDATE SET auction_id = EXCLUDED.auction_id
		RETURNING id
	`, orderID, auctionID, winnerID, amountCents, amountCents/10, expireAt).Scan(&existing)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO scheduler_jobs (id, job_type, target_type, target_id, idempotency_key, run_at, status)
		VALUES ($1, 'EXPIRE_ORDER', 'order', $2, $3, $4, 'PENDING')
		ON CONFLICT(job_type, target_type, target_id, idempotency_key) DO NOTHING
	`, "job_expire_"+existing, existing, "expire:"+existing, expireAt)
	return existing, err
}

func markSettlementSettled(ctx context.Context, tx pgx.Tx, auctionID string, streamID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE redis_engine_settlements
		SET status = 'SETTLED', settled_at = now(), updated_at = now()
		WHERE auction_id = $1 AND stream_id = $2
	`, auctionID, streamID)
	return err
}

func markSettlementFailed(ctx context.Context, tx pgx.Tx, auctionID string, streamID string, message string) error {
	_, err := tx.Exec(ctx, `
		UPDATE redis_engine_settlements
		SET status = 'FAILED', last_error = $3, updated_at = now()
		WHERE auction_id = $1 AND stream_id = $2
	`, auctionID, streamID, message)
	return err
}

func (w *Worker) settlementAttempts(ctx context.Context, message LedgerMessage) int {
	if w == nil || w.db == nil {
		return 0
	}
	var attempts int
	_ = w.db.QueryRow(ctx, `
		SELECT attempts
		FROM redis_engine_settlements
		WHERE stream_id = $1
	`, message.ID).Scan(&attempts)
	return attempts
}

func (w *Worker) markDLQ(ctx context.Context, message LedgerMessage, eventErr error) error {
	if w == nil || w.db == nil {
		return nil
	}
	_, err := w.db.Exec(ctx, `
		UPDATE redis_engine_settlements
		SET status = 'FAILED',
		    dlq_topic = $2,
		    dlq_error = $3,
		    dlq_at = now(),
		    last_error = $3,
		    updated_at = now()
		WHERE stream_id = $1
	`, message.ID, w.dlqTopic, eventErr.Error())
	return err
}

type permanentSettlementError struct {
	err error
}

func (e permanentSettlementError) Error() string {
	return e.err.Error()
}

func (e permanentSettlementError) Unwrap() error {
	return e.err
}

func isPermanentSettlementError(err error) bool {
	var target permanentSettlementError
	return errors.As(err, &target)
}

func (w *Worker) pause(ctx context.Context, auctionID string, reason string, message string, traceID string, details map[string]any) error {
	tx, err := w.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if details == nil {
		details = map[string]any{}
	}
	details["trace_id"] = traceID
	if err := pauseTx(ctx, tx, auctionID, reason, message, details); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if w.redis != nil {
		_ = w.redis.HSet(ctx, redisx.BidEngineStateKey(auctionID), "paused", 1, "pause_reason", reason).Err()
	}
	return nil
}

func pauseTx(ctx context.Context, tx pgx.Tx, auctionID string, reason string, message string, details map[string]any) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = true,
		    engine_pause_reason = $2,
		    engine_paused_at = now(),
		    updated_at = now()
		WHERE id = $1
	`, auctionID, reason); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('HIGH', $1, $2, $3, $4)
	`, reason, auctionID, message, payload)
	return err
}

func (w *Worker) Reconcile(ctx context.Context, auctionID string) (Report, error) {
	report := Report{CheckedAt: time.Now().UTC(), AuctionID: auctionID, Status: "OK"}
	var dbSeq int64
	var paused bool
	if err := w.db.QueryRow(ctx, `
		SELECT a.engine_seq,
		       a.engine_paused,
		       COALESCE(count(s.id) FILTER (WHERE s.status = 'PROCESSING'), 0) AS pending_settlements,
		       COALESCE(count(s.id) FILTER (WHERE s.status = 'FAILED'), 0) AS failed_settlements,
		       COALESCE(count(s.id) FILTER (WHERE s.dlq_at IS NOT NULL), 0) AS dlq_settlements
		FROM auctions a
		LEFT JOIN redis_engine_settlements s ON s.auction_id = a.id
		WHERE a.id = $1
		GROUP BY a.id
	`, auctionID).Scan(&dbSeq, &paused, &report.PendingSettlements, &report.FailedSettlements, &report.DLQSettlements); err != nil {
		return report, err
	}
	report.DBSeq = dbSeq
	report.Paused = paused
	redisSeq, err := w.redis.HGet(ctx, redisx.BidEngineStateKey(auctionID), "engine_seq").Int64()
	if err != nil && !errors.Is(err, redis.Nil) {
		report.Status = "REDIS_READ_FAILED"
		report.DriftCount = 1
		report.Message = err.Error()
		_ = w.pause(ctx, auctionID, "REDIS_ENGINE_RECONCILE_READ_FAILED", err.Error(), "", nil)
		return report, nil
	}
	report.RedisSeq = redisSeq
	pending, err := w.redis.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		report.Status = "REDIS_PENDING_READ_FAILED"
		report.DriftCount = 1
		report.Message = err.Error()
		_ = w.pause(ctx, auctionID, "REDIS_ENGINE_PENDING_READ_FAILED", err.Error(), "", nil)
		return report, nil
	}
	if pending > 0 {
		report.Status = "REDIS_PENDING_WITHOUT_KAFKA_LEDGER"
		report.DriftCount = 1
		report.Message = "Redis has hot-engine decisions not acknowledged by Kafka ledger"
		_ = w.pause(ctx, auctionID, "REDIS_ENGINE_PENDING_KAFKA_APPEND_UNKNOWN", report.Message, "", map[string]any{"pending_decisions": pending})
		return report, nil
	}
	if redisSeq < dbSeq {
		report.Status = "REDIS_BEHIND_DB"
		report.DriftCount = 1
		report.Message = "Redis engine seq is behind PostgreSQL settlement"
		_ = w.pause(ctx, auctionID, "REDIS_ENGINE_REDIS_BEHIND_DB", report.Message, "", map[string]any{"redis_seq": redisSeq, "db_seq": dbSeq})
	}
	if redisSeq > dbSeq {
		report.Status = "DB_BEHIND_REDIS"
		report.DriftCount = 1
		report.Message = "PostgreSQL settlement is behind Redis engine ledger"
		_ = w.pause(ctx, auctionID, "REDIS_ENGINE_DB_BEHIND_REDIS", report.Message, "", map[string]any{"redis_seq": redisSeq, "db_seq": dbSeq})
	}
	if report.DLQSettlements > 0 {
		report.Status = "KAFKA_LEDGER_DLQ"
		report.DriftCount++
		report.Message = "Kafka bid ledger settlement has dead-lettered events"
		_ = w.pause(ctx, auctionID, "KAFKA_LEDGER_DLQ_PRESENT", report.Message, "", map[string]any{"dlq_settlements": report.DLQSettlements})
	}
	return report, nil
}

func shouldBroadcastReject(reason *string) bool {
	if reason == nil {
		return true
	}
	switch *reason {
	case string(apierrors.CodeBidTooLow), string(apierrors.CodeAuctionEnded), string(apierrors.CodeAuctionNotActive):
		return false
	default:
		return true
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
