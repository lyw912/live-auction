package redisengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	drainRelayTriggersForTest()
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
	if first.Seq != 0 {
		t.Fatalf("first pending public seq = %d, want snapshot seq 0", first.Seq)
	}
	settleAllLedgerMessages(t, ctx, worker, 1, true)
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")
	assertAuctionPublicSeq(t, db, auctionID, 1)

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
	if low.Seq != 0 || low.EngineSeq != 2 {
		t.Fatalf("low response seq=%d engine_seq=%d, want snapshot public seq 0 engine seq 2", low.Seq, low.EngineSeq)
	}
	settleAllLedgerMessages(t, ctx, worker, 1, true)
	assertAuctionEngineSeq(t, db, auctionID, 2, 15_000, "ACTIVE")
	assertAuctionPublicSeq(t, db, auctionID, 1)
	assertBidPublicSeq(t, db, auctionID, "redis-ledger-low", nil)

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
	if sold.Seq != 0 {
		t.Fatalf("sold pending public seq = %d, want snapshot seq 0 before settlement", sold.Seq)
	}
	settleAllLedgerMessages(t, ctx, worker, 1, true)
	assertAuctionEngineSeq(t, db, auctionID, 3, 20_000, "SOLD")
	assertAuctionPublicSeq(t, db, auctionID, 2)
	assertBidPublicSeq(t, db, auctionID, "redis-ledger-cap", int64Ptr(2))
	var orders int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE auction_id = $1`, auctionID).Scan(&orders); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orders != 1 {
		t.Fatalf("orders = %d, want 1", orders)
	}
}

func TestRedisLedgerHotStateDoesNotLoadPostgresSnapshot(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 30_000)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-hot-state-seed", auction.BidInput{
		ClientBidID:   "redis-ledger-hot-state-seed",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_hot_state_seed"); err != nil {
		t.Fatalf("seed redis state: %v", err)
	}
	db.Close()

	resp, err := engine.PlaceBid(ctx, auctionID, "user_2", "redis-ledger-hot-state-no-pg", auction.BidInput{
		ClientBidID:   "redis-ledger-hot-state-no-pg",
		AmountCents:   20_000,
		ClientSeenSeq: 0,
	}, "tr_hot_state_no_pg")
	if err != nil {
		t.Fatalf("hot redis state bid should not need postgres snapshot: %v", err)
	}
	if resp.EngineSeq != 2 || resp.CurrentPriceCents != 20_000 {
		t.Fatalf("hot redis response = %#v, want engine seq 2 price 20000", resp)
	}
}

func TestRedisLedgerMissingHotStateWithUnsettledLedgerFailsClosed(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-state-loss-seed", auction.BidInput{
		ClientBidID:   "redis-ledger-state-loss-seed",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_state_loss_seed"); err != nil {
		t.Fatalf("seed redis state: %v", err)
	}
	if err := rdb.Del(ctx, redisx.BidEngineStateKey(auctionID)).Err(); err != nil {
		t.Fatalf("delete redis state: %v", err)
	}

	_, err := engine.PlaceBid(ctx, auctionID, "user_2", "redis-ledger-state-loss-next", auction.BidInput{
		ClientBidID:   "redis-ledger-state-loss-next",
		AmountCents:   20_000,
		ClientSeenSeq: 0,
	}, "tr_state_loss_next")
	assertAPIErrorCode(t, err, apierrors.CodeEngineReconciling)
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_STATE_MISSING_REQUIRES_RECONCILE")
}

func TestRedisLedgerHotStateAndLogTTLExceedsLongSoakWindow(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-long-soak-ttl", auction.BidInput{
		ClientBidID:   "redis-ledger-long-soak-ttl",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_long_soak_ttl"); err != nil {
		t.Fatalf("seed redis state: %v", err)
	}

	stateTTL, err := rdb.TTL(ctx, redisx.BidEngineStateKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("state ttl: %v", err)
	}
	logTTL, err := rdb.TTL(ctx, redisx.BidEngineLogStreamKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("log ttl: %v", err)
	}
	minTTL := 2 * time.Hour
	if stateTTL < minTTL || logTTL < minTTL {
		t.Fatalf("hot ledger ttl too short: state=%s log=%s want both >= %s", stateTTL, logTTL, minTTL)
	}
}

func TestRedisLedgerPauseDoesNotCreatePartialHotState(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	if err := rdb.Del(ctx, redisx.BidEngineStateKey(auctionID)).Err(); err != nil {
		t.Fatalf("delete redis state: %v", err)
	}
	if err := engine.pause(ctx, auctionID, "TEST_PAUSE_WITHOUT_STATE", "test pause", "tr_partial_state"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	exists, err := rdb.Exists(ctx, redisx.BidEngineStateKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("state exists: %v", err)
	}
	if exists != 0 {
		fields, _ := rdb.HGetAll(ctx, redisx.BidEngineStateKey(auctionID)).Result()
		t.Fatalf("pause created partial redis state: %#v", fields)
	}
	assertEnginePaused(t, db, auctionID, "TEST_PAUSE_WITHOUT_STATE")
}

func TestRedisLedgerMissingHotStateAfterSettledLedgerFailsClosed(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-settled-state-loss-seed", auction.BidInput{
		ClientBidID:   "redis-ledger-settled-state-loss-seed",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_settled_state_loss_seed"); err != nil {
		t.Fatalf("seed redis state: %v", err)
	}
	settleAllLedgerMessages(t, ctx, worker, 1)
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")
	if err := rdb.Del(ctx, redisx.BidEngineStateKey(auctionID)).Err(); err != nil {
		t.Fatalf("delete redis state: %v", err)
	}

	_, err := engine.PlaceBid(ctx, auctionID, "user_2", "redis-ledger-settled-state-loss-next", auction.BidInput{
		ClientBidID:   "redis-ledger-settled-state-loss-next",
		AmountCents:   20_000,
		ClientSeenSeq: 0,
	}, "tr_settled_state_loss_next")
	assertAPIErrorCode(t, err, apierrors.CodeEngineReconciling)
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_STATE_MISSING_REQUIRES_RECONCILE")
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")
}

func TestRedisLedgerMissingHotStateWithDurableSettlementAttemptFailsClosed(t *testing.T) {
	drainRelayTriggersForTest()
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-attempt-state-loss-seed", auction.BidInput{
		ClientBidID:   "redis-ledger-attempt-state-loss-seed",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_attempt_state_loss_seed"); err != nil {
		t.Fatalf("seed redis state: %v", err)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	msg := memoryLedgerMessage(t, ctx, worker, 0)
	var result engineResult
	if err := json.Unmarshal(msg.Value, &result); err != nil {
		t.Fatalf("decode ledger payload: %v", err)
	}
	if _, err := worker.recordSettlementAttempt(ctx, auctionID, msg.ID, result, msg); err != nil {
		t.Fatalf("insert settlement attempt: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 0, 10_000, "ACTIVE")
	if err := rdb.Del(ctx, redisx.BidEngineStateKey(auctionID)).Err(); err != nil {
		t.Fatalf("delete redis state: %v", err)
	}

	_, err := engine.PlaceBid(ctx, auctionID, "user_2", "redis-ledger-attempt-state-loss-next", auction.BidInput{
		ClientBidID:   "redis-ledger-attempt-state-loss-next",
		AmountCents:   20_000,
		ClientSeenSeq: 0,
	}, "tr_attempt_state_loss_next")
	assertAPIErrorCode(t, err, apierrors.CodeEngineReconciling)
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_STATE_MISSING_REQUIRES_RECONCILE")
	assertAuctionEngineSeq(t, db, auctionID, 0, 10_000, "ACTIVE")
}

func TestRedisLedgerMissingHotStateAfterPrewarmCheckpointFailsClosed(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	report, err := worker.resumeRedisEngine(ctx, auctionID)
	if err != nil {
		t.Fatalf("prewarm redis engine: %v", err)
	}
	if !report.Resumed || !report.Rebuilt || report.EngineSeq != 0 {
		t.Fatalf("prewarm report = %#v, want rebuilt seq 0", report)
	}
	var checkpointSeq int64
	if err := db.QueryRow(ctx, `SELECT engine_seq FROM auction_engine_checkpoints WHERE auction_id = $1`, auctionID).Scan(&checkpointSeq); err != nil {
		t.Fatalf("load prewarm checkpoint: %v", err)
	}
	if checkpointSeq != 0 {
		t.Fatalf("checkpoint seq = %d, want 0", checkpointSeq)
	}
	if err := rdb.Del(ctx, redisx.BidEngineStateKey(auctionID)).Err(); err != nil {
		t.Fatalf("delete redis state: %v", err)
	}

	_, err = engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-prewarm-state-loss-next", auction.BidInput{
		ClientBidID:   "redis-ledger-prewarm-state-loss-next",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_prewarm_state_loss_next")
	assertAPIErrorCode(t, err, apierrors.CodeEngineReconciling)
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_STATE_MISSING_REQUIRES_RECONCILE")
	assertAuctionEngineSeq(t, db, auctionID, 0, 10_000, "ACTIVE")
}

func TestRedisLedgerConcurrentAppendRecordsEveryEngineSeq(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	const bidders = 32
	respCh := make(chan auction.BidResponse, bidders)
	errCh := make(chan error, bidders)
	start := make(chan struct{})
	for i := 0; i < bidders; i++ {
		i := i
		go func() {
			<-start
			clientBidID := "redis-ledger-concurrent-" + uuid.NewString()
			resp, err := engine.PlaceBid(ctx, auctionID, "user_concurrent_"+strconv.Itoa(i), clientBidID, auction.BidInput{
				ClientBidID:   clientBidID,
				AmountCents:   15_000 + int64(i)*5_000,
				ClientSeenSeq: 0,
			}, "tr_concurrent")
			respCh <- resp
			errCh <- err
		}()
	}
	close(start)
	for i := 0; i < bidders; i++ {
		err := <-errCh
		resp := <-respCh
		// v3: every bid returns a synchronous DECIDED result — no 202.
		if err != nil {
			t.Fatalf("concurrent bid %d err = %v, want DECIDED result", i, err)
		}
		if resp.DecisionStatus != auction.DecisionStatusDecided {
			t.Fatalf("bid %d decision_status = %q, want DECIDED", i, resp.DecisionStatus)
		}
		if resp.DurabilityStatus != auction.DurabilityStatusEngineDurable {
			t.Fatalf("bid %d durability_status = %q, want ENGINE_DURABLE", i, resp.DurabilityStatus)
		}
		if resp.EngineSeq == 0 {
			t.Fatalf("bid %d engine_seq = 0, want assigned seq", i)
		}
	}
	// Relay group-commit: batch-produce all stream entries to Kafka.
	ensureLedgerMessages(t, ctx, worker, bidders)
	if ledger.Len() != bidders {
		t.Fatalf("ledger messages = %d, want %d", ledger.Len(), bidders)
	}
	seen := make(map[int64]bool, bidders)
	for i := 0; i < bidders; i++ {
		msg, ok := ledger.Message(i)
		if !ok {
			t.Fatalf("missing ledger message %d", i)
		}
		var result engineResult
		if err := json.Unmarshal(msg.Value, &result); err != nil {
			t.Fatalf("decode ledger message %d: %v", i, err)
		}
		if result.EngineSeq < 1 || result.EngineSeq > bidders {
			t.Fatalf("ledger offset %d has engine_seq=%d outside 1..%d", i, result.EngineSeq, bidders)
		}
		if seen[result.EngineSeq] {
			t.Fatalf("duplicate engine_seq=%d in ledger", result.EngineSeq)
		}
		seen[result.EngineSeq] = true
	}
	for seq := int64(1); seq <= bidders; seq++ {
		if !seen[seq] {
			t.Fatalf("missing engine_seq=%d in ledger", seq)
		}
	}
	for i := 0; i < bidders; i++ {
		msg, ok := ledger.Message(i)
		if !ok {
			t.Fatalf("missing ordered ledger message %d", i)
		}
		var result engineResult
		if err := json.Unmarshal(msg.Value, &result); err != nil {
			t.Fatalf("decode ordered ledger message %d: %v", i, err)
		}
		wantSeq := int64(i + 1)
		if result.EngineSeq != wantSeq {
			t.Fatalf("ledger offset %d engine_seq=%d, want Kafka offset order to match engine_seq=%d", i, result.EngineSeq, wantSeq)
		}
	}
}

// TestRedisLedgerSettlementDoesNotWriteBackToRedis verifies that settlement never
// rewinds the live Redis engine state. In v3, refreshRedisSettledState is removed;
// this test proves the invariant holds after settlement runs.
func TestRedisLedgerSettlementDoesNotWriteBackToRedis(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	for i := 1; i <= 8; i++ {
		clientBidID := "redis-ledger-no-rewind-" + strconv.Itoa(i)
		if _, err := engine.PlaceBid(ctx, auctionID, "user_"+strconv.Itoa(i), clientBidID, auction.BidInput{
			ClientBidID:   clientBidID,
			AmountCents:   15_000 + int64(i)*5_000,
			ClientSeenSeq: 0,
		}, "tr_no_rewind"); err != nil {
			t.Fatalf("live bid %d: %v", i, err)
		}
	}
	redisSeqBefore, err := rdb.HGet(ctx, redisx.BidEngineStateKey(auctionID), "engine_seq").Int64()
	if err != nil {
		t.Fatalf("load redis seq before settlement: %v", err)
	}
	if redisSeqBefore <= 1 {
		t.Fatalf("redis seq before settlement = %d, want live state ahead of first settlement", redisSeqBefore)
	}
	// Relay all decisions to Kafka.
	ensureLedgerMessages(t, ctx, worker, 8)
	// Settle only the first decision.
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle first message: %v", err)
	}
	// v3: settlement MUST NOT write back to Redis. Redis seq stays at live value.
	redisSeqAfter, err := rdb.HGet(ctx, redisx.BidEngineStateKey(auctionID), "engine_seq").Int64()
	if err != nil {
		t.Fatalf("load redis seq after settlement: %v", err)
	}
	if redisSeqAfter != redisSeqBefore {
		t.Fatalf("redis seq after settlement = %d, want live seq %d (settlement wrote back and rewound state)", redisSeqAfter, redisSeqBefore)
	}
	// Continue placing bids — must still advance engine_seq.
	clientBidID := "redis-ledger-no-rewind-9"
	resp, err := engine.PlaceBid(ctx, auctionID, "user_9", clientBidID, auction.BidInput{
		ClientBidID:   clientBidID,
		AmountCents:   60_000,
		ClientSeenSeq: 0,
	}, "tr_no_rewind_9")
	if err != nil {
		t.Fatalf("next live bid: %v", err)
	}
	if resp.EngineSeq <= redisSeqBefore {
		t.Fatalf("next engine seq = %d, want greater than prior live seq %d", resp.EngineSeq, redisSeqBefore)
	}
}

func TestRedisLedgerConcurrentSoftCloseExtendsOnlyOnce(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	originalEndAt := time.Now().UTC().Add(5 * time.Second).Truncate(time.Millisecond)
	if _, err := db.Exec(ctx, `UPDATE auctions SET end_at = $2 WHERE id = $1`, auctionID, originalEndAt); err != nil {
		t.Fatalf("configure soft-close auction: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE auction_rules SET extend_window_seconds = 10, extend_by_seconds = 10, max_extend_count = 1 WHERE auction_id = $1`, auctionID); err != nil {
		t.Fatalf("configure soft-close rules: %v", err)
	}

	if err := db.QueryRow(ctx, `SELECT end_at FROM auctions WHERE id = $1`, auctionID).Scan(&originalEndAt); err != nil {
		t.Fatalf("load original end_at: %v", err)
	}

	const bidders = 8
	insertEngineUsers(t, db, "user_soft_", bidders)
	respCh := make(chan auction.BidResponse, bidders)
	errCh := make(chan error, bidders)
	start := make(chan struct{})
	for i := 0; i < bidders; i++ {
		i := i
		go func() {
			<-start
			clientBidID := "redis-ledger-soft-close-" + uuid.NewString()
			resp, err := engine.PlaceBid(ctx, auctionID, "user_soft_"+strconv.Itoa(i), clientBidID, auction.BidInput{
				ClientBidID:   clientBidID,
				AmountCents:   15_000 + int64(i)*5_000,
				ClientSeenSeq: 0,
			}, "tr_soft_close")
			respCh <- resp
			errCh <- err
		}()
	}
	close(start)
	for i := 0; i < bidders; i++ {
		err := <-errCh
		resp := <-respCh
		if err != nil {
			t.Fatalf("concurrent soft-close bid %d err = %v, want DECIDED result", i, err)
		}
		if resp.DecisionStatus != auction.DecisionStatusDecided {
			t.Fatalf("soft-close bid %d decision_status = %q, want DECIDED", i, resp.DecisionStatus)
		}
	}
	ensureLedgerMessages(t, ctx, worker, bidders)
	settleAllLedgerMessages(t, ctx, worker, bidders)

	var endAt time.Time
	var extendCount int
	var acceptedBidCount int64
	if err := db.QueryRow(ctx, `SELECT end_at, extend_count, accepted_bid_count FROM auctions WHERE id = $1`, auctionID).Scan(&endAt, &extendCount, &acceptedBidCount); err != nil {
		t.Fatalf("load soft-close auction: %v", err)
	}
	wantEndAt := time.UnixMilli(originalEndAt.UnixMilli()).UTC().Add(10 * time.Second)
	if !endAt.UTC().Equal(wantEndAt) {
		t.Fatalf("end_at = %s, want one extension to %s", endAt.UTC().Format(time.RFC3339Nano), wantEndAt.Format(time.RFC3339Nano))
	}
	if extendCount != 1 {
		t.Fatalf("extend_count = %d, want 1 under concurrent final-window bids", extendCount)
	}
	var settledAccepted int64
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM redis_engine_settlements
		WHERE auction_id = $1
		  AND status = 'SETTLED'
		  AND result IN ('ENGINE_ACCEPTED', 'ENGINE_SOLD')
	`, auctionID).Scan(&settledAccepted); err != nil {
		t.Fatalf("count accepted settlements: %v", err)
	}
	if settledAccepted == 0 {
		t.Fatalf("settled accepted decisions = 0, want at least one accepted soft-close bid")
	}
	if acceptedBidCount != settledAccepted {
		t.Fatalf("accepted_bid_count = %d, want settled accepted decisions %d", acceptedBidCount, settledAccepted)
	}
	var extensionEvents int
	var extendedMS int64
	if err := db.QueryRow(ctx, `
		SELECT count(*), COALESCE(max((payload_json->>'extend_ms')::bigint), 0)
		FROM auction_events
		WHERE auction_id = $1
		  AND event_type = 'auction_extended'
	`, auctionID).Scan(&extensionEvents, &extendedMS); err != nil {
		t.Fatalf("count extension events: %v", err)
	}
	if extensionEvents != 1 {
		t.Fatalf("auction_extended events = %d, want 1", extensionEvents)
	}
	if extendedMS != int64((10 * time.Second).Milliseconds()) {
		t.Fatalf("auction_extended extend_ms = %d, want 10000", extendedMS)
	}
}

func TestRedisLedgerConcurrentCapOnlyOneSoldAndLoserSeesTerminal(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 15_000)

	type bidOutcome struct {
		resp auction.BidResponse
		err  error
	}
	outcomes := make(chan bidOutcome, 2)
	start := make(chan struct{})
	users := []string{"user_1", "user_2"}
	for _, userID := range users {
		userID := userID
		go func() {
			<-start
			clientBidID := "redis-ledger-cap-race-" + userID
			resp, err := engine.PlaceBid(ctx, auctionID, userID, clientBidID, auction.BidInput{
				ClientBidID:   clientBidID,
				AmountCents:   15_000,
				ClientSeenSeq: 0,
			}, "tr_cap_race")
			outcomes <- bidOutcome{resp: resp, err: err}
		}()
	}
	close(start)

	// v3: every PlaceBid returns DECIDED synchronously — no pending path.
	sold := 0
	terminalReject := 0
	for i := 0; i < 2; i++ {
		out := <-outcomes
		if out.err != nil {
			t.Fatalf("cap race bid %d returned error: %v", i, out.err)
		}
		if out.resp.DecisionStatus != auction.DecisionStatusDecided {
			t.Fatalf("cap bid decision_status = %q, want DECIDED", out.resp.DecisionStatus)
		}
		switch out.resp.Result {
		case auction.BidResultEngineSold:
			sold++
		case auction.BidResultEngineRejected:
			if out.resp.RejectReason == nil {
				t.Fatalf("cap loser reject missing reason: %#v", out.resp)
			}
			if *out.resp.RejectReason == "BID_TOO_LOW" {
				t.Fatalf("cap loser was misclassified as BID_TOO_LOW: %#v", out.resp)
			}
			if *out.resp.RejectReason != "AUCTION_NOT_ACTIVE" {
				t.Fatalf("cap loser reject reason = %q, want AUCTION_NOT_ACTIVE", *out.resp.RejectReason)
			}
			terminalReject++
		default:
			t.Fatalf("unexpected cap race result: %#v", out.resp)
		}
	}
	if sold != 1 || terminalReject != 1 {
		t.Fatalf("cap race sold=%d terminalReject=%d, want 1/1", sold, terminalReject)
	}
	// Relay all to Kafka.
	ensureLedgerMessages(t, ctx, worker, 2)
	settleAllLedgerMessages(t, ctx, worker, 2)
	var orders int
	var soldSettlements int
	var tooLow int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE auction_id = $1`, auctionID).Scan(&orders); err != nil {
		t.Fatalf("count cap race orders: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND result = 'ENGINE_SOLD' AND status = 'SETTLED'`, auctionID).Scan(&soldSettlements); err != nil {
		t.Fatalf("count sold settlements: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND reject_reason = 'BID_TOO_LOW'`, auctionID).Scan(&tooLow); err != nil {
		t.Fatalf("count too-low rejects: %v", err)
	}
	if orders != 1 || soldSettlements != 1 || tooLow != 0 {
		t.Fatalf("orders=%d soldSettlements=%d tooLowRejects=%d, want 1/1/0", orders, soldSettlements, tooLow)
	}
}

func TestPendingAppendUsesRedisAuctionIndexWhenPgActiveListIsNoisy(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	for i := 0; i < 120; i++ {
		noisyID := createEngineAuction(t, db, 0)
		if _, err := db.Exec(ctx, `UPDATE auctions SET updated_at = now() + make_interval(secs => $2::int) WHERE id = $1`, noisyID, i+1); err != nil {
			t.Fatalf("make noisy auction newer: %v", err)
		}
	}
	if _, err := db.Exec(ctx, `UPDATE auctions SET updated_at = now() - interval '1 hour' WHERE id = $1`, auctionID); err != nil {
		t.Fatalf("make target old: %v", err)
	}

	// v3: PlaceBid atomically XADDs to the log stream; the relay reads that stream.
	// No manual seeding needed.
	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-indexed-pending", auction.BidInput{
		ClientBidID:   "redis-ledger-indexed-pending",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_indexed_pending")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if resp.DecisionStatus != auction.DecisionStatusDecided {
		t.Fatalf("decision_status = %q, want DECIDED", resp.DecisionStatus)
	}
	processed, err := worker.ProcessPendingAppends(ctx, 100)
	if err != nil {
		t.Fatalf("relay pending appends: %v", err)
	}
	if processed < 1 {
		t.Fatalf("relay processed = %d, want at least 1", processed)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle indexed pending: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, resp.EngineSeq, 15_000, "ACTIVE")
}

