package reconcile

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/outbox"
)

func TestCheckerReportsCleanRelayProjectionAndDetectsSeqDrift(t *testing.T) {
	db := openReconcileDB(t)
	rdb := openReconcileRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForReconcile(t, repo, db)

	bid := auction.BidInput{ClientBidID: "reconcile-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_reconcile"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	prioritizeReconcileOutbox(t, db, auctionRow.ID)

	relay := outbox.NewRelay(db, rdb, "reconcile-worker")
	ownReconcileAuctionShard(t, db, auctionRow.ID, "reconcile-worker")
	for i := 0; i < 20; i++ {
		ok, err := relay.ProcessOne(ctx)
		if err != nil {
			t.Fatalf("ProcessOne %d: %v", i, err)
		}
		if !ok {
			break
		}
	}

	report, err := NewChecker(db, rdb).Check(ctx, Options{AuctionIDs: []string{auctionRow.ID}})
	if err != nil {
		t.Fatalf("Check clean projection: %v", err)
	}
	if report.DriftCount != 0 {
		t.Fatalf("clean projection drift count = %d, drifts = %#v", report.DriftCount, report.Drifts)
	}
	if len(report.Results) != 1 || report.Results[0].RedisSnapshotShape == "" {
		t.Fatalf("missing result snapshot shape: %#v", report.Results)
	}

	var snapshot map[string]any
	snapshotKey := "auction:" + auctionRow.ID + ":snapshot"
	stored, err := rdb.Get(ctx, snapshotKey).Bytes()
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	snapshotTTL := rdb.PTTL(ctx, snapshotKey).Val()
	snapshotExpiration := snapshotTTL
	if snapshotExpiration < 0 {
		snapshotExpiration = 0
	}
	t.Cleanup(func() {
		_ = rdb.Set(context.Background(), snapshotKey, stored, snapshotExpiration).Err()
	})
	if err := json.Unmarshal(stored, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	snapshot["seq"] = float64(1)
	mutated, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal mutated snapshot: %v", err)
	}
	if err := rdb.Set(ctx, snapshotKey, mutated, snapshotExpiration).Err(); err != nil {
		t.Fatalf("set mutated snapshot: %v", err)
	}

	report, err = NewChecker(db, rdb).Check(ctx, Options{AuctionIDs: []string{auctionRow.ID}})
	if err != nil {
		t.Fatalf("Check drift projection: %v", err)
	}
	if report.DriftCount == 0 {
		t.Fatalf("expected drift after snapshot seq mutation")
	}
	if len(report.Anomalies) != 0 {
		t.Fatalf("default checker wrote anomalies: %#v", report.Anomalies)
	}
	var anomalyCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM system_anomaly_events
		WHERE auction_id = $1 AND type = $2
	`, auctionRow.ID, AnomalyReconciliationDrift).Scan(&anomalyCount); err != nil {
		t.Fatalf("count anomalies before write: %v", err)
	}
	if anomalyCount != 0 {
		t.Fatalf("default anomaly count = %d, want 0", anomalyCount)
	}

	report, err = NewChecker(db, rdb).Check(ctx, Options{AuctionIDs: []string{auctionRow.ID}, WriteAnomalies: true})
	if err != nil {
		t.Fatalf("Check drift projection with anomalies: %v", err)
	}
	if len(report.Anomalies) != 1 {
		t.Fatalf("written anomalies = %#v, want one", report.Anomalies)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM system_anomaly_events
		WHERE auction_id = $1 AND type = $2
	`, auctionRow.ID, AnomalyReconciliationDrift).Scan(&anomalyCount); err != nil {
		t.Fatalf("count anomalies: %v", err)
	}
	if anomalyCount != 1 {
		t.Fatalf("anomaly count = %d, want 1", anomalyCount)
	}
}

func TestValidateSnapshotComparesFullSnapshotPayloadFields(t *testing.T) {
	winner := "user_1"
	row := dbAuction{
		id:                "auc_snapshot",
		status:            "ACTIVE",
		currentPriceCents: 20_000,
		currentWinnerID:   &winner,
		seq:               7,
		acceptedBidCount:  2,
		extendCount:       1,
	}
	payload := []byte(`{
		"event_type": "snapshot",
		"auction_id": "auc_snapshot",
		"seq": 7,
		"source": "db",
		"payload": {
			"status": "ACTIVE",
			"current_price_cents": 15000,
			"current_winner_id": "user_1",
			"accepted_bid_count": 2,
			"extend_count": 1
		}
	}`)

	var result AuctionResult
	drifts := validateSnapshot(row, payload, &result)
	if len(drifts) != 1 || drifts[0].Type != DriftSnapshotFieldDrift {
		t.Fatalf("drifts = %#v, want one field drift", drifts)
	}
	if result.RedisSnapshotShape != "snapshot:db" {
		t.Fatalf("snapshot shape = %q", result.RedisSnapshotShape)
	}
}

func TestCheckerRejectsMissingRequestedAuction(t *testing.T) {
	db := openReconcileDB(t)
	rdb := openReconcileRedis(t)
	_, err := NewChecker(db, rdb).Check(context.Background(), Options{AuctionIDs: []string{"auc_missing_reconcile_test"}})
	if err == nil {
		t.Fatalf("expected missing requested auction error")
	}
}

func openReconcileDB(t *testing.T) *pgxpool.Pool {
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

func openReconcileRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6380"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createActiveAuctionForReconcile(t *testing.T, repo *auction.Repository, db *pgxpool.Pool) auction.Auction {
	t.Helper()
	roomID := "room_reconcile_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN') ON CONFLICT DO NOTHING`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	item, err := repo.CreateItem(context.Background(), auction.CreateItemInput{Title: "Reconcile Item"})
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
	}, "tr_reconcile")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if _, err := repo.Schedule(context.Background(), created.ID, nil, "tr_reconcile"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	started, err := repo.Start(context.Background(), created.ID, "tr_reconcile")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return started
}

func prioritizeReconcileOutbox(t *testing.T, db *pgxpool.Pool, auctionID string) {
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

func ownReconcileAuctionShard(t *testing.T, db *pgxpool.Pool, auctionID string, ownerID string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM outbox_relay_shard_leases WHERE owner_id = $1`, ownerID)
	})
	_, err := db.Exec(context.Background(), `
		INSERT INTO outbox_relay_shard_leases (shard_id, owner_id, lease_until)
		SELECT DISTINCT shard_id, $2, now() + interval '5 seconds'
		FROM outbox_delivery
		WHERE auction_id = $1
		ON CONFLICT (shard_id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id,
		    lease_until = EXCLUDED.lease_until,
		    renewed_at = now()
	`, auctionID, ownerID)
	if err != nil {
		t.Fatalf("own reconcile auction shard: %v", err)
	}
}
