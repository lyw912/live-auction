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
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle first: %v", err)
	}
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
	if low.Seq != 1 || low.EngineSeq != 2 {
		t.Fatalf("low response seq=%d engine_seq=%d, want public seq 1 engine seq 2", low.Seq, low.EngineSeq)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle low: %v", err)
	}
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
	if sold.Seq != 1 {
		t.Fatalf("sold pending public seq = %d, want snapshot seq 1 before settlement", sold.Seq)
	}
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle sold: %v", err)
	}
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

func TestRedisLedgerConcurrentAppendRecordsEveryEngineSeq(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	const bidders = 32
	errCh := make(chan error, bidders)
	start := make(chan struct{})
	for i := 0; i < bidders; i++ {
		i := i
		go func() {
			<-start
			clientBidID := "redis-ledger-concurrent-" + uuid.NewString()
			_, err := engine.PlaceBid(ctx, auctionID, "user_concurrent_"+strconv.Itoa(i), clientBidID, auction.BidInput{
				ClientBidID:   clientBidID,
				AmountCents:   15_000 + int64(i)*5_000,
				ClientSeenSeq: 0,
			}, "tr_concurrent")
			errCh <- err
		}()
	}
	close(start)
	for i := 0; i < bidders; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent bid %d failed: %v", i, err)
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

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-indexed-pending", auction.BidInput{
		ClientBidID:   "redis-ledger-indexed-pending",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_indexed_pending")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	processed, err := worker.ProcessPendingAppends(ctx, 100)
	if err != nil {
		t.Fatalf("process pending appends: %v", err)
	}
	if processed < 1 {
		t.Fatalf("processed pending appends = %d, want at least 1", processed)
	}
	pending, err := rdb.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if pending != 0 {
		t.Fatalf("pending decisions = %d, want 0", pending)
	}
	indexed, err := rdb.SIsMember(ctx, redisx.BidEnginePendingAuctionsKey(), auctionID).Result()
	if err != nil {
		t.Fatalf("check pending auction index: %v", err)
	}
	if indexed {
		t.Fatalf("auction remained in pending index after drain")
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

func TestRedisLedgerReplayBeforeSettlementUsesAppendStatus(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	first, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-acked", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-acked",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_acked")
	if err != nil {
		t.Fatalf("first bid: %v", err)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after first = %d, want 1", ledger.Len())
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

func TestRedisLedgerReplayUnknownAndFailedAppendStatusDoesNotAppendAgain(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	_, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_status")
	if err != nil {
		t.Fatalf("first bid: %v", err)
	}
	idemKey := redisx.BidEngineIdempotencyKey(auctionID, "redis-ledger-phase2-status")
	if err := rdb.HSet(ctx, idemKey, "kafka_append_status", kafkaAppendStatusUnknown).Err(); err != nil {
		t.Fatalf("set unknown status: %v", err)
	}
	_, err = engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_status_unknown")
	assertAPIErrorCode(t, err, apierrors.CodeProcessingRetryLater)
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after UNKNOWN replay = %d, want no duplicate append", ledger.Len())
	}
	if err := rdb.HSet(ctx, idemKey, "kafka_append_status", kafkaAppendStatusFailed).Err(); err != nil {
		t.Fatalf("set failed status: %v", err)
	}
	_, err = engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_status_failed")
	assertAPIErrorCode(t, err, apierrors.CodeEngineReconciling)
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after FAILED replay = %d, want no duplicate append", ledger.Len())
	}
	_, err = engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-status", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-status",
		AmountCents:   20_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_status_conflict")
	assertAPIErrorCode(t, err, apierrors.CodeIdempotencyKeyReusedWithDifferentRequest)
}

func TestRedisLedgerAppendTimeoutAfterWriteIsUnknownAndReplayDoesNotAppendAgain(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := newAppendThenTimeoutLedger()
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	_, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-timeout", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-timeout",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_timeout")
	assertAPIErrorCode(t, err, apierrors.CodeProcessingRetryLater)
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after write-then-timeout = %d, want 1 durable-but-unknown append", ledger.Len())
	}
	assertEnginePaused(t, db, auctionID, "KAFKA_APPEND_UNKNOWN_BEFORE_RETURN")
	var appendStatus string
	if err := rdb.HGet(ctx, redisx.BidEngineIdempotencyKey(auctionID, "redis-ledger-phase2-timeout"), "kafka_append_status").Scan(&appendStatus); err != nil {
		t.Fatalf("load append status: %v", err)
	}
	if appendStatus != kafkaAppendStatusUnknown {
		t.Fatalf("append status = %q, want %q", appendStatus, kafkaAppendStatusUnknown)
	}
	_, err = engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-timeout", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-timeout",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_timeout_retry")
	assertAPIErrorCode(t, err, apierrors.CodeProcessingRetryLater)
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after UNKNOWN replay = %d, want no duplicate append", ledger.Len())
	}
}