func TestPendingAppendActiveAuctionNotStarvedByNoisyPendingIndex(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if err := rdb.Del(ctx, redisx.BidEnginePendingAuctionsKey()).Err(); err != nil {
		t.Fatalf("clear pending auction index: %v", err)
	}
	for i := 0; i < 150; i++ {
		if err := rdb.SAdd(ctx, redisx.BidEnginePendingAuctionsKey(), "stale_pending_"+strconv.Itoa(i)).Err(); err != nil {
			t.Fatalf("seed stale pending id: %v", err)
		}
	}
	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-noisy-pending", auction.BidInput{
		ClientBidID:   "redis-ledger-noisy-pending",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_noisy_pending")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if resp.DecisionStatus != auction.DecisionStatusDecided {
		t.Fatalf("decision_status = %q, want DECIDED", resp.DecisionStatus)
	}
	processed, err := worker.ProcessPendingAppends(ctx, 100)
	if err != nil {
		t.Fatalf("relay pending appends: %v", err)
	}
	if processed < 1 {
		t.Fatalf("relay processed = %d, want active auction to be relayed despite stale pending noise", processed)
	}
}

func TestRedisLedgerDuplicateReplayAndReconcilePause(t *testing.T) {
	drainRelayTriggersForTest()
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
	ensureLedgerMessages(t, ctx, worker, 1)
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "DB_BEHIND_REDIS" || report.DriftCount != 0 {
		t.Fatalf("report before settlement = %#v", report)
	}
	var paused bool
	if err := db.QueryRow(ctx, `SELECT engine_paused FROM auctions WHERE id = $1`, auctionID).Scan(&paused); err != nil {
		t.Fatalf("load paused after drift: %v", err)
	}
	if paused {
		t.Fatalf("reconcile with ordinary Redis ahead lag must not pause the engine")
	}
	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = false, engine_pause_reason = NULL, engine_paused_at = NULL
		WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("clear pause for settlement: %v", err)
	}
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{"paused": 0, "pause_reason": ""})
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
	}
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

func TestRedisLedgerReplayBeforeSettlementUsesAppendStatus(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-acked", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-acked",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_acked")
	if err != nil {
		t.Fatalf("first bid: %v", err)
	}
	// v3: relay must run before ledger is populated.
	ensureLedgerMessages(t, ctx, worker, 1)
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after relay = %d, want 1", ledger.Len())
	}
	replay, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-acked", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-acked",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_acked_retry")
	if err != nil {
		t.Fatalf("acked replay: %v", err)
	}
	if replay.BidID != first.BidID || replay.EngineSeq != first.EngineSeq {
		t.Fatalf("replay = %#v want same bid/engine seq as %#v", replay, first)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after acked replay = %d, want no duplicate append", ledger.Len())
	}
}

// TestRedisLedgerReplayUnknownAndFailedReturnsDecided verifies that a replayed bid
// whose kafka_append_status is UNKNOWN returns ENGINE_DURABLE (decided, relay pending)
// and that a FAILED status returns ENGINE_RECONCILING. Neither path triggers a new
// ledger append — idempotency is enforced by the Redis idem key match.
func TestRedisLedgerReplayUnknownAndFailedReturnsDecided(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_status")
	if err != nil {
		t.Fatalf("first bid: %v", err)
	}
	if first.DecisionStatus != auction.DecisionStatusDecided {
		t.Fatalf("first bid decision_status = %q, want DECIDED", first.DecisionStatus)
	}
	// Relay to Kafka so the idem key gets ACKED status.
	ensureLedgerMessages(t, ctx, worker, 1)
	// Replay after ACKED: should return KAFKA_ACKED + DECIDED.
	replayAcked, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_status_acked_replay")
	if err != nil {
		t.Fatalf("acked replay: %v", err)
	}
	if replayAcked.DurabilityStatus != auction.DurabilityStatusKafkaAcked || replayAcked.DecisionStatus != auction.DecisionStatusDecided {
		t.Fatalf("acked replay = %#v, want KAFKA_ACKED+DECIDED", replayAcked)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after acked replay = %d, want no duplicate append", ledger.Len())
	}
	// Force UNKNOWN on idem key to test that replay path.
	idemKey := redisx.BidEngineIdempotencyKey(auctionID, "redis-ledger-phase2-status")
	if err := rdb.HSet(ctx, idemKey, "kafka_append_status", kafkaAppendStatusUnknown).Err(); err != nil {
		t.Fatalf("set unknown status: %v", err)
	}
	replayUnknown, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_status_unknown_replay")
	if err != nil {
		t.Fatalf("UNKNOWN replay: %v", err)
	}
	// v3: UNKNOWN → relay still pending → return ENGINE_DURABLE (still decided).
	if replayUnknown.DurabilityStatus != auction.DurabilityStatusEngineDurable || replayUnknown.DecisionStatus != auction.DecisionStatusDecided {
		t.Fatalf("UNKNOWN replay = %#v, want ENGINE_DURABLE+DECIDED", replayUnknown)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after UNKNOWN replay = %d, want no duplicate append", ledger.Len())
	}
	// FAILED: replay returns ENGINE_RECONCILING.
	if err := rdb.HSet(ctx, idemKey, "kafka_append_status", kafkaAppendStatusFailed).Err(); err != nil {
		t.Fatalf("set failed status: %v", err)
	}
	_, errFailed := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_status_failed_replay")
	assertAPIErrorCode(t, errFailed, apierrors.CodeEngineReconciling)
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after FAILED replay = %d, want no duplicate append", ledger.Len())
	}
	// Conflict: different request_hash for same idem key.
	_, err = engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   20_000, // different amount → different request_hash
		ClientSeenSeq: 0,
	}, "tr_phase2_status_conflict")
	assertAPIErrorCode(t, err, apierrors.CodeIdempotencyKeyReusedWithDifferentRequest)
}

func TestRedisLedgerCorruptIdempotencyReplayPausesInsteadOfNewDecision(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)
	clientBidID := "redis-ledger-phase2-corrupt-replay"
	requestHash := requestHash(auctionID, "user_1", clientBidID, 15_000)

	if err := rdb.HSet(ctx, redisx.BidEngineIdempotencyKey(auctionID, clientBidID), "request_hash", requestHash).Err(); err != nil {
		t.Fatalf("seed corrupt idempotency record: %v", err)
	}
	_, err := engine.PlaceBid(ctx, auctionID, "user_1", clientBidID, auction.BidInput{
		ClientBidID:   clientBidID,
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_corrupt_replay")
	assertAPIErrorCode(t, err, apierrors.CodeEnginePaused)
	if ledger.Len() != 0 {
		t.Fatalf("ledger len after corrupt replay = %d, want no new decision", ledger.Len())
	}
	assertEnginePaused(t, db, auctionID, "REDIS_IDEMPOTENCY_REPLAY_FAILED")
}

func TestRedisLedgerMissingHotStateAfterExistingAuctionSeqFailsClosed(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
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

	_, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-after-seq", auction.BidInput{
		ClientBidID:   "redis-ledger-after-seq",
		AmountCents:   15_000,
		ClientSeenSeq: 5,
	}, "tr_after_seq")
	assertAPIErrorCode(t, err, apierrors.CodeEngineReconciling)
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_STATE_MISSING_REQUIRES_RECONCILE")
	var events int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND seq = 6`, auctionID).Scan(&events); err != nil {
		t.Fatalf("count event seq 6: %v", err)
	}
	if events != 0 {
		t.Fatalf("event seq 6 count = %d, want 0", events)
	}
	var auctionSeq int64
	if err := db.QueryRow(ctx, `SELECT engine_seq FROM auctions WHERE id = $1`, auctionID).Scan(&auctionSeq); err != nil {
		t.Fatalf("load auction seq: %v", err)
	}
	if auctionSeq != 5 {
		t.Fatalf("auction engine seq = %d, want 5", auctionSeq)
	}
}

func TestRedisLedgerFatFingerConfirmThenAccepts(t *testing.T) {
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
	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-fat-finger", auction.BidInput{
		ClientBidID:   "redis-ledger-fat-finger",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_fat_finger")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != string(apierrors.CodeFatFingerConfirmRequired) || resp.ConfirmToken == "" {
		t.Fatalf("fat-finger response = %#v, want confirm token", resp)
	}
	var engineSeq int64
	if err := db.QueryRow(ctx, `SELECT engine_seq FROM auctions WHERE id = $1`, auctionID).Scan(&engineSeq); err != nil {
		t.Fatalf("load engine_seq: %v", err)
	}
	if engineSeq != 0 {
		t.Fatalf("engine_seq after confirm-required = %d, want 0", engineSeq)
	}
	confirmed, err := engine.ConfirmBid(ctx, auctionID, "user_1", "redis-ledger-fat-finger", auction.ConfirmBidInput{
		ConfirmToken:   resp.ConfirmToken,
		IdempotencyKey: "redis-ledger-fat-finger",
	}, "tr_fat_finger_confirm")
	if err != nil {
		t.Fatalf("ConfirmBid: %v", err)
	}
	if confirmed.Result != auction.BidResultEngineAccepted || confirmed.CurrentPriceCents != 15_000 {
		t.Fatalf("confirmed = %#v, want accepted at 15000", confirmed)
	}
	if _, err := NewWorker(db, rdb, ledger, "test-"+uuid.NewString()).ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
	}
	if _, err := NewWorker(db, rdb, ledger, "test-"+uuid.NewString()).ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT engine_seq FROM auctions WHERE id = $1`, auctionID).Scan(&engineSeq); err != nil {
		t.Fatalf("load settled engine_seq: %v", err)
	}
	if engineSeq != 1 {
		t.Fatalf("settled engine_seq = %d, want 1", engineSeq)
	}
}

