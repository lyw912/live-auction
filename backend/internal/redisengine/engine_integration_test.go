package redisengine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/redisx"
)

func TestRedisLedgerAcceptSettleRejectSoftCloseAndCap(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 20_000)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-1", auction.BidInput{
		ClientBidID:   "redis-ledger-1",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_redis_ledger_1")
	if err != nil {
		t.Fatalf("engine first bid: %v", err)
	}
	if first.Result != auction.BidResultEngineAccepted || first.EngineSeq != 1 || first.SettlementStatus != auction.SettlementStatusPending {
		t.Fatalf("first response = %#v", first)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle first: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")

	low, err := engine.PlaceBid(ctx, auctionID, "user_2", "redis-ledger-low", auction.BidInput{
		ClientBidID:   "redis-ledger-low",
		AmountCents:   15_000,
		ClientSeenSeq: 1,
	}, "tr_redis_ledger_low")
	if err != nil {
		t.Fatalf("engine low bid: %v", err)
	}
	if low.Result != auction.BidResultEngineRejected || low.RejectReason == nil || *low.RejectReason != "BID_TOO_LOW" {
		t.Fatalf("low response = %#v", low)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle low: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 2, 15_000, "ACTIVE")

	sold, err := engine.PlaceBid(ctx, auctionID, "user_2", "redis-ledger-cap", auction.BidInput{
		ClientBidID:   "redis-ledger-cap",
		AmountCents:   20_000,
		ClientSeenSeq: 2,
	}, "tr_redis_ledger_cap")
	if err != nil {
		t.Fatalf("engine cap bid: %v", err)
	}
	if sold.Result != auction.BidResultEngineSold || sold.EngineSeq != 3 {
		t.Fatalf("sold response = %#v", sold)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle sold: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 3, 20_000, "SOLD")
	var orders int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE auction_id = $1`, auctionID).Scan(&orders); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orders != 1 {
		t.Fatalf("orders = %d, want 1", orders)
	}
}

func TestRedisLedgerDuplicateReplayAndReconcilePause(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-replay", auction.BidInput{
		ClientBidID:   "redis-ledger-replay",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_replay")
	if err != nil {
		t.Fatalf("first bid: %v", err)
	}
	replay, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-replay", auction.BidInput{
		ClientBidID:   "redis-ledger-replay",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_replay")
	if err != nil {
		t.Fatalf("replay bid: %v", err)
	}
	if replay.BidID != first.BidID || replay.EngineSeq != first.EngineSeq {
		t.Fatalf("replay = %#v want %#v", replay, first)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "DB_BEHIND_REDIS" || report.DriftCount != 1 {
		t.Fatalf("report before settlement = %#v", report)
	}
	var paused bool
	if err := db.QueryRow(ctx, `SELECT engine_paused FROM auctions WHERE id = $1`, auctionID).Scan(&paused); err != nil {
		t.Fatalf("load paused after drift: %v", err)
	}
	if !paused {
		t.Fatalf("reconcile with Redis ahead must pause the engine")
	}
	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = false, engine_pause_reason = NULL, engine_paused_at = NULL
		WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("clear pause for settlement: %v", err)
	}
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{"paused": 0, "pause_reason": ""})
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle: %v", err)
	}
	report, err = worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile after settlement: %v", err)
	}
	if report.Status != "OK" || report.DriftCount != 0 {
		t.Fatalf("report after settlement = %#v", report)
	}
}

func TestRedisLedgerStartsAfterExistingAuctionSeq(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := db.Exec(ctx, `
		UPDATE auctions SET seq = 5, engine_seq = 5 WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("seed legacy auction seq: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO auction_events (auction_id, seq, event_type, payload_json, server_time_ms, trace_id, engine_epoch, engine_seq)
		VALUES ($1, 5, 'legacy_event', '{}'::jsonb, $2, 'tr_legacy', 1, 5)
	`, auctionID, time.Now().UTC().UnixMilli()); err != nil {
		t.Fatalf("seed legacy seq: %v", err)
	}

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-after-seq", auction.BidInput{
		ClientBidID:   "redis-ledger-after-seq",
		AmountCents:   15_000,
		ClientSeenSeq: 5,
	}, "tr_after_seq")
	if err != nil {
		t.Fatalf("engine bid after legacy seq: %v", err)
	}
	if resp.EngineSeq != 6 {
		t.Fatalf("engine seq = %d, want 6", resp.EngineSeq)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle after legacy seq: %v", err)
	}
	var events int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND seq = 6`, auctionID).Scan(&events); err != nil {
		t.Fatalf("count event seq 6: %v", err)
	}
	if events != 1 {
		t.Fatalf("event seq 6 count = %d, want 1", events)
	}
	var auctionSeq int64
	if err := db.QueryRow(ctx, `SELECT seq FROM auctions WHERE id = $1`, auctionID).Scan(&auctionSeq); err != nil {
		t.Fatalf("load auction seq: %v", err)
	}
	if auctionSeq != 6 {
		t.Fatalf("auction seq = %d, want 6", auctionSeq)
	}
}

