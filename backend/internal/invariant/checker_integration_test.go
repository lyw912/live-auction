package invariant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCheckerPassesCleanFixture(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Status != StatusPass {
		t.Fatalf("status = %s, want PASS; failing checks: %#v", report.Status, nonPassing(report))
	}
}

func TestCheckerDetectsSeqGap(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	if _, err := db.Exec(ctx, `
		DELETE FROM outbox_delivery d
		USING outbox_events e
		WHERE e.id = d.outbox_id
		  AND e.auction_id = $1
		  AND e.seq = 2
	`, auctionID); err != nil {
		t.Fatalf("delete delivery: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM outbox_events WHERE auction_id = $1 AND seq = 2`, auctionID); err != nil {
		t.Fatalf("delete outbox: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM auction_events WHERE auction_id = $1 AND seq = 2`, auctionID); err != nil {
		t.Fatalf("delete event: %v", err)
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "auction_event_seq_contiguous", StatusFail)
}

func TestCheckerDetectsWinnerOrderMismatch(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	if _, err := db.Exec(ctx, `UPDATE orders SET amount_cents = amount_cents + 100 WHERE auction_id = $1`, auctionID); err != nil {
		t.Fatalf("corrupt order: %v", err)
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "sold_order_matches_auction", StatusFail)
}

func TestCheckerDetectsOutboxOrderingAndDeliveryGaps(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	if _, err := db.Exec(ctx, `
		WITH targets AS (
			SELECT d.outbox_id, e.seq
			FROM outbox_delivery d
			JOIN outbox_events e ON e.id = d.outbox_id
			WHERE e.auction_id = $1
			  AND e.seq IN (2, 3)
		)
		UPDATE outbox_delivery d
		SET status = CASE WHEN targets.seq = 2 THEN 'PENDING' WHEN targets.seq = 3 THEN 'PUBLISHED' ELSE d.status END,
		    published_at = CASE WHEN targets.seq = 3 THEN now() ELSE d.published_at END
		FROM targets
		WHERE targets.outbox_id = d.outbox_id
	`, auctionID); err != nil {
		t.Fatalf("corrupt outbox delivery: %v", err)
	}
	var corrupted int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1
		  AND ((e.seq = 2 AND d.status = 'PENDING') OR (e.seq = 3 AND d.status = 'PUBLISHED'))
	`, auctionID).Scan(&corrupted); err != nil {
		t.Fatalf("count corrupted delivery rows: %v", err)
	}
	if corrupted != 2 {
		t.Fatalf("corrupted delivery rows = %d, want 2", corrupted)
	}
	if _, err := db.Exec(ctx, `
		DELETE FROM outbox_delivery
		WHERE outbox_id = (SELECT id FROM outbox_events WHERE auction_id = $1 AND seq = 1)
	`, auctionID); err != nil {
		t.Fatalf("delete outbox delivery: %v", err)
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "outbox_same_auction_head_of_line", StatusFail)
	assertCheck(t, report, "outbox_delivery_exists", StatusFail)
}

func TestCheckerDetectsOutboxPayloadDrift(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	if _, err := db.Exec(ctx, `
		UPDATE outbox_events
		SET payload_json = payload_json || '{"drift":true}'::jsonb
		WHERE auction_id = $1 AND seq = 4
	`, auctionID); err != nil {
		t.Fatalf("corrupt outbox payload: %v", err)
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "outbox_payload_matches_auction_event", StatusFail)
}

func TestCheckerDetectsIdempotencyHashMismatch(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	if _, err := db.Exec(ctx, `
		UPDATE idempotency_records
		SET request_hash = repeat('0', 64)
		WHERE scope_type = 'bid' AND scope_id = $1
	`, auctionID); err != nil {
		t.Fatalf("corrupt idempotency: %v", err)
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "bid_idempotency_consistent", StatusFail)
}

func TestCheckerDetectsPaymentIdempotencyMismatch(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	var orderID string
	var winnerID string
	if err := db.QueryRow(ctx, `SELECT id, winner_id FROM orders WHERE auction_id = $1`, auctionID).Scan(&orderID, &winnerID); err != nil {
		t.Fatalf("read order: %v", err)
	}
	paymentHash := hashFixture(fmt.Sprintf("payment:v1|%s|%s|pay-key-1", orderID, winnerID))
	if _, err := db.Exec(ctx, `
		INSERT INTO idempotency_records (
			scope_type, scope_id, user_id, idempotency_key, request_hash, status,
			attempts, http_status, result_code, response_json, completed_at
		) VALUES ('payment', $1, $2, 'pay-key-1', $3, 'COMPLETED', 1, 200, 'PAID', '{"order_status":"PAID"}'::jsonb, now())
	`, orderID, winnerID, paymentHash); err != nil {
		t.Fatalf("insert payment idempotency: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE idempotency_records
		SET request_hash = repeat('1', 64)
		WHERE scope_type = 'payment' AND scope_id = $1
	`, orderID); err != nil {
		t.Fatalf("corrupt payment idempotency: %v", err)
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "payment_idempotency_consistent", StatusFail)
}

func TestCheckerDetectsRedisEngineSettlementFailures(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	if _, err := db.Exec(ctx, `
		UPDATE auctions SET engine_epoch = 1, engine_seq = 4 WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("seed engine auction fields: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO redis_engine_settlements (
			auction_id, stream_id, engine_epoch, engine_seq, result, status,
			attempts, payload_json, ledger_source, dlq_topic, dlq_error, dlq_at
		) VALUES (
			$1, 'kafka:auction.bid-events:0:4', 1, 4, 'ENGINE_SOLD', 'FAILED',
			4, '{}'::jsonb, 'kafka', 'auction.dlq', 'poison', now()
		)
	`, auctionID); err != nil {
		t.Fatalf("insert failed settlement: %v", err)
	}
	for _, seq := range []int{1, 3} {
		if _, err := db.Exec(ctx, `
			INSERT INTO redis_engine_settlements (
				auction_id, stream_id, engine_epoch, engine_seq, result, status,
				attempts, payload_json, ledger_source, settled_at
			) VALUES (
				$1, $2, 1, $3, 'ENGINE_REJECTED', 'SETTLED',
				1, '{}'::jsonb, 'kafka', now()
			)
		`, auctionID, fmt.Sprintf("kafka:auction.bid-events:0:%d", seq), seq); err != nil {
			t.Fatalf("insert gap settlement seq %d: %v", seq, err)
		}
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "redis_engine_settlement_seq_contiguous", StatusFail)
	assertCheck(t, report, "redis_engine_ledger_no_failed_or_dlq", StatusFail)
	assertCheck(t, report, "redis_engine_seq_matches_settlement", StatusFail)
}

func TestCheckerPassesCleanRedisEngineSettlement(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	if _, err := db.Exec(ctx, `
		UPDATE auctions SET engine_epoch = 1, engine_seq = 4 WHERE id = $1
	`, auctionID); err != nil {
		t.Fatalf("seed engine auction fields: %v", err)
	}
	for seq := 1; seq <= 4; seq++ {
		result := "ENGINE_REJECTED"
		if seq == 4 {
			result = "ENGINE_SOLD"
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO redis_engine_settlements (
				auction_id, stream_id, engine_epoch, engine_seq, result, status,
				attempts, payload_json, ledger_source, ledger_topic, ledger_partition, ledger_offset, ledger_key, settled_at
			) VALUES (
				$1, $2, 1, $3, $4, 'SETTLED',
				1, '{}'::jsonb, 'kafka', 'auction.bid-events', 0, $3, $1, now()
			)
		`, auctionID, fmt.Sprintf("kafka:auction.bid-events:0:%d", seq), seq, result); err != nil {
			t.Fatalf("insert settlement seq %d: %v", seq, err)
		}
	}
	if _, err := db.Exec(ctx, `
		UPDATE bids SET engine_epoch = 1, engine_seq = 4 WHERE auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("seed bid engine fields: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE auction_events SET engine_epoch = 1, engine_seq = seq WHERE auction_id = $1
	`, auctionID); err != nil {
		t.Fatalf("seed event engine fields: %v", err)
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "redis_engine_settlement_seq_contiguous", StatusPass)
	assertCheck(t, report, "redis_engine_ledger_no_failed_or_dlq", StatusPass)
	assertCheck(t, report, "redis_engine_seq_matches_settlement", StatusPass)
	assertCheck(t, report, "redis_engine_accepted_settlement_has_bid_event", StatusPass)
}

func TestCheckerDetectsCrossRoomPayloadLeak(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	auctionID := createCleanSoldAuction(t, db)

	if _, err := db.Exec(ctx, `
		UPDATE auction_events
		SET payload_json = payload_json || '{"room_id":"room_wrong"}'::jsonb
		WHERE auction_id = $1 AND seq = 1
	`, auctionID); err != nil {
		t.Fatalf("corrupt payload: %v", err)
	}

	report, err := NewChecker(db).Run(ctx, Options{AuctionID: auctionID, MaxDetails: 10})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCheck(t, report, "no_cross_room_payload_leak", StatusFail)
}

func openIntegrationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	cleanupInvariantFixtures(t, db)
	t.Cleanup(db.Close)
	return db
}

func createCleanSoldAuction(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()
	hostID := "host_inv_" + suffix
	userID := "user_inv_" + suffix
	roomID := "room_inv_" + suffix
	itemID := "item_inv_" + suffix
	auctionID := "auc_inv_" + suffix
	bidID := "bid_inv_" + suffix
	orderID := "ord_inv_" + suffix
	now := time.Now().UTC()
	bidHash := hashFixture(fmt.Sprintf("bid:v1|%s|%s|client_bid_1|2000", auctionID, userID))
	bidResponse := fmt.Sprintf(`{"result":"ACCEPTED_SOLD","bid_id":"%s","auction_id":"%s","seq":4,"current_price_cents":2000,"current_winner_id":"%s","server_time_ms":%d}`, bidID, auctionID, userID, now.UnixMilli())
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	mustExecTx(t, ctx, tx, `INSERT INTO users (id, role, display_name) VALUES ($1, 'host', 'Invariant Host'), ($2, 'user', 'Invariant User')`, hostID, userID)
	mustExecTx(t, ctx, tx, `INSERT INTO rooms (id, host_id, status) VALUES ($1, $2, 'OPEN')`, roomID, hostID)
	mustExecTx(t, ctx, tx, `INSERT INTO items (id, title, status) VALUES ($1, 'Invariant Item', 'READY')`, itemID)
	mustExecTx(t, ctx, tx, `
		INSERT INTO auctions (
			id, room_id, item_id, status, current_price_cents, current_winner_id,
			start_price_cents, increment_cents, cap_price_cents, start_at, end_at,
			version, seq, accepted_bid_count, rule_version
		) VALUES ($1, $2, $3, 'SOLD', 2000, $4, 1000, 1000, 2000, $5, $6, 4, 4, 1, 1)
	`, auctionID, roomID, itemID, userID, now, now.Add(time.Minute))
	mustExecTx(t, ctx, tx, `
		INSERT INTO auction_rules (
			auction_id, rule_version, duration_seconds, extend_window_seconds, extend_by_seconds,
			max_extend_count, deposit_bps, deposit_floor_cents, deposit_cap_cents, frozen_at
		) VALUES ($1, 1, 60, 10, 10, 3, 1000, 0, 1000000, $2)
	`, auctionID, now)
	mustExecTx(t, ctx, tx, `
		INSERT INTO bids (
			id, auction_id, user_id, client_bid_id, amount_cents, seq, status, request_hash,
			response_json, trace_id
		) VALUES ($1, $2, $3, 'client_bid_1', 2000, 4, 'ACCEPTED', $4, $5::jsonb, 'tr_inv')
	`, bidID, auctionID, userID, bidHash, bidResponse)
	mustExecTx(t, ctx, tx, `
		INSERT INTO orders (
			id, auction_id, winner_id, amount_cents, status, deposit_cents, deposit_status, expire_at
		) VALUES ($1, $2, $3, 2000, 'ORDER_PENDING', 200, 'HELD', $4)
	`, orderID, auctionID, userID, now.Add(15*time.Minute))
	mustExecTx(t, ctx, tx, `
		INSERT INTO idempotency_records (
			scope_type, scope_id, user_id, idempotency_key, request_hash, status,
			attempts, http_status, result_code, response_json, completed_at
		) VALUES ('bid', $1, $2, 'client_bid_1', $3, 'COMPLETED', 1, 200, 'ACCEPTED_SOLD', $4::jsonb, $5)
	`, auctionID, userID, bidHash, bidResponse, now)
	events := []struct {
		seq       int
		eventType string
		payload   string
	}{
		{1, "auction_created", `{"state_version":1}`},
		{2, "auction_scheduled", `{"state_version":2}`},
		{3, "auction_started", `{"state_version":3}`},
		{4, "auction_sold", fmt.Sprintf(`{"state_version":4,"bid_id":"%s","user_id":"%s","amount_cents":2000,"current_price_cents":2000,"result":"ACCEPTED_SOLD","order_id":"%s"}`, bidID, userID, orderID)},
	}
	for _, event := range events {
		var outboxID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO auction_events (auction_id, seq, event_type, payload_json, server_time_ms, trace_id)
			VALUES ($1, $2, $3, $4::jsonb, $5, 'tr_inv')
			RETURNING id
		`, auctionID, event.seq, event.eventType, event.payload, now.UnixMilli()).Scan(new(int64)); err != nil {
			t.Fatalf("insert auction event %d: %v", event.seq, err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO outbox_events (
				aggregate_type, aggregate_id, auction_id, seq, event_type,
				event_schema_version, event_key, payload_json, payload_sha256
			)
			VALUES ('auction', $1, $1, $2, $3, 1, $1, $4::jsonb,
			        encode(digest(convert_to($4::jsonb::text, 'UTF8'), 'sha256'), 'hex'))
			RETURNING id
		`, auctionID, event.seq, event.eventType, event.payload).Scan(&outboxID); err != nil {
			t.Fatalf("insert outbox event %d: %v", event.seq, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_delivery (outbox_id, status, attempts, published_at, auction_id, auction_seq, event_created_at)
			VALUES ($1, 'PUBLISHED', 1, $2, $3, $4, $2)
		`, outboxID, now, auctionID, event.seq); err != nil {
			t.Fatalf("insert outbox delivery %d: %v", event.seq, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return auctionID
}

func hashFixture(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mustExecTx(t *testing.T, ctx context.Context, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec fixture SQL: %v", err)
	}
}

func cleanupInvariantFixtures(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`DELETE FROM outbox_delivery WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id LIKE 'auc_inv_%')`,
		`DELETE FROM outbox_events WHERE auction_id LIKE 'auc_inv_%'`,
		`DELETE FROM auction_events WHERE auction_id LIKE 'auc_inv_%'`,
		`DELETE FROM idempotency_records WHERE (scope_type = 'bid' AND scope_id LIKE 'auc_inv_%') OR (scope_type = 'payment' AND scope_id LIKE 'ord_inv_%')`,
		`DELETE FROM bids WHERE auction_id LIKE 'auc_inv_%'`,
		`DELETE FROM orders WHERE auction_id LIKE 'auc_inv_%' OR id LIKE 'ord_inv_%'`,
		`DELETE FROM auction_rules WHERE auction_id LIKE 'auc_inv_%'`,
		`DELETE FROM auctions WHERE id LIKE 'auc_inv_%'`,
		`DELETE FROM items WHERE id LIKE 'item_inv_%'`,
		`DELETE FROM rooms WHERE id LIKE 'room_inv_%'`,
		`DELETE FROM users WHERE id LIKE 'user_inv_%' OR id LIKE 'host_inv_%'`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatalf("cleanup invariant fixtures: %v", err)
		}
	}
}

func nonPassing(report Report) []CheckResult {
	var checks []CheckResult
	for _, check := range report.Checks {
		if check.Status != StatusPass {
			checks = append(checks, check)
		}
	}
	return checks
}

func assertCheck(t *testing.T, report Report, name string, status Status) {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			if check.Status != status {
				t.Fatalf("%s status = %s, want %s; check=%#v; nonPassing=%#v", name, check.Status, status, check, nonPassing(report))
			}
			return
		}
	}
	t.Fatalf("check %s not found in %#v", name, report.Checks)
}