func TestRedisLedgerFatFingerConfirmRevalidatesCurrentPrice(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := db.Exec(ctx, `
		UPDATE auction_rules
		SET fat_finger_threshold_cents = 1000
		WHERE auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("enable fat-finger: %v", err)
	}
	pending, err := engine.PlaceBid(ctx, auctionID, "user_1", "ff-low-after-race", auction.BidInput{
		ClientBidID:   "ff-low-after-race",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_ff_race")
	if err != nil {
		t.Fatalf("pending bid: %v", err)
	}
	if pending.ConfirmToken == "" {
		t.Fatalf("pending token empty: %#v", pending)
	}
	racePending, err := engine.PlaceBid(ctx, auctionID, "user_2", "ff-race-winner", auction.BidInput{
		ClientBidID:   "ff-race-winner",
		AmountCents:   20_000,
		ClientSeenSeq: 0,
	}, "tr_ff_race_winner")
	if err != nil {
		t.Fatalf("race pending bid: %v", err)
	}
	if racePending.ConfirmToken == "" {
		t.Fatalf("race pending token empty: %#v", racePending)
	}
	race, err := engine.ConfirmBid(ctx, auctionID, "user_2", "ff-race-winner", auction.ConfirmBidInput{
		ConfirmToken:   racePending.ConfirmToken,
		IdempotencyKey: "ff-race-winner",
	}, "tr_ff_race_winner_confirm")
	if err != nil {
		t.Fatalf("race confirm: %v", err)
	}
	if race.Result != auction.BidResultEngineAccepted {
		t.Fatalf("race bid = %#v, want accepted", race)
	}
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay race: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle race: %v", err)
	}
	rejected, err := engine.ConfirmBid(ctx, auctionID, "user_1", "ff-low-after-race", auction.ConfirmBidInput{
		ConfirmToken:   pending.ConfirmToken,
		IdempotencyKey: "ff-low-after-race",
	}, "tr_ff_race_confirm")
	if err != nil {
		t.Fatalf("confirm after price moved: %v", err)
	}
	if rejected.Result != auction.BidResultEngineRejected || rejected.RejectReason == nil || *rejected.RejectReason != string(apierrors.CodeBidTooLow) {
		t.Fatalf("rejected = %#v, want BID_TOO_LOW", rejected)
	}
}

func TestRedisLedgerFatFingerConfirmRejectsTokenAbuseWithoutDecision(t *testing.T) {
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
	pending, err := engine.PlaceBid(ctx, auctionID, "user_1", "ff-token-abuse", auction.BidInput{
		ClientBidID:   "ff-token-abuse",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_ff_token_abuse")
	if err != nil {
		t.Fatalf("pending bid: %v", err)
	}
	if pending.ConfirmToken == "" {
		t.Fatalf("pending confirm token empty: %#v", pending)
	}
	if _, err := engine.ConfirmBid(ctx, auctionID, "user_2", "ff-token-abuse", auction.ConfirmBidInput{
		ConfirmToken:   pending.ConfirmToken,
		IdempotencyKey: "ff-token-abuse",
	}, "tr_ff_wrong_user"); err == nil {
		t.Fatalf("wrong-user confirm succeeded")
	} else {
		assertAPIErrorCode(t, err, apierrors.CodeIdempotencyKeyReusedWithDifferentRequest)
	}
	if _, err := engine.ConfirmBid(ctx, auctionID, "user_1", "ff-token-abuse", auction.ConfirmBidInput{
		ConfirmToken:   "ft_wrong_token",
		IdempotencyKey: "ff-token-abuse",
	}, "tr_ff_wrong_token"); err == nil {
		t.Fatalf("wrong-token confirm succeeded")
	} else {
		assertAPIErrorCode(t, err, apierrors.CodeConfirmUsed)
	}
	assertAuctionEngineSeq(t, db, auctionID, 0, 10_000, "ACTIVE")

	confirmed, err := engine.ConfirmBid(ctx, auctionID, "user_1", "ff-token-abuse", auction.ConfirmBidInput{
		ConfirmToken:   pending.ConfirmToken,
		IdempotencyKey: "ff-token-abuse",
	}, "tr_ff_good_after_abuse")
	if err != nil {
		t.Fatalf("valid confirm after rejected abuse attempts: %v", err)
	}
	if confirmed.Result != auction.BidResultEngineAccepted || confirmed.EngineSeq != 1 || confirmed.CurrentPriceCents != 15_000 {
		t.Fatalf("confirmed = %#v, want accepted seq 1 at 15000", confirmed)
	}
	replay, err := engine.ConfirmBid(ctx, auctionID, "user_1", "ff-token-abuse", auction.ConfirmBidInput{
		ConfirmToken:   pending.ConfirmToken,
		IdempotencyKey: "ff-token-abuse",
	}, "tr_ff_confirm_replay")
	if err != nil {
		t.Fatalf("confirm replay: %v", err)
	}
	if replay.Result != confirmed.Result || replay.EngineSeq != confirmed.EngineSeq || replay.CurrentPriceCents != confirmed.CurrentPriceCents {
		t.Fatalf("confirm replay = %#v, want stable replay of %#v", replay, confirmed)
	}
	assertAuctionEngineSeq(t, db, auctionID, 0, 10_000, "ACTIVE")
}

func TestRedisLedgerFatFingerConfirmExpiredTokenDoesNotConsumeValidReplay(t *testing.T) {
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
	pending, err := engine.PlaceBid(ctx, auctionID, "user_1", "ff-token-expired", auction.BidInput{
		ClientBidID:   "ff-token-expired",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_ff_token_expired")
	if err != nil {
		t.Fatalf("pending bid: %v", err)
	}
	if pending.ConfirmToken == "" {
		t.Fatalf("pending confirm token empty: %#v", pending)
	}
	if err := rdb.HSet(ctx, redisx.BidEnginePendingConfirmKey(auctionID, "user_1", "ff-token-expired"), "expires_at_ms", time.Now().UTC().Add(-time.Second).UnixMilli()).Err(); err != nil {
		t.Fatalf("force token expiry: %v", err)
	}
	if _, err := engine.ConfirmBid(ctx, auctionID, "user_1", "ff-token-expired", auction.ConfirmBidInput{
		ConfirmToken:   pending.ConfirmToken,
		IdempotencyKey: "ff-token-expired",
	}, "tr_ff_expired_confirm"); err == nil {
		t.Fatalf("expired confirm succeeded")
	} else {
		assertAPIErrorCode(t, err, apierrors.CodeConfirmUsed)
	}
	assertAuctionEngineSeq(t, db, auctionID, 0, 10_000, "ACTIVE")

	replayed, err := engine.PlaceBid(ctx, auctionID, "user_1", "ff-token-expired", auction.BidInput{
		ClientBidID:   "ff-token-expired",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_ff_pending_replay_after_expiry")
	if err != nil {
		t.Fatalf("pending replay after expired confirm: %v", err)
	}
	if replayed.Result != string(apierrors.CodeFatFingerConfirmRequired) || replayed.ConfirmToken != pending.ConfirmToken {
		t.Fatalf("pending replay = %#v, want original confirm-required token", replayed)
	}
}

func TestRedisLedgerHotProxyAutoOutbidsThroughSameCAS(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	repo := auction.NewRepository(db)
	intent, err := repo.UpsertMaxBidIntent(ctx, auctionID, "user_2", auction.MaxBidIntentInput{
		MaxAmountCents: 30_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "proxy-trigger-manual", auction.BidInput{
		ClientBidID:   "proxy-trigger-manual",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_proxy_hot")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != auction.BidResultEngineAccepted {
		t.Fatalf("manual response = %#v, want accepted", resp)
	}
	if resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_2" || resp.CurrentPriceCents != 20_000 || resp.EngineSeq != 2 {
		t.Fatalf("manual response after proxy = %#v, want user_2 at 20000 seq 2", resp)
	}
	state, err := rdb.HGetAll(ctx, redisx.BidEngineStateKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("redis state: %v", err)
	}
	if state["current_winner_id"] != "user_2" || state["current_price_cents"] != "20000" || state["engine_seq"] != "2" {
		t.Fatalf("redis state after proxy = %#v, want user_2 at 20000 seq 2", state)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle: %v", err)
	}
	var winner string
	var price int64
	var engineSeq int64
	if err := db.QueryRow(ctx, `SELECT current_winner_id, current_price_cents, engine_seq FROM auctions WHERE id = $1`, auctionID).Scan(&winner, &price, &engineSeq); err != nil {
		t.Fatalf("load auction: %v", err)
	}
	if winner != "user_2" || price != 20_000 || engineSeq != 2 {
		t.Fatalf("auction winner=%s price=%d engineSeq=%d, want user_2/20000/2", winner, price, engineSeq)
	}
	var source string
	if err := db.QueryRow(ctx, `
		SELECT source
		FROM bids
		WHERE auction_id = $1 AND user_id = 'user_2'
		ORDER BY engine_seq DESC
		LIMIT 1
	`, auctionID).Scan(&source); err != nil {
		t.Fatalf("load proxy bid source: %v", err)
	}
	if source != auction.BidSourceAutoMaxBid {
		t.Fatalf("proxy bid source = %s, want %s", source, auction.BidSourceAutoMaxBid)
	}
	var lastApplied *int64
	if err := db.QueryRow(ctx, `SELECT last_applied_seq FROM max_bid_intents WHERE id = $1`, intent.ID).Scan(&lastApplied); err != nil {
		t.Fatalf("load intent: %v", err)
	}
	if lastApplied == nil || *lastApplied != 2 {
		t.Fatalf("last_applied_seq = %v, want 2", lastApplied)
	}
}

func TestRedisLedgerHotProxyCancelledIntentDoesNotDefend(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	repo := auction.NewRepository(db)

	if _, err := repo.PutMaxBidIntent(ctx, auctionID, "user_2", "cancelled-proxy-create", auction.MaxBidIntentInput{
		MaxAmountCents: 30_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	}); err != nil {
		t.Fatalf("PutMaxBidIntent: %v", err)
	}
	cancelled, err := repo.DeleteMaxBidIntent(ctx, auctionID, "user_2", "cancelled-proxy-delete")
	if err != nil {
		t.Fatalf("DeleteMaxBidIntent: %v", err)
	}
	if cancelled.Intent.Status != auction.MaxBidIntentStatusCancelled {
		t.Fatalf("cancelled status = %s, want CANCELLED", cancelled.Intent.Status)
	}

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "cancelled-proxy-trigger", auction.BidInput{
		ClientBidID:   "cancelled-proxy-trigger",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_cancelled_proxy")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != auction.BidResultEngineAccepted {
		t.Fatalf("manual response = %#v, want accepted", resp)
	}
	if resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_1" || resp.CurrentPriceCents != 15_000 || resp.EngineSeq != 1 {
		t.Fatalf("manual response after cancelled proxy = %#v, want user_1 at 15000 seq 1", resp)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle: %v", err)
	}
	var autoBids int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM bids
		WHERE auction_id = $1 AND source = $2
	`, auctionID, auction.BidSourceAutoMaxBid).Scan(&autoBids); err != nil {
		t.Fatalf("count auto bids: %v", err)
	}
	if autoBids != 0 {
		t.Fatalf("auto bids after cancellation = %d, want 0", autoBids)
	}
}

func TestRedisLedgerHotProxyEqualMaxPreemptsLaterCapChallenge(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 30_000)
	repo := auction.NewRepository(db)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "equal-proxy-first", auction.BidInput{
		ClientBidID:   "equal-proxy-first",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_equal_proxy")
	if err != nil || first.Result != auction.BidResultEngineAccepted {
		t.Fatalf("first bid = %#v err=%v", first, err)
	}
	if _, err := repo.PutMaxBidIntent(ctx, auctionID, "user_1", "equal-proxy-intent", auction.MaxBidIntentInput{
		MaxAmountCents: 30_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	}); err != nil {
		t.Fatalf("PutMaxBidIntent: %v", err)
	}

	resp, err := engine.PlaceBid(ctx, auctionID, "user_3", "equal-proxy-cap-challenge", auction.BidInput{
		ClientBidID:   "equal-proxy-cap-challenge",
		AmountCents:   30_000,
		ClientSeenSeq: first.EngineSeq,
	}, "tr_equal_proxy")
	if err != nil {
		t.Fatalf("cap challenge: %v", err)
	}
	if resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_1" || resp.CurrentPriceCents != 30_000 || resp.Result != auction.BidResultEngineRejected {
		t.Fatalf("challenge response = %#v, want rejected after user_1 proxy sold/leading at equal max", resp)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle: %v", err)
	}
	var winner string
	var price int64
	if err := db.QueryRow(ctx, `SELECT current_winner_id, current_price_cents FROM auctions WHERE id = $1`, auctionID).Scan(&winner, &price); err != nil {
		t.Fatalf("load auction: %v", err)
	}
	if winner != "user_1" || price != 30_000 {
		t.Fatalf("auction winner=%s price=%d, want user_1/30000", winner, price)
	}
	var orders int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE auction_id = $1 AND winner_id = 'user_1' AND amount_cents = 30000`, auctionID).Scan(&orders); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orders != 1 {
		t.Fatalf("orders = %d, want user_1 sold order", orders)
	}
}

func TestRedisLedgerHotProxyBatchReplayAfterWorkerCrashIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	repo := auction.NewRepository(db)
	if _, err := repo.UpsertMaxBidIntent(ctx, auctionID, "user_2", auction.MaxBidIntentInput{
		MaxAmountCents: 30_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	}); err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "proxy-replay-manual", auction.BidInput{
		ClientBidID:   "proxy-replay-manual",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_proxy_batch_replay")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != auction.BidResultEngineAccepted || resp.EngineSeq != 2 || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_2" {
		t.Fatalf("proxy response = %#v, want final seq 2 winner user_2", resp)
	}
	ensureLedgerMessages(t, ctx, worker, 2)
	first := memoryLedgerMessage(t, ctx, worker, 0)
	second := memoryLedgerMessage(t, ctx, worker, 1)
	if first.ID == second.ID || first.Offset+1 != second.Offset {
		t.Fatalf("proxy ledger messages not contiguous: first=%#v second=%#v", first, second)
	}

	processed, err := worker.ProcessKafka(ctx, 10)
	if err != nil {
		t.Fatalf("initial settlement: %v", err)
	}
	if processed != 2 {
		t.Fatalf("initial processed=%d, want 2", processed)
	}
	assertAuctionEngineSeq(t, db, auctionID, 2, 20_000, "ACTIVE")

	// Simulate a settlement worker crash after DB commit but before Kafka offset commit:
	// the same contiguous accepted batch is fetched again by a fresh worker.
	replayLedger := &staticBatchLedger{messages: []LedgerMessage{first, second}}
	replayWorker := NewWorker(db, rdb, replayLedger, "test-"+uuid.NewString())
	replayed, err := replayWorker.ProcessKafka(ctx, 10)
	if err != nil {
		t.Fatalf("replay settlement after crash: %v", err)
	}
	if replayed != 2 {
		t.Fatalf("replayed=%d, want 2 duplicate messages committed", replayed)
	}

	var settlements, bids, events, nonTerminal, duplicateSeqs int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1`, auctionID).Scan(&settlements); err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1`, auctionID).Scan(&bids); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND type = 'bid_accepted'`, auctionID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND status <> 'SETTLED'`, auctionID).Scan(&nonTerminal); err != nil {
		t.Fatalf("count non-terminal settlements: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM (
		  SELECT engine_seq
		  FROM redis_engine_settlements
		  WHERE auction_id = $1
		  GROUP BY engine_seq
		  HAVING count(*) > 1
		) dup
	`, auctionID).Scan(&duplicateSeqs); err != nil {
		t.Fatalf("count duplicate settlement seqs: %v", err)
	}
	if settlements != 2 || bids != 2 || events != 2 || nonTerminal != 0 || duplicateSeqs != 0 {
		t.Fatalf("after replay settlements=%d bids=%d events=%d nonTerminal=%d duplicateSeqs=%d, want 2/2/2/0/0", settlements, bids, events, nonTerminal, duplicateSeqs)
	}
	assertAuctionEngineSeq(t, db, auctionID, 2, 20_000, "ACTIVE")
}

