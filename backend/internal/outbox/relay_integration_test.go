package outbox

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/redisx"
)

func TestRelayPublishesPendingOutboxToRedisInOrder(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	if err := rdb.Del(ctx, "auction:"+auctionRow.ID+":events", "auction:"+auctionRow.ID+":snapshot").Err(); err != nil {
		t.Fatalf("redis cleanup: %v", err)
	}
	quiesceOutboxExcept(t, db, auctionRow.ID)

	bid1 := auction.BidInput{ClientBidID: "relay-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_1", bid1.ClientBidID, bid1, "tr_relay"); err != nil {
		t.Fatalf("PlaceBid 1: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ('user_2', 'user', 'Relay User 2') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert user_2: %v", err)
	}
	bid2 := auction.BidInput{ClientBidID: "relay-bid-2", AmountCents: 20_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_2", bid2.ClientBidID, bid2, "tr_relay"); err != nil {
		t.Fatalf("PlaceBid 2: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)

	relay := NewRelay(db, rdb, "test-worker")
	ownAuctionShard(t, db, auctionRow.ID, relay.workerID)
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
	if len(values) == 0 {
		t.Fatalf("expected redis history, got %d", len(values))
	}
	var lastSeq int64
	for _, value := range values {
		var envelope struct {
			OutboxID      int64           `json:"outbox_id"`
			Seq           int64           `json:"seq"`
			StreamEpoch   string          `json:"stream_epoch"`
			SchemaVersion int             `json:"event_schema_version"`
			EventKey      string          `json:"event_key"`
			Payload       json.RawMessage `json:"payload"`
			PayloadSHA256 string          `json:"payload_sha256"`
			PublishedAtMS int64           `json:"published_at_ms"`
		}
		if err := json.Unmarshal([]byte(value), &envelope); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if envelope.OutboxID <= 0 {
			t.Fatalf("missing delivery message id/outbox id in envelope: %#v", envelope)
		}
		if envelope.SchemaVersion != EventSchemaVersion {
			t.Fatalf("schema version = %d, want %d", envelope.SchemaVersion, EventSchemaVersion)
		}
		if envelope.EventKey != auctionRow.ID {
			t.Fatalf("event key = %q, want %q", envelope.EventKey, auctionRow.ID)
		}
		if envelope.PayloadSHA256 == "" || len(envelope.Payload) == 0 {
			t.Fatalf("missing payload hash/envelope payload: %#v", envelope)
		}
		if envelope.PublishedAtMS <= 0 {
			t.Fatalf("missing published_at_ms in realtime envelope: %#v", envelope)
		}
		if envelope.Seq <= lastSeq {
			t.Fatalf("redis seq not increasing: %d after %d", envelope.Seq, lastSeq)
		}
		lastSeq = envelope.Seq
	}
	var lastEnvelope struct {
		StreamEpoch string `json:"stream_epoch"`
	}
	if err := json.Unmarshal([]byte(values[len(values)-1]), &lastEnvelope); err != nil {
		t.Fatalf("unmarshal last envelope: %v", err)
	}
	if lastEnvelope.StreamEpoch == "" {
		t.Fatalf("empty stream epoch in last envelope: %s", values[len(values)-1])
	}
	projection, err := rdb.HGetAll(ctx, redisx.BidGuardProjectionKey(auctionRow.ID)).Result()
	if err != nil {
		t.Fatalf("read guard projection: %v", err)
	}
	if projection["status"] != "ACTIVE" || projection["current_price_cents"] != "20000" || projection["seq"] == "" || projection["projected_at_ms"] == "" {
		t.Fatalf("unexpected guard projection: %#v", projection)
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

func TestRelayStreamEpochStableAcrossEventsAndSnapshot(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	if err := rdb.Del(ctx, "auction:"+auctionRow.ID+":events", "auction:"+auctionRow.ID+":snapshot").Err(); err != nil {
		t.Fatalf("redis cleanup: %v", err)
	}
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid := auction.BidInput{ClientBidID: "epoch-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_epoch"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED', published_at = COALESCE(published_at, now()), locked_by = NULL, locked_until = NULL
		WHERE outbox_id IN (
			SELECT id FROM outbox_events
			WHERE auction_id = $1 AND event_type <> 'bid_accepted'
		)
	`, auctionRow.ID); err != nil {
		t.Fatalf("publish setup events: %v", err)
	}

	relay := NewRelay(db, rdb, "epoch-worker")
	ownAuctionShard(t, db, auctionRow.ID, relay.workerID)
	ok, err := relay.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected one outbox event")
	}

	values, err := rdb.LRange(ctx, "auction:"+auctionRow.ID+":events", -1, -1).Result()
	if err != nil || len(values) != 1 {
		t.Fatalf("history last err=%v values=%v", err, values)
	}
	var event struct {
		StreamEpoch     string `json:"stream_epoch"`
		SnapshotVersion int64  `json:"snapshot_version"`
	}
	if err := json.Unmarshal([]byte(values[0]), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if event.StreamEpoch == "" || event.SnapshotVersion == 0 {
		t.Fatalf("event missing epoch/version: %#v", event)
	}

	snapshot, err := relay.RebuildSnapshot(ctx, auctionRow.ID)
	if err != nil {
		t.Fatalf("RebuildSnapshot: %v", err)
	}
	var snap struct {
		StreamEpoch     string `json:"stream_epoch"`
		SnapshotVersion int64  `json:"snapshot_version"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.StreamEpoch != event.StreamEpoch {
		t.Fatalf("snapshot epoch = %q, event epoch = %q", snap.StreamEpoch, event.StreamEpoch)
	}
	if snap.SnapshotVersion == 0 {
		t.Fatalf("snapshot version missing: %#v", snap)
	}
}

func TestRelayRedisUnavailableRemainsRetriableAfterMaxAttempts(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid := auction.BidInput{ClientBidID: "redis-retry-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_redis_retry"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	var targetOutboxID int64
	if err := db.QueryRow(ctx, `
		SELECT id
		FROM outbox_events
		WHERE auction_id = $1
		  AND event_type = 'auction_created'
		ORDER BY seq
		LIMIT 1
	`, auctionRow.ID).Scan(&targetOutboxID); err != nil {
		t.Fatalf("select target outbox: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET max_attempts = 1
		WHERE outbox_id = $1
	`, targetOutboxID); err != nil {
		t.Fatalf("set max attempts: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED',
		    published_at = COALESCE(published_at, now()),
		    locked_by = NULL,
		    locked_until = NULL
		WHERE outbox_id IN (
			SELECT id FROM outbox_events
			WHERE auction_id = $1
			  AND event_type <> 'auction_created'
		)
	`, auctionRow.ID); err != nil {
		t.Fatalf("publish non-poison outbox: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PENDING',
		    next_attempt_at = now(),
		    locked_by = NULL,
		    locked_until = NULL
		WHERE outbox_id = $1
	`, targetOutboxID); err != nil {
		t.Fatalf("make target outbox claimable: %v", err)
	}
	isolateClaimableOutbox(t, db, auctionRow.ID, targetOutboxID)
	if _, err := db.Exec(ctx, `
		DELETE FROM outbox_relay_watermarks
		WHERE shard_id IN (SELECT DISTINCT shard_id FROM outbox_delivery WHERE auction_id = $1)
	`, auctionRow.ID); err != nil {
		t.Fatalf("clear watermarks: %v", err)
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
	ownAuctionShard(t, db, auctionRow.ID, relay.workerID)
	ok, err := relay.ProcessOne(ctx)
	if !ok {
		t.Fatalf("expected claimed event")
	}
	if err == nil {
		t.Fatalf("expected publish error")
	}

	var status string
	var attempts int
	var errClass string
	var retriable bool
	var nextAttemptAt time.Time
	if err := db.QueryRow(ctx, `
		SELECT d.status, d.attempts, d.last_error_class, d.last_error_retriable, d.next_attempt_at
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.id = $1
	`, targetOutboxID).Scan(&status, &attempts, &errClass, &retriable, &nextAttemptAt); err != nil {
		t.Fatalf("select retriable delivery: %v", err)
	}
	if status != StatusFailed || attempts != 1 || errClass != ErrorClassRedisUnavailable || !retriable {
		t.Fatalf("delivery = status=%s attempts=%d class=%s retriable=%v, want FAILED/1/%s/true", status, attempts, errClass, retriable, ErrorClassRedisUnavailable)
	}
	if !nextAttemptAt.After(time.Now().Add(-1 * time.Second)) {
		t.Fatalf("next attempt not scheduled in future-ish window: %s", nextAttemptAt)
	}
	var actualDeadForShard int64
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_delivery d
		JOIN outbox_events e ON e.id = d.outbox_id
		WHERE e.auction_id = $1
		  AND d.status = 'DEAD'
	`, auctionRow.ID).Scan(&actualDeadForShard); err != nil {
		t.Fatalf("select actual dead count for auction: %v", err)
	}
	if actualDeadForShard != 0 {
		t.Fatalf("dead deliveries for retriable Redis error in auction = %d, want 0", actualDeadForShard)
	}
	var anomalies int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM system_anomaly_events WHERE auction_id = $1 AND type = 'OUTBOX_DEAD_LETTER'`, auctionRow.ID).Scan(&anomalies); err != nil {
		t.Fatalf("count anomalies: %v", err)
	}
	if anomalies != 0 {
		t.Fatalf("dead-letter anomalies for retriable Redis error = %d, want 0", anomalies)
	}
	if publishedAuctionID != "" || publishedPayload != nil {
		t.Fatalf("gap notice published for retriable Redis error: auction=%q payload=%s", publishedAuctionID, string(publishedPayload))
	}
}

func TestRelayInvalidEnvelopeDeadLettersWithoutRetry(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid := auction.BidInput{ClientBidID: "invalid-envelope-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_invalid_envelope"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	var targetOutboxID int64
	if err := db.QueryRow(ctx, `
		SELECT id
		FROM outbox_events
		WHERE auction_id = $1
		  AND event_type = 'bid_accepted'
		ORDER BY seq
		LIMIT 1
	`, auctionRow.ID).Scan(&targetOutboxID); err != nil {
		t.Fatalf("select invalid target outbox: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED',
		    published_at = COALESCE(published_at, now()),
		    locked_by = NULL,
		    locked_until = NULL
		WHERE outbox_id IN (
			SELECT id FROM outbox_events
			WHERE auction_id = $1
			  AND event_type <> 'bid_accepted'
		)
	`, auctionRow.ID); err != nil {
		t.Fatalf("publish setup events: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PENDING',
		    next_attempt_at = now(),
		    locked_by = NULL,
		    locked_until = NULL
		WHERE outbox_id = $1
	`, targetOutboxID); err != nil {
		t.Fatalf("make invalid envelope outbox claimable: %v", err)
	}
	isolateClaimableOutbox(t, db, auctionRow.ID, targetOutboxID)
	if _, err := db.Exec(ctx, `
		UPDATE outbox_events
		SET payload_sha256 = repeat('0', 64)
		WHERE id = $1
	`, targetOutboxID); err != nil {
		t.Fatalf("corrupt payload hash: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)

	var gapNotices int
	relay := NewRelay(db, rdb, "invalid-envelope-worker").WithPublisher(func(_ context.Context, auctionID string, payload []byte) {
		if auctionID == auctionRow.ID {
			gapNotices++
		}
	})
	ownAuctionShard(t, db, auctionRow.ID, relay.workerID)
	ok, err := relay.ProcessOne(ctx)
	if !ok {
		t.Fatalf("expected invalid event to be claimed")
	}
	if err == nil {
		t.Fatalf("expected invalid envelope error")
	}
	if class, retriable := classifyPublishError(err); class != ErrorClassPayloadInvalid || retriable {
		t.Fatalf("publish error classification = (%s,%v), want (%s,false): %v", class, retriable, ErrorClassPayloadInvalid, err)
	}

	var deadPayloadInvalid int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1
		  AND d.status = 'DEAD'
		  AND d.attempts = 1
		  AND d.last_error_class = $2
		  AND d.last_error_retriable = false
	`, auctionRow.ID, ErrorClassPayloadInvalid).Scan(&deadPayloadInvalid); err != nil {
		t.Fatalf("count invalid delivery: %v", err)
	}
	if deadPayloadInvalid != 1 {
		t.Fatalf("payload-invalid dead deliveries = %d, want 1", deadPayloadInvalid)
	}
	if gapNotices != 1 {
		t.Fatalf("gap notices = %d, want 1", gapNotices)
	}
	var anomalyPayload struct {
		ErrorClass  string `json:"error_class"`
		Retriable   bool   `json:"retriable"`
		Attempts    int    `json:"attempts"`
		MaxAttempts int    `json:"max_attempts"`
	}
	var anomalyPayloadJSON []byte
	if err := db.QueryRow(ctx, `
		SELECT payload_json
		FROM system_anomaly_events
		WHERE auction_id = $1 AND type = 'OUTBOX_DEAD_LETTER'
		ORDER BY id DESC
		LIMIT 1
	`, auctionRow.ID).Scan(&anomalyPayloadJSON); err != nil {
		t.Fatalf("select dead-letter anomaly payload: %v", err)
	}
	if err := json.Unmarshal(anomalyPayloadJSON, &anomalyPayload); err != nil {
		t.Fatalf("unmarshal dead-letter anomaly payload: %v", err)
	}
	if anomalyPayload.ErrorClass != ErrorClassPayloadInvalid || anomalyPayload.Retriable || anomalyPayload.Attempts != 1 {
		t.Fatalf("invalid envelope did not produce TERM-style dead letter payload: %#v", anomalyPayload)
	}
}

func TestRelayWatermarkTracksAckPendingAndRedelivery(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid := auction.BidInput{ClientBidID: "watermark-redelivery-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_watermark_redelivery"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)
	var outboxID int64
	var shardID int
	if err := db.QueryRow(ctx, `
		SELECT d.outbox_id, d.shard_id
		FROM outbox_delivery d
		JOIN outbox_events e ON e.id = d.outbox_id
		WHERE e.auction_id = $1
		ORDER BY d.outbox_id
		LIMIT 1
	`, auctionRow.ID).Scan(&outboxID, &shardID); err != nil {
		t.Fatalf("select outbox shard: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHING',
		    attempts = 3,
		    locked_by = 'ack-worker',
		    locked_until = now() + interval '30 seconds',
		    last_error = 'previous timeout',
		    last_error_class = $2,
		    last_error_retriable = true,
		    last_error_at = now() - interval '5 seconds'
		WHERE outbox_id = $1
	`, outboxID, ErrorClassPublishTimeout); err != nil {
		t.Fatalf("mark ack pending: %v", err)
	}
	relay := NewRelay(db, openRedis(t), "watermark-worker")
	if err := relay.refreshWatermark(ctx, shardID); err != nil {
		t.Fatalf("refreshWatermark: %v", err)
	}
	var ackPending float64
	var redelivered float64
	if err := db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'PUBLISHING')::double precision,
		       count(*) FILTER (WHERE attempts > 1)::double precision
		FROM outbox_delivery
		WHERE shard_id = $1
	`, shardID).Scan(&ackPending, &redelivered); err != nil {
		t.Fatalf("select ack pending/redelivered: %v", err)
	}
	if ackPending < 1 || redelivered < 1 {
		t.Fatalf("ackPending=%v redelivered=%v, want both >= 1", ackPending, redelivered)
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
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_lease"); err != nil {
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

	relay := NewRelay(db, rdb, "lease-reclaimer")
	ownAuctionShard(t, db, auctionRow.ID, relay.workerID)
	ok, err := relay.ProcessOne(ctx)
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
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_1", bid1.ClientBidID, bid1, "tr_blocked_head"); err != nil {
		t.Fatalf("PlaceBid 1: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ('user_blocked_head_2', 'user', 'Blocked Head 2') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	bid2 := auction.BidInput{ClientBidID: "blocked-head-bid-2", AmountCents: 20_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_blocked_head_2", bid2.ClientBidID, bid2, "tr_blocked_head"); err != nil {
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

	relay := NewRelay(db, rdb, "blocked-head-worker")
	ownAuctionShard(t, db, auctionRow.ID, relay.workerID)
	ok, err := relay.ProcessOne(ctx)
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
	cleanupSyntheticRelayPlanBacklog(t, db)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	auctionID := auctionRow.ID
	quiesceOutboxExcept(t, db, auctionID)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `
			DELETE FROM outbox_delivery
			WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id = $1);
			DELETE FROM outbox_events WHERE auction_id = $1;
			DELETE FROM realtime_stream_epochs WHERE auction_id = $1;
		`, auctionID)
	})
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED', published_at = COALESCE(published_at, now()), locked_by = NULL, locked_until = NULL
		WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id = $1)
	`, auctionID); err != nil {
		t.Fatalf("publish existing outbox: %v", err)
	}
	var baseSeq int64
	if err := db.QueryRow(ctx, `SELECT COALESCE(max(seq), 0) FROM outbox_events WHERE auction_id = $1`, auctionID).Scan(&baseSeq); err != nil {
		t.Fatalf("select base seq: %v", err)
	}
	const pendingRows = 5000
	for i := 1; i <= pendingRows; i++ {
		var outboxID int64
		seq := baseSeq + int64(i)
		if err := db.QueryRow(ctx, `
			INSERT INTO outbox_events (
				aggregate_type, aggregate_id, auction_id, seq, event_type,
				event_schema_version, event_key, payload_json, payload_sha256, created_at
			)
			VALUES (
				'auction', $1, $1, $2, 'bid_accepted', $4, $1, '{}'::jsonb,
				encode(digest(convert_to('{}'::jsonb::text, 'UTF8'), 'sha256'), 'hex'),
				now() - interval '1 second' + ($3::bigint * interval '1 microsecond')
			)
			RETURNING id
		`, auctionID, seq, i, EventSchemaVersion).Scan(&outboxID); err != nil {
			t.Fatalf("insert outbox event %d: %v", i, err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO outbox_delivery (outbox_id, status) VALUES ($1, 'PENDING')`, outboxID); err != nil {
			t.Fatalf("insert outbox delivery %d: %v", i, err)
		}
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED', published_at = COALESCE(published_at, now()), locked_by = NULL, locked_until = NULL
		WHERE outbox_id IN (
			SELECT id FROM outbox_events
			WHERE auction_id = $1 AND seq < 1
		)
	`, auctionID); err != nil {
		t.Fatalf("publish setup outbox: %v", err)
	}

	start := time.Now()
	relay := NewRelay(db, openRedis(t), "claim-plan-worker")
	ownAuctionShard(t, db, auctionID, relay.workerID)
	ok, err := relay.ProcessOne(ctx)
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
		WHERE e.auction_id = $1 AND e.seq = $2 AND e.event_type = 'bid_accepted'
	`, auctionID, baseSeq+1).Scan(&firstStatus); err != nil {
		t.Fatalf("select first status: %v", err)
	}
	if firstStatus != StatusPublished {
		t.Fatalf("first status = %s, want %s", firstStatus, StatusPublished)
	}
}

func cleanupSyntheticRelayPlanBacklog(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		DELETE FROM outbox_delivery
		WHERE outbox_id IN (
			SELECT id FROM outbox_events
			WHERE auction_id LIKE 'relay_plan_%'
		);
		DELETE FROM outbox_events WHERE auction_id LIKE 'relay_plan_%';
		DELETE FROM realtime_stream_epochs WHERE auction_id LIKE 'relay_plan_%';
		DELETE FROM outbox_relay_shard_leases WHERE owner_id IN ('claim-plan-worker');
	`); err != nil {
		t.Fatalf("cleanup synthetic relay plan backlog: %v", err)
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
		if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, userID, bid.ClientBidID, bid, "tr_batch"); err != nil {
			t.Fatalf("PlaceBid %d: %v", i, err)
		}
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)
	var initiallyPublished int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND d.status = 'PUBLISHED'
	`, auctionRow.ID).Scan(&initiallyPublished); err != nil {
		t.Fatalf("count initially published: %v", err)
	}

	relay := NewRelay(db, rdb, "batch-worker")
	ownAuctionShard(t, db, auctionRow.ID, relay.workerID)
	processed, err := relay.ProcessBatch(ctx, 4)
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
	if published-initiallyPublished != 4 {
		t.Fatalf("published delta = %d total=%d initially=%d, want 4", published-initiallyPublished, published, initiallyPublished)
	}
}

func TestRelayShardLeasePreventsDuplicateOwnersAndAllowsFailover(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid := auction.BidInput{ClientBidID: "shard-lease-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_shard_lease"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)

	owner := NewRelay(db, rdb, "owner-worker")
	ownAuctionShard(t, db, auctionRow.ID, owner.workerID)
	contender := NewRelay(db, rdb, "contender-worker")
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM outbox_relay_shard_leases WHERE owner_id IN ('owner-worker','contender-worker')`)
	})
	if err := contender.renewShardLeases(ctx); err != nil {
		t.Fatalf("contender renew leases: %v", err)
	}
	var contenderOwned int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_relay_shard_leases l
		JOIN outbox_delivery d ON d.shard_id = l.shard_id
		WHERE d.auction_id = $1 AND l.owner_id = 'contender-worker'
	`, auctionRow.ID).Scan(&contenderOwned); err != nil {
		t.Fatalf("count contender leases: %v", err)
	}
	if contenderOwned != 0 {
		t.Fatalf("contender took %d live leases from owner", contenderOwned)
	}
	ok, err := contender.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("contender ProcessOne: %v", err)
	}
	if ok {
		t.Fatalf("contender processed event without owning a shard")
	}

	if _, err := db.Exec(ctx, `UPDATE outbox_relay_shard_leases SET lease_until = now() - interval '1 second' WHERE owner_id = 'owner-worker'`); err != nil {
		t.Fatalf("expire owner leases: %v", err)
	}
	if err := contender.renewShardLeases(ctx); err != nil {
		t.Fatalf("contender renew leases after failover: %v", err)
	}
	ok, err = contender.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("contender ProcessOne after failover: %v", err)
	}
	if !ok {
		t.Fatalf("contender did not process after owner lease expired")
	}
}

func TestRelayRunWakesFromPostgresNotify(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)
	bid := auction.BidInput{ClientBidID: "notify-bid-1", AmountCents: 15_000}
	if _, err := repo.PlaceBidPostgresLegacyForTests(context.Background(), auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_notify"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	prioritizeOutboxForAuction(t, db, auctionRow.ID)

	relay := NewRelay(db, rdb, "notify-worker").WithNotify(true)
	ownAuctionShard(t, db, auctionRow.ID, relay.workerID)
	go relay.Run(ctx, nilLogger(), time.Hour)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var published int
		if err := db.QueryRow(context.Background(), `
			SELECT count(*)
			FROM outbox_events e
			JOIN outbox_delivery d ON d.outbox_id = e.id
			WHERE e.auction_id = $1 AND d.status = 'PUBLISHED'
		`, auctionRow.ID).Scan(&published); err != nil {
			t.Fatalf("count published: %v", err)
		}
		if published > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("relay did not wake from outbox notify before poll interval")
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
	var statuses []string
	rows, err := db.Query(ctx, `
		SELECT status
		FROM snapshot_rebuild_events
		WHERE auction_id = $1
		ORDER BY id
	`, auctionRow.ID)
	if err != nil {
		t.Fatalf("query snapshot audit: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			t.Fatalf("scan snapshot audit: %v", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("snapshot audit rows: %v", err)
	}
	wantStatuses := []string{"REQUESTED", "STARTED", "COMPLETED"}
	if len(statuses) != len(wantStatuses) {
		t.Fatalf("snapshot statuses = %#v, want %#v", statuses, wantStatuses)
	}
	for i := range wantStatuses {
		if statuses[i] != wantStatuses[i] {
			t.Fatalf("snapshot statuses = %#v, want %#v", statuses, wantStatuses)
		}
	}
}

func TestRelayProcessSignalsRebuildsSnapshotAndRetriesDeadOutbox(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	clearRelaySignals(t, db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)

	var outboxID int64
	if err := db.QueryRow(ctx, `
		SELECT e.id
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1
		ORDER BY e.seq
		LIMIT 1
	`, auctionRow.ID).Scan(&outboxID); err != nil {
		t.Fatalf("select outbox id: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'DEAD',
		    attempts = 3,
		    last_error = 'test poison',
		    last_error_class = $2,
		    last_error_retriable = false,
		    last_error_at = now()
		WHERE outbox_id = $1
	`, outboxID, ErrorClassPayloadInvalid); err != nil {
		t.Fatalf("mark dead outbox: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO system_control_signals (signal_type, target_type, target_id, requested_by, reason)
		VALUES
		  ($1, 'auction', $2, 'host_1', 'test rebuild'),
		  ($3, 'outbox', $4::bigint::text, 'host_1', 'test retry')
	`, SignalForceSnapshotRebuild, auctionRow.ID, SignalRetryDeadOutbox, outboxID); err != nil {
		t.Fatalf("insert signals: %v", err)
	}

	relay := NewRelay(db, rdb, "signal-worker")
	processRelaySignalsUntil(t, relay, 4, func() bool {
		var succeeded int
		if err := db.QueryRow(ctx, `
			SELECT count(*)
			FROM system_control_signals
			WHERE status = 'SUCCEEDED'
			  AND signal_type IN ($1, $2)
			  AND target_id IN ($3, $4::bigint::text)
		`, SignalForceSnapshotRebuild, SignalRetryDeadOutbox, auctionRow.ID, outboxID).Scan(&succeeded); err != nil {
			t.Fatalf("count succeeded signals: %v", err)
		}
		return succeeded == 2
	})
	var succeeded int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM system_control_signals
		WHERE status = 'SUCCEEDED'
		  AND signal_type IN ($1, $2)
		  AND target_id IN ($3, $4::bigint::text)
	`, SignalForceSnapshotRebuild, SignalRetryDeadOutbox, auctionRow.ID, outboxID).Scan(&succeeded); err != nil {
		t.Fatalf("count succeeded signals: %v", err)
	}
	if succeeded != 2 {
		t.Fatalf("succeeded signals = %d, want 2", succeeded)
	}
	var deliveryStatus string
	var lastError *string
	if err := db.QueryRow(ctx, `
		SELECT status, last_error
		FROM outbox_delivery
		WHERE outbox_id = $1
	`, outboxID).Scan(&deliveryStatus, &lastError); err != nil {
		t.Fatalf("select retried delivery: %v", err)
	}
	if deliveryStatus != StatusPending || lastError != nil {
		t.Fatalf("retried delivery status=%s last_error=%v, want PENDING nil", deliveryStatus, lastError)
	}
	stored, err := rdb.Get(ctx, "auction:"+auctionRow.ID+":snapshot").Bytes()
	if err != nil {
		t.Fatalf("Get snapshot after signal: %v", err)
	}
	if len(stored) == 0 {
		t.Fatalf("empty snapshot after signal")
	}
}

func TestRelayProcessSignalsPauseAndResumeShard(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	clearRelaySignals(t, db)
	auctionRow := createActiveAuctionForRelay(t, repo, db)
	quiesceOutboxExcept(t, db, auctionRow.ID)

	var shardID int
	if err := db.QueryRow(ctx, `
		SELECT shard_id
		FROM outbox_delivery
		WHERE auction_id = $1
		ORDER BY outbox_id
		LIMIT 1
	`, auctionRow.ID).Scan(&shardID); err != nil {
		t.Fatalf("select shard id: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM outbox_relay_shard_leases WHERE shard_id = $1`, shardID); err != nil {
		t.Fatalf("clear shard lease: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO system_control_signals (signal_type, target_type, target_id, requested_by, reason)
		VALUES
		  ($1, 'relay_shard', $2::int::text, 'host_1', 'test pause'),
		  ($3, 'relay_shard', $2::int::text, 'host_1', 'test resume')
	`, SignalPauseRelayShard, shardID, SignalResumeRelayShard); err != nil {
		t.Fatalf("insert shard signals: %v", err)
	}

	relay := NewRelay(db, rdb, "signal-shard-worker")
	processRelaySignalsUntil(t, relay, 1, func() bool {
		var ownerID string
		err := db.QueryRow(ctx, `
			SELECT owner_id
			FROM outbox_relay_shard_leases
			WHERE shard_id = $1
		`, shardID).Scan(&ownerID)
		return err == nil && ownerID == "paused:"+relay.workerID
	})
	var ownerID string
	if err := db.QueryRow(ctx, `
		SELECT owner_id
		FROM outbox_relay_shard_leases
		WHERE shard_id = $1
	`, shardID).Scan(&ownerID); err != nil {
		t.Fatalf("select paused shard lease: %v", err)
	}
	if ownerID != "paused:"+relay.workerID {
		t.Fatalf("paused owner = %q, want %q", ownerID, "paused:"+relay.workerID)
	}
	if _, err := db.Exec(ctx, `
		UPDATE system_control_signals
		SET locked_until = NULL
		WHERE signal_type = $1
		  AND target_type = 'relay_shard'
		  AND target_id = $2::int::text
		  AND reason = 'test resume'
	`, SignalResumeRelayShard, shardID); err != nil {
		t.Fatalf("unlock resume signal: %v", err)
	}
	processRelaySignalsUntil(t, relay, 1, func() bool {
		var remaining int
		if err := db.QueryRow(ctx, `
			SELECT count(*)
			FROM outbox_relay_shard_leases
			WHERE shard_id = $1 AND owner_id LIKE 'paused:%'
		`, shardID).Scan(&remaining); err != nil {
			t.Fatalf("count paused shard leases: %v", err)
		}
		return remaining == 0
	})
	var remaining int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_relay_shard_leases
		WHERE shard_id = $1 AND owner_id LIKE 'paused:%'
	`, shardID).Scan(&remaining); err != nil {
		t.Fatalf("count paused shard leases: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("paused shard leases = %d, want 0", remaining)
	}
}

func TestRelayProcessSignalsIgnoresRedisEngineSignals(t *testing.T) {
	db := openDB(t)
	rdb := openRedis(t)
	ctx := context.Background()
	clearRelaySignals(t, db)

	if _, err := db.Exec(ctx, `
		INSERT INTO system_control_signals (signal_type, target_type, target_id, requested_by, reason)
		VALUES ('reconcile_redis_engine', 'auction', 'auc_live', 'host_1', 'redis engine should own this')
	`); err != nil {
		t.Fatalf("insert redis engine signal: %v", err)
	}

	relay := NewRelay(db, rdb, "signal-filter-worker")
	processed, err := relay.ProcessSignals(ctx, 4)
	if err != nil {
		t.Fatalf("ProcessSignals: %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0", processed)
	}

	var status string
	if err := db.QueryRow(ctx, `
		SELECT status
		FROM system_control_signals
		WHERE signal_type = 'reconcile_redis_engine'
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&status); err != nil {
		t.Fatalf("select redis engine signal status: %v", err)
	}
	if status != "PENDING" {
		t.Fatalf("redis engine signal status = %q, want PENDING", status)
	}
}

func clearRelaySignals(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		DELETE FROM system_control_signals
		WHERE signal_type = ANY($1)
	`, []string{
		SignalForceSnapshotRebuild,
		SignalRetryDeadOutbox,
		SignalPauseRelayShard,
		SignalResumeRelayShard,
	}); err != nil {
		t.Fatalf("clear relay signals: %v", err)
	}
}

func processRelaySignalsUntil(t *testing.T, relay *Relay, limit int, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		_, err := relay.ProcessSignals(context.Background(), limit)
		if err != nil {
			lastErr = err
		}
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("ProcessSignals did not reach expected state: %v", lastErr)
	}
	t.Fatalf("ProcessSignals did not reach expected state")
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
		addr = "localhost:6380"
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
	isolateRelayTestAuction(t, db, started.ID)
	return started
}

func isolateRelayTestAuction(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	quiesceOutboxExcept(t, db, auctionID)
	if _, err := db.Exec(context.Background(), `
		INSERT INTO outbox_relay_shard_leases (shard_id, owner_id, lease_until)
		SELECT DISTINCT shard_id, 'relay-test-isolation', now() + interval '30 seconds'
		FROM outbox_delivery
		WHERE auction_id = $1
		ON CONFLICT (shard_id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id,
		    lease_until = EXCLUDED.lease_until,
		    renewed_at = now()
	`, auctionID); err != nil {
		t.Fatalf("reserve relay test shard leases: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM outbox_relay_shard_leases WHERE owner_id = 'relay-test-isolation'`)
	})
	if _, err := db.Exec(context.Background(), `
		UPDATE system_control_signals
		SET status = 'FAILED',
		    processed_at = COALESCE(processed_at, now()),
		    locked_by = NULL,
		    locked_until = NULL,
		    error_message = COALESCE(error_message, 'test isolation cleanup')
		WHERE status IN ('PENDING','PROCESSING')
	`); err != nil {
		t.Fatalf("quiesce pending control signals: %v", err)
	}
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
		    locked_until = NULL,
		    next_attempt_at = now() + interval '1 day'
		FROM outbox_events e
		WHERE e.id = d.outbox_id
		  AND (e.auction_id IS DISTINCT FROM $1)
		  AND d.status NOT IN ('PUBLISHED','DEAD')
	`, auctionID); err != nil {
		t.Fatalf("quiesce outbox: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		UPDATE outbox_delivery d
		SET published_at = '1900-01-01 00:00:00+00'::timestamptz,
		    next_attempt_at = now() + interval '1 day'
		FROM outbox_events e
		WHERE e.id = d.outbox_id
		  AND (e.auction_id IS DISTINCT FROM $1)
		  AND d.status = 'PUBLISHED'
	`, auctionID); err != nil {
		t.Fatalf("quiesce published outbox timestamps: %v", err)
	}
}

func isolateClaimableOutbox(t *testing.T, db *pgxpool.Pool, auctionID string, outboxID int64) {
	t.Helper()
	var shardID int
	if err := db.QueryRow(context.Background(), `
		SELECT shard_id
		FROM outbox_delivery
		WHERE outbox_id = $1
	`, outboxID).Scan(&shardID); err != nil {
		t.Fatalf("select target shard: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED',
		    published_at = COALESCE(published_at, now()),
		    locked_by = NULL,
		    locked_until = NULL
		WHERE shard_id = $1
		  AND outbox_id <> $2
		  AND status NOT IN ('PUBLISHED','DEAD')
	`, shardID, outboxID); err != nil {
		t.Fatalf("quiesce target shard: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		UPDATE outbox_delivery
		SET status = CASE WHEN outbox_id = $2 THEN 'PENDING' ELSE 'PUBLISHED' END,
		    published_at = CASE WHEN outbox_id = $2 THEN NULL ELSE COALESCE(published_at, now()) END,
		    next_attempt_at = now(),
		    locked_by = NULL,
		    locked_until = NULL
		WHERE auction_id = $1
	`, auctionID, outboxID); err != nil {
		t.Fatalf("isolate claimable outbox: %v", err)
	}
}

func renewRelayLeases(t *testing.T, relay *Relay) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = relay.db.Exec(context.Background(), `DELETE FROM outbox_relay_shard_leases WHERE owner_id = $1`, relay.workerID)
	})
	if err := relay.renewShardLeases(context.Background()); err != nil {
		t.Fatalf("renew relay leases: %v", err)
	}
}

func ownAuctionShard(t *testing.T, db *pgxpool.Pool, auctionID string, ownerID string) {
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
		t.Fatalf("own auction shard: %v", err)
	}
}

func nilLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
