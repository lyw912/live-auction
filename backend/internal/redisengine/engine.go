package redisengine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/redisx"
	apptracing "live-auction/backend/internal/tracing"
)

const (
	engineStateTTL             = 30 * time.Minute
	idempotencyTTL             = 24 * time.Hour
	maxSettleAttempts          = 3
	kafkaFetchTimeout          = 2 * time.Second
	reconcilePendingDrainLimit = 10_000
	// relayBatchSize is the max number of stream entries read in one relay batch.
	// Tuned to balance throughput (large batch) with tail latency (small batch).
	relayBatchSize = 512
	// relayBatchBlock is the XREAD block timeout; keeps the relay responsive.
	relayBatchBlock   = 2 * time.Millisecond
	resultAccepted    = "ENGINE_ACCEPTED"
	resultRejected    = "ENGINE_REJECTED"
	resultSold        = "ENGINE_SOLD"
	resultReconciling = "RECONCILING"
)

const (
	kafkaAppendStatusAcked   = "ACKED"
	kafkaAppendStatusFailed  = "FAILED"
	kafkaAppendStatusUnknown = "UNKNOWN"
)

var ledgerRunner = redisx.NewScriptRunner(redisx.ScriptBidRedisLedger, `
local state_key = KEYS[1]
local idem_key = KEYS[2]
local pending_key = KEYS[3]
local pending_auctions_key = KEYS[4]
local log_stream_key = KEYS[5]
local acl_key = KEYS[6]

-- ACL membership check: runs atomically inside the script, so no separate
-- Redis round-trip is needed. The gateway pre-seeds acl:membership:{auction}:{user}
-- when the room opens; the bid hot path never touches the DB for ACL.
-- acl_key is empty string when called without ACL context (tests, non-gateway paths).
if acl_key and acl_key ~= '' then
    local acl_val = redis.call('GET', acl_key)
    if not acl_val or acl_val == false or acl_val == '' then
        return {'ERROR', 'ACL_FORBIDDEN', 'user does not have active room membership'}
    end
end


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
  if state_json == '' then
    return {'ERROR', 'RECONCILING', 'redis engine state missing and no postgres snapshot supplied'}
  end
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
  redis.call('HSET', state_key, 'seq', tostring(decoded['seq'] or 0))
  redis.call('HSET', state_key, 'engine_seq', tostring(decoded['engine_seq'] or 0))
  redis.call('HSET', state_key, 'engine_epoch', tostring(decoded['engine_epoch'] or 1))
  redis.call('HSET', state_key, 'paused', tostring(decoded['paused'] or false))
  redis.call('HSET', state_key, 'pause_reason', tostring(decoded['pause_reason'] or ''))
  redis.call('HSET', state_key, 'requires_postgres', tostring(decoded['requires_postgres'] or ''))
  -- Seed the hard ceiling for soft-close extensions. If the snapshot already carries
  -- absolute_end_ms use it; otherwise compute from initial end + max total extension.
  local abs_end = tonumber(decoded['absolute_end_ms']) or 0
  if abs_end <= 0 then
    local seed_end = tonumber(decoded['end_at_ms']) or 0
    local seed_max_ext = tonumber(decoded['max_extend_count']) or 0
    local seed_ext_by = tonumber(decoded['extend_by_ms']) or 0
    abs_end = seed_end + seed_max_ext * seed_ext_by
  end
  redis.call('HSET', state_key, 'absolute_end_ms', tostring(abs_end))
  redis.call('PEXPIRE', state_key, state_ttl_ms)
end

local s = redis.call('HMGET', state_key,
  'status', 'current_price_cents', 'current_winner_id', 'start_price_cents',
  'increment_cents', 'cap_price_cents', 'end_at_ms', 'extend_window_ms',
  'extend_by_ms', 'max_extend_count', 'extend_count', 'accepted_bid_count',
  'seq', 'engine_seq', 'engine_epoch', 'paused', 'pause_reason', 'requires_postgres',
  'absolute_end_ms')

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
local public_seq = tonumber(s[13]) or 0
local engine_seq = tonumber(s[14]) or 0
local engine_epoch = tonumber(s[15]) or 1
local paused = tostring(s[16])
local pause_reason = tostring(s[17] or '')
local requires_postgres = tostring(s[18] or '')
local absolute_end_ms = tonumber(s[19]) or 0

if status == '' or s[2] == false or s[4] == false or s[5] == false or s[7] == false or
   s[12] == false or s[13] == false or s[14] == false or s[15] == false or s[16] == false then
  return {'ERROR', 'RECONCILING', 'redis engine state is incomplete'}
end

local function store_result(result)
  local encoded = cjson.encode(result)
  redis.call('HSET', idem_key, 'request_hash', request_hash)
  redis.call('HSET', idem_key, 'result_json', encoded)
  redis.call('HSET', idem_key, 'engine_seq', tostring(result['engine_seq'] or 0))
  redis.call('HSET', idem_key, 'engine_epoch', tostring(result['engine_epoch'] or 0))
  redis.call('HSET', idem_key, 'kafka_append_status', 'UNKNOWN')
  redis.call('HSET', idem_key, 'kafka_append_attempted', '0')
  redis.call('HSET', idem_key, 'expires_at_ms', tostring(now_ms + idem_ttl_ms))
  redis.call('PEXPIRE', idem_key, idem_ttl_ms)
  return encoded
end

local function store_decision(result)
  local encoded = store_result(result)
  -- Keep pending_key for reconciler visibility (backward-compat).
  redis.call('HSET', pending_key, tostring(result['engine_seq']), encoded)
  redis.call('PEXPIRE', pending_key, state_ttl_ms)
  redis.call('SADD', pending_auctions_key, auction_id)
  -- XADD to the decision log stream — atomic with the decision itself.
  -- The group-commit relay reads this stream and batch-produces to Kafka.
  -- The stream entry ID returned by XADD is not used here; the relay tracks
  -- its position via the relay-cursor key.
  redis.call('XADD', log_stream_key, '*',
    'engine_seq', tostring(result['engine_seq']),
    'engine_epoch', tostring(result['engine_epoch']),
    'result', tostring(result['result']),
    'auction_id', auction_id,
    'payload', encoded)
  redis.call('PEXPIRE', log_stream_key, state_ttl_ms)
  return encoded
end

local function reject(reason)
  engine_seq = engine_seq + 1
  redis.call('HSET', state_key, 'engine_seq', engine_seq)
  redis.call('PEXPIRE', state_key, state_ttl_ms)
  local basis_base = current_price
  if accepted_bid_count <= 0 then
    basis_base = start_price
  end
  local basis_required_min = basis_base + increment
  local result = {
    result = 'ENGINE_REJECTED',
    bid_id = bid_id,
    auction_id = auction_id,
    user_id = user_id,
    client_bid_id = client_bid_id,
    amount_cents = amount,
    seq = public_seq,
    engine_seq = engine_seq,
    engine_epoch = engine_epoch,
    settlement_status = 'PENDING',
    reject_reason = reason,
    current_price_cents = current_price,
    current_winner_id = current_winner,
    end_at_ms = end_at_ms,
    server_time_ms = now_ms,
    trace_id = trace_id,
    request_hash = request_hash,
    decision_basis = {
      previous_price_cents = basis_base,
      required_min_price_cents = basis_required_min,
      current_price_cents = current_price,
      reason = reason,
      engine_seq = engine_seq
    }
  }
  return {'OK', store_decision(result)}
end

if paused == '1' or paused == 'true' then
  return {'ERROR', 'ENGINE_PAUSED', pause_reason}
end
if status == 'RECONCILING' then
  return {'ERROR', 'RECONCILING', pause_reason}
end
if requires_postgres ~= '' then
  return {'ERROR', 'ENGINE_PAUSED', requires_postgres}
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
local required_min_price = base + increment

local function with_basis(result, reason)
  result['decision_basis'] = {
    previous_price_cents = base,
    required_min_price_cents = required_min_price,
    current_price_cents = current_price,
    reason = reason,
    engine_seq = result['engine_seq'] or 0
  }
  return result
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
    local candidate = end_at_ms + extend_by_ms
    if candidate > end_at_ms then
      -- Hard ceiling: clamp to absolute_end_ms (original_end + max_total_extension).
      -- Prevents infinite-extension attacks (bots bidding every 29s to hold auction open).
      if absolute_end_ms > 0 and candidate > absolute_end_ms then
        candidate = absolute_end_ms
      end
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
  seq = public_seq,
  engine_seq = engine_seq,
  engine_epoch = engine_epoch,
  settlement_status = 'PENDING',
  current_price_cents = amount,
  current_winner_id = user_id,
  end_at_ms = new_end_at_ms,
  extend_count = new_extend_count,
  server_time_ms = now_ms,
  trace_id = trace_id,
  request_hash = request_hash
}
return {'OK', store_decision(with_basis(result, nil))}
`)

type Engine struct {
	db             *pgxpool.Pool
	redis          *redis.Client
	ledger         BidLedger
	snapshotLoads  singleflight.Group
	coldStartGroup singleflight.Group // serialises cold-start recovery per auction
}

type snapshot struct {
	Status            string `json:"status"`
	CurrentPriceCents int64  `json:"current_price_cents"`
	CurrentWinnerID   string `json:"current_winner_id,omitempty"`
	StartPriceCents   int64  `json:"start_price_cents"`
	IncrementCents    int64  `json:"increment_cents"`
	CapPriceCents     int64  `json:"cap_price_cents,omitempty"`
	EndAtMS           int64  `json:"end_at_ms"`
	// AbsoluteEndMS is the hard ceiling for soft-close extensions:
	// original_end_at + max_extend_count * extend_by. Set once at seed time and
	// restored on rebuild. The Lua clamps new_end_at to this value.
	AbsoluteEndMS    int64  `json:"absolute_end_ms,omitempty"`
	ExtendWindowMS   int64  `json:"extend_window_ms"`
	ExtendByMS       int64  `json:"extend_by_ms"`
	MaxExtendCount   int    `json:"max_extend_count"`
	ExtendCount      int    `json:"extend_count"`
	AcceptedBidCount int64  `json:"accepted_bid_count"`
	Seq              int64  `json:"seq"`
	EngineSeq        int64  `json:"engine_seq"`
	EngineEpoch      int64  `json:"engine_epoch"`
	Paused           bool   `json:"paused"`
	PauseReason      string `json:"pause_reason,omitempty"`
	RequiresPostgres string `json:"requires_postgres,omitempty"`
}

type engineResult struct {
	Result            string        `json:"result"`
	BidID             string        `json:"bid_id"`
	AuctionID         string        `json:"auction_id"`
	UserID            string        `json:"user_id"`
	ClientBidID       string        `json:"client_bid_id"`
	AmountCents       int64         `json:"amount_cents"`
	Seq               int64         `json:"seq"`
	EngineSeq         int64         `json:"engine_seq"`
	EngineEpoch       int64         `json:"engine_epoch"`
	SettlementStatus  string        `json:"settlement_status"`
	RejectReason      *string       `json:"reject_reason,omitempty"`
	CurrentPriceCents int64         `json:"current_price_cents"`
	CurrentWinnerID   string        `json:"current_winner_id,omitempty"`
	EndAtMS           int64         `json:"end_at_ms"`
	ExtendCount       int           `json:"extend_count"`
	ServerTimeMS      int64         `json:"server_time_ms"`
	TraceID           string        `json:"trace_id"`
	RequestHash       string        `json:"request_hash"`
	DecisionBasis     decisionBasis `json:"decision_basis"`
}

type decisionBasis struct {
	PreviousPriceCents    int64   `json:"previous_price_cents"`
	RequiredMinPriceCents int64   `json:"required_min_price_cents"`
	CurrentPriceCents     int64   `json:"current_price_cents"`
	Reason                *string `json:"reason,omitempty"`
	EngineSeq             int64   `json:"engine_seq,omitempty"`
}

type redisIdempotencyReplay struct {
	RequestHash       string
	ResultJSON        string
	KafkaAppendStatus string
	EngineEpoch       int64
	EngineSeq         int64
	ExpiresAtMS       int64
}

type redisIdempotencyReplayLoad struct {
	Record redisIdempotencyReplay
	Found  bool
}

func New(db *pgxpool.Pool, redisClient *redis.Client, ledger BidLedger) *Engine {
	return &Engine{db: db, redis: redisClient, ledger: ledger}
}