func TestRedisLedgerRichRulesPressureSmoke(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	if _, err := db.Exec(ctx, `
		UPDATE auction_rules
		SET fat_finger_threshold_cents = 5000
		WHERE auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("enable fat-finger: %v", err)
	}
	repo := auction.NewRepository(db)
	if _, err := repo.UpsertMaxBidIntent(ctx, auctionID, "user_2", auction.MaxBidIntentInput{
		MaxAmountCents: 400_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	}); err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	totalDecisions := 0
	expectedPrice := int64(10_000)
	expectedWinner := ""
	for i := 0; i < 24; i++ {
		if i%4 == 0 {
			clientBidID := fmt.Sprintf("rich-pressure-ff-%02d", i)
			amount := expectedPrice + 20_000
			pending, err := engine.PlaceBid(ctx, auctionID, "user_1", clientBidID, auction.BidInput{
				ClientBidID:   clientBidID,
				AmountCents:   amount,
				ClientSeenSeq: int64(totalDecisions),
			}, "tr_rich_pressure_ff")
			if err != nil {
				t.Fatalf("fat-finger PlaceBid %d: %v", i, err)
			}
			if pending.Result != string(apierrors.CodeFatFingerConfirmRequired) || pending.ConfirmToken == "" {
				t.Fatalf("fat-finger pending %d = %#v, want confirm-required", i, pending)
			}
			confirmed, err := engine.ConfirmBid(ctx, auctionID, "user_1", clientBidID, auction.ConfirmBidInput{
				ConfirmToken:   pending.ConfirmToken,
				IdempotencyKey: clientBidID,
			}, "tr_rich_pressure_ff_confirm")
			if err != nil {
				t.Fatalf("fat-finger ConfirmBid %d: %v", i, err)
			}
			if confirmed.Result != auction.BidResultEngineAccepted || confirmed.CurrentWinnerID == nil || *confirmed.CurrentWinnerID != "user_1" {
				t.Fatalf("fat-finger confirmed %d = %#v, want user_1 accepted", i, confirmed)
			}
			totalDecisions++
			expectedPrice = amount
			expectedWinner = "user_1"
			settleAllLedgerMessages(t, ctx, worker, 1, true)
			continue
		}

		clientBidID := fmt.Sprintf("rich-pressure-proxy-%02d", i)
		amount := expectedPrice + 5_000
		resp, err := engine.PlaceBid(ctx, auctionID, "user_3", clientBidID, auction.BidInput{
			ClientBidID:   clientBidID,
			AmountCents:   amount,
			ClientSeenSeq: int64(totalDecisions),
		}, "tr_rich_pressure_proxy")
		if err != nil {
			t.Fatalf("proxy PlaceBid %d: %v", i, err)
		}
		if resp.Result != auction.BidResultEngineAccepted || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_2" {
			t.Fatalf("proxy response %d = %#v, want user_2 auto winner", i, resp)
		}
		totalDecisions += 2
		expectedPrice = amount + 5_000
		expectedWinner = "user_2"
		settleAllLedgerMessages(t, ctx, worker, 2, true)
	}

	var winner string
	var price int64
	var engineSeq int64
	var paused bool
	if err := db.QueryRow(ctx, `
		SELECT current_winner_id, current_price_cents, engine_seq, engine_paused
		FROM auctions
		WHERE id = $1
	`, auctionID).Scan(&winner, &price, &engineSeq, &paused); err != nil {
		t.Fatalf("load final auction: %v", err)
	}
	if winner != expectedWinner || price != expectedPrice || engineSeq != int64(totalDecisions) || paused {
		t.Fatalf("final winner=%s price=%d seq=%d paused=%v, want %s/%d/%d/false", winner, price, engineSeq, paused, expectedWinner, expectedPrice, totalDecisions)
	}
	var settlements, bids, nonTerminal, duplicateSeqs, proxyBids, fatFingerUserBids int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1`, auctionID).Scan(&settlements); err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1`, auctionID).Scan(&bids); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND status <> 'SETTLED'`, auctionID).Scan(&nonTerminal); err != nil {
		t.Fatalf("count non-terminal settlements: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM (
		  SELECT engine_seq
		  FROM redis_engine_settlements
		  WHERE auction_id = $1
		  GROUP BY engine_seq
		  HAVING count(*) > 1
		) dup
	`, auctionID).Scan(&duplicateSeqs); err != nil {
		t.Fatalf("count duplicate settlement seqs: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND source = $2`, auctionID, auction.BidSourceAutoMaxBid).Scan(&proxyBids); err != nil {
		t.Fatalf("count proxy bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND user_id = 'user_1'`, auctionID).Scan(&fatFingerUserBids); err != nil {
		t.Fatalf("count fat-finger confirmed bids: %v", err)
	}
	if settlements != totalDecisions || bids != totalDecisions || nonTerminal != 0 || duplicateSeqs != 0 {
		t.Fatalf("settlements=%d bids=%d nonTerminal=%d duplicateSeqs=%d, want %d/%d/0/0", settlements, bids, nonTerminal, duplicateSeqs, totalDecisions, totalDecisions)
	}
	if proxyBids != 18 || fatFingerUserBids != 6 {
		t.Fatalf("proxyBids=%d fatFingerUserBids=%d, want 18/6", proxyBids, fatFingerUserBids)
	}
	assertMaxBidIntentForUser(t, db, auctionID, "user_2", auction.MaxBidIntentStatusActive, int64Ptr(int64(totalDecisions)))
}