func TestRedisLedgerPausesUnsupportedRuleAuctions(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	if _, err := db.Exec(ctx, `
		UPDATE auction_rules
		SET fat_finger_threshold_cents = 1000
		WHERE auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("enable fat-finger: %v", err)
	}
	_, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-fat-finger", auction.BidInput{
		ClientBidID:   "redis-ledger-fat-finger",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_fat_finger")
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierrors.CodeEnginePaused {
		t.Fatalf("error = %v, want ENGINE_PAUSED", err)
	}
	var paused bool
	var reason string
	if err := db.QueryRow(ctx, `SELECT engine_paused, COALESCE(engine_pause_reason, '') FROM auctions WHERE id = $1`, auctionID).Scan(&paused, &reason); err != nil {
		t.Fatalf("load pause: %v", err)
	}
	if !paused || reason == "" {
		t.Fatalf("pause state paused=%v reason=%q", paused, reason)
	}
}

func TestRedisLedgerPausesWhenKafkaAppendFails(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	engine := New(db, rdb, failingLedger{})
	auctionID := createEngineAuction(t, db, 0)

	_, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-kafka-fail", auction.BidInput{
		ClientBidID:   "redis-ledger-kafka-fail",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_kafka_fail")
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierrors.CodeEnginePaused {
		t.Fatalf("error = %v, want ENGINE_PAUSED", err)
	}
	var paused bool
	var reason string
	if err := db.QueryRow(ctx, `SELECT engine_paused, COALESCE(engine_pause_reason, '') FROM auctions WHERE id = $1`, auctionID).Scan(&paused, &reason); err != nil {
		t.Fatalf("load pause: %v", err)
	}
	if !paused || reason != "KAFKA_LEDGER_APPEND_FAILED" {
		t.Fatalf("pause state paused=%v reason=%q", paused, reason)
	}
}

func TestKafkaSettlementDuplicateMessageIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-dup-kafka", auction.BidInput{
		ClientBidID:   "redis-ledger-dup-kafka",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_dup_kafka")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	msg, ok := ledger.Message(0)
	if !ok {
		t.Fatalf("missing memory ledger message")
	}
	if err := worker.settleLedgerMessage(ctx, msg); err != nil {
		t.Fatalf("first settlement: %v", err)
	}
	if err := worker.settleLedgerMessage(ctx, msg); err != nil {
		t.Fatalf("duplicate settlement: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, resp.EngineSeq, 15_000, "ACTIVE")
	var bids int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND client_bid_id = $2`, auctionID, "redis-ledger-dup-kafka").Scan(&bids); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if bids != 1 {
		t.Fatalf("bids = %d, want 1", bids)
	}
}

