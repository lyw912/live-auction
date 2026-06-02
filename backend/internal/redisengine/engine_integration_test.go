package redisengine

import (
	"context"
	"encoding/json"
	"errors"
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
	auctionID := createEngineAuction(t, db, 0)

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
	for ledger.Len() < bidders {
		processed, err := worker.ProcessPendingAppends(ctx, 100)
		if err != nil {
			t.Fatalf("relay batch: %v", err)
		}
		if processed == 0 && ledger.Len() < bidders {
			t.Fatalf("ledger messages = %d, want %d but relay processed none", ledger.Len(), bidders)
		}
	}
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
	for ledger.Len() < 8 {
		if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
			t.Fatalf("relay: %v", err)
		}
	}
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
	for ledger.Len() < bidders {
		processed, err := worker.ProcessPendingAppends(ctx, 100)
		if err != nil {
			t.Fatalf("relay soft-close appends: %v", err)
		}
		if processed == 0 && ledger.Len() < bidders {
			t.Fatalf("ledger messages = %d, want %d but relay processed none", ledger.Len(), bidders)
		}
	}
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
	for ledger.Len() < 2 {
		if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
			t.Fatalf("relay cap race appends: %v", err)
		}
	}
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
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
	}
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
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
	}
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
	settleAllLedgerMessages(t, ctx, worker, 1, true)
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
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
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

	// Single relay pass should batch all decisions.
	processed, err := worker.ProcessPendingAppends(ctx, 100)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	if processed < bidders {
		t.Fatalf("relay processed = %d, want at least %d", processed, bidders)
	}
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
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
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
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
	}
	msg, ok := ledger.Message(0)
	if !ok {
		t.Fatalf("missing memory ledger message")
	}
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
	if _, err := worker.ProcessPendingAppends(ctx, 100); err != nil {
		t.Fatalf("relay: %v", err)
	}
	msg, ok := ledger.Message(0)
	if !ok {
		t.Fatalf("missing memory ledger message")
	}
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
	startLen := memoryLedgerLen(t, worker.ledger)
	startNext := memoryLedgerNext(t, worker.ledger)
	// Relay: drain all stream entries to the memory ledger.
	for relay := 0; relay < 20; relay++ {
		n, err := worker.ProcessPendingAppends(ctx, 100)
		if err != nil {
			t.Fatalf("relay: %v", err)
		}
		if n == 0 {
			break
		}
	}
	targetLen := memoryLedgerLen(t, worker.ledger)
	expected := targetLen - startLen
	if expected < want && targetLen-startNext < want {
		t.Fatalf("available ledger messages = %d relayed=%d, want at least %d", targetLen-startNext, expected, want)
	}
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