func TestRedisLedgerHotProxyIgnoresCancelledIntent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	repo := auction.NewRepository(db)
	intent, err := repo.UpsertMaxBidIntent(ctx, auctionID, "user_2", auction.MaxBidIntentInput{
		MaxAmountCents: 30_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}
	if _, err := repo.CancelMaxBidIntent(ctx, auctionID, "user_2"); err != nil {
		t.Fatalf("CancelMaxBidIntent: %v", err)
	}

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "proxy-cancelled-trigger", auction.BidInput{
		ClientBidID:   "proxy-cancelled-trigger",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_proxy_cancelled")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != auction.BidResultEngineAccepted || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_1" || resp.CurrentPriceCents != 15_000 || resp.EngineSeq != 1 {
		t.Fatalf("response = %#v, want manual user_1 accepted without proxy", resp)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")
	assertMaxBidIntentState(t, db, intent.ID, auction.MaxBidIntentStatusCancelled, nil)
}

func TestRedisLedgerHotProxyCancelAfterDefensePreventsLaterDefense(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	repo := auction.NewRepository(db)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "proxy-cancel-after-defense-first", auction.BidInput{
		ClientBidID:   "proxy-cancel-after-defense-first",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_proxy_cancel_after_defense_first"); err != nil {
		t.Fatalf("first bid: %v", err)
	}
	if _, err := engine.PlaceBid(ctx, auctionID, "user_2", "proxy-cancel-after-defense-rival", auction.BidInput{
		ClientBidID:   "proxy-cancel-after-defense-rival",
		AmountCents:   20_000,
		ClientSeenSeq: 1,
	}, "tr_proxy_cancel_after_defense_rival"); err != nil {
		t.Fatalf("rival bid: %v", err)
	}
	intent, err := repo.UpsertMaxBidIntent(ctx, auctionID, "user_1", auction.MaxBidIntentInput{
		MaxAmountCents: 40_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	defended, err := engine.PlaceBid(ctx, auctionID, "user_3", "proxy-cancel-after-defense-trigger", auction.BidInput{
		ClientBidID:   "proxy-cancel-after-defense-trigger",
		AmountCents:   25_000,
		ClientSeenSeq: 2,
	}, "tr_proxy_cancel_after_defense_trigger")
	if err != nil {
		t.Fatalf("trigger bid: %v", err)
	}
	if defended.Result != auction.BidResultEngineRejected || defended.CurrentWinnerID == nil || *defended.CurrentWinnerID != "user_1" || defended.CurrentPriceCents != 30_000 || defended.EngineSeq != 3 {
		t.Fatalf("defended response = %#v, want user_1 proxy defense at 30000 seq 3", defended)
	}
	ensureLedgerMessages(t, ctx, worker, 3)
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle first defense: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 3, 30_000, "ACTIVE")
	assertMaxBidIntentState(t, db, intent.ID, auction.MaxBidIntentStatusActive, int64Ptr(3))

	cancelled, err := repo.CancelMaxBidIntent(ctx, auctionID, "user_1")
	if err != nil {
		t.Fatalf("CancelMaxBidIntent: %v", err)
	}
	if cancelled.Status != auction.MaxBidIntentStatusCancelled {
		t.Fatalf("cancelled intent status = %s, want CANCELLED", cancelled.Status)
	}

	afterCancel, err := engine.PlaceBid(ctx, auctionID, "user_3", "proxy-cancel-after-defense-next", auction.BidInput{
		ClientBidID:   "proxy-cancel-after-defense-next",
		AmountCents:   35_000,
		ClientSeenSeq: 3,
	}, "tr_proxy_cancel_after_defense_next")
	if err != nil {
		t.Fatalf("post-cancel challenger bid: %v", err)
	}
	if afterCancel.Result != auction.BidResultEngineAccepted || afterCancel.CurrentWinnerID == nil || *afterCancel.CurrentWinnerID != "user_3" || afterCancel.CurrentPriceCents != 35_000 || afterCancel.EngineSeq != 4 {
		t.Fatalf("post-cancel response = %#v, want user_3 accepted at 35000 without proxy defense", afterCancel)
	}
	ensureLedgerMessages(t, ctx, worker, 4)
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle post-cancel bid: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 4, 35_000, "ACTIVE")
	assertMaxBidIntentState(t, db, intent.ID, auction.MaxBidIntentStatusCancelled, int64Ptr(3))

	var autoBids int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM bids
		WHERE auction_id = $1 AND source = $2
	`, auctionID, auction.BidSourceAutoMaxBid).Scan(&autoBids); err != nil {
		t.Fatalf("count proxy bids: %v", err)
	}
	if autoBids != 1 {
		t.Fatalf("auto proxy bids = %d, want exactly 1 before cancellation", autoBids)
	}
}

func TestRedisLedgerHotProxyExhaustsIntentBelowNextBid(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	repo := auction.NewRepository(db)
	intent, err := repo.UpsertMaxBidIntent(ctx, auctionID, "user_2", auction.MaxBidIntentInput{
		MaxAmountCents: 15_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "proxy-exhaust-trigger", auction.BidInput{
		ClientBidID:   "proxy-exhaust-trigger",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_proxy_exhaust")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != auction.BidResultEngineAccepted || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_1" || resp.EngineSeq != 1 {
		t.Fatalf("response = %#v, want manual user_1 accepted without proxy", resp)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")
	assertMaxBidIntentState(t, db, intent.ID, auction.MaxBidIntentStatusExhausted, nil)
}

func TestRedisLedgerHotProxyCanReachCapAndSell(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 25_000)
	repo := auction.NewRepository(db)
	intent, err := repo.UpsertMaxBidIntent(ctx, auctionID, "user_2", auction.MaxBidIntentInput{
		MaxAmountCents: 25_000,
		Source:         auction.MaxBidIntentSourceMaxBid,
	})
	if err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "proxy-cap-trigger", auction.BidInput{
		ClientBidID:   "proxy-cap-trigger",
		AmountCents:   20_000,
		ClientSeenSeq: 0,
	}, "tr_proxy_cap")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if resp.Result != auction.BidResultEngineSold || resp.CurrentWinnerID == nil || *resp.CurrentWinnerID != "user_2" || resp.CurrentPriceCents != 25_000 || resp.EngineSeq != 2 {
		t.Fatalf("response = %#v, want proxy user_2 sold at cap seq 2", resp)
	}
	ensureLedgerMessages(t, ctx, worker, 2)
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 2, 25_000, "SOLD")
	assertMaxBidIntentState(t, db, intent.ID, auction.MaxBidIntentStatusTerminal, int64Ptr(2))
	var orders int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE auction_id = $1 AND user_id = 'user_2' AND amount_cents = 25000`, auctionID).Scan(&orders); err != nil {
		t.Fatalf("count proxy cap orders: %v", err)
	}
	if orders != 1 {
		t.Fatalf("proxy cap orders = %d, want 1", orders)
	}
}

func TestCancelFencesHotEngine(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "cancel-fence-first", auction.BidInput{
		ClientBidID:   "cancel-fence-first",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_cancel_fence_first")
	if err != nil {
		t.Fatalf("first bid: %v", err)
	}
	if first.Result != auction.BidResultEngineAccepted {
		t.Fatalf("first result = %s, want ENGINE_ACCEPTED", first.Result)
	}
	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET status = 'CANCELLED', updated_at = now()
		WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("cancel pg auction: %v", err)
	}

	engine.FenceAuction(ctx, auctionID, "HOST_CANCELLED")
	_, err = engine.PlaceBid(ctx, auctionID, "user_2", "cancel-fence-after", auction.BidInput{
		ClientBidID:   "cancel-fence-after",
		AmountCents:   20_000,
		ClientSeenSeq: first.EngineSeq,
	}, "tr_cancel_fence_after")
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierrors.CodeEnginePaused {
		t.Fatalf("post-cancel bid error = %v, want ENGINE_PAUSED", err)
	}
	values, err := rdb.HGetAll(ctx, redisx.BidEngineStateKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("read redis state: %v", err)
	}
	if values["paused"] != "1" || values["pause_reason"] != "HOST_CANCELLED" || values["status"] != "CANCELLED" {
		t.Fatalf("redis fence fields = %#v", values)
	}
}

func TestReconcileFencesTerminalHotEngine(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "terminal-fence-first", auction.BidInput{
		ClientBidID:   "terminal-fence-first",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_terminal_fence_first")
	if err != nil {
		t.Fatalf("first bid: %v", err)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET status = 'CANCELLED', updated_at = now()
		WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("cancel pg auction: %v", err)
	}
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{
		"status":       "ACTIVE",
		"paused":       0,
		"pause_reason": "",
	})

	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "REDIS_TERMINAL_UNFENCED" || report.DriftCount == 0 {
		t.Fatalf("report = %#v", report)
	}
	values, err := rdb.HGetAll(ctx, redisx.BidEngineStateKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("read redis state: %v", err)
	}
	if values["paused"] != "1" || values["pause_reason"] != "PG_TERMINAL_REDIS_UNFENCED" || values["status"] != "CANCELLED" {
		t.Fatalf("redis reconcile fence fields = %#v", values)
	}
	_, err = engine.PlaceBid(ctx, auctionID, "user_2", "terminal-fence-after", auction.BidInput{
		ClientBidID:   "terminal-fence-after",
		AmountCents:   20_000,
		ClientSeenSeq: first.EngineSeq,
	}, "tr_terminal_fence_after")
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierrors.CodeEnginePaused {
		t.Fatalf("post-reconcile bid error = %v, want ENGINE_PAUSED", err)
	}
}

// TestReconcileFencesTerminalSold verifies that when PG status is SOLD but Redis
// still reports ACTIVE (e.g., scheduler wrote SOLD before calling FenceAuction),
// the reconciler detects the drift and fences Redis, preventing new bids.
func TestReconcileFencesTerminalSold(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "sold-fence-first", auction.BidInput{
		ClientBidID:   "sold-fence-first",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_sold_fence")
	if err != nil {
		t.Fatalf("first bid: %v", err)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle: %v", err)
	}
	// Force PG to SOLD (simulates scheduler completing after Redis bid won).
	if _, err := db.Exec(ctx, `
		UPDATE auctions SET status = 'SOLD', updated_at = now() WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("force PG SOLD: %v", err)
	}
	// Simulate Redis still reporting ACTIVE (fencer not yet called).
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{
		"status":       "ACTIVE",
		"paused":       0,
		"pause_reason": "",
	})

	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "REDIS_TERMINAL_UNFENCED" || report.DriftCount == 0 {
		t.Fatalf("report = %#v, want REDIS_TERMINAL_UNFENCED with drift", report)
	}
	values, err := rdb.HGetAll(ctx, redisx.BidEngineStateKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("read redis state: %v", err)
	}
	if values["paused"] != "1" || values["status"] != "SOLD" {
		t.Fatalf("redis after reconcile = %#v, want paused=1 status=SOLD", values)
	}
	// Any further bid must be rejected ENGINE_PAUSED.
	_, err = engine.PlaceBid(ctx, auctionID, "user_2", "sold-fence-after", auction.BidInput{
		ClientBidID:   "sold-fence-after",
		AmountCents:   20_000,
		ClientSeenSeq: first.EngineSeq,
	}, "tr_sold_fence_after")
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierrors.CodeEnginePaused {
		t.Fatalf("post-reconcile bid error = %v, want ENGINE_PAUSED", err)
	}
}

// TestRelayFailsGracefullyWhenKafkaUnavailable verifies that when the relay's Kafka
// batch produce fails, the stream entries remain intact (cursor not advanced) and
// the hot PlaceBid path is unaffected — decisions are still DECIDED+ENGINE_DURABLE.
// This is the v3 contract: Kafka failure is a relay concern, not a decision concern.
func TestRelayFailsGracefullyWhenKafkaUnavailable(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	engine := New(db, rdb, failingLedger{})
	worker := NewWorker(db, rdb, failingLedger{}, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	// Hot path: PlaceBid succeeds and returns DECIDED even when Kafka relay will fail.
	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "relay-fail-bid", auction.BidInput{
		ClientBidID:   "relay-fail-bid",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_relay_fail")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if resp.DecisionStatus != auction.DecisionStatusDecided || resp.DurabilityStatus != auction.DurabilityStatusEngineDurable {
		t.Fatalf("response = %#v, want DECIDED+ENGINE_DURABLE", resp)
	}
	// The decision is in the log stream.
	streamLen, err := rdb.XLen(ctx, redisx.BidEngineLogStreamKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("stream len: %v", err)
	}
	if streamLen != 1 {
		t.Fatalf("stream len = %d, want 1", streamLen)
	}
	// Relay fails: stream cursor stays at 0-0 (no progress).
	_, relayErr := worker.ProcessPendingAppends(ctx, 100)
	if relayErr == nil {
		t.Fatalf("relay with failing Kafka should return error")
	}
	// Stream still has 1 entry — cursor not advanced.
	streamLenAfter, _ := rdb.XLen(ctx, redisx.BidEngineLogStreamKey(auctionID)).Result()
	if streamLenAfter != 1 {
		t.Fatalf("stream len after failed relay = %d, want 1 (cursor not advanced)", streamLenAfter)
	}
	assertEngineNotPaused(t, db, auctionID)
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
	// v3: relay must run before ledger is populated.
	ensureLedgerMessages(t, ctx, worker, 1)
	msg := memoryLedgerMessage(t, ctx, worker, 0)
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

// TestRelayAcksIdemKeyAndRelayIsIdempotent verifies that after PlaceBid and relay:
// idem key becomes KAFKA_ACKED, second relay pass produces no duplicate, settlement succeeds.
func TestRelayAcksIdemKeyAndRelayIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "relay-ack-idem", auction.BidInput{
		ClientBidID: "relay-ack-idem", AmountCents: 15_000,
	}, "tr_relay_ack")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if resp.Result != auction.BidResultEngineAccepted || resp.DecisionStatus != auction.DecisionStatusDecided {
		t.Fatalf("response = %#v", resp)
	}
	// Relay: produces 1 message, marks idem key ACKED.
	processed, err := worker.ProcessPendingAppends(ctx, 100)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	if processed != 1 || ledger.Len() != 1 {
		t.Fatalf("relay processed=%d ledger=%d, want 1/1", processed, ledger.Len())
	}
	var appendStatus string
	if err := rdb.HGet(ctx, redisx.BidEngineIdempotencyKey(auctionID, "relay-ack-idem"), "kafka_append_status").Scan(&appendStatus); err != nil {
		t.Fatalf("load append status: %v", err)
	}
	if appendStatus != kafkaAppendStatusAcked {
		t.Fatalf("idem kafka_append_status = %q after relay, want ACKED", appendStatus)
	}
	// Second relay: cursor advanced — no duplicate.
	if n, err := worker.ProcessPendingAppends(ctx, 100); err != nil || n != 0 {
		t.Fatalf("relay pass 2: n=%d err=%v, want 0", n, err)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger after idempotent relay = %d, want 1", ledger.Len())
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, resp.EngineSeq, 15_000, "ACTIVE")
}

// TestRedisIdempotencyReplayReturnsSameResult verifies that replaying a bid
// (same idempotency key + same request hash) returns the same engine decision.
// In v3 there is no append lock; idempotency is enforced atomically in the Lua idem key.
func TestRedisIdempotencyReplayReturnsSameResult(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "idem-replay-test", auction.BidInput{
		ClientBidID:   "idem-replay-test",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_idem_first")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if resp.Result != auction.BidResultEngineAccepted {
		t.Fatalf("response = %#v", resp)
	}
	// Replay: same key, same hash — must return same BidID and EngineSeq.
	replay, err := engine.PlaceBid(ctx, auctionID, "user_1", "idem-replay-test", auction.BidInput{
		ClientBidID:   "idem-replay-test",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_idem_replay")
	if err != nil {
		t.Fatalf("idempotency replay: %v", err)
	}
	if replay.BidID != resp.BidID || replay.EngineSeq != resp.EngineSeq {
		t.Fatalf("replay = %#v, want BidID=%s EngineSeq=%d", replay, resp.BidID, resp.EngineSeq)
	}
	// Only 1 entry in the stream (not 2) — idempotency prevented a second decision.
	streamLen, err := rdb.XLen(ctx, redisx.BidEngineLogStreamKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("stream len: %v", err)
	}
	if streamLen != 1 {
		t.Fatalf("stream len = %d, want 1 (replay must not produce duplicate)", streamLen)
	}
}

// TestRelayGroupCommitBatchesAllDecisions verifies that the group-commit relay
// produces all concurrent decisions to Kafka in a single batch and marks each idem
// key as KAFKA_ACKED. After relay, idempotency replays return KAFKA_ACKED+DECIDED.
func TestRelayGroupCommitBatchesAllDecisions(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	const bidders = 6
	insertEngineUsers(t, db, "user_relay_", bidders)

	// Fire all concurrent bids — all must return DECIDED synchronously.
	started := time.Now()
	respCh := make(chan auction.BidResponse, bidders)
	errCh := make(chan error, bidders)
	start := make(chan struct{})
	for i := 0; i < bidders; i++ {
		i := i
		go func() {
			<-start
			clientBidID := "relay-batch-" + strconv.Itoa(i)
			resp, err := engine.PlaceBid(ctx, auctionID, "user_relay_"+strconv.Itoa(i), clientBidID, auction.BidInput{
				ClientBidID:   clientBidID,
				AmountCents:   15_000 + int64(i)*5_000,
				ClientSeenSeq: 0,
			}, "tr_relay_batch")
			respCh <- resp
			errCh <- err
		}()
	}
	close(start)
	for i := 0; i < bidders; i++ {
		err := <-errCh
		resp := <-respCh
		if err != nil {
			t.Fatalf("bid %d: %v", i, err)
		}
		if resp.DecisionStatus != auction.DecisionStatusDecided || resp.DurabilityStatus != auction.DurabilityStatusEngineDurable {
			t.Fatalf("bid %d = %#v, want DECIDED+ENGINE_DURABLE", i, resp)
		}
	}
	elapsed := time.Since(started)
	if elapsed > 200*time.Millisecond {
		t.Logf("warning: concurrent bids took %s (expected < 200ms)", elapsed)
	}
	assertEngineNotPaused(t, db, auctionID)

	// Relay all decisions; DB/ledger assertions below verify the batch result.
	ensureLedgerMessages(t, ctx, worker, bidders)
	if ledger.Len() != bidders {
		t.Fatalf("ledger len = %d, want %d unique decisions", ledger.Len(), bidders)
	}
	assertEngineNotPaused(t, db, auctionID)

	// After relay: idempotency replay returns KAFKA_ACKED.
	for i := 0; i < bidders; i++ {
		clientBidID := "relay-batch-" + strconv.Itoa(i)
		replay, err := engine.PlaceBid(ctx, auctionID, "user_relay_"+strconv.Itoa(i), clientBidID, auction.BidInput{
			ClientBidID:   clientBidID,
			AmountCents:   15_000 + int64(i)*5_000,
			ClientSeenSeq: 0,
		}, "tr_relay_batch_replay")
		if err != nil {
			t.Fatalf("acked replay %s: %v", clientBidID, err)
		}
		if replay.DurabilityStatus != auction.DurabilityStatusKafkaAcked {
			t.Fatalf("replay %d durability = %q, want KAFKA_ACKED", i, replay.DurabilityStatus)
		}
	}
}

func TestKafkaSettlementBatchesAcceptedPrefix(t *testing.T) {
	drainRelayTriggersForTest()
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	if _, err := db.Exec(ctx, `UPDATE auction_rules SET extend_window_seconds = 10, extend_by_seconds = 10, max_extend_count = 2 WHERE auction_id = $1`, auctionID); err != nil {
		t.Fatalf("set extension rule: %v", err)
	}
	originalEnd := time.Now().UTC().Add(5 * time.Second).Truncate(time.Millisecond)
	if _, err := db.Exec(ctx, `UPDATE auctions SET end_at = $2 WHERE id = $1`, auctionID, originalEnd); err != nil {
		t.Fatalf("set auction end_at: %v", err)
	}

	const bids = 5
	insertEngineUsers(t, db, "accepted_batch_user_", bids)
	for i := 0; i < bids; i++ {
		clientBidID := "accepted-batch-" + strconv.Itoa(i)
		resp, err := engine.PlaceBid(ctx, auctionID, "accepted_batch_user_"+strconv.Itoa(i), clientBidID, auction.BidInput{
			ClientBidID:   clientBidID,
			AmountCents:   15_000 + int64(i)*5_000,
			ClientSeenSeq: 0,
		}, "tr_accepted_batch")
		if err != nil {
			t.Fatalf("place bid %d: %v", i, err)
		}
		if resp.Result != auction.BidResultEngineAccepted || resp.EngineSeq != int64(i+1) {
			t.Fatalf("bid %d response = %#v", i, resp)
		}
	}
	ensureLedgerMessages(t, ctx, worker, bids)
	processed, err := worker.ProcessKafka(ctx, 10)
	if err != nil {
		t.Fatalf("accepted batch settlement: %v", err)
	}
	if processed != bids {
		t.Fatalf("accepted batch processed = %d, want %d", processed, bids)
	}
	assertAuctionEngineSeq(t, db, auctionID, bids, 15_000+int64(bids-1)*5_000, "ACTIVE")
	assertAuctionPublicSeq(t, db, auctionID, bids)

	var bidRows, events, outboxDeliveries, settlements, idems, seqGaps int
	var settledEnd time.Time
	var extendCount int
	if err := db.QueryRow(ctx, `SELECT end_at, extend_count FROM auctions WHERE id = $1`, auctionID).Scan(&settledEnd, &extendCount); err != nil {
		t.Fatalf("load settled auction extension state: %v", err)
	}
	if !settledEnd.After(originalEnd) || extendCount != 1 {
		t.Fatalf("settled extension end_at=%s extend_count=%d, want after %s and count 1", settledEnd, extendCount, originalEnd)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND status = 'ACCEPTED' AND settlement_status = 'SETTLED'`, auctionID).Scan(&bidRows); err != nil {
		t.Fatalf("count accepted bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND event_type IN ('bid_accepted', 'auction_extended')`, auctionID).Scan(&events); err != nil {
		t.Fatalf("count auction events: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_delivery d
		JOIN outbox_events e ON e.id = d.outbox_id
		WHERE e.auction_id = $1
	`, auctionID).Scan(&outboxDeliveries); err != nil {
		t.Fatalf("count outbox deliveries: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND status = 'SETTLED'`, auctionID).Scan(&settlements); err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM idempotency_records WHERE scope_type = 'bid' AND scope_id = $1 AND status = 'COMPLETED'`, auctionID).Scan(&idems); err != nil {
		t.Fatalf("count idempotency records: %v", err)
	}
	if err := db.QueryRow(ctx, `
		WITH accepted AS (
		  SELECT seq, engine_seq, lag(seq) over (order by engine_seq) AS prev_seq
		  FROM bids
		  WHERE auction_id = $1 AND status = 'ACCEPTED'
		)
		SELECT count(*)
		FROM accepted
		WHERE seq <> engine_seq OR (prev_seq IS NOT NULL AND seq <> prev_seq + 1)
	`, auctionID).Scan(&seqGaps); err != nil {
		t.Fatalf("count sequence gaps: %v", err)
	}
	if bidRows != bids || events != bids || outboxDeliveries != bids || settlements != bids || idems != bids || seqGaps != 0 {
		t.Fatalf("batch rows bids=%d events=%d outbox=%d settlements=%d idems=%d seq_gaps=%d, want all %d and gaps 0",
			bidRows, events, outboxDeliveries, settlements, idems, seqGaps, bids)
	}
}

func TestKafkaSettlementBatchesAcceptedPrefixBeforeReject(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	const accepted = 3
	insertEngineUsers(t, db, "accepted_prefix_user_", accepted+1)
	for i := 0; i < accepted; i++ {
		clientBidID := "accepted-prefix-" + strconv.Itoa(i)
		resp, err := engine.PlaceBid(ctx, auctionID, "accepted_prefix_user_"+strconv.Itoa(i), clientBidID, auction.BidInput{
			ClientBidID:   clientBidID,
			AmountCents:   15_000 + int64(i)*5_000,
			ClientSeenSeq: 0,
		}, "tr_accepted_prefix")
		if err != nil {
			t.Fatalf("place accepted bid %d: %v", i, err)
		}
		if resp.Result != auction.BidResultEngineAccepted || resp.EngineSeq != int64(i+1) {
			t.Fatalf("accepted bid %d response = %#v", i, resp)
		}
	}
	rejectResp, err := engine.PlaceBid(ctx, auctionID, "accepted_prefix_user_3", "accepted-prefix-reject", auction.BidInput{
		ClientBidID:   "accepted-prefix-reject",
		AmountCents:   10_000,
		ClientSeenSeq: 0,
	}, "tr_accepted_prefix_reject")
	if err != nil {
		t.Fatalf("place rejected bid: %v", err)
	}
	if rejectResp.Result != auction.BidResultEngineRejected || rejectResp.EngineSeq != accepted+1 {
		t.Fatalf("reject response = %#v", rejectResp)
	}
	ensureLedgerMessages(t, ctx, worker, accepted+1)

	processed, err := worker.ProcessKafka(ctx, 10)
	if err != nil {
		t.Fatalf("mixed settlement: %v", err)
	}
	if processed != accepted+1 {
		t.Fatalf("mixed settlement processed = %d, want accepted prefix plus rejected suffix %d", processed, accepted+1)
	}
	assertAuctionEngineSeq(t, db, auctionID, accepted+1, 25_000, "ACTIVE")
	assertAuctionPublicSeq(t, db, auctionID, accepted)

	var acceptedRows, rejectedRows, settlements int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND status = 'ACCEPTED' AND settlement_status = 'SETTLED'`, auctionID).Scan(&acceptedRows); err != nil {
		t.Fatalf("count accepted bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND status = 'REJECTED' AND settlement_status = 'SETTLED'`, auctionID).Scan(&rejectedRows); err != nil {
		t.Fatalf("count rejected bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND status = 'SETTLED'`, auctionID).Scan(&settlements); err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if acceptedRows != accepted || rejectedRows != 1 || settlements != accepted+1 {
		t.Fatalf("rows accepted=%d rejected=%d settlements=%d, want %d/1/%d", acceptedRows, rejectedRows, settlements, accepted, accepted+1)
	}
}

func TestKafkaSettlementBatchesAcceptedSuffixAfterReject(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	const accepted = 4
	insertEngineUsers(t, db, "accepted_suffix_user_", accepted+1)
	bids := []struct {
		user   string
		id     string
		amount int64
		result string
	}{
		{"accepted_suffix_user_0", "accepted-suffix-0", 15_000, auction.BidResultEngineAccepted},
		{"accepted_suffix_user_1", "accepted-suffix-reject", 10_000, auction.BidResultEngineRejected},
		{"accepted_suffix_user_2", "accepted-suffix-1", 20_000, auction.BidResultEngineAccepted},
		{"accepted_suffix_user_3", "accepted-suffix-2", 25_000, auction.BidResultEngineAccepted},
		{"accepted_suffix_user_4", "accepted-suffix-3", 30_000, auction.BidResultEngineAccepted},
	}
	for i, bid := range bids {
		resp, err := engine.PlaceBid(ctx, auctionID, bid.user, bid.id, auction.BidInput{
			ClientBidID:   bid.id,
			AmountCents:   bid.amount,
			ClientSeenSeq: 0,
		}, "tr_accepted_suffix")
		if err != nil {
			t.Fatalf("place bid %d: %v", i, err)
		}
		if resp.Result != bid.result || resp.EngineSeq != int64(i+1) {
			t.Fatalf("bid %d response = %#v, want result=%s engine_seq=%d", i, resp, bid.result, i+1)
		}
	}
	ensureLedgerMessages(t, ctx, worker, len(bids))

	processed, err := worker.ProcessKafka(ctx, 10)
	if err != nil {
		t.Fatalf("suffix settlement: %v", err)
	}
	if processed != len(bids) {
		t.Fatalf("suffix settlement processed = %d, want %d", processed, len(bids))
	}
	assertAuctionEngineSeq(t, db, auctionID, int64(len(bids)), 30_000, "ACTIVE")
	assertAuctionPublicSeq(t, db, auctionID, accepted)

	var acceptedRows, rejectedRows, settlements int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND status = 'ACCEPTED' AND settlement_status = 'SETTLED'`, auctionID).Scan(&acceptedRows); err != nil {
		t.Fatalf("count accepted bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND status = 'REJECTED' AND settlement_status = 'SETTLED'`, auctionID).Scan(&rejectedRows); err != nil {
		t.Fatalf("count rejected bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND status = 'SETTLED'`, auctionID).Scan(&settlements); err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if acceptedRows != accepted || rejectedRows != 1 || settlements != len(bids) {
		t.Fatalf("rows accepted=%d rejected=%d settlements=%d, want %d/1/%d", acceptedRows, rejectedRows, settlements, accepted, len(bids))
	}
}

func TestRelayBackpressureDrainsBeyondBatchCeiling(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	const bidders = relayBatchSize + 88
	insertEngineUsers(t, db, "user_relay_pressure_", bidders)

	for i := 0; i < bidders; i++ {
		clientBidID := fmt.Sprintf("relay-pressure-%03d", i)
		resp, err := engine.PlaceBid(ctx, auctionID, "user_relay_pressure_"+strconv.Itoa(i), clientBidID, auction.BidInput{
			ClientBidID:   clientBidID,
			AmountCents:   15_000 + int64(i)*5_000,
			ClientSeenSeq: 0,
		}, "tr_relay_pressure")
		if err != nil {
			t.Fatalf("bid %d: %v", i, err)
		}
		if resp.DecisionStatus != auction.DecisionStatusDecided || resp.DurabilityStatus != auction.DurabilityStatusEngineDurable {
			t.Fatalf("bid %d = %#v, want DECIDED+ENGINE_DURABLE", i, resp)
		}
	}

	streamLen, err := rdb.XLen(ctx, redisx.BidEngineLogStreamKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("xlen: %v", err)
	}
	if streamLen != bidders {
		t.Fatalf("stream len = %d, want %d", streamLen, bidders)
	}
	probe, err := rdb.XRangeN(ctx, redisx.BidEngineLogStreamKey(auctionID), "-", "+", relayBatchSize).Result()
	if err != nil {
		t.Fatalf("probe xrange: %v", err)
	}
	if len(probe) == 0 {
		t.Fatalf("probe xrange returned no messages")
	}
	validPayloads := 0
	invalidPayloads := 0
	minSeq := int64(1<<63 - 1)
	maxSeq := int64(0)
	for _, msg := range probe {
		payload, ok := msg.Values["payload"].(string)
		if !ok || payload == "" {
			invalidPayloads++
			continue
		}
		var result engineResult
		if err := json.Unmarshal([]byte(payload), &result); err != nil || result.AuctionID != auctionID {
			invalidPayloads++
			continue
		}
		if result.EngineSeq < minSeq {
			minSeq = result.EngineSeq
		}
		if result.EngineSeq > maxSeq {
			maxSeq = result.EngineSeq
		}
		validPayloads++
	}
	if validPayloads != relayBatchSize || invalidPayloads != 0 || minSeq != 1 || maxSeq != relayBatchSize {
		t.Fatalf("first stream page valid=%d invalid=%d min_seq=%d max_seq=%d, want %d/0/1/%d", validPayloads, invalidPayloads, minSeq, maxSeq, relayBatchSize, relayBatchSize)
	}
	if err := rdb.Del(ctx, redisx.BidEngineRelayCursorKey(auctionID)).Err(); err != nil {
		t.Fatalf("reset relay cursor: %v", err)
	}
	ledger = NewMemoryLedger()
	worker = NewWorker(db, rdb, ledger, "test-"+uuid.NewString())

	first, err := worker.relayAuctionLogBatch(ctx, auctionID)
	if err != nil {
		t.Fatalf("relay first: %v", err)
	}
	if first <= 0 || first >= bidders {
		t.Fatalf("first relay processed = %d, want partial progress below full backlog %d", first, bidders)
	}
	if ledger.Len() != first {
		t.Fatalf("ledger len after first relay = %d, want %d", ledger.Len(), first)
	}
	total := first
	for i := 0; i < 20 && total < bidders; i++ {
		n, err := worker.relayAuctionLogBatch(ctx, auctionID)
		if err != nil {
			t.Fatalf("relay drain %d: %v", i, err)
		}
		total += n
		if n == 0 {
			break
		}
	}
	if total != bidders {
		cursor, _ := rdb.Get(ctx, redisx.BidEngineRelayCursorKey(auctionID)).Result()
		afterCursor, _ := rdb.XRange(ctx, redisx.BidEngineLogStreamKey(auctionID), cursor, "+").Result()
		t.Fatalf("total relayed = %d, want %d; stream len=%d cursor=%s ledger=%d xrange_from_cursor=%d", total, bidders, streamLen, cursor, ledger.Len(), len(afterCursor))
	}
	if ledger.Len() != bidders {
		t.Fatalf("ledger len after drain = %d, want %d", ledger.Len(), bidders)
	}
	pendingFinal, err := rdb.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("pending final: %v", err)
	}
	if pendingFinal != 0 {
		t.Fatalf("pending final = %d, want 0", pendingFinal)
	}
	n, err := worker.relayAuctionLogBatch(ctx, auctionID)
	if err != nil {
		t.Fatalf("relay after cursor: %v", err)
	}
	if n != 0 {
		t.Fatalf("relay after cursor processed = %d, want 0", n)
	}
	assertEngineNotPaused(t, db, auctionID)
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
	ensureLedgerMessages(t, ctx, worker, 1)
	msg := memoryLedgerMessage(t, ctx, worker, 0)
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

func TestKafkaSettlementTripleDuplicateMessageHasSingleBusinessEffect(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 20_000)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-triple-dup", auction.BidInput{
		ClientBidID:   "redis-ledger-triple-dup",
		AmountCents:   20_000,
		ClientSeenSeq: 0,
	}, "tr_triple_dup"); err != nil {
		t.Fatalf("place sold bid: %v", err)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	msg := memoryLedgerMessage(t, ctx, worker, 0)

	for i := 0; i < 3; i++ {
		if err := worker.settleLedgerMessage(ctx, msg); err != nil {
			t.Fatalf("duplicate settlement %d: %v", i+1, err)
		}
	}

	var settlementRows, settledRows, bidRows, orderRows, outboxEvents, outboxDeliveries, missingDeliveries, duplicateDeliveries int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND engine_seq = 1`, auctionID).Scan(&settlementRows); err != nil {
		t.Fatalf("count settlements: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM redis_engine_settlements WHERE auction_id = $1 AND engine_seq = 1 AND status = 'SETTLED'`, auctionID).Scan(&settledRows); err != nil {
		t.Fatalf("count settled: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND engine_seq = 1`, auctionID).Scan(&bidRows); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE auction_id = $1`, auctionID).Scan(&orderRows); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE auction_id = $1`, auctionID).Scan(&outboxEvents); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_delivery d
		JOIN outbox_events e ON e.id = d.outbox_id
		WHERE e.auction_id = $1
	`, auctionID).Scan(&outboxDeliveries); err != nil {
		t.Fatalf("count outbox deliveries: %v", err)
	}

	if settlementRows != 1 || settledRows != 1 || bidRows != 1 || orderRows != 1 {
		t.Fatalf("business rows settlement=%d settled=%d bids=%d orders=%d, want 1/1/1/1", settlementRows, settledRows, bidRows, orderRows)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		LEFT JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND d.outbox_id IS NULL
	`, auctionID).Scan(&missingDeliveries); err != nil {
		t.Fatalf("count missing deliveries: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM (
			SELECT d.outbox_id, count(*) AS cnt
			FROM outbox_delivery d
			JOIN outbox_events e ON e.id = d.outbox_id
			WHERE e.auction_id = $1
			GROUP BY d.outbox_id
			HAVING count(*) > 1
		) dup
	`, auctionID).Scan(&duplicateDeliveries); err != nil {
		t.Fatalf("count duplicate deliveries: %v", err)
	}

	if outboxEvents != 1 || outboxDeliveries != 1 || missingDeliveries != 0 || duplicateDeliveries != 0 {
		t.Fatalf("outbox events=%d deliveries=%d missing=%d duplicate=%d, want 1/1/0/0", outboxEvents, outboxDeliveries, missingDeliveries, duplicateDeliveries)
	}
	assertEngineNotPaused(t, db, auctionID)
}

func TestKafkaSettlementSameSeqDifferentPayloadFailsAndPauses(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-seq-payload-conflict", auction.BidInput{
		ClientBidID:   "redis-ledger-seq-payload-conflict",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_seq_payload_conflict"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	msg := memoryLedgerMessage(t, ctx, worker, 0)
	if err := worker.settleLedgerMessage(ctx, msg); err != nil {
		t.Fatalf("first settlement: %v", err)
	}
	var conflicting engineResult
	if err := json.Unmarshal(msg.Value, &conflicting); err != nil {
		t.Fatalf("decode original payload: %v", err)
	}
	conflicting.AmountCents = 20_000
	conflicting.RequestHash = requestHash(auctionID, "user_1", conflicting.ClientBidID, 20_000)
	conflicting.CurrentPriceCents = 20_000
	payload, err := json.Marshal(conflicting)
	if err != nil {
		t.Fatalf("encode conflicting payload: %v", err)
	}
	conflictMsg := msg
	conflictMsg.ID = "kafka:auction.bid-events:0:9998"
	conflictMsg.Offset = 9998
	conflictMsg.Value = payload

	err = worker.settleLedgerMessage(ctx, conflictMsg)
	if !isSettlementIdentityConflictError(err) {
		t.Fatalf("conflicting duplicate err = %v, want settlement identity conflict", err)
	}
	assertEnginePaused(t, db, auctionID, "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT")
	var status string
	var conflictStream string
	var conflictHash string
	if err := db.QueryRow(ctx, `
		SELECT status, COALESCE(conflict_stream_id, ''), COALESCE(conflict_payload_sha256, '')
		FROM redis_engine_settlements
		WHERE auction_id = $1 AND engine_seq = 1
	`, auctionID).Scan(&status, &conflictStream, &conflictHash); err != nil {
		t.Fatalf("load settlement conflict marker: %v", err)
	}
	if status != "FAILED" || conflictStream != conflictMsg.ID || conflictHash == "" {
		t.Fatalf("settlement status=%q conflictStream=%q conflictHash=%q, want FAILED with conflict details", status, conflictStream, conflictHash)
	}
}

func TestKafkaSettlementSameOffsetDifferentPayloadFailsAndPauses(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-offset-payload-conflict", auction.BidInput{
		ClientBidID:   "redis-ledger-offset-payload-conflict",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_offset_payload_conflict"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	ensureLedgerMessages(t, ctx, worker, 1)
	msg := memoryLedgerMessage(t, ctx, worker, 0)
	if err := worker.settleLedgerMessage(ctx, msg); err != nil {
		t.Fatalf("first settlement: %v", err)
	}
	var conflicting engineResult
	if err := json.Unmarshal(msg.Value, &conflicting); err != nil {
		t.Fatalf("decode original payload: %v", err)
	}
	conflicting.EngineSeq = 2
	conflicting.BidID = "bid_offset_conflict_" + uuid.NewString()
	conflicting.ClientBidID = "redis-ledger-offset-payload-conflict-2"
	conflicting.AmountCents = 20_000
	conflicting.CurrentPriceCents = 20_000
	conflicting.RequestHash = requestHash(auctionID, "user_1", conflicting.ClientBidID, 20_000)
	payload, err := json.Marshal(conflicting)
	if err != nil {
		t.Fatalf("encode conflicting payload: %v", err)
	}
	conflictMsg := msg
	conflictMsg.Value = payload

	err = worker.settleLedgerMessage(ctx, conflictMsg)
	if !isSettlementIdentityConflictError(err) {
		t.Fatalf("same offset different payload err = %v, want settlement identity conflict", err)
	}
	assertEnginePaused(t, db, auctionID, "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT")
	var lastErr string
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(last_error, '')
		FROM redis_engine_settlements
		WHERE auction_id = $1 AND engine_seq = 1
	`, auctionID).Scan(&lastErr); err != nil {
		t.Fatalf("load settlement conflict marker: %v", err)
	}
	if !strings.Contains(lastErr, "stream") && !strings.Contains(lastErr, "offset") {
		t.Fatalf("lastErr=%q, want stream/offset payload conflict", lastErr)
	}
}

func TestKafkaSettlementSameClientBidDifferentRequestHashFailsAndPauses(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	clientBidID := "redis-ledger-client-hash-conflict"
	first := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_client_hash_1_" + uuid.NewString(),
		AuctionID:         auctionID,
		UserID:            "user_1",
		ClientBidID:       clientBidID,
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_client_hash_1",
		RequestHash:       requestHash(auctionID, "user_1", clientBidID, 15_000),
	}
	second := first
	second.BidID = "bid_client_hash_2_" + uuid.NewString()
	second.AmountCents = 20_000
	second.EngineSeq = 2
	second.CurrentPriceCents = 20_000
	second.TraceID = "tr_client_hash_2"
	second.RequestHash = requestHash(auctionID, "user_1", clientBidID, 20_000)
	if _, err := ledger.Append(ctx, first); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if _, err := ledger.Append(ctx, second); err != nil {
		t.Fatalf("append second: %v", err)
	}
	processed, err := worker.ProcessKafka(ctx, 1)
	if err != nil {
		t.Fatalf("settle first: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed first = %d, want 1", processed)
	}
	processed, err = worker.ProcessKafka(ctx, 1)
	if err != nil {
		t.Fatalf("process client hash conflict: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want conflict offset committed", processed)
	}
	assertEnginePaused(t, db, auctionID, "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT")
	var status string
	var lastErr string
	if err := db.QueryRow(ctx, `
		SELECT status, COALESCE(last_error, '')
		FROM redis_engine_settlements
		WHERE auction_id = $1 AND engine_seq = 2
	`, auctionID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("load second settlement: %v", err)
	}
	if status != "FAILED" || !strings.Contains(lastErr, "bid idempotency conflict") {
		t.Fatalf("second settlement status=%q lastErr=%q, want FAILED bid idempotency conflict", status, lastErr)
	}
	var bids int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND client_bid_id = $2`, auctionID, clientBidID).Scan(&bids); err != nil {
		t.Fatalf("count bids: %v", err)
	}
	if bids != 1 {
		t.Fatalf("bids = %d, want original bid only", bids)
	}
}

func TestRejectedBatchSetBasedSettlementSettlesContiguousRejects(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	rejectReason := "BID_TOO_LOW"
	for i := 1; i <= 2; i++ {
		userID := "user_" + strconv.Itoa(i)
		clientBidID := "redis-ledger-reject-batch-ok-" + strconv.Itoa(i)
		result := engineResult{
			Result:            resultRejected,
			BidID:             "bid_reject_batch_ok_" + strconv.Itoa(i) + "_" + uuid.NewString(),
			AuctionID:         auctionID,
			UserID:            userID,
			ClientBidID:       clientBidID,
			AmountCents:       10_000,
			RejectReason:      &rejectReason,
			EngineSeq:         int64(i),
			EngineEpoch:       1,
			SettlementStatus:  auction.SettlementStatusPending,
			CurrentPriceCents: 10_000,
			ServerTimeMS:      time.Now().UTC().UnixMilli(),
			TraceID:           "tr_reject_batch_ok_" + strconv.Itoa(i),
			RequestHash:       requestHash(auctionID, userID, clientBidID, 10_000),
		}
		if _, err := ledger.Append(ctx, result); err != nil {
			t.Fatalf("append rejected %d: %v", i, err)
		}
	}

	processed, err := worker.ProcessKafka(ctx, 10)
	if err != nil {
		t.Fatalf("process rejected batch: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want 2", processed)
	}
	if memoryLedgerNext(t, ledger) != 2 {
		t.Fatalf("memory ledger next = %d, want 2", memoryLedgerNext(t, ledger))
	}
	var engineSeq int64
	if err := db.QueryRow(ctx, `SELECT engine_seq FROM auctions WHERE id = $1`, auctionID).Scan(&engineSeq); err != nil {
		t.Fatalf("load auction engine_seq: %v", err)
	}
	if engineSeq != 2 {
		t.Fatalf("auction engine_seq = %d, want 2", engineSeq)
	}
	var settled int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM redis_engine_settlements
		WHERE auction_id = $1 AND status = 'SETTLED'
	`, auctionID).Scan(&settled); err != nil {
		t.Fatalf("count settled settlements: %v", err)
	}
	if settled != 2 {
		t.Fatalf("settled settlements = %d, want 2", settled)
	}
	var rejectedBids int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM bids
		WHERE auction_id = $1 AND status = 'REJECTED' AND settlement_status = 'SETTLED'
	`, auctionID).Scan(&rejectedBids); err != nil {
		t.Fatalf("count rejected bids: %v", err)
	}
	if rejectedBids != 2 {
		t.Fatalf("rejected bids = %d, want 2", rejectedBids)
	}
	var completedIdem int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM idempotency_records
		WHERE scope_type = 'bid' AND scope_id = $1 AND status = 'COMPLETED' AND result_code = $2
	`, auctionID, auction.BidResultRejected).Scan(&completedIdem); err != nil {
		t.Fatalf("count idempotency records: %v", err)
	}
	if completedIdem != 2 {
		t.Fatalf("completed idempotency records = %d, want 2", completedIdem)
	}
}