func TestKafkaSettlementUniqueSeqConflictWithSamePayloadIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-seq-conflict", auction.BidInput{
		ClientBidID:   "redis-ledger-seq-conflict",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_seq_conflict"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	msg, ok := ledger.Message(0)
	if !ok {
		t.Fatalf("missing memory ledger message")
	}
	if err := worker.settleLedgerMessage(ctx, msg); err != nil {
		t.Fatalf("first settlement: %v", err)
	}
	msg.ID = "kafka:auction.bid-events:0:9999"
	msg.Offset = 9999
	if err := worker.settleLedgerMessage(ctx, msg); err != nil {
		t.Fatalf("same payload unique seq conflict should be idempotent: %v", err)
	}
	var rows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND engine_seq = 1`, auctionID).Scan(&rows); err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if rows != 1 {
		t.Fatalf("settlement rows = %d, want 1", rows)
	}
}

func TestKafkaSettlementGapAndStaleEpochPause(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	gapAuctionID := createEngineAuction(t, db, 0)
	gapResult := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_gap_" + uuid.NewString(),
		AuctionID:         gapAuctionID,
		UserID:            "user_1",
		ClientBidID:       "gap-bid",
		AmountCents:       15_000,
		EngineSeq:         2,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_gap",
		RequestHash:       requestHash(gapAuctionID, "user_1", "gap-bid", 15_000),
	}
	_, err := ledger.Append(ctx, gapResult)
	if err != nil {
		t.Fatalf("append gap: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("process gap: %v", err)
	}
	assertEnginePaused(t, db, gapAuctionID, "REDIS_ENGINE_LEDGER_GAP")

	staleAuctionID := createEngineAuction(t, db, 0)
	staleResult := gapResult
	staleResult.BidID = "bid_stale_" + uuid.NewString()
	staleResult.AuctionID = staleAuctionID
	staleResult.ClientBidID = "stale-bid"
	staleResult.EngineSeq = 1
	staleResult.EngineEpoch = 0
	staleResult.RequestHash = requestHash(staleAuctionID, "user_1", "stale-bid", 15_000)
	if _, err := ledger.Append(ctx, staleResult); err != nil {
		t.Fatalf("append stale: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("process stale: %v", err)
	}
	assertEnginePaused(t, db, staleAuctionID, "REDIS_ENGINE_STALE_EPOCH")
}

func TestKafkaSettlementRetriesThenDLQAndReconcilePause(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	missingUserID := "missing_user_" + uuid.NewString()
	result := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_retry_" + uuid.NewString(),
		AuctionID:         auctionID,
		UserID:            missingUserID,
		ClientBidID:       "retry-bid",
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   missingUserID,
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_retry",
		RequestHash:       requestHash(auctionID, missingUserID, "retry-bid", 15_000),
	}
	if _, err := ledger.Append(ctx, result); err != nil {
		t.Fatalf("append retry: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("process should retry, DLQ, and commit poison: %v", err)
	}
	if ledger.DLQLen() != 1 {
		t.Fatalf("dlq len = %d, want 1", ledger.DLQLen())
	}
	var dlqAt *time.Time
	var attempts int
	if err := db.QueryRow(ctx, `SELECT dlq_at, attempts FROM redis_engine_settlements WHERE auction_id = $1 AND engine_seq = 1`, auctionID).Scan(&dlqAt, &attempts); err != nil {
		t.Fatalf("load dlq marker: %v", err)
	}
	if dlqAt == nil {
		t.Fatalf("dlq_at is nil")
	}
	if attempts != maxSettleAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, maxSettleAttempts)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "KAFKA_LEDGER_DLQ" || report.DLQSettlements != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestReconcileRecoversRedisDecisionWithoutKafkaAck(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-pending", auction.BidInput{
		ClientBidID:   "redis-ledger-pending",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_pending")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	msg, ok := ledger.Message(0)
	if !ok {
		t.Fatalf("missing memory ledger message")
	}
	if err := rdb.HSet(ctx, redisx.BidEnginePendingKey(auctionID), resp.EngineSeq, string(msg.Value)).Err(); err != nil {
		t.Fatalf("seed pending: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.RecoveredPending != 1 || report.Status != "DB_BEHIND_REDIS" {
		t.Fatalf("report = %#v", report)
	}
	pending, err := rdb.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("count pending after recover: %v", err)
	}
	if pending != 0 {
		t.Fatalf("pending decisions = %d, want 0", pending)
	}
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_DB_BEHIND_REDIS")
	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = false, engine_pause_reason = NULL, engine_paused_at = NULL
		WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("clear pause for recovered settlement: %v", err)
	}
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{"paused": 0, "pause_reason": ""})
	if _, err := worker.ProcessKafka(ctx, 2); err != nil {
		t.Fatalf("settle recovered pending: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, resp.EngineSeq, 15_000, "ACTIVE")
}

func TestReconcileBackfillsKafkaLedgerFromRedisPendingCrashWindow(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	result := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_pending_" + uuid.NewString(),
		AuctionID:         auctionID,
		UserID:            "user_1",
		ClientBidID:       "pending-crash-bid",
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_pending_crash",
		RequestHash:       requestHash(auctionID, "user_1", "pending-crash-bid", 15_000),
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal pending result: %v", err)
	}
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{
		"status":              "ACTIVE",
		"current_price_cents": "15000",
		"current_winner_id":   "user_1",
		"engine_seq":          "1",
		"engine_epoch":        "1",
		"paused":              "0",
		"pause_reason":        "",
	})
	if err := rdb.HSet(ctx, redisx.BidEnginePendingKey(auctionID), "1", string(raw)).Err(); err != nil {
		t.Fatalf("seed redis pending: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile pending crash: %v", err)
	}
	if report.RecoveredPending != 1 || report.Status != "DB_BEHIND_REDIS" {
		t.Fatalf("report = %#v", report)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger len = %d, want 1", ledger.Len())
	}
	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = false, engine_pause_reason = NULL, engine_paused_at = NULL
		WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("clear pause for recovered settlement: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle recovered kafka backfill: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")
}

type failingLedger struct{}

func (failingLedger) Append(context.Context, engineResult) (LedgerMessage, error) {
	return LedgerMessage{}, errors.New("kafka unavailable")
}

func (failingLedger) Fetch(context.Context) (LedgerMessage, error) {
	return LedgerMessage{}, context.Canceled
}

func (failingLedger) Commit(context.Context, LedgerMessage) error          { return nil }
func (failingLedger) WriteDLQ(context.Context, LedgerMessage, error) error { return nil }
func (failingLedger) Close() error                                         { return nil }

func assertEnginePaused(t *testing.T, db *pgxpool.Pool, auctionID string, wantReason string) {
	t.Helper()
	var paused bool
	var reason string
	if err := db.QueryRow(context.Background(), `SELECT engine_paused, COALESCE(engine_pause_reason, '') FROM auctions WHERE id = $1`, auctionID).Scan(&paused, &reason); err != nil {
		t.Fatalf("load pause: %v", err)
	}
	if !paused || reason != wantReason {
		t.Fatalf("pause state paused=%v reason=%q, want %q", paused, reason, wantReason)
	}
}

func setRedisHashFields(t *testing.T, rdb *redis.Client, key string, fields map[string]any) {
	t.Helper()
	for field, value := range fields {
		if err := rdb.HSet(context.Background(), key, field, value).Err(); err != nil {
			t.Fatalf("set redis hash %s[%s]: %v", key, field, err)
		}
	}
}

func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func openStreamsRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_STREAMS_ADDR")
	if addr == "" {
		addr = os.Getenv("REDIS_ADDR")
	}
	if addr == "" {
		addr = "localhost:6380"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis required for redis engine tests: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createEngineAuction(t *testing.T, db *pgxpool.Pool, capCents int64) string {
	t.Helper()
	ctx := context.Background()
	roomID := "room_engine_" + uuid.NewString()
	itemID := "item_engine_" + uuid.NewString()
	auctionID := "auc_engine_" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ('user_2', 'user', 'Engine User 2') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert user2: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN')`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO items (id, title, status) VALUES ($1, 'Engine Item', 'READY')`, itemID); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	var capArg any
	if capCents > 0 {
		capArg = capCents
	}
	endAt := time.Now().UTC().Add(5 * time.Second)
	if _, err := db.Exec(ctx, `
		INSERT INTO auctions (
		  id, room_id, item_id, status, current_price_cents, start_price_cents,
		  increment_cents, cap_price_cents, end_at
		)
		VALUES ($1, $2, $3, 'ACTIVE', 10000, 10000, 5000, $4, $5)
	`, auctionID, roomID, itemID, capArg, endAt); err != nil {
		t.Fatalf("insert auction: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO auction_rules (
		  auction_id, rule_version, duration_seconds, extend_window_seconds,
		  extend_by_seconds, max_extend_count, deposit_bps, deposit_floor_cents, deposit_cap_cents
		)
		VALUES ($1, 1, 60, 10, 10, 3, 1000, 10000, 100000000)
	`, auctionID); err != nil {
		t.Fatalf("insert rules: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, 'user_1', 'viewer', 'ACTIVE'), ($1, 'user_2', 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id) DO UPDATE SET status = 'ACTIVE'
	`, roomID); err != nil {
		t.Fatalf("insert memberships: %v", err)
	}
	return auctionID
}

func assertAuctionEngineSeq(t *testing.T, db *pgxpool.Pool, auctionID string, wantSeq int64, wantPrice int64, wantStatus string) {
	t.Helper()
	var seq int64
	var price int64
	var status string
	if err := db.QueryRow(context.Background(), `SELECT engine_seq, current_price_cents, status FROM auctions WHERE id = $1`, auctionID).Scan(&seq, &price, &status); err != nil {
		t.Fatalf("load auction: %v", err)
	}
	if seq != wantSeq || price != wantPrice || status != wantStatus {
		t.Fatalf("auction state seq=%d price=%d status=%s, want seq=%d price=%d status=%s", seq, price, status, wantSeq, wantPrice, wantStatus)
	}
}