// PlaceBid executes the bid hot path. Pass aclKey to enforce room-membership
// inside the Lua script atomically (no extra Redis RTT). Omit it (or pass "")
// for test/non-gateway callers — Lua skips the ACL check in that case.
func (e *Engine) PlaceBid(ctx context.Context, auctionID string, userID string, idempotencyKey string, input auction.BidInput, traceID string, aclKey ...string) (auction.BidResponse, error) {
	acl := ""
	if len(aclKey) > 0 {
		acl = aclKey[0]
	}
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
	traceCtx := ctx
	var stageErr error
	var stageSpan trace.Span

	nowMS := time.Now().UTC().UnixMilli()
	bidID := "bid_" + uuid.NewString()

	// Optimistic execution: skip the pre-Lua EXISTS check and go straight to EVAL.
	// For the warm hot path (100% of PTS-1B and normal production load), state is
	// already in Redis and EXISTS would always return 1 — paying an extra Redis RTT
	// and joining the single-threaded queue twice for nothing.
	//
	// If Redis state is missing (cold start after crash/restart), the Lua script
	// returns {ERROR, RECONCILING, "state missing..."} with len(values)==3. We
	// detect that signal below and run the cold-start path exactly once.
	stateJSON := ""
	return e.placeBidWithSnapshot(ctx, traceCtx, auctionID, userID, idempotencyKey, input,
		requestHash, traceID, nowMS, bidID, stateJSON, acl, &stageErr, &stageSpan)
}

// placeBidWithSnapshot runs the Lua CAS and, on cold-start signal, loads a
// PostgreSQL snapshot and retries once. Splitting this out keeps PlaceBid readable
// while avoiding a goto or closure workaround.
func (e *Engine) placeBidWithSnapshot(
	ctx context.Context, traceCtx context.Context,
	auctionID, userID, idempotencyKey string, input auction.BidInput,
	requestHash, traceID string, nowMS int64, bidID, stateJSON, aclKey string,
	stageErr *error, stageSpan *trace.Span,
) (auction.BidResponse, error) {
	totalStart := time.Now()
	start := time.Now()
	_, *stageSpan = apptracing.Start(traceCtx, "bid.redis_lua", attribute.String("auction.id", auctionID))
	cmd := ledgerRunner.Run(ctx, e.redis, []string{
		redisx.BidEngineStateKey(auctionID),
		redisx.BidEngineIdempotencyKey(auctionID, input.ClientBidID),
		redisx.BidEnginePendingKey(auctionID),
		redisx.BidEnginePendingAuctionsKey(),
		redisx.BidEngineLogStreamKey(auctionID), // KEYS[5]: decision log stream (WAL)
		aclKey,                                  // KEYS[6]: ACL check in-Lua (empty = skip)
	}, nowMS, auctionID, userID, input.ClientBidID, input.AmountCents, requestHash, traceID, bidID, stateJSON, engineStateTTL.Milliseconds(), idempotencyTTL.Milliseconds())
	values, err := cmd.Slice()
	if err != nil {
		apptracing.End(*stageSpan, err)
		ledgerRunner.Record(redisx.ClassifyScriptError(err), time.Since(start))
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_SCRIPT_ERROR", err.Error(), traceID)
		return auction.BidResponse{}, apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine paused after script failure: "+err.Error(), http.StatusServiceUnavailable)
	}
	if len(values) < 2 {
		err := fmt.Errorf("redis ledger engine returned invalid result: %v", values)
		apptracing.End(*stageSpan, err)
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_BAD_SCRIPT_RESULT", fmt.Sprintf("%v", values), traceID)
		return auction.BidResponse{}, apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine returned invalid result", http.StatusServiceUnavailable)
	}
	status := stringValue(values[0])
	if status == "ERROR" {
		code := apierrors.Code(stringValue(values[1]))
		apiErr := apierrors.New(code, "redis ledger engine rejected request", http.StatusConflict)
		apptracing.End(*stageSpan, apiErr)

		// Lua signals state missing + no snapshot: run cold-start path and retry once.
		// This only happens after Redis data loss (restart/eviction); normal hot path
		// never hits this branch.
		if stringValue(values[1]) == "ACL_FORBIDDEN" {
			return auction.BidResponse{}, apierrors.New(apierrors.CodeForbiddenRoom, "room access denied", http.StatusForbidden)
		}
		if code == apierrors.CodeEngineReconciling && stateJSON == "" &&
			len(values) >= 3 && strings.HasPrefix(stringValue(values[2]), "redis engine state missing") {
			// Serialise cold-start per auction: only one goroutine runs the safety
			// checks and snapshot load; all others wait and share the result.
			type coldResult struct {
				resp     auction.BidResponse
				snapJSON string
			}
			val, sfErr, _ := e.coldStartGroup.Do(auctionID, func() (any, error) {
				resp, snapJSON, err := e.runColdStart(ctx, traceCtx, auctionID, userID, idempotencyKey, requestHash, traceID, stageErr, stageSpan)
				return coldResult{resp, snapJSON}, err
			})
			if sfErr != nil {
				return val.(coldResult).resp, sfErr
			}
			cr := val.(coldResult)
			if cr.resp != (auction.BidResponse{}) {
				return cr.resp, nil
			}
			return e.placeBidWithSnapshot(ctx, traceCtx, auctionID, userID, idempotencyKey, input,
				requestHash, traceID, nowMS, bidID, cr.snapJSON, aclKey, stageErr, stageSpan)
		}

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
		apptracing.End(*stageSpan, err)
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_RESULT_DECODE_FAILED", err.Error(), traceID)
		return auction.BidResponse{}, err
	}
	(*stageSpan).SetAttributes(
		attribute.String("bid.result", result.Result),
		attribute.Int64("bid.engine_seq", result.EngineSeq),
		attribute.Int64("bid.engine_epoch", result.EngineEpoch),
	)
	apptracing.End(*stageSpan, nil)
	redisElapsed := time.Since(start)
	ledgerRunner.Record("ok", redisElapsed)
	recordDecision(result.Result, redisElapsed)
	recordHTTPStage("redis_lua", result.Result, "ok", redisElapsed)
	if status == "REPLAY" {
		_, *stageSpan = apptracing.Start(traceCtx, "bid.redis_replay_response", attribute.String("auction.id", auctionID))
		resp, err := e.redisIdempotencyReplayResponse(ctx, auctionID, input.ClientBidID, requestHash, result, totalStart)
		apptracing.End(*stageSpan, err)
		return resp, err
	}
	recordHTTPStage("total", result.Result, "ok", time.Since(totalStart))
	return result.response(auction.DurabilityStatusEngineDurable, auction.DecisionStatusDecided), nil
}

// runColdStart performs the safety checks and snapshot load that must precede
// seeding Redis hot state. Called only when placeBidWithSnapshot detects a
// cold-start signal from the Lua script.
func (e *Engine) runColdStart(
	ctx context.Context, traceCtx context.Context,
	auctionID, userID, idempotencyKey, requestHash, traceID string,
	stageErr *error, stageSpan *trace.Span,
) (auction.BidResponse, string, error) {
	if appendSeq, err := e.redisAppendHighWater(ctx, auctionID); err != nil {
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_APPEND_MARKER_CHECK_FAILED", err.Error(), traceID)
		return auction.BidResponse{}, "", apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine append marker check failed", http.StatusServiceUnavailable)
	} else if appendSeq > 0 {
		if exists, existsErr := e.redis.Exists(ctx, redisx.BidEngineStateKey(auctionID)).Result(); existsErr != nil {
			_ = e.pause(ctx, auctionID, "REDIS_ENGINE_STATE_RECHECK_FAILED", existsErr.Error(), traceID)
			return auction.BidResponse{}, "", apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine state recheck failed", http.StatusServiceUnavailable)
		} else if exists > 0 {
			// State appeared while we were checking: retry without snapshot.
			return auction.BidResponse{}, "", nil
		}
		err := fmt.Errorf("redis engine state is missing after kafka append high-water engine_seq=%d", appendSeq)
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_STATE_MISSING_REQUIRES_RECONCILE", err.Error(), traceID)
		return auction.BidResponse{}, "", apierrors.New(apierrors.CodeEngineReconciling, "auction engine is reconciling after redis state loss", http.StatusConflict)
	}
	pendingCount, err := e.redis.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result()
	if err != nil {
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_PENDING_CHECK_FAILED", err.Error(), traceID)
		return auction.BidResponse{}, "", apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine pending check failed", http.StatusServiceUnavailable)
	}
	if pendingCount > 0 {
		if exists, existsErr := e.redis.Exists(ctx, redisx.BidEngineStateKey(auctionID)).Result(); existsErr != nil {
			_ = e.pause(ctx, auctionID, "REDIS_ENGINE_STATE_RECHECK_FAILED", existsErr.Error(), traceID)
			return auction.BidResponse{}, "", apierrors.New(apierrors.CodeEnginePaused, "redis ledger engine state recheck failed", http.StatusServiceUnavailable)
		} else if exists > 0 {
			return auction.BidResponse{}, "", nil
		}
		err := fmt.Errorf("redis engine state is missing while %d pending decisions remain", pendingCount)
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_STATE_MISSING_REQUIRES_RECONCILE", err.Error(), traceID)
		return auction.BidResponse{}, "", apierrors.New(apierrors.CodeEngineReconciling, "auction engine is reconciling after redis state loss", http.StatusConflict)
	}
	_, *stageSpan = apptracing.Start(traceCtx, "bid.idempotency.pg_cold", attribute.String("auction.id", auctionID))
	if replay, ok, err := e.completedReplay(ctx, auctionID, userID, idempotencyKey, requestHash); err != nil || ok {
		*stageErr = err
		if ok {
			(*stageSpan).SetAttributes(attribute.Bool("bid.idempotency.replay", true))
		}
		apptracing.End(*stageSpan, *stageErr)
		return replay, "", err
	}
	apptracing.End(*stageSpan, nil)

	_, *stageSpan = apptracing.Start(traceCtx, "bid.snapshot_load_cold", attribute.String("auction.id", auctionID))
	snap, err := e.loadSnapshotSingleflight(ctx, auctionID)
	if err != nil {
		apptracing.End(*stageSpan, err)
		return auction.BidResponse{}, "", err
	}
	if err := e.ensureColdSnapshotCanSeedRedis(ctx, auctionID, snap); err != nil {
		_ = e.pause(ctx, auctionID, "REDIS_ENGINE_STATE_MISSING_REQUIRES_RECONCILE", err.Error(), traceID)
		apptracing.End(*stageSpan, err)
		return auction.BidResponse{}, "", apierrors.New(apierrors.CodeEngineReconciling, "auction engine is reconciling after redis state loss", http.StatusConflict)
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		apptracing.End(*stageSpan, err)
		return auction.BidResponse{}, "", err
	}
	apptracing.End(*stageSpan, nil)
	return auction.BidResponse{}, string(raw), nil
}

func (e *Engine) redisIdempotencyReplayResponse(ctx context.Context, auctionID string, clientBidID string, requestHash string, result engineResult, totalStart time.Time) (auction.BidResponse, error) {
	load, err := e.loadRedisIdempotencyReplay(ctx, auctionID, clientBidID)
	if err != nil {
		return auction.BidResponse{}, err
	}
	if !load.Found {
		return auction.BidResponse{}, apierrors.New(apierrors.CodeProcessingRetryLater, "bid confirmation state is unavailable; retry with the same idempotency key", http.StatusConflict)
	}
	replay := load.Record
	if replay.RequestHash != requestHash {
		return auction.BidResponse{}, apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "idempotency key reused with different request", http.StatusConflict)
	}
	return e.redisIdempotencyReplayResponseFromRecord(replay, result, totalStart)
}