func TestRedisLedgerKafkaAckMarkerFailureDoesNotReturnSuccess(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := newAppendThenCloseRedisLedger(rdb)
	engine := New(db, rdb, ledger)
	auctionID := createEngineAuction(t, db, 0)

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-phase2-marker-fail", auction.BidInput{
		ClientBidID:   "redis-ledger-phase2-marker-fail",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_phase2_marker_fail")
	assertAPIErrorCode(t, err, apierrors.CodeProcessingRetryLater)
	if resp.Result != "" {
		t.Fatalf("response = %#v, want no engine success when Kafka ACK marker is not durable", resp)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after ack-marker failure = %d, want Kafka decision retained", ledger.Len())
	}
	assertEnginePaused(t, db, auctionID, "KAFKA_APPEND_ACK_MARKER_FAILED")
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

func TestRedisLedgerLeavesPendingWhenKafkaAppendFails(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	engine := New(db, rdb, failingLedger{})
	auctionID := createEngineAuction(t, db, 0)

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-kafka-fail", auction.BidInput{
		ClientBidID:   "redis-ledger-kafka-fail",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_kafka_fail")
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierrors.CodeEngineReconciling {
		t.Fatalf("place bid error = %v, want ENGINE_RECONCILING", err)
	}
	if resp.Result != "" {
		t.Fatalf("response = %#v, want no engine success", resp)
	}
	pending, err := rdb.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if pending != 1 {
		t.Fatalf("pending decisions = %d, want 1", pending)
	}
	assertEnginePaused(t, db, auctionID, "KAFKA_APPEND_FAILED_BEFORE_RETURN")
	var appendStatus string
	if err := rdb.HGet(ctx, redisx.BidEngineIdempotencyKey(auctionID, "redis-ledger-kafka-fail"), "kafka_append_status").Scan(&appendStatus); err != nil {
		t.Fatalf("load append status: %v", err)
	}
	if appendStatus != kafkaAppendStatusFailed {
		t.Fatalf("append status = %q, want %q", appendStatus, kafkaAppendStatusFailed)
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

func TestKafkaAckedDecisionKeepsPendingAndWorkerDuplicateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)

	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", "redis-ledger-sync-ack-pending", auction.BidInput{
		ClientBidID:   "redis-ledger-sync-ack-pending",
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_sync_ack_pending")
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if resp.Result != auction.BidResultEngineAccepted || resp.SettlementStatus != auction.SettlementStatusPending {
		t.Fatalf("response = %#v", resp)
	}
	if ledger.Len() != 1 {
		t.Fatalf("ledger len after HTTP return = %d, want 1", ledger.Len())
	}
	pending, err := rdb.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result()
	if err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if pending != 1 {
		t.Fatalf("pending after HTTP return = %d, want 1", pending)
	}
	var appendStatus string
	if err := rdb.HGet(ctx, redisx.BidEngineIdempotencyKey(auctionID, "redis-ledger-sync-ack-pending"), "kafka_append_status").Scan(&appendStatus); err != nil {
		t.Fatalf("load append status: %v", err)
	}
	if appendStatus != kafkaAppendStatusAcked {
		t.Fatalf("append status = %q, want %q", appendStatus, kafkaAppendStatusAcked)
	}
	processPendingAppends(t, worker, ctx, auctionID, 1)
	if ledger.Len() != 2 {
		t.Fatalf("ledger len after pending drain = %d, want duplicate append len 2", ledger.Len())
	}
	if _, err := worker.ProcessKafka(ctx, 2); err != nil {
		t.Fatalf("settle duplicate kafka decisions: %v", err)
	}
	assertAuctionEngineSeq(t, db, auctionID, resp.EngineSeq, 15_000, "ACTIVE")
	var bids int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bids WHERE auction_id = $1 AND client_bid_id = $2`, auctionID, "redis-ledger-sync-ack-pending").Scan(&bids); err != nil {
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
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
		t.Fatalf("settle first: %v", err)
	}
	processed, err := worker.ProcessKafka(ctx, 1)
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
	if _, err := worker.ProcessKafka(ctx, 1); err != nil {
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

func TestReconcileDoesNotPauseWhilePendingAppendInProgress(t *testing.T) {
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
		ClientBidID:       "pending-in-flight-bid",
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_pending_in_flight",
		RequestHash:       requestHash(auctionID, "user_1", "pending-in-flight-bid", 15_000),
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
	if err := rdb.Set(ctx, pendingAppendLockKey(auctionID), "other-worker", time.Second).Err(); err != nil {
		t.Fatalf("seed pending append lock: %v", err)
	}
	report, err := worker.Reconcile(ctx, auctionID)
	if err != nil {
		t.Fatalf("reconcile pending in flight: %v", err)
	}
	if report.Status != "REDIS_PENDING_APPEND_IN_PROGRESS" || report.DriftCount != 0 {
		t.Fatalf("report = %#v", report)
	}
	var paused bool
	var reason string
	if err := db.QueryRow(ctx, `SELECT engine_paused, COALESCE(engine_pause_reason, '') FROM auctions WHERE id = $1`, auctionID).Scan(&paused, &reason); err != nil {
		t.Fatalf("load pause: %v", err)
	}
	if paused || reason != "" {
		t.Fatalf("pause state paused=%v reason=%q, want not paused", paused, reason)
	}
}

func TestAppendPendingDecisionsKeepsLeaseUntilDelete(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	ledger := NewMemoryLedger()
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	auctionID := createEngineAuction(t, db, 0)
	result := engineResult{
		Result:           resultAccepted,
		AuctionID:        auctionID,
		BidID:            uuid.NewString(),
		UserID:           "user_1",
		ClientBidID:      "lease-refresh-bid",
		AmountCents:      15_000,
		EngineSeq:        1,
		EngineEpoch:      1,
		SettlementStatus: "PENDING",
		ServerTimeMS:     time.Now().UTC().UnixMilli(),
		TraceID:          "tr_lease_refresh",
		RequestHash:      requestHash(auctionID, "user_1", "lease-refresh-bid", 15_000),
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal pending result: %v", err)
	}
	if err := rdb.HSet(ctx, redisx.BidEnginePendingKey(auctionID), "1", string(raw)).Err(); err != nil {
		t.Fatalf("seed redis pending: %v", err)
	}
	processed, err := worker.appendPendingDecisions(ctx, auctionID, 1)
	if err != nil {
		t.Fatalf("append pending decisions: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if exists, err := rdb.Exists(ctx, pendingAppendLockKey(auctionID)).Result(); err != nil || exists != 0 {
		t.Fatalf("pending append lock exists=%d err=%v, want removed", exists, err)
	}
	if pending, err := rdb.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result(); err != nil || pending != 0 {
		t.Fatalf("pending decisions = %d err=%v, want 0", pending, err)
	}
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

type appendThenTimeoutLedger struct {
	*MemoryLedger
}

func newAppendThenTimeoutLedger() *appendThenTimeoutLedger {
	return &appendThenTimeoutLedger{MemoryLedger: NewMemoryLedger()}
}

func (l *appendThenTimeoutLedger) Append(ctx context.Context, result engineResult) (LedgerMessage, error) {
	_, _ = l.MemoryLedger.Append(ctx, result)
	return LedgerMessage{}, context.DeadlineExceeded
}

type appendThenCloseRedisLedger struct {
	*MemoryLedger
	redis *redis.Client
}

func newAppendThenCloseRedisLedger(redisClient *redis.Client) *appendThenCloseRedisLedger {
	return &appendThenCloseRedisLedger{MemoryLedger: NewMemoryLedger(), redis: redisClient}
}

func (l *appendThenCloseRedisLedger) Append(ctx context.Context, result engineResult) (LedgerMessage, error) {
	msg, err := l.MemoryLedger.Append(ctx, result)
	if err != nil {
		return LedgerMessage{}, err
	}
	_ = l.redis.Close()
	return msg, nil
}

func processPendingAppends(t *testing.T, worker *Worker, ctx context.Context, auctionID string, want int) {
	t.Helper()
	processed, err := worker.appendPendingDecisions(ctx, auctionID, want)
	if err != nil {
		t.Fatalf("append pending decisions: %v", err)
	}
	if processed != want {
		t.Fatalf("pending appended = %d, want %d", processed, want)
	}
}

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

func int64Ptr(value int64) *int64 {
	return &value
}
