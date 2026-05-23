package outbox

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
)

func TestRelayPublishesPendingOutboxToRedisInOrder(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)

	bid1 := auction.BidInput{ClientBidID: "relay-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, auctionRow.ID, "user_1", bid1.ClientBidID, bid1, "tr_relay"); err != nil {
		t.Fatalf("PlaceBid 1: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ('user_2', 'user', 'Relay User 2') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert user_2: %v", err)
	}
	bid2 := auction.BidInput{ClientBidID: "relay-bid-2", AmountCents: 20_000}
	if _, err := repo.PlaceBid(ctx, auctionRow.ID, "user_2", bid2.ClientBidID, bid2, "tr_relay"); err != nil {
		t.Fatalf("PlaceBid 2: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)

	relay := NewRelay(db, rdb, "test-worker")
	for i := 0; i < 20; i++ {
		ok, err := relay.ProcessOne(ctx)
		if err != nil {
			t.Fatalf("ProcessOne %d: %v", i, err)
		}
		if !ok {
			break
		}
	}

	values, err := rdb.LRange(ctx, "auction:"+auctionRow.ID+":events", 0, -1).Result()
	if err != nil {
		t.Fatalf("LRange: %v", err)
	}
	if len(values) < 2 {
		t.Fatalf("expected redis history, got %d", len(values))
	}
	var lastSeq int64
	for _, value := range values {
		var envelope struct {
			Seq int64 `json:"seq"`
		}
		if err := json.Unmarshal([]byte(value), &envelope); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if envelope.Seq <= lastSeq {
			t.Fatalf("redis seq not increasing: %d after %d", envelope.Seq, lastSeq)
		}
		lastSeq = envelope.Seq
	}

	var pending int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND d.status <> 'PUBLISHED'
	`, auctionRow.ID).Scan(&pending); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if pending != 0 {
		t.Fatalf("pending outbox = %d, want 0", pending)
	}
}

func TestRelayPoisonMarksDeadAndWritesAnomaly(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid := auction.BidInput{ClientBidID: "poison-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_poison"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET max_attempts = 1
		WHERE outbox_id = (SELECT id FROM outbox_events WHERE auction_id = $1 AND event_type = 'auction_created')
	`, auctionRow.ID); err != nil {
		t.Fatalf("set max attempts: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED', published_at = COALESCE(published_at, now()), locked_by = NULL, locked_until = NULL
		WHERE outbox_id IN (
			SELECT id FROM outbox_events
			WHERE auction_id = $1 AND event_type <> 'auction_created'
		)
	`, auctionRow.ID); err != nil {
		t.Fatalf("publish non-poison outbox: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)

	badRedis := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: 0})
	t.Cleanup(func() { _ = badRedis.Close() })
	var publishedAuctionID string
	var publishedPayload []byte
	relay := NewRelay(db, badRedis, "poison-worker").WithPublisher(func(_ context.Context, auctionID string, payload []byte) {
		publishedAuctionID = auctionID
		publishedPayload = append([]byte(nil), payload...)
	})
	ok, err := relay.ProcessOne(ctx)
	if !ok {
		t.Fatalf("expected claimed event")
	}
	if err == nil {
		t.Fatalf("expected publish error")
	}

	var dead int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND d.status = 'DEAD'
	`, auctionRow.ID).Scan(&dead); err != nil {
		t.Fatalf("count dead: %v", err)
	}
	if dead != 1 {
		t.Fatalf("dead deliveries = %d, want 1", dead)
	}
	var anomalies int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM system_anomaly_events WHERE auction_id = $1 AND type = 'OUTBOX_DEAD_LETTER'`, auctionRow.ID).Scan(&anomalies); err != nil {
		t.Fatalf("count anomalies: %v", err)
	}
	if anomalies != 1 {
		t.Fatalf("anomalies = %d, want 1", anomalies)
	}
	if publishedAuctionID != auctionRow.ID {
		t.Fatalf("gap notice auction = %q, want %q", publishedAuctionID, auctionRow.ID)
	}
	var notice struct {
		EventType  string  `json:"event_type"`
		AuctionID  string  `json:"auction_id"`
		MissingSeq []int64 `json:"missing_seq"`
	}
	if err := json.Unmarshal(publishedPayload, &notice); err != nil {
		t.Fatalf("unmarshal gap notice: %v", err)
	}
	if notice.EventType != "outbox_gap_notice" || notice.AuctionID != auctionRow.ID || len(notice.MissingSeq) != 1 {
		t.Fatalf("unexpected gap notice: %#v", notice)
	}
}

func TestRelayReclaimsExpiredPublishingLease(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid := auction.BidInput{ClientBidID: "lease-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_lease"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHING', locked_by = 'dead-worker', locked_until = now() - interval '1 second'
		WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id = $1)
	`, auctionRow.ID); err != nil {
		t.Fatalf("force expired lease: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)

	ok, err := NewRelay(db, rdb, "lease-reclaimer").ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected expired publishing event to be reclaimed")
	}
	var published int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND d.status = 'PUBLISHED'
	`, auctionRow.ID).Scan(&published); err != nil {
		t.Fatalf("count published: %v", err)
	}
	if published == 0 {
		t.Fatalf("expected at least one reclaimed event to publish")
	}
}

func TestRelayDoesNotSkipBlockedAuctionHead(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid1 := auction.BidInput{ClientBidID: "blocked-head-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, auctionRow.ID, "user_1", bid1.ClientBidID, bid1, "tr_blocked_head"); err != nil {
		t.Fatalf("PlaceBid 1: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ('user_blocked_head_2', 'user', 'Blocked Head 2') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	bid2 := auction.BidInput{ClientBidID: "blocked-head-bid-2", AmountCents: 20_000}
	if _, err := repo.PlaceBid(ctx, auctionRow.ID, "user_blocked_head_2", bid2.ClientBidID, bid2, "tr_blocked_head"); err != nil {
		t.Fatalf("PlaceBid 2: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED', published_at = COALESCE(published_at, now()), locked_by = NULL, locked_until = NULL
		WHERE outbox_id IN (
			SELECT id FROM outbox_events
			WHERE auction_id = $1
			  AND seq < (SELECT min(seq) FROM outbox_events WHERE auction_id = $1 AND event_type = 'bid_accepted')
		)
	`, auctionRow.ID); err != nil {
		t.Fatalf("publish setup events: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'FAILED', next_attempt_at = now() + interval '1 hour'
		WHERE outbox_id = (
			SELECT id FROM outbox_events
			WHERE auction_id = $1 AND event_type = 'bid_accepted'
			ORDER BY seq
			LIMIT 1
		)
	`, auctionRow.ID); err != nil {
		t.Fatalf("block first bid event: %v", err)
	}

	ok, err := NewRelay(db, rdb, "blocked-head-worker").ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if ok {
		t.Fatalf("relay skipped blocked auction head")
	}
	var publishedSecond int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1
		  AND e.event_type = 'bid_accepted'
		  AND e.seq > (SELECT min(seq) FROM outbox_events WHERE auction_id = $1 AND event_type = 'bid_accepted')
		  AND d.status = 'PUBLISHED'
	`, auctionRow.ID).Scan(&publishedSecond); err != nil {
		t.Fatalf("count published second: %v", err)
	}
	if publishedSecond != 0 {
		t.Fatalf("published later same-auction event while head was blocked")
	}
}

func TestRelayClaimPlanScalesWithPendingBacklog(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	auctionID := "relay_plan_" + uuid.NewString()
	quiesceOutboxExcept(t, db, auctionID)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `
			DELETE FROM outbox_delivery
			WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id = $1);
			DELETE FROM outbox_events WHERE auction_id = $1;
		`, auctionID)
	})
	const pendingRows = 5000
	for i := 1; i <= pendingRows; i++ {
		var outboxID int64
		if err := db.QueryRow(ctx, `
			INSERT INTO outbox_events (aggregate_type, aggregate_id, auction_id, seq, event_type, payload_json, created_at)
			VALUES ('auction', $1, $1, $2, 'bid_accepted', '{}'::jsonb, now() + ($2::bigint * interval '1 millisecond'))
			RETURNING id
		`, auctionID, i).Scan(&outboxID); err != nil {
			t.Fatalf("insert outbox event %d: %v", i, err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO outbox_delivery (outbox_id, status) VALUES ($1, 'PENDING')`, outboxID); err != nil {
			t.Fatalf("insert outbox delivery %d: %v", i, err)
		}
	}

	start := time.Now()
	ok, err := NewRelay(db, openRedis(t), "claim-plan-worker").ProcessOne(ctx)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected one claimed event")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("claim/process elapsed %s with %d pending rows; possible outbox claim regression", elapsed, pendingRows)
	}
	var firstStatus string
	if err := db.QueryRow(ctx, `
		SELECT d.status
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND e.seq = 1
	`, auctionID).Scan(&firstStatus); err != nil {
		t.Fatalf("select first status: %v", err)
	}
	if firstStatus != StatusPublished {
		t.Fatalf("first status = %s, want %s", firstStatus, StatusPublished)
	}
}

func TestRelayProcessBatchDrainsMultipleEvents(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	for i := 0; i < 8; i++ {
		userID := "batch_user_" + uuid.NewString()
		if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ($1, 'user', 'Batch User')`, userID); err != nil {
			t.Fatalf("insert user %d: %v", i, err)
		}
		bid := auction.BidInput{ClientBidID: "batch-bid-" + uuid.NewString(), AmountCents: 15_000 + int64(i)*5_000}
		if _, err := repo.PlaceBid(ctx, auctionRow.ID, userID, bid.ClientBidID, bid, "tr_batch"); err != nil {
			t.Fatalf("PlaceBid %d: %v", i, err)
		}
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)

	processed, err := NewRelay(db, rdb, "batch-worker").ProcessBatch(ctx, 4)
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if processed != 4 {
		t.Fatalf("processed = %d, want 4", processed)
	}
	var published int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND d.status = 'PUBLISHED'
	`, auctionRow.ID).Scan(&published); err != nil {
		t.Fatalf("count published: %v", err)
	}
	if published != 4 {
		t.Fatalf("published = %d, want 4", published)
	}
}

func TestRelayRebuildSnapshotWritesRedisSnapshot(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)

	payload, err := NewRelay(db, rdb, "snapshot-worker").RebuildSnapshot(ctx, auctionRow.ID)
	if err != nil {
		t.Fatalf("RebuildSnapshot: %v", err)
	}
	if len(payload) == 0 {
		t.Fatalf("empty snapshot payload")
	}
	stored, err := rdb.Get(ctx, "auction:"+auctionRow.ID+":snapshot").Bytes()
	if err != nil {
		t.Fatalf("Get snapshot: %v", err)
	}
	if len(stored) == 0 {
		t.Fatalf("empty redis snapshot")
	}
}

func openDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func openRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createActiveAuctionForRelay(t *testing.T, repo *auction.Repository, db *pgxpool.Pool) auction.Auction {
	t.Helper()
	roomID := "room_relay_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN') ON CONFLICT DO NOTHING`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	item, err := repo.CreateItem(context.Background(), auction.CreateItemInput{Title: "Relay Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	created, err := repo.CreateAuction(context.Background(), auction.CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		Rule: auction.Rule{
			DurationSeconds:     60,
			ExtendWindowSeconds: 10,
			ExtendBySeconds:     10,
			MaxExtendCount:      3,
			DepositBPS:          1000,
			DepositFloorCents:   10_000,
			DepositCapCents:     100_000_000,
		},
	}, "tr_relay")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if _, err := repo.Schedule(context.Background(), created.ID, nil, "tr_relay"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	started, err := repo.Start(context.Background(), created.ID, "tr_relay")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return started
}

func prioritizeOutboxForAuction(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		UPDATE outbox_events
		SET created_at = '1900-01-01 00:00:00+00'::timestamptz + (seq * interval '1 second')
		WHERE auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("prioritize outbox: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		UPDATE outbox_delivery d
		SET event_created_at = e.created_at
		FROM outbox_events e
		WHERE e.id = d.outbox_id
		  AND e.auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("prioritize outbox delivery: %v", err)
	}
}

func quiesceOutboxExcept(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		UPDATE outbox_delivery d
		SET status = 'PUBLISHED',
		    published_at = COALESCE(published_at, now()),
		    locked_by = NULL,
		    locked_until = NULL
		FROM outbox_events e
		WHERE e.id = d.outbox_id
		  AND (e.auction_id IS DISTINCT FROM $1)
		  AND d.status NOT IN ('PUBLISHED','DEAD')
	`, auctionID); err != nil {
		t.Fatalf("quiesce outbox: %v", err)
	}
}