func TestRejectedBatchIdentityConflictFallsBackToSequentialSettlement(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	clientBidID := "redis-ledger-reject-batch-conflict"
	conflictingHash := requestHash(auctionID, "user_2", clientBidID, 10_000)
	if _, err := db.Exec(ctx, `
		INSERT INTO bids (
		  id, auction_id, user_id, client_bid_id, amount_cents, status,
		  reject_reason, request_hash, response_json, trace_id, source,
		  engine_epoch, engine_seq, settlement_status
		)
		VALUES ($1, $2, 'user_1', $3, 10000, 'REJECTED',
		        'BID_TOO_LOW', $4, '{}'::jsonb, 'tr_conflict_seed', 'MANUAL',
		        1, 99, 'SETTLED')
	`, "bid_conflict_seed_"+uuid.NewString(), auctionID, clientBidID, conflictingHash); err != nil {
		t.Fatalf("seed conflicting bid: %v", err)
	}
	rejectReason := "BID_TOO_LOW"
	first := engineResult{
		Result:            resultRejected,
		BidID:             "bid_reject_batch_conflict_1_" + uuid.NewString(),
		AuctionID:         auctionID,
		UserID:            "user_1",
		ClientBidID:       clientBidID,
		AmountCents:       10_000,
		RejectReason:      &rejectReason,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 10_000,
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_reject_batch_conflict_1",
		RequestHash:       requestHash(auctionID, "user_1", clientBidID, 10_000),
	}
	second := first
	second.BidID = "bid_reject_batch_conflict_2_" + uuid.NewString()
	second.UserID = "user_2"
	second.ClientBidID = "redis-ledger-reject-batch-conflict-2"
	second.EngineSeq = 2
	second.TraceID = "tr_reject_batch_conflict_2"
	second.RequestHash = requestHash(auctionID, "user_2", second.ClientBidID, second.AmountCents)
	if _, err := ledger.Append(ctx, first); err != nil {
		t.Fatalf("append first rejected: %v", err)
	}
	if _, err := ledger.Append(ctx, second); err != nil {
		t.Fatalf("append second rejected: %v", err)
	}

	processed, err := worker.ProcessKafka(ctx, 10)
	if err != nil {
		t.Fatalf("process batch fallback conflict: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want conflicted offset committed then contiguous follower settled", processed)
	}
	if memoryLedgerNext(t, ledger) != 2 {
		t.Fatalf("memory ledger next = %d, want both fetched offsets committed", memoryLedgerNext(t, ledger))
	}
	assertEnginePaused(t, db, auctionID, "KAFKA_LEDGER_SETTLEMENT_IDENTITY_CONFLICT")
	var status string
	var lastErr string
	if err := db.QueryRow(ctx, `
		SELECT status, COALESCE(last_error, '')
		FROM redis_engine_settlements
		WHERE auction_id = $1 AND engine_seq = 1
	`, auctionID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("load conflicted settlement: %v", err)
	}
	if status != "FAILED" || !strings.Contains(lastErr, "bid idempotency conflict") {
		t.Fatalf("settlement status=%q lastErr=%q, want FAILED bid idempotency conflict", status, lastErr)
	}
	if err := db.QueryRow(ctx, `
		SELECT status
		FROM redis_engine_settlements
		WHERE auction_id = $1 AND engine_seq = 2
	`, auctionID).Scan(&status); err != nil {
		t.Fatalf("load follower settlement: %v", err)
	}
	if status != "SETTLED" {
		t.Fatalf("follower settlement status=%q, want SETTLED", status)
	}
}