func (e *Engine) redisIdempotencyReplayResponseFromRecord(replay redisIdempotencyReplay, result engineResult, totalStart time.Time) (auction.BidResponse, error) {
	status := replay.KafkaAppendStatus
	switch status {
	case kafkaAppendStatusAcked:
		// Relay has already committed to Kafka — return full durability.
		recordHTTPStage("total", result.Result, "replay_kafka_acked", time.Since(totalStart))
		return result.response(auction.DurabilityStatusKafkaAcked, auction.DecisionStatusDecided), nil
	case kafkaAppendStatusFailed:
		// Only set on explicit pause/reconciling; should be rare post-v3.
		recordHTTPStage("total", result.Result, "replay_kafka_failed", time.Since(totalStart))
		resp := result.response(auction.DurabilityStatusKafkaFailed, auction.DecisionStatusReconciling)
		resp.Result = string(apierrors.CodeEngineReconciling)
		return resp, apierrors.New(apierrors.CodeEngineReconciling, "bid engine is reconciling; retry with the same idempotency key", http.StatusConflict)
	default:
		// UNKNOWN or empty: relay has not run yet. Decision is still final (ENGINE_DURABLE).
		// In v3, this is a normal transient state — replay returns the decided result.
		recordHTTPStage("total", result.Result, "replay_engine_durable", time.Since(totalStart))
		return result.response(auction.DurabilityStatusEngineDurable, auction.DecisionStatusDecided), nil
	}
}

func (e *Engine) loadRedisIdempotencyReplay(ctx context.Context, auctionID string, clientBidID string) (redisIdempotencyReplayLoad, error) {
	values, err := e.redis.HGetAll(ctx, redisx.BidEngineIdempotencyKey(auctionID, clientBidID)).Result()
	if err != nil {
		return redisIdempotencyReplayLoad{}, err
	}
	if len(values) == 0 {
		return redisIdempotencyReplayLoad{}, nil
	}
	replay := redisIdempotencyReplay{
		RequestHash:       values["request_hash"],
		ResultJSON:        values["result_json"],
		KafkaAppendStatus: values["kafka_append_status"],
		EngineEpoch:       parseInt64(values["engine_epoch"]),
		EngineSeq:         parseInt64(values["engine_seq"]),
		ExpiresAtMS:       parseInt64(values["expires_at_ms"]),
	}
	if replay.RequestHash == "" || replay.ResultJSON == "" {
		return redisIdempotencyReplayLoad{}, fmt.Errorf("redis engine idempotency record %s is missing request_hash/result_json", redisx.BidEngineIdempotencyKey(auctionID, clientBidID))
	}
	return redisIdempotencyReplayLoad{Record: replay, Found: true}, nil
}

func (e *Engine) redisPreDecisionReplay(ctx context.Context, auctionID string, clientBidID string, requestHash string) (auction.BidResponse, bool, error) {
	load, err := e.loadRedisIdempotencyReplay(ctx, auctionID, clientBidID)
	if err != nil {
		return auction.BidResponse{}, false, err
	}
	if !load.Found {
		return auction.BidResponse{}, false, nil
	}
	replay := load.Record
	if replay.RequestHash != requestHash {
		return auction.BidResponse{}, true, apierrors.New(apierrors.CodeIdempotencyKeyReusedWithDifferentRequest, "idempotency key reused with different request", http.StatusConflict)
	}
	var result engineResult
	if err := json.Unmarshal([]byte(replay.ResultJSON), &result); err != nil {
		return auction.BidResponse{}, true, err
	}
	resp, err := e.redisIdempotencyReplayResponseFromRecord(replay, result, time.Now())
	return resp, true, err
}

type pendingDecision struct {
	seq    int64
	field  string
	result engineResult
}

func nextPendingDecision(ctx context.Context, redisClient *redis.Client, auctionID string, pendingKey string) (pendingDecision, bool, error) {
	decisions, err := pendingDecisions(ctx, redisClient, auctionID, pendingKey, 1)
	if err != nil || len(decisions) == 0 {
		return pendingDecision{}, false, err
	}
	return decisions[0], true, nil
}

func pendingDecisions(ctx context.Context, redisClient *redis.Client, auctionID string, pendingKey string, limit int) ([]pendingDecision, error) {
	entries, err := redisClient.HGetAll(ctx, pendingKey).Result()
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	fields := make([]string, 0, len(entries))
	for field := range entries {
		fields = append(fields, field)
	}
	sort.Slice(fields, func(i, j int) bool {
		left, leftErr := strconv.ParseInt(fields[i], 10, 64)
		right, rightErr := strconv.ParseInt(fields[j], 10, 64)
		if leftErr != nil || rightErr != nil {
			return fields[i] < fields[j]
		}
		return left < right
	})
	if limit > 0 && len(fields) > limit {
		fields = fields[:limit]
	}
	decisions := make([]pendingDecision, 0, len(fields))
	for _, field := range fields {
		seq, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid pending redis engine seq %q: %w", field, err)
		}
		var result engineResult
		if err := json.Unmarshal([]byte(entries[field]), &result); err != nil {
			return nil, err
		}
		if result.AuctionID != auctionID {
			return nil, fmt.Errorf("pending redis engine auction mismatch key=%s payload=%s", auctionID, result.AuctionID)
		}
		decisions = append(decisions, pendingDecision{seq: seq, field: field, result: result})
	}
	return decisions, nil
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

func (e *Engine) loadSnapshotSingleflight(ctx context.Context, auctionID string) (snapshot, error) {
	value, err, _ := e.snapshotLoads.Do(auctionID, func() (any, error) {
		return e.loadSnapshot(ctx, auctionID)
	})
	if err != nil {
		return snapshot{}, err
	}
	snap, ok := value.(snapshot)
	if !ok {
		return snapshot{}, fmt.Errorf("redis ledger engine snapshot singleflight returned %T", value)
	}
	return snap, nil
}

func (e *Engine) redisAppendHighWater(ctx context.Context, auctionID string) (int64, error) {
	markerSeq, err := e.redis.HGet(ctx, redisx.BidEngineAppendMarkerKey(auctionID), "engine_seq").Int64()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
	}
	statsSeq, err := e.redis.HGet(ctx, redisx.BidEngineAppendStatsKey(auctionID), "last_engine_seq").Int64()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
	}
	if statsSeq > markerSeq {
		return statsSeq, nil
	}
	return markerSeq, nil
}

func (e *Engine) ensureColdSnapshotCanSeedRedis(ctx context.Context, auctionID string, snap snapshot) error {
	if snap.EngineSeq <= 0 {
		return nil
	}
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var pendingSettlements int64
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM redis_engine_settlements
		WHERE auction_id = $1
		  AND status NOT IN ('SETTLED','SKIPPED')
	`, auctionID).Scan(&pendingSettlements); err != nil {
		return err
	}
	if pendingSettlements > 0 {
		return fmt.Errorf("redis state missing while %d settlement decisions are not terminal", pendingSettlements)
	}

	var settlementCount int64
	var settledSeq sql.NullInt64
	if err := tx.QueryRow(ctx, `
		SELECT count(*), max(engine_seq)
		FROM redis_engine_settlements
		WHERE auction_id = $1
		  AND engine_epoch = $2
		  AND status IN ('SETTLED','SKIPPED')
	`, auctionID, snap.EngineEpoch).Scan(&settlementCount, &settledSeq); err != nil {
		return err
	}
	if settlementCount == 0 {
		return tx.Commit(ctx)
	}
	if !settledSeq.Valid || settledSeq.Int64 != snap.EngineSeq {
		return fmt.Errorf("redis state missing but settled ledger seq %d does not cover postgres engine seq %d", settledSeq.Int64, snap.EngineSeq)
	}

	var checkpointEpoch int64
	var checkpointSeq int64
	var storedHash string
	var snapshotText string
	err = tx.QueryRow(ctx, `
		SELECT engine_epoch, engine_seq, state_hash, snapshot_json::text
		FROM auction_engine_checkpoints
		WHERE auction_id = $1
	`, auctionID).Scan(&checkpointEpoch, &checkpointSeq, &storedHash, &snapshotText)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("redis state missing after settled ledger decisions without checkpoint")
	}
	if err != nil {
		return err
	}
	if checkpointEpoch != snap.EngineEpoch || checkpointSeq != snap.EngineSeq {
		return fmt.Errorf("redis state missing but checkpoint epoch/seq %d/%d does not cover postgres epoch/seq %d/%d", checkpointEpoch, checkpointSeq, snap.EngineEpoch, snap.EngineSeq)
	}
	var storedSnapshot engineCheckpointSnapshot
	if err := json.Unmarshal([]byte(snapshotText), &storedSnapshot); err != nil {
		return fmt.Errorf("redis state missing and checkpoint snapshot is invalid: %w", err)
	}
	storedPayload, err := json.Marshal(storedSnapshot)
	if err != nil {
		return err
	}
	_, payload, stateHash, err := checkpointSnapshot(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	if storedHash != sha256Hex(storedPayload) || storedHash != stateHash || sha256Hex(payload) != stateHash {
		return fmt.Errorf("redis state missing but checkpoint hash does not match current postgres state")
	}
	return tx.Commit(ctx)
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
		       a.seq, a.engine_seq, a.engine_epoch, a.engine_paused,
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
	`, auctionID).Scan(&s.Status, &s.CurrentPriceCents, &winner, &s.StartPriceCents, &s.IncrementCents, &capPrice, &endAt, &s.ExtendWindowMS, &s.ExtendByMS, &s.MaxExtendCount, &s.ExtendCount, &s.AcceptedBidCount, &s.Seq, &s.EngineSeq, &s.EngineEpoch, &s.Paused, &s.PauseReason, &s.RequiresPostgres)
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
	observability.Inc("auction_bid_engine_pause_total", map[string]string{"reason": reason})
	return nil
}

