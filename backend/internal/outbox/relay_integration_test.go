package outbox

import (
	"context"
	"encoding/json"
	"os"
	"testing"

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
}