func TestKafkaSettlementStaleEpochPauses(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	staleAuctionID := createEngineAuction(t, db, 0)
	staleResult := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_stale_" + uuid.NewString(),
		AuctionID:         staleAuctionID,
		UserID:            "user_1",
		ClientBidID:       "stale-bid",
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       0,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_stale",
		RequestHash:       requestHash(staleAuctionID, "user_1", "stale-bid", 15_000),
	}
	if _, err := ledger.Append(ctx, staleResult); err != nil {
		t.Fatalf("append stale: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("process stale: %v", err)
	}
	assertEnginePaused(t, db, staleAuctionID, "REDIS_ENGINE_STALE_EPOCH")
}

func TestKafkaSettlementFutureSeqIsTransientAndKeepsOffsetUncommitted(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	result := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_future_" + uuid.NewString(),
		AuctionID:         auctionID,
		UserID:            "user_1",
		ClientBidID:       "future-bid",
		AmountCents:       15_000,
		EngineSeq:         2,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_future",
		RequestHash:       requestHash(auctionID, "user_1", "future-bid", 15_000),
	}
	if _, err := ledger.Append(ctx, result); err != nil {
		t.Fatalf("append future seq: %v", err)
	}
	processed, err := worker.ProcessKafka(ctx, 1)
	if !isTransientSettlementError(err) {
		t.Fatalf("process future seq err = %v, want transient", err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0", processed)
	}
	if ledger.next != 0 {
		t.Fatalf("ledger next = %d, want uncommitted offset 0", ledger.next)
	}
	var paused bool
	if err := db.QueryRow(ctx, `SELECT engine_paused FROM auctions WHERE id = $1`, auctionID).Scan(&paused); err != nil {
		t.Fatalf("load paused: %v", err)
	}
	if paused {
		t.Fatalf("future seq should not pause auction")
	}
	var status string
	var lastErr string
	if err := db.QueryRow(ctx, `SELECT status, COALESCE(last_error, '') FROM redis_engine_settlements WHERE auction_id = $1 AND engine_seq = 2`, auctionID).Scan(&status, &lastErr); err != nil {
		t.Fatalf("load settlement attempt: %v", err)
	}
	if status != "PROCESSING" || lastErr == "" {
		t.Fatalf("settlement status=%q lastErr=%q, want PROCESSING with waiting reason", status, lastErr)
	}
}

func TestKafkaSettlementDBUnavailableKeepsOffsetUncommitted(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	result := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_db_unavailable_" + uuid.NewString(),
		AuctionID:         auctionID,
		UserID:            "user_1",
		ClientBidID:       "db-unavailable-bid",
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_db_unavailable",
		RequestHash:       requestHash(auctionID, "user_1", "db-unavailable-bid", 15_000),
	}
	if _, err := ledger.Append(ctx, result); err != nil {
		t.Fatalf("append db-unavailable seq: %v", err)
	}

	db.Close()
	processed, err := worker.ProcessKafka(ctx, 1)
	if !isTransientSettlementError(err) {
		t.Fatalf("process with db unavailable err = %v, want transient", err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0", processed)
	}
	if ledger.next != 0 {
		t.Fatalf("ledger next = %d, want uncommitted offset 0", ledger.next)
	}
	if len(ledger.dlq) != 0 {
		t.Fatalf("dlq messages = %d, want 0 for transient DB outage", len(ledger.dlq))
	}
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

// TestReconcileRecoversStreamDecisionWithoutKafkaAck simulates an auction where Redis
// has a decision in the log stream (engine_seq=1) but it has not yet been relayed to
// Kafka or settled in PG. Reconcile should drain the stream via relayAuctionLogBatch,
// then settle confirms the decision.
func TestReconcileRecoversStreamDecisionWithoutKafkaAck(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	clientBidID := "reconcile-stream-pending"
	result := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_stream_" + uuid.NewString(),
		AuctionID:         auctionID,
		UserID:            "user_1",
		ClientBidID:       clientBidID,
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_stream_pending",
		RequestHash:       requestHash(auctionID, "user_1", clientBidID, 15_000),
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	// Seed Redis state ahead of DB (engine_seq=1, ACTIVE).
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{
		"status":              "ACTIVE",
		"current_price_cents": "15000",
		"current_winner_id":   "user_1",
		"engine_seq":          "1",
		"engine_epoch":        "1",
		"paused":              "0",
		"pause_reason":        "",
	})
	// Seed the decision log stream (as if the Lua XADD fired but relay hasn't run).
	if err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: redisx.BidEngineLogStreamKey(auctionID),
		Values: map[string]any{
			"engine_seq":   "1",
			"engine_epoch": "1",
			"result":       resultAccepted,
			"auction_id":   auctionID,
			"payload":      string(raw),
		},
	}).Err(); err != nil {
		t.Fatalf("seed log stream: %v", err)
	}
	if err := rdb.SAdd(ctx, redisx.BidEnginePendingAuctionsKey(), auctionID).Err(); err != nil {
		t.Fatalf("seed pending auctions set: %v", err)
	}
	if err := rdb.HSet(ctx, redisx.BidEngineIdempotencyKey(auctionID, clientBidID),
		"request_hash", result.RequestHash,
		"result_json", string(raw),
		"engine_seq", result.EngineSeq,
		"engine_epoch", result.EngineEpoch,
		"kafka_append_status", kafkaAppendStatusUnknown,
	).Err(); err != nil {
		t.Fatalf("seed idem key: %v", err)
	}

	// Reconcile: should find DB behind Redis and relay the stream entry.
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.RecoveredPending != 1 || report.Status != "DB_BEHIND_REDIS" {
		t.Fatalf("report = %#v, want RecoveredPending=1 Status=DB_BEHIND_REDIS", report)
	}
	assertEngineNotPaused(t, db, auctionID)

	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = false, engine_pause_reason = NULL, engine_paused_at = NULL
		WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("clear pause for recovered settlement: %v", err)
	}
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{"paused": 0, "pause_reason": ""})
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle recovered: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, result.EngineSeq, 15_000, "ACTIVE")
}

// TestReconcileBackfillsKafkaFromLogStream simulates a crash-window scenario where
// Redis has a live decision (engine_seq=1) in the log stream but no relay cursor yet
// (as if the relay worker crashed before its first tick). Reconcile should drain the
// stream into Kafka and then settlement can proceed.
func TestReconcileBackfillsKafkaFromLogStream(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	clientBidID := "stream-crash-bid"
	result := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_stream_crash_" + uuid.NewString(),
		AuctionID:         auctionID,
		UserID:            "user_1",
		ClientBidID:       clientBidID,
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_stream_crash",
		RequestHash:       requestHash(auctionID, "user_1", clientBidID, 15_000),
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	setRedisHashFields(t, rdb, redisx.BidEngineStateKey(auctionID), map[string]any{
		"status": "ACTIVE", "current_price_cents": "15000",
		"current_winner_id": "user_1", "engine_seq": "1", "engine_epoch": "1",
		"paused": "0", "pause_reason": "",
	})
	// Seed the log stream (relay cursor not set → relay will read from 0-0).
	if err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: redisx.BidEngineLogStreamKey(auctionID),
		Values: map[string]any{
			"engine_seq": "1", "engine_epoch": "1",
			"result": resultAccepted, "auction_id": auctionID, "payload": string(raw),
		},
	}).Err(); err != nil {
		t.Fatalf("seed log stream: %v", err)
	}
	if err := rdb.HSet(ctx, redisx.BidEngineIdempotencyKey(auctionID, clientBidID),
		"request_hash", result.RequestHash, "result_json", string(raw),
		"engine_seq", 1, "engine_epoch", 1, "kafka_append_status", kafkaAppendStatusUnknown,
	).Err(); err != nil {
		t.Fatalf("seed idem: %v", err)
	}

	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.RecoveredPending != 1 || report.Status != "DB_BEHIND_REDIS" {
		t.Fatalf("report = %#v, want RecoveredPending=1 Status=DB_BEHIND_REDIS", report)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger len = %d, want 1 (stream entry relayed to Kafka)", ledger.Len())
	}
	if _, err := db.Exec(ctx, `
		UPDATE auctions
		SET engine_paused = false, engine_pause_reason = NULL, engine_paused_at = NULL
		WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("clear pause: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")
}

func TestReconcileHealthyActiveAuctionWithoutAcceptedBids(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "OK" || report.DriftCount != 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestReconcileDetectsAcceptedPublicSeqGap(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-reconcile-gap-1", auction.BidInput{
		ClientBidID:   "redis-ledger-reconcile-gap-1",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_reconcile_gap_1"); err != nil {
		t.Fatalf("place first: %v", err)
	}
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay first: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle first: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE auction_events SET seq = 2 WHERE auction_id = $1 AND seq = 1`, auctionID); err != nil {
		t.Fatalf("corrupt event seq: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "ACCEPTED_PUBLIC_SEQ_GAP" || report.DriftCount < 1 {
		t.Fatalf("report = %#v", report)
	}
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_ACCEPTED_PUBLIC_SEQ_GAP")
}

func TestReconcileDetectsWinnerPriceDrift(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-reconcile-price", auction.BidInput{
		ClientBidID:   "redis-ledger-reconcile-price",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_reconcile_price"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	settleAllLedgerMessages(t, ctx, worker, 1)
	if _, err := db.Exec(ctx, `UPDATE auctions SET current_price_cents = 12_000 WHERE id = $1`, auctionID); err != nil {
		t.Fatalf("corrupt auction price: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "AUCTION_WINNER_PRICE_DRIFT" || report.DriftCount < 1 {
		t.Fatalf("report = %#v", report)
	}
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_WINNER_PRICE_DRIFT")
}

func TestReconcileDetectsOutboxCoverageMissing(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-reconcile-outbox", auction.BidInput{
		ClientBidID:   "redis-ledger-reconcile-outbox",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_reconcile_outbox"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay bid: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle bid: %v", err)
	}
	if _, err := db.Exec(ctx, `
		DELETE FROM outbox_delivery
		WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id = $1)
	`, auctionID); err != nil {
		t.Fatalf("delete outbox delivery: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "OUTBOX_COVERAGE_MISSING" || report.DriftCount < 1 {
		t.Fatalf("report = %#v", report)
	}
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_OUTBOX_COVERAGE_MISSING")
}