func (r engineResult) response(durabilityStatus string, decisionStatus string) auction.BidResponse {
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
	if decisionStatus == "" {
		decisionStatus = auction.DecisionStatusDecided
	}
	if durabilityStatus == "" {
		durabilityStatus = auction.DurabilityStatusKafkaAcked
	}
	var basis *auction.BidDecisionBasis
	if r.DecisionBasis.RequiredMinPriceCents > 0 || r.DecisionBasis.CurrentPriceCents > 0 || r.DecisionBasis.PreviousPriceCents > 0 {
		basis = &auction.BidDecisionBasis{
			PreviousPriceCents:    r.DecisionBasis.PreviousPriceCents,
			RequiredMinPriceCents: r.DecisionBasis.RequiredMinPriceCents,
			CurrentPriceCents:     r.DecisionBasis.CurrentPriceCents,
			Reason:                r.DecisionBasis.Reason,
			EngineSeq:             r.DecisionBasis.EngineSeq,
		}
		if basis.EngineSeq == 0 {
			basis.EngineSeq = r.EngineSeq
		}
	}
	return auction.BidResponse{
		Result:            result,
		BidID:             r.BidID,
		AuctionID:         r.AuctionID,
		Seq:               r.Seq,
		EngineSeq:         r.EngineSeq,
		EngineEpoch:       r.EngineEpoch,
		DecisionStatus:    decisionStatus,
		DurabilityStatus:  durabilityStatus,
		SettlementStatus:  auction.SettlementStatusPending,
		DecisionBasis:     basis,
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

func parseInt64(value string) int64 {
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func isUnknownKafkaAppendFailure(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func recordDecision(result string, elapsed time.Duration) {
	observability.Inc("auction_bid_redis_ledger_total", map[string]string{"result": result})
	observability.Observe("auction_bid_redis_ledger_seconds", elapsed.Seconds(), map[string]string{"result": result}, observability.DefaultLatencyBuckets)
}

func recordKafkaAppend(result string, status string, elapsed time.Duration) {
	observability.Inc("auction_bid_kafka_append_total", map[string]string{"result": result, "status": status})
	observability.Observe("auction_bid_kafka_append_seconds", elapsed.Seconds(), map[string]string{"result": result, "status": status}, observability.DefaultLatencyBuckets)
}

func recordHTTPStage(stage string, result string, status string, elapsed time.Duration) {
	observability.Observe("auction_bid_http_stage_seconds", elapsed.Seconds(), map[string]string{"stage": stage, "result": result, "status": status}, observability.DefaultLatencyBuckets)
}

type Worker struct {
	db         *pgxpool.Pool
	redis      *redis.Client
	ledger     BidLedger
	dlqTopic   string
	consumerID string
	batchSize  int64
	block      time.Duration
	log        *slog.Logger
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
	RecoveredPending   int64     `json:"recovered_pending"`
	Paused             bool      `json:"paused"`
	DriftCount         int       `json:"drift_count"`
	Message            string    `json:"message,omitempty"`
}

type reconcileViolation struct {
	status  string
	reason  string
	message string
	details map[string]any
}

type engineCheckpointSnapshot struct {
	AuctionID             string `json:"auction_id"`
	Status                string `json:"status"`
	CurrentPriceCents     int64  `json:"current_price_cents"`
	CurrentWinnerID       string `json:"current_winner_id,omitempty"`
	PublicSeq             int64  `json:"public_seq"`
	EngineEpoch           int64  `json:"engine_epoch"`
	EngineSeq             int64  `json:"engine_seq"`
	AcceptedBidCount      int64  `json:"accepted_bid_count"`
	ExtendCount           int    `json:"extend_count"`
	LastAcceptedBidID     string `json:"last_accepted_bid_id,omitempty"`
	LastAcceptedUserID    string `json:"last_accepted_user_id,omitempty"`
	LastAcceptedAmount    int64  `json:"last_accepted_amount_cents,omitempty"`
	LastAcceptedEngineSeq int64  `json:"last_accepted_engine_seq,omitempty"`
}

type redisEngineResumeReport struct {
	AuctionID        string `json:"auction_id"`
	Resumed          bool   `json:"resumed"`
	Rebuilt          bool   `json:"rebuilt"`
	EngineEpoch      int64  `json:"engine_epoch"`
	EngineSeq        int64  `json:"engine_seq"`
	PublicSeq        int64  `json:"public_seq"`
	CheckpointHash   string `json:"checkpoint_hash,omitempty"`
	RTOms            int64  `json:"rto_ms"`
	PreflightStatus  string `json:"preflight_status"`
	PostflightStatus string `json:"postflight_status"`
}

func NewWorker(db *pgxpool.Pool, redisClient *redis.Client, ledger BidLedger, consumerID string) *Worker {
	if consumerID == "" {
		consumerID = "settlement-" + uuid.NewString()
	}
	return &Worker{db: db, redis: redisClient, ledger: ledger, dlqTopic: ledgerDLQTopic(ledger), consumerID: consumerID, batchSize: 32, block: time.Second}
}

func (w *Worker) WithLogger(log *slog.Logger) *Worker {
	if w != nil {
		w.log = log
	}
	return w
}

func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	log := w.logger()
	log.Info("redis engine worker starting", slog.String("consumer_id", w.consumerID), slog.Duration("interval", interval))
	done := make(chan struct{}, 3)
	go w.runPeriodic(ctx, "control", interval, done, func(loopCtx context.Context) (int, error) {
		if err := w.ProcessSignals(loopCtx, 16); err != nil {
			return 0, err
		}
		return w.ProcessPendingAppends(loopCtx, 100)
	})
	go w.runPeriodic(ctx, "kafka-settlement", 10*time.Millisecond, done, func(loopCtx context.Context) (int, error) {
		if w.ledger == nil {
			return 0, nil
		}
		return w.ProcessKafka(loopCtx, 100)
	})
	go w.runPeriodic(ctx, "reconcile", 30*time.Second, done, func(loopCtx context.Context) (int, error) {
		return w.ProcessReconcile(loopCtx, 20)
	})
	<-ctx.Done()
	log.Info("redis engine worker stopping", slog.String("consumer_id", w.consumerID), slog.String("reason", ctx.Err().Error()))
	for i := 0; i < cap(done); i++ {
		<-done
	}
}

func (w *Worker) runPeriodic(ctx context.Context, name string, interval time.Duration, done chan<- struct{}, fn func(context.Context) (int, error)) {
	defer func() {
		if recovered := recover(); recovered != nil {
			w.logger().Error("redis engine worker loop panicked", slog.String("consumer_id", w.consumerID), slog.String("loop", name), slog.Any("panic", recovered))
		}
		done <- struct{}{}
	}()
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		processed, err := fn(ctx)
		if err != nil {
			w.logger().Warn("redis engine worker loop failed", slog.String("consumer_id", w.consumerID), slog.String("loop", name), slog.Int("processed", processed), slog.String("error", err.Error()))
		} else if processed > 0 {
			w.logger().Info("redis engine worker loop processed", slog.String("consumer_id", w.consumerID), slog.String("loop", name), slog.Int("processed", processed))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessPendingAppends is the group-commit relay loop.
// It discovers auctions that have unrelayed log-stream entries and calls
// relayAuctionLogBatch for each, batch-producing all pending decisions to Kafka
// in a single WriteMessages round-trip per auction. In v3 this replaces the old
// per-decision lock-and-append path entirely.
func (w *Worker) ProcessPendingAppends(ctx context.Context, limit int) (int, error) {
	if w == nil || w.db == nil || w.redis == nil || w.ledger == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 1
	}
	// Collect candidate auctions from the pending-set (set by the Lua on every decision)
	// plus active auctions (in case the pending-set TTL expired).
	auctionIDs, err := w.pendingAuctionIDs(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(auctionIDs) < limit {
		activeIDs, err := w.activeAuctionIDs(ctx, limit)
		if err != nil {
			return 0, err
		}
		seen := make(map[string]struct{}, len(auctionIDs)+len(activeIDs))
		for _, id := range auctionIDs {
			seen[id] = struct{}{}
		}
		for _, id := range activeIDs {
			if _, ok := seen[id]; !ok {
				auctionIDs = append(auctionIDs, id)
				seen[id] = struct{}{}
			}
			if len(auctionIDs) >= limit {
				break
			}
		}
	}
	processed := 0
	var firstErr error
	for _, auctionID := range auctionIDs {
		n, err := w.relayAuctionLogBatch(ctx, auctionID)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("relay batch auction=%s: %w", auctionID, err)
			}
			continue
		}
		processed += n
	}
	return processed, firstErr
}

func (w *Worker) pendingAuctionIDs(ctx context.Context, limit int) ([]string, error) {
	if w == nil || w.redis == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 1
	}
	values, err := w.redis.SRandMemberN(ctx, redisx.BidEnginePendingAuctionsKey(), int64(limit)).Result()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, id := range values {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

// relayAuctionLogBatch reads a batch of decisions from the auction's Redis Stream
// (bid:{auction}:engine:log) and batch-produces them to Kafka in a single round-trip
// (group commit). This is the durable-relay path in v3 — it runs off the hot path.
//
// After a successful Kafka batch-write, it marks each idem key as KAFKA_ACKED and
// advances the relay cursor. If Kafka is unavailable, the stream retains all entries
// and the relay retries on the next tick without data loss.
func (w *Worker) relayAuctionLogBatch(ctx context.Context, auctionID string) (int, error) {
	if w == nil || w.redis == nil || w.ledger == nil {
		return 0, nil
	}
	streamKey := redisx.BidEngineLogStreamKey(auctionID)
	cursorKey := redisx.BidEngineRelayCursorKey(auctionID)

	// Get last relayed stream entry ID (0-0 means start of stream).
	cursor, err := w.redis.Get(ctx, cursorKey).Result()
	if errors.Is(err, redis.Nil) {
		cursor = "0-0"
	} else if err != nil {
		return 0, err
	}

	// Read a batch of new entries from the stream.
	streams, err := w.redis.XRead(ctx, &redis.XReadArgs{
		Streams: []string{streamKey, cursor},
		Count:   relayBatchSize,
		Block:   relayBatchBlock,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return 0, nil
	}
	messages := streams[0].Messages

	// Decode each stream entry's payload field into engineResult.
	var results []engineResult
	var streamIDs []string
	for _, msg := range messages {
		payload, ok := msg.Values["payload"].(string)
		if !ok || payload == "" {
			continue
		}
		var result engineResult
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			w.logger().Warn("relay: skipping malformed stream entry",
				slog.String("auction_id", auctionID),
				slog.String("stream_id", msg.ID),
				slog.String("error", err.Error()),
			)
			continue
		}
		results = append(results, result)
		streamIDs = append(streamIDs, msg.ID)
	}
	if len(results) == 0 {
		return 0, nil
	}

	// Single batch produce to Kafka — this is the group-commit.
	// All results in the batch are durable once WriteMessages returns without error.
	ledgerMsgs, err := w.ledger.AppendBatch(ctx, results)
	if err != nil {
		observability.Inc("auction_bid_relay_kafka_batch_fail_total", map[string]string{"auction_id": auctionID})
		return 0, fmt.Errorf("relay batch produce auction=%s batch=%d: %w", auctionID, len(results), err)
	}

	observability.Inc("auction_bid_relay_kafka_batch_total", map[string]string{"auction_id": auctionID})
	observability.Observe("auction_bid_relay_batch_size", float64(len(results)), map[string]string{"auction_id": auctionID}, observability.DefaultLatencyBuckets)

	// Mark each idem key as KAFKA_ACKED using a pipeline (one network RTT for the batch).
	pipe := w.redis.Pipeline()
	now := time.Now().UTC()
	expiresAtMS := now.Add(idempotencyTTL).UnixMilli()
	for i, result := range results {
		idemKey := redisx.BidEngineIdempotencyKey(auctionID, result.ClientBidID)
		lm := ledgerMsgs[i]
		fields := []any{
			"kafka_append_status", kafkaAppendStatusAcked,
			"kafka_topic", lm.Topic,
			"expires_at_ms", expiresAtMS,
		}
		if lm.Partition >= 0 && lm.Offset >= 0 {
			fields = append(fields, "kafka_partition", lm.Partition, "kafka_offset", lm.Offset)
		}
		pipe.HSet(ctx, idemKey, fields...)
		pipe.HDel(ctx, redisx.BidEnginePendingKey(auctionID), strconv.FormatInt(result.EngineSeq, 10))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		// Non-fatal: Kafka already accepted the batch. idem keys will eventually be
		// re-acked on the next relay pass (idempotent Kafka produce + stream cursor).
		w.logger().Warn("relay: idem ack pipeline failed (non-fatal)", slog.String("auction_id", auctionID), slog.String("error", err.Error()))
	}

	// Advance relay cursor to the last processed stream entry ID.
	lastID := streamIDs[len(streamIDs)-1]
	if err := w.redis.Set(ctx, cursorKey, lastID, engineStateTTL).Err(); err != nil {
		// Non-fatal: worst case the relay re-processes this batch next tick.
		// AppendBatch with idempotent producer makes duplicate Kafka records harmless.
		w.logger().Warn("relay: cursor advance failed (non-fatal)", slog.String("auction_id", auctionID), slog.String("last_id", lastID), slog.String("error", err.Error()))
	}

	return len(results), nil
}

func (w *Worker) logger() *slog.Logger {
	if w != nil && w.log != nil {
		return w.log
	}
	return slog.Default()
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
		fetchCtx, cancel := context.WithTimeout(ctx, kafkaFetchTimeout)
		message, err := w.ledger.Fetch(fetchCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return processed, nil
			}
			return processed, err
		}
		if err := w.settleLedgerMessage(ctx, message); err != nil {
			if isSettlementIdentityConflictError(err) {
				if err := w.ledger.Commit(ctx, message); err != nil {
					return processed, err
				}
				processed++
				continue
			}
			if isTransientSettlementError(err) {
				return processed, err
			}
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

// recoverPendingDecisions drains unrelayed stream entries to Kafka.
// Called by the reconciler when it detects a stream lag.
func (w *Worker) recoverPendingDecisions(ctx context.Context, auctionID string) (int64, error) {
	if w == nil || w.redis == nil || w.ledger == nil {
		return 0, nil
	}
	total := int64(0)
	for i := 0; i < reconcilePendingDrainLimit; i++ {
		n, err := w.relayAuctionLogBatch(ctx, auctionID)
		if err != nil {
			return total, err
		}
		total += int64(n)
		if n == 0 {
			break
		}
	}
	return total, nil
}

// pendingDecisionCount returns the number of decisions in the log stream that
// have not yet been relay-acknowledged (i.e. stream entries after relay cursor).
func (w *Worker) pendingDecisionCount(ctx context.Context, auctionID string) (int64, error) {
	if w == nil || w.redis == nil {
		return 0, nil
	}
	streamKey := redisx.BidEngineLogStreamKey(auctionID)
	cursorKey := redisx.BidEngineRelayCursorKey(auctionID)

	cursor, err := w.redis.Get(ctx, cursorKey).Result()
	if errors.Is(err, redis.Nil) {
		cursor = "0-0"
	} else if err != nil {
		return 0, err
	}
	// Count entries with ID > cursor. XRANGE with + gives all; we want count after cursor.
	msgs, err := w.redis.XRangeN(ctx, streamKey, "("+cursor, "+", int64(reconcilePendingDrainLimit)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int64(len(msgs)), nil
}

// pendingAppendInProgress returns true if there are unrelayed stream entries.
func (w *Worker) pendingAppendInProgress(ctx context.Context, auctionID string) (bool, error) {
	n, err := w.pendingDecisionCount(ctx, auctionID)
	if err != nil {
		return false, err
	}
	return n > 0, nil
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
		report, err := w.resumeRedisEngine(ctx, auctionID)
		payload, _ := json.Marshal(report)
		var out map[string]any
		_ = json.Unmarshal(payload, &out)
		return out, err
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

func (w *Worker) resumeRedisEngine(ctx context.Context, auctionID string) (redisEngineResumeReport, error) {
	started := time.Now()
	report := redisEngineResumeReport{AuctionID: auctionID}
	preflight, err := w.Reconcile(ctx, auctionID)
	if err != nil {
		return report, err
	}
	report.PreflightStatus = preflight.Status
	if preflight.Status != "OK" && preflight.Status != "REDIS_STATE_MISSING" && preflight.Status != "REDIS_BEHIND_DB" {
		return report, fmt.Errorf("redis engine resume preflight failed: %s", preflight.Status)
	}
	if err := w.rebuildRedisFromCheckpoint(ctx, auctionID, &report); err != nil {
		return report, err
	}
	postflight, err := w.Reconcile(ctx, auctionID)
	if err != nil {
		return report, err
	}
	report.PostflightStatus = postflight.Status
	if postflight.Status != "OK" {
		return report, fmt.Errorf("redis engine resume postflight failed: %s", postflight.Status)
	}
	if _, err := w.db.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = false,
		    engine_pause_reason = NULL,
		    engine_paused_at = NULL,
		    updated_at = now()
		WHERE id = $1
	`, auctionID); err != nil {
		return report, err
	}
	if w.redis != nil {
		_ = w.redis.HSet(ctx, redisx.BidEngineStateKey(auctionID), "paused", 0, "pause_reason", "").Err()
	}
	report.RTOms = time.Since(started).Milliseconds()
	report.Resumed = true
	observability.Observe("auction_bid_engine_resume_rto_seconds", time.Since(started).Seconds(), map[string]string{"status": "ok"}, observability.DefaultLatencyBuckets)
	return report, nil
}

func (w *Worker) rebuildRedisFromCheckpoint(ctx context.Context, auctionID string, report *redisEngineResumeReport) error {
	if w == nil || w.redis == nil {
		return nil
	}
	var checkpointEpoch int64
	var checkpointSeq int64
	var storedHash string
	var snapshotText string
	err := w.db.QueryRow(ctx, `
		SELECT engine_epoch, engine_seq, state_hash, snapshot_json::text
		FROM auction_engine_checkpoints
		WHERE auction_id = $1
	`, auctionID).Scan(&checkpointEpoch, &checkpointSeq, &storedHash, &snapshotText)
	if errors.Is(err, pgx.ErrNoRows) {
		var dbSeq int64
		if scanErr := w.db.QueryRow(ctx, `SELECT engine_seq FROM auctions WHERE id = $1`, auctionID).Scan(&dbSeq); scanErr != nil {
			return scanErr
		}
		if dbSeq > 0 {
			return fmt.Errorf("redis engine checkpoint missing for auction=%s engine_seq=%d", auctionID, dbSeq)
		}
		snap, err := loadSnapshotForRedisState(ctx, w.db, auctionID)
		if err != nil {
			return err
		}
		return w.writeRedisStateSnapshot(ctx, auctionID, snap, report, "")
	}
	if err != nil {
		return err
	}
	var checkpoint engineCheckpointSnapshot
	if err := json.Unmarshal([]byte(snapshotText), &checkpoint); err != nil {
		return err
	}
	payload, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	if sha256Hex(payload) != storedHash {
		return fmt.Errorf("redis engine checkpoint hash mismatch auction=%s", auctionID)
	}
	snap, err := loadSnapshotForRedisState(ctx, w.db, auctionID)
	if err != nil {
		return err
	}
	if snap.EngineEpoch != checkpointEpoch || snap.EngineSeq != checkpointSeq || snap.EngineSeq != checkpoint.EngineSeq || snap.Seq != checkpoint.PublicSeq || snap.CurrentPriceCents != checkpoint.CurrentPriceCents || snap.CurrentWinnerID != checkpoint.CurrentWinnerID || snap.Status != checkpoint.Status {
		return fmt.Errorf("redis engine checkpoint does not match PostgreSQL settlement auction=%s", auctionID)
	}
	return w.writeRedisStateSnapshot(ctx, auctionID, snap, report, storedHash)
}

func (w *Worker) writeRedisStateSnapshot(ctx context.Context, auctionID string, snap snapshot, report *redisEngineResumeReport, stateHash string) error {
	fields := []any{
		"status", snap.Status,
		"current_price_cents", snap.CurrentPriceCents,
		"current_winner_id", snap.CurrentWinnerID,
		"start_price_cents", snap.StartPriceCents,
		"increment_cents", snap.IncrementCents,
		"cap_price_cents", snap.CapPriceCents,
		"end_at_ms", snap.EndAtMS,
		"absolute_end_ms", snap.AbsoluteEndMS, // hard ceiling restored on rebuild
		"extend_window_ms", snap.ExtendWindowMS,
		"extend_by_ms", snap.ExtendByMS,
		"max_extend_count", snap.MaxExtendCount,
		"extend_count", snap.ExtendCount,
		"accepted_bid_count", snap.AcceptedBidCount,
		"seq", snap.Seq,
		"engine_seq", snap.EngineSeq,
		"engine_epoch", snap.EngineEpoch,
		"paused", boolInt(snap.Paused),
		"pause_reason", snap.PauseReason,
		"requires_postgres", snap.RequiresPostgres,
	}
	pipe := w.redis.TxPipeline()
	pipe.HSet(ctx, redisx.BidEngineStateKey(auctionID), fields...)
	pipe.PExpire(ctx, redisx.BidEngineStateKey(auctionID), engineStateTTL)
	// Reset the relay cursor so the relay re-processes from the stream beginning on resume.
	// This is safe: AppendBatch is idempotent (Kafka idempotent producer).
	pipe.Del(ctx, redisx.BidEngineRelayCursorKey(auctionID))
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	if report != nil {
		report.Rebuilt = true
		report.EngineEpoch = snap.EngineEpoch
		report.EngineSeq = snap.EngineSeq
		report.PublicSeq = snap.Seq
		report.CheckpointHash = stateHash
	}
	return nil
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
	if err := tx.QueryRow(ctx, `
		SELECT engine_epoch, engine_seq
		FROM auctions
		WHERE id = $1
		FOR UPDATE
	`, auctionID).Scan(&dbEpoch, &dbSeq); err != nil {
		return err
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
	if result.EngineSeq > dbSeq+1 {
		_ = markSettlementDelayed(ctx, tx, auctionID, ledgerID, fmt.Sprintf("engine seq waiting for predecessor redis=%d db_next=%d", result.EngineSeq, dbSeq+1))
		_ = tx.Commit(ctx)
		return transientSettlementError{err: fmt.Errorf("engine seq waiting for predecessor auction=%s redis=%d db_next=%d", auctionID, result.EngineSeq, dbSeq+1)}
	}
	if result.EngineSeq <= dbSeq {
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
		_, err = settleAccepted(ctx, tx, result)
		if err != nil {
			_ = markSettlementFailed(ctx, tx, auctionID, ledgerID, err.Error())
			if isPermanentSettlementError(err) || isSettlementIdentityConflictError(err) {
				_ = pauseTx(ctx, tx, auctionID, "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT", err.Error(), map[string]any{
					"ledger_id":     ledgerID,
					"engine_epoch":  result.EngineEpoch,
					"engine_seq":    result.EngineSeq,
					"client_bid_id": result.ClientBidID,
					"request_hash":  result.RequestHash,
				})
				_ = tx.Commit(ctx)
			}
			return err
		}
	case resultRejected:
		_, err = settleRejected(ctx, tx, result)
		if err != nil {
			_ = markSettlementFailed(ctx, tx, auctionID, ledgerID, err.Error())
			if isPermanentSettlementError(err) || isSettlementIdentityConflictError(err) {
				_ = pauseTx(ctx, tx, auctionID, "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT", err.Error(), map[string]any{
					"ledger_id":     ledgerID,
					"engine_epoch":  result.EngineEpoch,
					"engine_seq":    result.EngineSeq,
					"client_bid_id": result.ClientBidID,
					"request_hash":  result.RequestHash,
				})
				_ = tx.Commit(ctx)
			}
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
	if err := upsertEngineCheckpoint(ctx, tx, auctionID, message); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// v3: settlement NEVER writes back to Redis live state.
	// Redis hot state is the single writer — only the Lua CAS advances it.
	// Feeding settled PG state back would rewind engine_seq and corrupt decisions.
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
	payloadHash := sha256Hex(payload)
	ledgerSource := "redis_stream"
	if message.Topic != "" && message.Partition >= 0 && message.Offset >= 0 {
		ledgerSource = "kafka"
	}
	var attempt settlementAttempt
	err = w.db.QueryRow(ctx, `
		INSERT INTO redis_engine_settlements (
		  auction_id, stream_id, engine_epoch, engine_seq, result, status, attempts, payload_json,
		  payload_sha256, ledger_source, ledger_topic, ledger_partition, ledger_offset, ledger_key
		)
		VALUES ($1, $2, $3, $4, $5, 'PROCESSING', 1, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (auction_id, stream_id) DO UPDATE
		SET attempts = CASE
	      WHEN redis_engine_settlements.status IN ('SETTLED','SKIPPED') THEN redis_engine_settlements.attempts
	      ELSE redis_engine_settlements.attempts + 1
	    END,
		    status = CASE
		      WHEN redis_engine_settlements.status IN ('SETTLED','SKIPPED') THEN redis_engine_settlements.status
	      ELSE 'PROCESSING'
	    END,
		    last_error = CASE
	      WHEN redis_engine_settlements.payload_sha256 <> EXCLUDED.payload_sha256 THEN 'stream payload hash changed for existing settlement'
	      ELSE redis_engine_settlements.last_error
	    END,
		    updated_at = now()
		WHERE redis_engine_settlements.payload_sha256 = EXCLUDED.payload_sha256
		RETURNING attempts, status
	`, auctionID, streamID, result.EngineEpoch, result.EngineSeq, result.Result, payload, payloadHash, ledgerSource, message.Topic, message.Partition, message.Offset, message.Key).Scan(&attempt.attempts, &attempt.status)
	if isUniqueViolation(err) {
		return w.existingSettlementAttempt(ctx, auctionID, streamID, result, message, payloadHash)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return settlementAttempt{}, w.markSettlementIdentityConflict(ctx, auctionID, result, streamID, message, payloadHash, "same stream id has different payload")
	}
	return attempt, err
}

func (w *Worker) existingSettlementAttempt(ctx context.Context, auctionID string, streamID string, result engineResult, message LedgerMessage, payloadHash string) (settlementAttempt, error) {
	var attempt settlementAttempt
	var existingStreamID string
	var existingRequestHash string
	var existingPayloadHash string
	var conflictScope string
	err := w.db.QueryRow(ctx, `
		SELECT stream_id, attempts, status, COALESCE(payload_json->>'request_hash', ''), payload_sha256, conflict_scope
		FROM (
		  SELECT stream_id, attempts, status, payload_json, payload_sha256, 'stream_id' AS conflict_scope, 1 AS priority
		  FROM redis_engine_settlements
		  WHERE auction_id = $1 AND stream_id = $4
		  UNION ALL
		  SELECT stream_id, attempts, status, payload_json, payload_sha256, 'kafka_offset' AS conflict_scope, 2 AS priority
		  FROM redis_engine_settlements
		  WHERE ledger_source = 'kafka'
		    AND ledger_topic = $5
		    AND ledger_partition = $6
		    AND ledger_offset = $7
		    AND $5 <> ''
		    AND $6 >= 0
		    AND $7 >= 0
		  UNION ALL
		  SELECT stream_id, attempts, status, payload_json, payload_sha256, 'engine_seq' AS conflict_scope, 3 AS priority
		  FROM redis_engine_settlements
		  WHERE auction_id = $1 AND engine_epoch = $2 AND engine_seq = $3
		) matches
		ORDER BY priority
		LIMIT 1
	`, auctionID, result.EngineEpoch, result.EngineSeq, streamID, message.Topic, message.Partition, message.Offset).Scan(&existingStreamID, &attempt.attempts, &attempt.status, &existingRequestHash, &existingPayloadHash, &conflictScope)
	if err != nil {
		return settlementAttempt{}, err
	}
	if existingPayloadHash == payloadHash {
		if attempt.status == "SETTLED" || attempt.status == "SKIPPED" {
			return attempt, nil
		}
		return attempt, transientSettlementError{err: fmt.Errorf("duplicate settlement waiting for first attempt auction=%s epoch=%d seq=%d existing_stream=%s new_stream=%s status=%s", auctionID, result.EngineEpoch, result.EngineSeq, existingStreamID, streamID, attempt.status)}
	}
	reason := "engine seq payload hash conflict"
	if conflictScope == "stream_id" {
		reason = "stream payload hash conflict"
	}
	if conflictScope == "kafka_offset" {
		reason = "kafka offset payload hash conflict"
	}
	if existingRequestHash != "" && existingRequestHash != result.RequestHash {
		reason = conflictScope + " request hash conflict"
	}
	return settlementAttempt{}, w.markSettlementIdentityConflict(ctx, auctionID, result, streamID, message, payloadHash, reason)
}

func (w *Worker) markSettlementIdentityConflict(ctx context.Context, auctionID string, result engineResult, streamID string, message LedgerMessage, payloadHash string, reason string) error {
	tx, err := w.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existingStreamID string
	var existingPayloadHash string
	var existingStatus string
	err = tx.QueryRow(ctx, `
		WITH target AS (
		  SELECT stream_id, 1 AS priority
		  FROM redis_engine_settlements
		  WHERE auction_id = $1 AND stream_id = $4
		  UNION ALL
		  SELECT stream_id, 2 AS priority
		  FROM redis_engine_settlements
		  WHERE ledger_source = 'kafka'
		    AND ledger_topic = $5
		    AND ledger_partition = $6
		    AND ledger_offset = $7
		    AND $5 <> ''
		    AND $6 >= 0
		    AND $7 >= 0
		  UNION ALL
		  SELECT stream_id, 3 AS priority
		  FROM redis_engine_settlements
		  WHERE auction_id = $1 AND engine_epoch = $2 AND engine_seq = $3
		  ORDER BY priority
		  LIMIT 1
		)
		SELECT s.stream_id, s.payload_sha256, s.status
		FROM redis_engine_settlements s
		JOIN target t ON t.stream_id = s.stream_id
		FOR UPDATE
	`, auctionID, result.EngineEpoch, result.EngineSeq, streamID, message.Topic, message.Partition, message.Offset).Scan(&existingStreamID, &existingPayloadHash, &existingStatus)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE redis_engine_settlements
		SET status = 'FAILED',
		    last_error = $3,
		    conflict_stream_id = $4,
		    conflict_payload_sha256 = $5,
		    updated_at = now()
		WHERE auction_id = $1 AND stream_id = $2
	`, auctionID, existingStreamID, reason, streamID, payloadHash); err != nil {
		return err
	}
	if err := pauseTx(ctx, tx, auctionID, "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT", reason, map[string]any{
		"existing_stream_id":           existingStreamID,
		"existing_payload_sha256":      existingPayloadHash,
		"existing_status":              existingStatus,
		"conflicting_stream_id":        streamID,
		"conflicting_payload_sha256":   payloadHash,
		"conflicting_ledger_topic":     message.Topic,
		"conflicting_ledger_partition": message.Partition,
		"conflicting_ledger_offset":    message.Offset,
		"auction_id":                   auctionID,
		"engine_epoch":                 result.EngineEpoch,
		"engine_seq":                   result.EngineSeq,
		"client_bid_id":                result.ClientBidID,
		"request_hash":                 result.RequestHash,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if w.redis != nil {
		_ = w.redis.HSet(ctx, redisx.BidEngineStateKey(auctionID), "paused", 1, "pause_reason", "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT").Err()
	}
	observability.Inc("auction_bid_engine_pause_total", map[string]string{"reason": "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT"})
	return settlementIdentityConflictError{err: fmt.Errorf("%s auction=%s epoch=%d seq=%d existing_stream=%s new_stream=%s", reason, auctionID, result.EngineEpoch, result.EngineSeq, existingStreamID, streamID)}
}

func loadSnapshotForRedisState(ctx context.Context, db *pgxpool.Pool, auctionID string) (snapshot, error) {
	var s snapshot
	var winner *string
	var capPrice *int64
	var endAt time.Time
	err := db.QueryRow(ctx, `
		SELECT a.status, a.current_price_cents, a.current_winner_id,
		       a.start_price_cents, a.increment_cents, a.cap_price_cents,
		       a.end_at, ar.extend_window_seconds, ar.extend_by_seconds,
		       ar.max_extend_count, a.extend_count, a.accepted_bid_count,
		       a.seq, a.engine_seq, a.engine_epoch, a.engine_paused,
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
	`, auctionID).Scan(&s.Status, &s.CurrentPriceCents, &winner, &s.StartPriceCents, &s.IncrementCents, &capPrice, &endAt, &s.ExtendWindowMS, &s.ExtendByMS, &s.MaxExtendCount, &s.ExtendCount, &s.AcceptedBidCount, &s.Seq, &s.EngineSeq, &s.EngineEpoch, &s.Paused, &s.PauseReason, &s.RequiresPostgres)
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
	// Compute the hard ceiling for soft-close extensions.
	// This is the total possible extent from the current settled state:
	// current_end + remaining_extensions * extend_by.
	// After a checkpoint rebuild this correctly gives original_end + max_total_extension.
	remaining := int64(s.MaxExtendCount-s.ExtendCount) * s.ExtendByMS
	if remaining < 0 {
		remaining = 0
	}
	s.AbsoluteEndMS = s.EndAtMS + remaining + int64(s.ExtendCount)*s.ExtendByMS
	return s, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func settleAccepted(ctx context.Context, tx pgx.Tx, result engineResult) (int64, error) {
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
	var publicSeq int64
	err := tx.QueryRow(ctx, `
		UPDATE auctions
		SET status = $2,
		    current_price_cents = $3,
		    current_winner_id = $4,
		    end_at = COALESCE($5, end_at),
		    extend_count = GREATEST(extend_count, $8),
		    accepted_bid_count = accepted_bid_count + 1,
		    seq = seq + 1,
		    engine_seq = $6,
		    engine_epoch = $7,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1 AND engine_epoch = $7 AND engine_seq = $6 - 1
		RETURNING seq
	`, result.AuctionID, newStatus, result.AmountCents, result.UserID, endAt, result.EngineSeq, result.EngineEpoch, result.ExtendCount).Scan(&publicSeq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("accepted settlement fenced out auction=%s epoch=%d seq=%d", result.AuctionID, result.EngineEpoch, result.EngineSeq)
	}
	if err != nil {
		return 0, err
	}
	if publicSeq <= 0 {
		return 0, fmt.Errorf("accepted settlement returned invalid public seq auction=%s epoch=%d seq=%d public_seq=%d", result.AuctionID, result.EngineEpoch, result.EngineSeq, publicSeq)
	}
	reason := (*string)(nil)
	resp := result.response(auction.DurabilityStatusKafkaAcked, auction.DecisionStatusDecided)
	resp.Result = responseResult
	resp.SettlementStatus = auction.SettlementStatusSettled
	resp.Seq = publicSeq
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return 0, err
	}
	if err := insertBid(ctx, tx, result, &publicSeq, "ACCEPTED", reason, respJSON); err != nil {
		return 0, err
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
		"decision_status":     auction.DecisionStatusDecided,
		"durability_status":   auction.DurabilityStatusKafkaAcked,
		"decision_basis":      result.DecisionBasis,
	}
	if result.Result == resultSold {
		orderID, err := createOrder(ctx, tx, result.AuctionID, result.UserID, result.AmountCents)
		if err != nil {
			return 0, err
		}
		payload["order_id"] = orderID
	}
	if err := appendEvent(ctx, tx, result.AuctionID, publicSeq, result.EngineEpoch, result.EngineSeq, eventType, result.TraceID, payload); err != nil {
		return 0, err
	}
	return publicSeq, completeIdem(ctx, tx, result, responseResult, respJSON)
}

func settleRejected(ctx context.Context, tx pgx.Tx, result engineResult) (int64, error) {
	broadcastReject := shouldBroadcastReject(result.RejectReason)
	var publicSeq int64
	err := tx.QueryRow(ctx, `
		UPDATE auctions
		SET engine_seq = $2,
		    engine_epoch = $3,
		    seq = CASE WHEN $4 THEN seq + 1 ELSE seq END,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1 AND engine_epoch = $3 AND engine_seq = $2 - 1
		RETURNING seq
	`, result.AuctionID, result.EngineSeq, result.EngineEpoch, broadcastReject).Scan(&publicSeq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("rejected settlement fenced out auction=%s epoch=%d seq=%d", result.AuctionID, result.EngineEpoch, result.EngineSeq)
	}
	if err != nil {
		return 0, err
	}
	resp := result.response(auction.DurabilityStatusKafkaAcked, auction.DecisionStatusDecided)
	resp.Result = auction.BidResultRejected
	resp.SettlementStatus = auction.SettlementStatusSettled
	resp.Seq = publicSeq
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return 0, err
	}
	var bidSeq *int64
	if broadcastReject {
		bidSeq = &publicSeq
	}
	if err := insertBid(ctx, tx, result, bidSeq, "REJECTED", result.RejectReason, respJSON); err != nil {
		return 0, err
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
			"decision_status":   auction.DecisionStatusDecided,
			"durability_status": auction.DurabilityStatusKafkaAcked,
			"decision_basis":    result.DecisionBasis,
		}
		if err := appendEvent(ctx, tx, result.AuctionID, publicSeq, result.EngineEpoch, result.EngineSeq, "bid_rejected", result.TraceID, payload); err != nil {
			return 0, err
		}
	}
	return publicSeq, completeIdem(ctx, tx, result, auction.BidResultRejected, respJSON)
}

func insertBid(ctx context.Context, tx pgx.Tx, result engineResult, seq *int64, status string, rejectReason *string, responseJSON []byte) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO bids (
		  id, auction_id, user_id, client_bid_id, amount_cents, seq, status,
		  reject_reason, request_hash, response_json, trace_id, source,
		  engine_epoch, engine_seq, settlement_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'SETTLED')
		ON CONFLICT (auction_id, user_id, client_bid_id) DO UPDATE
		SET response_json = bids.response_json
		WHERE bids.request_hash = EXCLUDED.request_hash
		  AND bids.amount_cents = EXCLUDED.amount_cents
		  AND bids.status = EXCLUDED.status
		  AND COALESCE(bids.engine_epoch, 0) = COALESCE(EXCLUDED.engine_epoch, 0)
		  AND COALESCE(bids.engine_seq, 0) = COALESCE(EXCLUDED.engine_seq, 0)
	`, result.BidID, result.AuctionID, result.UserID, result.ClientBidID, result.AmountCents, seq, status, rejectReason, result.RequestHash, responseJSON, result.TraceID, auction.BidSourceManual, result.EngineEpoch, result.EngineSeq)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return settlementIdentityConflictError{err: fmt.Errorf("bid idempotency conflict auction=%s user=%s client_bid_id=%s request_hash=%s", result.AuctionID, result.UserID, result.ClientBidID, result.RequestHash)}
	}
	return nil
}

func completeIdem(ctx context.Context, tx pgx.Tx, result engineResult, resultCode string, responseJSON []byte) error {
	tag, err := tx.Exec(ctx, `
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
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return settlementIdentityConflictError{err: fmt.Errorf("idempotency record conflict auction=%s user=%s client_bid_id=%s request_hash=%s", result.AuctionID, result.UserID, result.ClientBidID, result.RequestHash)}
	}
	return nil
}

func appendEvent(ctx context.Context, tx pgx.Tx, auctionID string, seq int64, epoch int64, engineSeq int64, eventType string, traceID string, payload map[string]any) error {
	payload["state_version"] = seq
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	serverTimeMS := time.Now().UTC().UnixMilli()
	if _, err := tx.Exec(ctx, `
		INSERT INTO auction_events (auction_id, seq, event_type, payload_json, server_time_ms, trace_id, engine_epoch, engine_seq)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (auction_id, seq) DO NOTHING
	`, auctionID, seq, eventType, payloadJSON, serverTimeMS, traceID, epoch, engineSeq); err != nil {
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

func checkpointSnapshot(ctx context.Context, tx pgx.Tx, auctionID string) (engineCheckpointSnapshot, []byte, string, error) {
	var s engineCheckpointSnapshot
	var winner *string
	var lastBidID *string
	var lastUserID *string
	var lastAmount *int64
	var lastEngineSeq *int64
	err := tx.QueryRow(ctx, `
		SELECT a.id,
		       a.status,
		       a.current_price_cents,
		       a.current_winner_id,
		       a.seq,
		       a.engine_epoch,
		       a.engine_seq,
		       a.accepted_bid_count,
		       a.extend_count,
		       b.id,
		       b.user_id,
		       b.amount_cents,
		       b.engine_seq
		FROM auctions a
		LEFT JOIN LATERAL (
		  SELECT id, user_id, amount_cents, engine_seq
		  FROM bids
		  WHERE auction_id = a.id AND status = 'ACCEPTED'
		  ORDER BY engine_epoch DESC NULLS LAST, engine_seq DESC NULLS LAST, seq DESC NULLS LAST
		  LIMIT 1
		) b ON true
		WHERE a.id = $1
	`, auctionID).Scan(&s.AuctionID, &s.Status, &s.CurrentPriceCents, &winner, &s.PublicSeq, &s.EngineEpoch, &s.EngineSeq, &s.AcceptedBidCount, &s.ExtendCount, &lastBidID, &lastUserID, &lastAmount, &lastEngineSeq)
	if err != nil {
		return engineCheckpointSnapshot{}, nil, "", err
	}
	if winner != nil {
		s.CurrentWinnerID = *winner
	}
	if lastBidID != nil {
		s.LastAcceptedBidID = *lastBidID
	}
	if lastUserID != nil {
		s.LastAcceptedUserID = *lastUserID
	}
	if lastAmount != nil {
		s.LastAcceptedAmount = *lastAmount
	}
	if lastEngineSeq != nil {
		s.LastAcceptedEngineSeq = *lastEngineSeq
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return engineCheckpointSnapshot{}, nil, "", err
	}
	return s, payload, sha256Hex(payload), nil
}

func upsertEngineCheckpoint(ctx context.Context, tx pgx.Tx, auctionID string, message LedgerMessage) error {
	if message.Topic == "" || message.Partition < 0 || message.Offset < 0 {
		return nil
	}
	snapshot, payload, stateHash, err := checkpointSnapshot(ctx, tx, auctionID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO auction_engine_checkpoints (
		  auction_id, engine_epoch, engine_seq, decision_topic, decision_partition,
		  next_decision_offset, state_hash, snapshot_json, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, now())
		ON CONFLICT (auction_id) DO UPDATE
		SET engine_epoch = EXCLUDED.engine_epoch,
		    engine_seq = EXCLUDED.engine_seq,
		    decision_topic = EXCLUDED.decision_topic,
		    decision_partition = EXCLUDED.decision_partition,
		    next_decision_offset = EXCLUDED.next_decision_offset,
		    state_hash = EXCLUDED.state_hash,
		    snapshot_json = EXCLUDED.snapshot_json,
		    updated_at = now()
		WHERE auction_engine_checkpoints.engine_epoch < EXCLUDED.engine_epoch
		   OR (
		     auction_engine_checkpoints.engine_epoch = EXCLUDED.engine_epoch
		     AND auction_engine_checkpoints.engine_seq <= EXCLUDED.engine_seq
		   )
	`, auctionID, snapshot.EngineEpoch, snapshot.EngineSeq, message.Topic, message.Partition, message.Offset+1, stateHash, string(payload))
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

func markSettlementDelayed(ctx context.Context, tx pgx.Tx, auctionID string, streamID string, message string) error {
	_, err := tx.Exec(ctx, `
		UPDATE redis_engine_settlements
		SET status = 'PROCESSING', last_error = $3, updated_at = now()
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

type settlementIdentityConflictError struct {
	err error
}

type transientSettlementError struct {
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

func (e settlementIdentityConflictError) Error() string {
	return e.err.Error()
}

func (e settlementIdentityConflictError) Unwrap() error {
	return e.err
}

func isSettlementIdentityConflictError(err error) bool {
	var target settlementIdentityConflictError
	return errors.As(err, &target)
}

func (e transientSettlementError) Error() string {
	return e.err.Error()
}

func (e transientSettlementError) Unwrap() error {
	return e.err
}

func isTransientSettlementError(err error) bool {
	var target transientSettlementError
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
	observability.Inc("auction_bid_engine_pause_total", map[string]string{"reason": reason})
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

func (w *Worker) checkSettlementTerminal(ctx context.Context, auctionID string) (*reconcileViolation, error) {
	var seq int64
	var status string
	var lastErr string
	err := w.db.QueryRow(ctx, `
		SELECT engine_seq, status, COALESCE(last_error, '')
		FROM redis_engine_settlements
		WHERE auction_id = $1
		  AND status NOT IN ('SETTLED','SKIPPED')
		ORDER BY engine_epoch, engine_seq
		LIMIT 1
	`, auctionID).Scan(&seq, &status, &lastErr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reconcileViolation{
		status:  "KAFKA_LEDGER_SETTLEMENT_NOT_TERMINAL",
		reason:  "KAFKA_LEDGER_SETTLEMENT_NOT_TERMINAL",
		message: "Kafka decision has not reached terminal settlement",
		details: map[string]any{"engine_seq": seq, "settlement_status": status, "last_error": lastErr},
	}, nil
}

func (w *Worker) checkSettlementGapless(ctx context.Context, auctionID string) (*reconcileViolation, error) {
	var prev int64
	var current int64
	err := w.db.QueryRow(ctx, `
		WITH ordered AS (
		  SELECT engine_seq, lag(engine_seq) OVER (ORDER BY engine_seq) AS prev_seq
		  FROM redis_engine_settlements
		  WHERE auction_id = $1 AND engine_epoch = (SELECT engine_epoch FROM auctions WHERE id = $1)
		)
		SELECT COALESCE(prev_seq, 0), engine_seq
		FROM ordered
		WHERE engine_seq <> COALESCE(prev_seq, 0) + 1
		ORDER BY engine_seq
		LIMIT 1
	`, auctionID).Scan(&prev, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reconcileViolation{
		status:  "KAFKA_LEDGER_ENGINE_SEQ_GAP",
		reason:  "KAFKA_LEDGER_ENGINE_SEQ_GAP",
		message: "Kafka decisions that reached settlement table are not engine-seq gapless",
		details: map[string]any{"previous_engine_seq": prev, "current_engine_seq": current},
	}, nil
}

func (w *Worker) checkAcceptedPublicSeqContiguous(ctx context.Context, auctionID string) (*reconcileViolation, error) {
	var prev int64
	var current int64
	err := w.db.QueryRow(ctx, `
		WITH ordered AS (
		  SELECT seq, lag(seq) OVER (ORDER BY seq) AS prev_seq
		  FROM auction_events
		  WHERE auction_id = $1
		)
		SELECT COALESCE(prev_seq, 0), seq
		FROM ordered
		WHERE seq <> COALESCE(prev_seq, 0) + 1
		ORDER BY seq
		LIMIT 1
	`, auctionID).Scan(&prev, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reconcileViolation{
		status:  "ACCEPTED_PUBLIC_SEQ_GAP",
		reason:  "REDIS_ENGINE_ACCEPTED_PUBLIC_SEQ_GAP",
		message: "Auction public event sequence is not contiguous",
		details: map[string]any{"previous_public_seq": prev, "current_public_seq": current},
	}, nil
}

func (w *Worker) checkAuctionWinnerPrice(ctx context.Context, auctionID string) (*reconcileViolation, error) {
	var dbPrice int64
	var dbWinner string
	var wantPrice int64
	var wantWinner string
	var acceptedCount int64
	err := w.db.QueryRow(ctx, `
		SELECT a.current_price_cents,
		       COALESCE(a.current_winner_id, ''),
		       COALESCE(b.amount_cents, 0),
		       COALESCE(b.user_id, ''),
		       COALESCE(b.accepted_count, 0)
		FROM auctions a
		LEFT JOIN LATERAL (
		  SELECT amount_cents, user_id, count(*) OVER () AS accepted_count
		  FROM bids
		  WHERE auction_id = a.id AND status = 'ACCEPTED'
		  ORDER BY engine_epoch DESC NULLS LAST, engine_seq DESC NULLS LAST, seq DESC NULLS LAST
		  LIMIT 1
		) b ON true
		WHERE a.id = $1
	`, auctionID).Scan(&dbPrice, &dbWinner, &wantPrice, &wantWinner, &acceptedCount)
	if err != nil {
		return nil, err
	}
	if acceptedCount == 0 {
		return nil, nil
	}
	if dbPrice == wantPrice && dbWinner == wantWinner {
		return nil, nil
	}
	return &reconcileViolation{
		status:  "AUCTION_WINNER_PRICE_DRIFT",
		reason:  "REDIS_ENGINE_WINNER_PRICE_DRIFT",
		message: "Auction current winner/price does not match latest accepted settled bid",
		details: map[string]any{"db_price_cents": dbPrice, "expected_price_cents": wantPrice, "db_winner_id": dbWinner, "expected_winner_id": wantWinner},
	}, nil
}

func (w *Worker) checkSoldOrderUniqueness(ctx context.Context, auctionID string) (*reconcileViolation, error) {
	var status string
	var orders int64
	var soldSettlements int64
	err := w.db.QueryRow(ctx, `
		SELECT a.status,
		       COALESCE(count(DISTINCT o.id), 0),
		       COALESCE(count(DISTINCT s.id) FILTER (WHERE s.result = 'ENGINE_SOLD' AND s.status = 'SETTLED'), 0)
		FROM auctions a
		LEFT JOIN orders o ON o.auction_id = a.id
		LEFT JOIN redis_engine_settlements s ON s.auction_id = a.id
		WHERE a.id = $1
		GROUP BY a.id
	`, auctionID).Scan(&status, &orders, &soldSettlements)
	if err != nil {
		return nil, err
	}
	if orders <= 1 && (status != "SOLD" || orders == 1) && soldSettlements <= 1 {
		return nil, nil
	}
	return &reconcileViolation{
		status:  "SOLD_ORDER_INVARIANT_FAILED",
		reason:  "REDIS_ENGINE_SOLD_ORDER_INVARIANT_FAILED",
		message: "Sold auction/order invariant failed",
		details: map[string]any{"auction_status": status, "order_count": orders, "sold_settlements": soldSettlements},
	}, nil
}

func (w *Worker) checkIdempotencyResponses(ctx context.Context, auctionID string) (*reconcileViolation, error) {
	var clientBidID string
	var bidHash string
	var idemHash string
	var bidResponse string
	var idemResponse string
	err := w.db.QueryRow(ctx, `
		SELECT b.client_bid_id,
		       b.request_hash,
		       COALESCE(i.request_hash, ''),
		       b.response_json::text,
		       COALESCE(i.response_json::text, '')
		FROM bids b
		LEFT JOIN idempotency_records i
		  ON i.scope_type = 'bid'
		 AND i.scope_id = b.auction_id
		 AND i.user_id = b.user_id
		 AND i.idempotency_key = b.client_bid_id
		WHERE b.auction_id = $1
		  AND (i.idempotency_key IS NULL OR i.status <> 'COMPLETED' OR i.request_hash <> b.request_hash OR i.response_json::text <> b.response_json::text)
		ORDER BY b.engine_epoch NULLS LAST, b.engine_seq NULLS LAST, b.created_at
		LIMIT 1
	`, auctionID).Scan(&clientBidID, &bidHash, &idemHash, &bidResponse, &idemResponse)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reconcileViolation{
		status:  "IDEMPOTENCY_RESPONSE_DRIFT",
		reason:  "REDIS_ENGINE_IDEMPOTENCY_RESPONSE_DRIFT",
		message: "Bid idempotency response does not match settled bid response",
		details: map[string]any{"client_bid_id": clientBidID, "bid_request_hash": bidHash, "idempotency_request_hash": idemHash, "bid_response_json": bidResponse, "idempotency_response_json": idemResponse},
	}, nil
}

func (w *Worker) checkOutboxCoverage(ctx context.Context, auctionID string) (*reconcileViolation, error) {
	var seq int64
	var eventType string
	var deliveryStatus string
	err := w.db.QueryRow(ctx, `
		SELECT e.seq, e.event_type, COALESCE(d.status, 'MISSING')
		FROM auction_events e
		LEFT JOIN outbox_events o
		  ON o.aggregate_type = 'auction'
		 AND o.aggregate_id = e.auction_id
		 AND o.event_type = e.event_type
		 AND o.seq = e.seq
		LEFT JOIN outbox_delivery d ON d.outbox_id = o.id
		WHERE e.auction_id = $1
		  AND (o.id IS NULL OR d.outbox_id IS NULL)
		ORDER BY e.seq
		LIMIT 1
	`, auctionID).Scan(&seq, &eventType, &deliveryStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reconcileViolation{
		status:  "OUTBOX_COVERAGE_MISSING",
		reason:  "REDIS_ENGINE_OUTBOX_COVERAGE_MISSING",
		message: "Auction event is missing matching outbox delivery record",
		details: map[string]any{"seq": seq, "event_type": eventType, "delivery_status": deliveryStatus},
	}, nil
}

func (w *Worker) checkEngineCheckpoint(ctx context.Context, auctionID string) (*reconcileViolation, error) {
	tx, err := w.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var dbEpoch int64
	var dbSeq int64
	err = tx.QueryRow(ctx, `
		SELECT engine_epoch, engine_seq
		FROM auctions
		WHERE id = $1
	`, auctionID).Scan(&dbEpoch, &dbSeq)
	if err != nil {
		return nil, err
	}

	var settledSeq int64
	var hasKafkaSettlement bool
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(max(engine_seq), 0),
		       COALESCE(bool_or(ledger_source = 'kafka' AND ledger_topic IS NOT NULL AND ledger_partition IS NOT NULL AND ledger_offset IS NOT NULL), false)
		FROM redis_engine_settlements
		WHERE auction_id = $1
		  AND engine_epoch = $2
		  AND status IN ('SETTLED','SKIPPED')
	`, auctionID, dbEpoch).Scan(&settledSeq, &hasKafkaSettlement)
	if err != nil {
		return nil, err
	}
	if !hasKafkaSettlement {
		return nil, tx.Commit(ctx)
	}

	var checkpointEpoch int64
	var checkpointSeq int64
	var checkpointTopic string
	var checkpointPartition int
	var nextOffset int64
	var storedHash string
	var snapshotText string
	err = tx.QueryRow(ctx, `
		SELECT engine_epoch, engine_seq, decision_topic, decision_partition,
		       next_decision_offset, state_hash, snapshot_json::text
		FROM auction_engine_checkpoints
		WHERE auction_id = $1
	`, auctionID).Scan(&checkpointEpoch, &checkpointSeq, &checkpointTopic, &checkpointPartition, &nextOffset, &storedHash, &snapshotText)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(ctx)
		return &reconcileViolation{
			status:  "ENGINE_CHECKPOINT_MISSING",
			reason:  "REDIS_ENGINE_CHECKPOINT_MISSING",
			message: "Settled Kafka decisions exist without a rebuild checkpoint",
			details: map[string]any{"db_engine_epoch": dbEpoch, "db_engine_seq": dbSeq, "settled_engine_seq": settledSeq},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if checkpointEpoch != dbEpoch || checkpointSeq != dbSeq || checkpointSeq < settledSeq {
		_ = tx.Commit(ctx)
		return &reconcileViolation{
			status:  "ENGINE_CHECKPOINT_LAG",
			reason:  "REDIS_ENGINE_CHECKPOINT_LAG",
			message: "Engine checkpoint does not cover the latest PostgreSQL settlement state",
			details: map[string]any{"db_engine_epoch": dbEpoch, "db_engine_seq": dbSeq, "checkpoint_epoch": checkpointEpoch, "checkpoint_seq": checkpointSeq, "settled_engine_seq": settledSeq},
		}, nil
	}
	if checkpointTopic == "" || checkpointPartition < 0 || nextOffset <= 0 {
		_ = tx.Commit(ctx)
		return &reconcileViolation{
			status:  "ENGINE_CHECKPOINT_OFFSET_INVALID",
			reason:  "REDIS_ENGINE_CHECKPOINT_OFFSET_INVALID",
			message: "Engine checkpoint cannot identify the next Kafka decision offset",
			details: map[string]any{"decision_topic": checkpointTopic, "decision_partition": checkpointPartition, "next_decision_offset": nextOffset},
		}, nil
	}

	_, payload, stateHash, err := checkpointSnapshot(ctx, tx, auctionID)
	if err != nil {
		return nil, err
	}
	var storedSnapshot engineCheckpointSnapshot
	if err := json.Unmarshal([]byte(snapshotText), &storedSnapshot); err != nil {
		_ = tx.Commit(ctx)
		return &reconcileViolation{
			status:  "ENGINE_CHECKPOINT_SNAPSHOT_INVALID",
			reason:  "REDIS_ENGINE_CHECKPOINT_SNAPSHOT_INVALID",
			message: "Engine checkpoint snapshot JSON is not decodable",
			details: map[string]any{"checkpoint_seq": checkpointSeq, "db_engine_seq": dbSeq, "error": err.Error()},
		}, nil
	}
	storedPayload, err := json.Marshal(storedSnapshot)
	if err != nil {
		return nil, err
	}
	if storedHash != sha256Hex(storedPayload) || storedHash != stateHash || sha256Hex(payload) != stateHash {
		_ = tx.Commit(ctx)
		return &reconcileViolation{
			status:  "ENGINE_CHECKPOINT_STATE_HASH_DRIFT",
			reason:  "REDIS_ENGINE_CHECKPOINT_STATE_HASH_DRIFT",
			message: "Engine checkpoint state hash does not match current settled PostgreSQL state",
			details: map[string]any{"checkpoint_hash": storedHash, "current_hash": stateHash, "checkpoint_seq": checkpointSeq, "db_engine_seq": dbSeq},
		}, nil
	}
	return nil, tx.Commit(ctx)
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
	recovered, err := w.recoverPendingDecisions(ctx, auctionID)
	if err != nil {
		report.Status = "REDIS_PENDING_KAFKA_RECOVERY_FAILED"
		report.DriftCount = 1
		report.Message = err.Error()
		_ = w.pause(ctx, auctionID, "REDIS_ENGINE_PENDING_KAFKA_RECOVERY_FAILED", err.Error(), "", nil)
		return report, nil
	}
	report.RecoveredPending = recovered
	pending, err := w.pendingDecisionCount(ctx, auctionID)
	if err != nil {
		report.Status = "REDIS_PENDING_READ_FAILED"
		report.DriftCount = 1
		report.Message = err.Error()
		_ = w.pause(ctx, auctionID, "REDIS_ENGINE_PENDING_READ_FAILED", err.Error(), "", nil)
		return report, nil
	}
	if pending > 0 {
		inProgress, err := w.pendingAppendInProgress(ctx, auctionID)
		if err != nil {
			report.Status = "REDIS_PENDING_LOCK_READ_FAILED"
			report.DriftCount = 1
			report.Message = err.Error()
			_ = w.pause(ctx, auctionID, "REDIS_ENGINE_PENDING_LOCK_READ_FAILED", err.Error(), "", nil)
			return report, nil
		}
		if inProgress {
			report.Status = "REDIS_PENDING_APPEND_IN_PROGRESS"
			report.Message = "Redis has pending hot-engine decisions currently owned by an append worker"
			return report, nil
		}
		report.Status = "REDIS_PENDING_WITHOUT_KAFKA_LEDGER"
		report.DriftCount = 1
		report.Message = "Redis has hot-engine decisions that could not be recovered into Kafka ledger after synchronous drain"
		_ = w.pause(ctx, auctionID, "REDIS_ENGINE_PENDING_KAFKA_APPEND_UNKNOWN", report.Message, "", map[string]any{"pending_decisions": pending, "recovered_pending": recovered})
		return report, nil
	}
	var first *reconcileViolation
	recordViolation := func(v *reconcileViolation) {
		if v == nil {
			return
		}
		report.DriftCount++
		if first == nil {
			copy := *v
			first = &copy
		}
	}
	if errors.Is(err, redis.Nil) && dbSeq > 0 {
		recordViolation(&reconcileViolation{status: "REDIS_STATE_MISSING", reason: "REDIS_ENGINE_STATE_MISSING_REQUIRES_REBUILD", message: "Redis hot-engine state is missing and must be rebuilt from checkpoint before resume", details: map[string]any{"db_seq": dbSeq}})
	}
	if redisSeq < dbSeq {
		recordViolation(&reconcileViolation{status: "REDIS_BEHIND_DB", reason: "REDIS_ENGINE_REDIS_BEHIND_DB", message: "Redis engine seq is behind PostgreSQL settlement", details: map[string]any{"redis_seq": redisSeq, "db_seq": dbSeq}})
	}
	if redisSeq > dbSeq {
		recordViolation(&reconcileViolation{status: "DB_BEHIND_REDIS", reason: "REDIS_ENGINE_DB_BEHIND_REDIS", message: "PostgreSQL settlement is behind Redis engine ledger", details: map[string]any{"redis_seq": redisSeq, "db_seq": dbSeq}})
	}
	if report.DLQSettlements > 0 {
		recordViolation(&reconcileViolation{status: "KAFKA_LEDGER_DLQ", reason: "KAFKA_LEDGER_DLQ_PRESENT", message: "Kafka bid ledger settlement has dead-lettered events", details: map[string]any{"dlq_settlements": report.DLQSettlements}})
	}
	checks := []func(context.Context, string) (*reconcileViolation, error){
		w.checkSettlementTerminal,
		w.checkSettlementGapless,
		w.checkAcceptedPublicSeqContiguous,
		w.checkAuctionWinnerPrice,
		w.checkSoldOrderUniqueness,
		w.checkIdempotencyResponses,
		w.checkOutboxCoverage,
		w.checkEngineCheckpoint,
	}
	for _, check := range checks {
		violation, err := check(ctx, auctionID)
		if err != nil {
			return report, err
		}
		recordViolation(violation)
	}
	if first != nil {
		report.Status = first.status
		report.Message = first.message
		if first.status != "REDIS_STATE_MISSING" {
			_ = w.pause(ctx, auctionID, first.reason, first.message, "", first.details)
		}
		return report, nil
	}
	if paused {
		cleared, err := w.clearRecoverablePause(ctx, auctionID)
		if err != nil {
			return report, err
		}
		if cleared {
			report.Paused = false
			report.Message = "recoverable redis engine pause cleared after reconcile completed with no invariant violations"
		}
	}
	return report, nil
}

func (w *Worker) clearRecoverablePause(ctx context.Context, auctionID string) (bool, error) {
	var reason string
	err := w.db.QueryRow(ctx, `
		SELECT COALESCE(engine_pause_reason, '')
		FROM auctions
		WHERE id = $1
		  AND engine_paused = true
	`, auctionID).Scan(&reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !isRecoverableRedisEnginePause(reason) {
		return false, nil
	}

	payload, _ := json.Marshal(map[string]any{"reason": reason, "cleared_by": "redis_engine_reconcile"})
	tx, err := w.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = false,
		    engine_pause_reason = NULL,
		    engine_paused_at = NULL,
		    updated_at = now()
		WHERE id = $1
		  AND engine_paused = true
		  AND engine_pause_reason = $2
	`, auctionID, reason)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('MED', 'REDIS_ENGINE_AUTO_RESUMED', $1, $2, $3)
	`, auctionID, "redis engine auto-resumed after reconcile proved the ledger and PostgreSQL are consistent", payload); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if w.redis != nil {
		_ = w.redis.HSet(ctx, redisx.BidEngineStateKey(auctionID), "paused", 0, "pause_reason", "").Err()
	}
	observability.Inc("auction_bid_engine_auto_resume_total", map[string]string{"reason": reason})
	return true, nil
}

func isRecoverableRedisEnginePause(reason string) bool {
	switch reason {
	case "REDIS_ENGINE_DB_BEHIND_REDIS",
		"REDIS_ENGINE_REDIS_BEHIND_DB",
		"REDIS_ENGINE_PENDING_KAFKA_RECOVERY_FAILED",
		"REDIS_ENGINE_PENDING_KAFKA_APPEND_UNKNOWN":
		return true
	default:
		return false
	}
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