func TestKafkaSettlementWritesEngineCheckpoint(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-checkpoint", auction.BidInput{
		ClientBidID:   "redis-ledger-checkpoint",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_checkpoint")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	// v3: relay must run before ProcessKafka can find messages.
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle bid: %v", err)
	}

	var checkpointSeq int64
	var nextOffset int64
	var stateHash string
	var snapshot struct {
		AuctionID         string `json:"auction_id"`
		EngineSeq         int64  `json:"engine_seq"`
		CurrentPriceCents int64  `json:"current_price_cents"`
		CurrentWinnerID   string `json:"current_winner_id"`
	}
	var snapshotRaw []byte
	if err := db.QueryRow(ctx, `
		SELECT engine_seq, next_decision_offset, state_hash, snapshot_json
		FROM auction_engine_checkpoints
		WHERE auction_id = $1
	`, auctionID).Scan(&checkpointSeq, &nextOffset, &stateHash, &snapshotRaw); err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if checkpointSeq != resp.EngineSeq || nextOffset != 1 || stateHash == "" {
		t.Fatalf("checkpoint seq=%d next_offset=%d hash=%q, want seq=%d offset=1 non-empty hash", checkpointSeq, nextOffset, stateHash, resp.EngineSeq)
	}
	if err := json.Unmarshal(snapshotRaw, &snapshot); err != nil {
		t.Fatalf("decode checkpoint snapshot: %v", err)
	}
	if snapshot.AuctionID != auctionID || snapshot.EngineSeq != resp.EngineSeq || snapshot.CurrentPriceCents != 15_000 || snapshot.CurrentWinnerID != "user_1" {
		t.Fatalf("checkpoint snapshot = %#v", snapshot)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "OK" || report.DriftCount != 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestReconcileDetectsMissingEngineCheckpoint(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-checkpoint-missing", auction.BidInput{
		ClientBidID:   "redis-ledger-checkpoint-missing",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_checkpoint_missing"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	settleAllLedgerMessages(t, ctx, worker, 1)
	if _, err := db.Exec(ctx, `DELETE FROM auction_engine_checkpoints WHERE auction_id = $1`, auctionID); err != nil {
		t.Fatalf("delete checkpoint: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "ENGINE_CHECKPOINT_MISSING" || report.DriftCount < 1 {
		t.Fatalf("report = %#v", report)
	}
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_CHECKPOINT_MISSING")
}

func TestReconcileDetectsEngineCheckpointHashDrift(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-checkpoint-drift", auction.BidInput{
		ClientBidID:   "redis-ledger-checkpoint-drift",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_checkpoint_drift"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay bid: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle bid: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE auction_engine_checkpoints
		SET state_hash = 'corrupted-checkpoint-hash'
		WHERE auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("corrupt checkpoint hash: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "ENGINE_CHECKPOINT_STATE_HASH_DRIFT" || report.DriftCount < 1 {
		t.Fatalf("report = %#v", report)
	}
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_CHECKPOINT_STATE_HASH_DRIFT")
}

// TestReconcileDoesNotPauseWhenStreamHasPendingEntries verifies that when the log
// stream has unrelayed entries and DB is behind Redis, reconcile calls recoverPendingDecisions
// (relays them) and reports DB_BEHIND_REDIS rather than pausing arbitrarily.
func TestReconcileDoesNotPauseWhenStreamHasPendingEntries(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	// Place a bid (XADD fires in Lua, relay not yet run).
	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "reconcile-pending-stream", auction.BidInput{
		ClientBidID: "reconcile-pending-stream", AmountCents: 15_000,
	}, "tr_reconcile_stream"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	// Reconcile before relay runs: should find stream entry, relay it, report DB_BEHIND_REDIS.
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "DB_BEHIND_REDIS" {
		t.Fatalf("reconcile status = %q, want DB_BEHIND_REDIS (stream was drained)", report.Status)
	}
	if report.RecoveredPending < 1 {
		t.Fatalf("reconcile recovered = %d, want at least 1", report.RecoveredPending)
	}
	if ledger.Len() < 1 {
		t.Fatalf("ledger len = %d after reconcile, want at least 1", ledger.Len())
	}
}

func TestReconcileClearsRecoverablePauseAfterSettlementCatchesUp(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "reconcile-auto-resume", auction.BidInput{
		ClientBidID: "reconcile-auto-resume", AmountCents: 15_000,
	}, "tr_reconcile_auto_resume"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if report.Status != "DB_BEHIND_REDIS" {
		t.Fatalf("first reconcile status = %q, want DB_BEHIND_REDIS", report.Status)
	}
	assertEngineNotPaused(t, db, auctionID)

	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle recovered decision: %v", err)
	}
	report, err = worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if report.Status != "OK" || report.Paused {
		t.Fatalf("second reconcile report = %#v, want OK and unpaused", report)
	}
	assertEngineNotPaused(t, db, auctionID)

	var anomalies int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM system_anomaly_events
		WHERE auction_id = $1
		  AND type = 'REDIS_ENGINE_AUTO_RESUMED'
	`, auctionID).Scan(&anomalies); err != nil {
		t.Fatalf("count auto-resume anomaly: %v", err)
	}
	if anomalies != 0 {
		t.Fatalf("auto-resume anomalies = %d, want 0", anomalies)
	}
}

func TestReconcileClearsRecoverableScriptErrorPauseWhenConsistent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	if _, err := engine.PlaceBid(ctx, auctionID, "user_1", "reconcile-script-error", auction.BidInput{
		ClientBidID: "reconcile-script-error", AmountCents: 15_000,
	}, "tr_reconcile_script_error"); err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if _, err := worker.ProcessPendingAppends(ctx, 10); err != nil {
		t.Fatalf("relay decision: %v", err)
	}
	if _, err := worker.ProcessKafka(ctx, 10); err != nil {
		t.Fatalf("settle decision: %v", err)
	}
	if err := worker.pause(ctx, auctionID, "REDIS_ENGINE_SCRIPT_ERROR", "redis connection refused during fault", "", nil); err != nil {
		t.Fatalf("pause script error: %v", err)
	}
	assertEnginePaused(t, db, auctionID, "REDIS_ENGINE_SCRIPT_ERROR")

	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Status != "OK" || report.Paused {
		t.Fatalf("reconcile report = %#v, want OK and unpaused", report)
	}
	assertEngineNotPaused(t, db, auctionID)
}

// TestRelayCursorAdvancesAfterBatch verifies the relay cursor advances so subsequent
// passes don't re-produce already-relayed stream entries.
func TestRelayCursorAdvancesAfterBatch(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	for i := 1; i <= 3; i++ {
		if _, err := engine.PlaceBid(ctx, auctionID, "user_"+strconv.Itoa(i),
			"cursor-"+strconv.Itoa(i), auction.BidInput{
				ClientBidID: "cursor-" + strconv.Itoa(i),
				AmountCents: 15_000 + int64(i)*5_000,
			}, "tr_cursor"); err != nil {
			t.Fatalf("bid %d: %v", i, err)
		}
	}
	n1, err := worker.ProcessPendingAppends(ctx, 100)
	if err != nil {
		t.Fatalf("relay 1: %v", err)
	}
	if n1 != 3 {
		t.Fatalf("relay 1 processed = %d, want 3", n1)
	}
	n2, err := worker.ProcessPendingAppends(ctx, 100)
	if err != nil {
		t.Fatalf("relay 2: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("relay 2 processed = %d, want 0 (cursor advanced)", n2)
	}
	if ledger.Len() != 3 {
		t.Fatalf("ledger len = %d, want 3 (no duplicates)", ledger.Len())
	}
}

// TestBatchSettlementInsertsSettledDirectly verifies that the rejected-batch
// and accepted-batch paths insert redis_engine_settlements with status=SETTLED
// and settled_at set immediately — never with an intermediate PROCESSING row.
// This documents P1b: the previous PROCESSING→SETTLED double-write is eliminated.
func TestBatchSettlementInsertsSettledDirectly(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	rejectReason := "BID_TOO_LOW"

	// Append two contiguous rejects to trigger the batch path.
	for i := 1; i <= 2; i++ {
		userID := "user_" + strconv.Itoa(i)
		clientBidID := "direct-settle-reject-" + strconv.Itoa(i)
		result := engineResult{
			Result:       resultRejected,
			BidID:        "bid_direct_" + strconv.Itoa(i) + "_" + uuid.NewString(),
			AuctionID:    auctionID,
			UserID:       userID,
			ClientBidID:  clientBidID,
			AmountCents:  5_000,
			RejectReason: &rejectReason,
			EngineSeq:    int64(i),
			EngineEpoch:  1,
			TraceID:      "tr_direct_" + strconv.Itoa(i),
			RequestHash:  requestHash(auctionID, userID, clientBidID, 5_000),
		}
		if _, err := ledger.Append(ctx, result); err != nil {
			t.Fatalf("append reject %d: %v", i, err)
		}
	}

	processed, err := worker.ProcessKafka(ctx, 10)
	if err != nil {
		t.Fatalf("process kafka: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed=%d want 2", processed)
	}

	// No PROCESSING rows should exist — the batch inserts SETTLED directly.
	var processingCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM redis_engine_settlements
		WHERE auction_id = $1 AND status = 'PROCESSING'
	`, auctionID).Scan(&processingCount); err != nil {
		t.Fatalf("count PROCESSING: %v", err)
	}
	if processingCount != 0 {
		t.Fatalf("PROCESSING rows=%d want 0: batch path must insert SETTLED directly (P1b)", processingCount)
	}

	// All rows must be SETTLED with settled_at set.
	var settledCount int
	var nullSettledAt int
	if err := db.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE settled_at IS NULL)
		FROM redis_engine_settlements
		WHERE auction_id = $1
	`, auctionID).Scan(&settledCount, &nullSettledAt); err != nil {
		t.Fatalf("count settled: %v", err)
	}
	if settledCount != 2 {
		t.Fatalf("settled=%d want 2", settledCount)
	}
	if nullSettledAt != 0 {
		t.Fatalf("null settled_at=%d want 0: settled_at must be set on batch insert", nullSettledAt)
	}
}

type failingLedger struct{}

func (failingLedger) Append(context.Context, engineResult) (LedgerMessage, error) {
	return LedgerMessage{}, errors.New("kafka unavailable")
}

func (failingLedger) AppendBatch(_ context.Context, results []engineResult) ([]LedgerMessage, error) {
	return nil, errors.New("kafka unavailable")
}

func (failingLedger) Fetch(context.Context) (LedgerMessage, error) {
	return LedgerMessage{}, context.Canceled
}

func (failingLedger) Commit(context.Context, LedgerMessage) error          { return nil }
func (failingLedger) WriteDLQ(context.Context, LedgerMessage, error) error { return nil }
func (failingLedger) Close() error                                         { return nil }

type staticBatchLedger struct {
	messages  []LedgerMessage
	fetched   bool
	committed int
}

func (l *staticBatchLedger) Append(context.Context, engineResult) (LedgerMessage, error) {
	return LedgerMessage{}, errors.New("static batch ledger is read-only")
}

func (l *staticBatchLedger) AppendBatch(context.Context, []engineResult) ([]LedgerMessage, error) {
	return nil, errors.New("static batch ledger is read-only")
}

func (l *staticBatchLedger) Fetch(ctx context.Context) (LedgerMessage, error) {
	messages, err := l.FetchBatch(ctx, 1)
	if err != nil {
		return LedgerMessage{}, err
	}
	if len(messages) == 0 {
		return LedgerMessage{}, context.Canceled
	}
	return messages[0], nil
}

func (l *staticBatchLedger) FetchBatch(context.Context, int) ([]LedgerMessage, error) {
	if l.fetched {
		return nil, context.Canceled
	}
	l.fetched = true
	return append([]LedgerMessage(nil), l.messages...), nil
}

func (l *staticBatchLedger) Commit(context.Context, LedgerMessage) error {
	return l.CommitBatch(context.Background(), []LedgerMessage{{}})
}

func (l *staticBatchLedger) CommitBatch(_ context.Context, messages []LedgerMessage) error {
	l.committed += len(messages)
	return nil
}

func (l *staticBatchLedger) WriteDLQ(context.Context, LedgerMessage, error) error { return nil }
func (l *staticBatchLedger) Close() error                                         { return nil }

func assertAPIErrorCode(t *testing.T, err error, want apierrors.Code) {
	t.Helper()
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != want {
		t.Fatalf("error = %v, want API code %s", err, want)
	}
}

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

func assertEngineNotPaused(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	var paused bool
	var reason string
	if err := db.QueryRow(context.Background(), `SELECT engine_paused, COALESCE(engine_pause_reason, '') FROM auctions WHERE id = $1`, auctionID).Scan(&paused, &reason); err != nil {
		t.Fatalf("load pause: %v", err)
	}
	if paused {
		t.Fatalf("pause state paused=%v reason=%q, want not paused", paused, reason)
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

func cleanupRedisEngineKeys(t *testing.T, rdb *redis.Client) {
	t.Helper()
	ctx := context.Background()
	patterns := []string{
		"bid:{*}:engine:state",
		"bid:{*}:engine:pending",
		"bid:{*}:engine:pending:append-lock",
		"bid:{*}:engine:idem:*",
		// v3 group-commit relay keys
		"bid:{*}:engine:log",          // decision log stream (WAL)
		"bid:{*}:engine:relay-cursor", // relay position cursor
	}
	for _, pattern := range patterns {
		var cursor uint64
		for {
			keys, next, err := rdb.Scan(ctx, cursor, pattern, 1000).Result()
			if err != nil {
				t.Fatalf("scan redis keys %s: %v", pattern, err)
			}
			if len(keys) > 0 {
				if err := rdb.Del(ctx, keys...).Err(); err != nil {
					t.Fatalf("delete redis keys %s: %v", pattern, err)
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	if err := rdb.Del(ctx, redisx.BidEnginePendingAuctionsKey()).Err(); err != nil {
		t.Fatalf("delete redis pending auction index: %v", err)
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
	drainRelayTriggersForTest()
	relayTriggerSuppressed.Store(true)
	kafkaRelayUnhealthy.Store(false)
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
	cleanupRedisEngineKeys(t, client)
	t.Cleanup(func() {
		drainRelayTriggersForTest()
		relayTriggerSuppressed.Store(false)
		kafkaRelayUnhealthy.Store(false)
	})
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
	if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ('user_3', 'user', 'Engine User 3') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert user3: %v", err)
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
	endAt := time.Now().UTC().Add(5 * time.Minute)
	if _, err := db.Exec(ctx, `
		INSERT INTO auctions (
		  id, room_id, item_id, status, current_price_cents, start_price_cents,
		  increment_cents, cap_price_cents, end_at
		)
		VALUES ($1, $2, $3, 'ACTIVE', 10000, 10000, 5000, $4, $5)
	`, auctionID, roomID, itemID, capArg, endAt); err != nil {
		t.Fatalf("insert auction: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `UPDATE auctions SET status = 'ENDED', updated_at = now() WHERE id = $1 AND status = 'ACTIVE'`, auctionID)
	})
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
		VALUES ($1, 'user_1', 'viewer', 'ACTIVE'), ($1, 'user_2', 'viewer', 'ACTIVE'), ($1, 'user_3', 'viewer', 'ACTIVE')
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

func assertMaxBidIntentState(t *testing.T, db *pgxpool.Pool, intentID string, wantStatus auction.MaxBidIntentStatus, wantLastApplied *int64) {
	t.Helper()
	var status string
	var lastApplied *int64
	if err := db.QueryRow(context.Background(), `
		SELECT status, last_applied_seq
		FROM max_bid_intents
		WHERE id = $1
	`, intentID).Scan(&status, &lastApplied); err != nil {
		t.Fatalf("load max bid intent: %v", err)
	}
	if status != string(wantStatus) {
		t.Fatalf("max bid intent status = %s, want %s", status, wantStatus)
	}
	if wantLastApplied == nil {
		if lastApplied != nil {
			t.Fatalf("max bid intent last_applied_seq = %v, want nil", *lastApplied)
		}
		return
	}
	if lastApplied == nil || *lastApplied != *wantLastApplied {
		t.Fatalf("max bid intent last_applied_seq = %v, want %d", lastApplied, *wantLastApplied)
	}
}

func assertMaxBidIntentForUser(t *testing.T, db *pgxpool.Pool, auctionID string, userID string, wantStatus auction.MaxBidIntentStatus, wantLastApplied *int64) {
	t.Helper()
	var intentID string
	if err := db.QueryRow(context.Background(), `
		SELECT id
		FROM max_bid_intents
		WHERE auction_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, auctionID, userID).Scan(&intentID); err != nil {
		t.Fatalf("load max bid intent for %s: %v", userID, err)
	}
	assertMaxBidIntentState(t, db, intentID, wantStatus, wantLastApplied)
}

func assertAuctionPublicSeq(t *testing.T, db *pgxpool.Pool, auctionID string, wantSeq int64) {
	t.Helper()
	var seq int64
	if err := db.QueryRow(context.Background(), `SELECT seq FROM auctions WHERE id = $1`, auctionID).Scan(&seq); err != nil {
		t.Fatalf("load auction public seq: %v", err)
	}
	if seq != wantSeq {
		t.Fatalf("auction public seq = %d, want %d", seq, wantSeq)
	}
}

func assertBidPublicSeq(t *testing.T, db *pgxpool.Pool, auctionID string, clientBidID string, wantSeq *int64) {
	t.Helper()
	var seq *int64
	if err := db.QueryRow(context.Background(), `SELECT seq FROM bids WHERE auction_id = $1 AND client_bid_id = $2`, auctionID, clientBidID).Scan(&seq); err != nil {
		t.Fatalf("load bid public seq: %v", err)
	}
	if wantSeq == nil {
		if seq != nil {
			t.Fatalf("bid public seq = %d, want nil", *seq)
		}
		return
	}
	if seq == nil || *seq != *wantSeq {
		if seq == nil {
			t.Fatalf("bid public seq = nil, want %d", *wantSeq)
		}
		t.Fatalf("bid public seq = %d, want %d", *seq, *wantSeq)
	}
}

// settleAllLedgerMessages relays all pending stream entries to Kafka (v3) and then
// settles them in PostgreSQL. In v3 the relay must run before ProcessKafka can
// consume any messages, since PlaceBid no longer produces to the ledger directly.
func settleAllLedgerMessages(t *testing.T, ctx context.Context, worker *Worker, want int, exact ...bool) {
	t.Helper()
	startNext := memoryLedgerNext(t, worker.ledger)
	ensureLedgerMessages(t, ctx, worker, startNext+want)
	targetLen := memoryLedgerLen(t, worker.ledger)
	processed := 0
	var lastErr error
	available := targetLen - startNext
	for attempts := 0; memoryLedgerNext(t, worker.ledger) < targetLen && attempts < available*available+20; attempts++ {
		n, err := worker.ProcessKafka(ctx, 1)
		if err != nil && !isTransientSettlementError(err) {
			t.Fatalf("settle ledger message: %v", err)
		}
		if err != nil {
			lastErr = err
		}
		processed += n
	}
	if memoryLedgerNext(t, worker.ledger) != targetLen || processed < want {
		t.Fatalf("settled ledger messages = %d next=%d target=%d want at least %d, last transient error: %v", processed, memoryLedgerNext(t, worker.ledger), targetLen, want, lastErr)
	}
	if len(exact) > 0 && exact[0] && processed != want {
		t.Fatalf("settled ledger messages = %d, want exactly %d", processed, want)
	}
}

func ensureLedgerMessages(t *testing.T, ctx context.Context, worker *Worker, wantLen int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if memoryLedgerLen(t, worker.ledger) >= wantLen {
			return
		}
		n, err := worker.ProcessPendingAppends(ctx, 100)
		if err != nil {
			t.Fatalf("relay: %v", err)
		}
		if n == 0 {
			time.Sleep(20 * time.Millisecond)
		}
		if memoryLedgerLen(t, worker.ledger) >= wantLen {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ledger messages = %d, want at least %d", memoryLedgerLen(t, worker.ledger), wantLen)
		}
	}
}

func drainRelayTriggersForTest() {
	for {
		select {
		case <-relayTriggerCh:
		default:
			return
		}
	}
}

func memoryLedgerMessage(t *testing.T, ctx context.Context, worker *Worker, index int) LedgerMessage {
	t.Helper()
	ensureLedgerMessages(t, ctx, worker, index+1)
	memory, ok := worker.ledger.(*MemoryLedger)
	if !ok {
		t.Fatalf("memoryLedgerMessage requires MemoryLedger, got %T", worker.ledger)
	}
	msg, ok := memory.Message(index)
	if !ok {
		t.Fatalf("missing memory ledger message index=%d len=%d", index, memoryLedgerLen(t, worker.ledger))
	}
	return msg
}

func memoryLedgerLen(t *testing.T, ledger BidLedger) int {
	t.Helper()
	memory, ok := ledger.(*MemoryLedger)
	if !ok {
		t.Fatalf("settleAllLedgerMessages requires MemoryLedger, got %T", ledger)
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	return len(memory.messages)
}

func memoryLedgerNext(t *testing.T, ledger BidLedger) int {
	t.Helper()
	memory, ok := ledger.(*MemoryLedger)
	if !ok {
		t.Fatalf("settleAllLedgerMessages requires MemoryLedger, got %T", ledger)
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	return memory.next
}

func insertEngineUsers(t *testing.T, db *pgxpool.Pool, prefix string, count int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		userID := prefix + strconv.Itoa(i)
		if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ($1, 'user', $2) ON CONFLICT DO NOTHING`, userID, userID); err != nil {
			t.Fatalf("insert engine user %s: %v", userID, err)
		}
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
