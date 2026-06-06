package scheduler

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-auction/backend/internal/auction"
	apierrors "live-auction/backend/internal/platform/errors"
)

// mockFencer records FenceAuction calls for H3 scheduler fencing tests.
type mockFencer struct {
	mu      sync.Mutex
	reasons []string
}

func (m *mockFencer) FenceAuction(_ context.Context, _ string, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reasons = append(m.reasons, reason)
}

func (m *mockFencer) recordedReasons() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.reasons...)
}

func TestEndAuctionNoWinnerMarksEndedAndWritesOutbox(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	row := createActiveAuction(t, repo, db)
	forceAuctionEndAt(t, db, row.ID, time.Now().UTC().Add(-time.Second))

	ok, err := NewRunner(db, "end-no-winner").ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected scheduler job")
	}
	got, err := repo.GetAuction(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetAuction: %v", err)
	}
	if got.Status != auction.StatusEnded {
		t.Fatalf("status = %s, want ENDED", got.Status)
	}
	assertNoOrder(t, db, row.ID)
	assertJobStatus(t, db, JobTypeEndAuction, row.ID, StatusSucceeded)
	assertAuctionEvent(t, db, row.ID, "auction_ended")
}

func TestEndAuctionWinnerMarksSoldAndCreatesOrder(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	row := createActiveAuction(t, repo, db)
	bid := auction.BidInput{ClientBidID: "end-winner-" + uuid.NewString(), AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, row.ID, "user_1", bid.ClientBidID, bid, "tr_scheduler"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	forceAuctionEndAt(t, db, row.ID, time.Now().UTC().Add(-time.Second))

	ok, err := NewRunner(db, "end-winner").ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected scheduler job")
	}
	got, err := repo.GetAuction(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetAuction: %v", err)
	}
	if got.Status != auction.StatusSold {
		t.Fatalf("status = %s, want SOLD", got.Status)
	}
	orderID, status, depositStatus := selectOrder(t, db, row.ID)
	if orderID == "" || status != auction.OrderStatusPending || depositStatus != auction.DepositStatusHeld {
		t.Fatalf("order = %s/%s/%s, want pending held", orderID, status, depositStatus)
	}
	assertJobStatus(t, db, JobTypeEndAuction, row.ID, StatusSucceeded)
	assertAuctionEvent(t, db, row.ID, "auction_sold")
}

func TestEndAuctionJobReschedulesWhenBidExtendedEndAt(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	row := createActiveAuction(t, repo, db)
	future := time.Now().UTC().Add(20 * time.Second)
	forceAuctionEndAt(t, db, row.ID, future)
	forceEndJobDue(t, db, row.ID)

	ok, err := NewRunner(db, "end-reschedule").ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected scheduler job")
	}
	got, err := repo.GetAuction(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetAuction: %v", err)
	}
	if got.Status != auction.StatusActive {
		t.Fatalf("status = %s, want ACTIVE", got.Status)
	}
	assertJobRunAtNear(t, db, row.ID, future)
}

func TestSchedulerCrashLeaseExpiresAndAnotherWorkerCompletesOnce(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	row := createActiveAuction(t, repo, db)
	forceAuctionEndAt(t, db, row.ID, time.Now().UTC().Add(-time.Second))
	forceEndJobRunningExpired(t, db, row.ID)

	ok, err := NewRunner(db, "lease-worker").ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected expired lease job")
	}
	ok, err = NewRunner(db, "second-worker").ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne second: %v", err)
	}
	if ok {
		t.Fatalf("second worker processed already succeeded job")
	}
	assertAuctionEventCount(t, db, row.ID, "auction_ended", 1)
}

func TestOrderExpireMarksPendingOrderOnceAndPaymentRejects(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	row := createActiveAuction(t, repo, db)
	bid := auction.BidInput{ClientBidID: "order-expire-" + uuid.NewString(), AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, row.ID, "user_1", bid.ClientBidID, bid, "tr_scheduler"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	forceAuctionEndAt(t, db, row.ID, time.Now().UTC().Add(-time.Second))
	if ok, err := NewRunner(db, "order-create").ProcessOne(ctx); err != nil || !ok {
		t.Fatalf("ProcessOne end ok=%v err=%v", ok, err)
	}
	orderID, _, _ := selectOrder(t, db, row.ID)
	forceOrderExpireAt(t, db, orderID, time.Now().UTC().Add(-time.Second))

	if ok, err := NewRunner(db, "order-expire").ProcessOne(ctx); err != nil || !ok {
		t.Fatalf("ProcessOne expire ok=%v err=%v", ok, err)
	}
	_, status, depositStatus := selectOrder(t, db, row.ID)
	if status != auction.OrderStatusExpired || depositStatus != auction.DepositStatusForfeit {
		t.Fatalf("order = %s/%s, want expired/forfeited", status, depositStatus)
	}
	if _, err := repo.PayMock(ctx, orderID, "user_1", "pay-expired", auction.PaymentInput{Confirm: true}, "tr_pay"); !hasCode(err, apierrors.CodeOrderAlreadyExpired) {
		t.Fatalf("PayMock expired err = %v, want ORDER_ALREADY_EXPIRED", err)
	}
	assertAuctionEvent(t, db, row.ID, "order_expired")
	assertOutboxEvent(t, db, row.ID, "order_expired")
	if ok, err := NewRunner(db, "order-expire-again").ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne again: %v", err)
	} else if ok {
		t.Fatalf("expired order job processed twice")
	}
}

func TestRetryJitterStaggersFailedJobs(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	jobA := "job_retry_a_" + uuid.NewString()
	jobB := "job_retry_b_" + uuid.NewString()
	insertUnsupportedJob(t, db, jobA, now.Add(-time.Second))
	insertUnsupportedJob(t, db, jobB, now.Add(-time.Second))

	runner := NewRunner(db, "retry-jitter")
	runner.now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		ok, err := runner.ProcessOne(ctx)
		if err == nil {
			t.Fatalf("ProcessOne %d err = nil, want unsupported job failure", i)
		}
		if !ok {
			t.Fatalf("ProcessOne %d ok = false, want failed job processed", i)
		}
	}

	nextA := selectNextAttemptAt(t, db, jobA)
	nextB := selectNextAttemptAt(t, db, jobB)
	if nextA.Equal(nextB) {
		t.Fatalf("next_attempt_at not staggered: %s", nextA)
	}
}

func TestClockStepBackwardWritesAnomalyAndSkipsJob(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	row := createActiveAuction(t, repo, db)
	forceAuctionEndAt(t, db, row.ID, time.Now().UTC().Add(-time.Second))

	runner := NewRunner(db, "clock-rollback")
	firstNow := time.Now().UTC()
	runner.now = func() time.Time { return firstNow }
	if ok, err := runner.ProcessOne(ctx); err != nil || !ok {
		t.Fatalf("initial ProcessOne ok=%v err=%v", ok, err)
	}

	second := createActiveAuction(t, repo, db)
	forceAuctionEndAt(t, db, second.ID, time.Now().UTC().Add(-time.Second))
	rolledBack := firstNow.Add(-2 * time.Second)
	runner.now = func() time.Time { return rolledBack }
	ok, err := runner.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("rollback ProcessOne: %v", err)
	}
	if ok {
		t.Fatalf("rollback ProcessOne processed job")
	}
	got, err := repo.GetAuction(ctx, second.ID)
	if err != nil {
		t.Fatalf("GetAuction: %v", err)
	}
	if got.Status != auction.StatusActive {
		t.Fatalf("status = %s, want ACTIVE because rollback pauses scheduler", got.Status)
	}
	var anomalies int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM system_anomaly_events WHERE type = 'CLOCK_STEP_BACKWARD'`).Scan(&anomalies); err != nil {
		t.Fatalf("count anomalies: %v", err)
	}
	if anomalies == 0 {
		t.Fatalf("expected CLOCK_STEP_BACKWARD anomaly")
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

func createActiveAuction(t *testing.T, repo *auction.Repository, db *pgxpool.Pool) auction.Auction {
	t.Helper()
	roomID := "room_scheduler_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN')`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	item, err := repo.CreateItem(context.Background(), auction.CreateItemInput{Title: "Scheduler Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	row, err := repo.CreateAuction(context.Background(), auction.CreateAuctionInput{
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
	}, "tr_scheduler")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if _, err := repo.Schedule(context.Background(), row.ID, nil, "tr_scheduler"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	started, err := repo.Start(context.Background(), row.ID, "tr_scheduler")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return started
}

func quiesceSchedulerJobs(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		UPDATE scheduler_jobs
		SET status = 'SUCCEEDED', locked_by = NULL, locked_until = NULL, updated_at = now()
		WHERE status <> 'SUCCEEDED'
	`); err != nil {
		t.Fatalf("quiesce scheduler jobs: %v", err)
	}
}

func forceAuctionEndAt(t *testing.T, db *pgxpool.Pool, auctionID string, endAt time.Time) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `UPDATE auctions SET end_at = $2 WHERE id = $1`, auctionID, endAt); err != nil {
		t.Fatalf("force end_at: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		UPDATE scheduler_jobs
		SET run_at = $2, next_attempt_at = $2, status = 'PENDING', locked_by = NULL, locked_until = NULL
		WHERE job_type = 'END_AUCTION' AND target_id = $1
	`, auctionID, endAt); err != nil {
		t.Fatalf("force end job run_at: %v", err)
	}
}

func forceEndJobDue(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		UPDATE scheduler_jobs SET run_at = now() - interval '1 second', next_attempt_at = now() - interval '1 second'
		WHERE job_type = 'END_AUCTION' AND target_id = $1
	`, auctionID); err != nil {
		t.Fatalf("force job due: %v", err)
	}
}

func forceEndJobRunningExpired(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		UPDATE scheduler_jobs
		SET status = 'RUNNING', locked_by = 'crashed-worker', locked_until = now() - interval '1 second',
		    run_at = now() - interval '1 second', next_attempt_at = now() - interval '1 second'
		WHERE job_type = 'END_AUCTION' AND target_id = $1
	`, auctionID); err != nil {
		t.Fatalf("force expired running job: %v", err)
	}
}

func forceOrderExpireAt(t *testing.T, db *pgxpool.Pool, orderID string, expireAt time.Time) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `UPDATE orders SET expire_at = $2 WHERE id = $1`, orderID, expireAt); err != nil {
		t.Fatalf("force order expire_at: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		UPDATE scheduler_jobs
		SET run_at = $2, next_attempt_at = $2, status = 'PENDING', locked_by = NULL, locked_until = NULL
		WHERE job_type = 'EXPIRE_ORDER' AND target_id = $1
	`, orderID, expireAt); err != nil {
		t.Fatalf("force order expire job: %v", err)
	}
}

func insertUnsupportedJob(t *testing.T, db *pgxpool.Pool, id string, runAt time.Time) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO scheduler_jobs (id, job_type, target_type, target_id, idempotency_key, run_at, status, next_attempt_at)
		VALUES ($1, 'UNSUPPORTED_TEST', 'test', $1, $1, $2, 'PENDING', $2)
	`, id, runAt); err != nil {
		t.Fatalf("insert unsupported job: %v", err)
	}
}

func selectNextAttemptAt(t *testing.T, db *pgxpool.Pool, id string) time.Time {
	t.Helper()
	var next time.Time
	if err := db.QueryRow(context.Background(), `SELECT next_attempt_at FROM scheduler_jobs WHERE id = $1`, id).Scan(&next); err != nil {
		t.Fatalf("select next_attempt_at: %v", err)
	}
	return next
}

func assertNoOrder(t *testing.T, db *pgxpool.Pool, auctionID string) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM orders WHERE auction_id = $1`, auctionID).Scan(&count); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if count != 0 {
		t.Fatalf("orders = %d, want 0", count)
	}
}

func selectOrder(t *testing.T, db *pgxpool.Pool, auctionID string) (string, string, string) {
	t.Helper()
	var id string
	var status string
	var depositStatus string
	if err := db.QueryRow(context.Background(), `SELECT id, status, deposit_status FROM orders WHERE auction_id = $1`, auctionID).Scan(&id, &status, &depositStatus); err != nil {
		t.Fatalf("select order: %v", err)
	}
	return id, status, depositStatus
}

func assertJobStatus(t *testing.T, db *pgxpool.Pool, jobType string, targetID string, want string) {
	t.Helper()
	var status string
	if err := db.QueryRow(context.Background(), `SELECT status FROM scheduler_jobs WHERE job_type = $1 AND target_id = $2`, jobType, targetID).Scan(&status); err != nil {
		t.Fatalf("select job status: %v", err)
	}
	if status != want {
		t.Fatalf("job status = %s, want %s", status, want)
	}
}

func assertJobRunAtNear(t *testing.T, db *pgxpool.Pool, auctionID string, want time.Time) {
	t.Helper()
	var runAt time.Time
	var status string
	if err := db.QueryRow(context.Background(), `SELECT run_at, status FROM scheduler_jobs WHERE job_type = 'END_AUCTION' AND target_id = $1`, auctionID).Scan(&runAt, &status); err != nil {
		t.Fatalf("select job run_at: %v", err)
	}
	if status != StatusPending {
		t.Fatalf("job status = %s, want PENDING", status)
	}
	if absDuration(runAt.Sub(want)) > time.Second {
		t.Fatalf("run_at = %s, want near %s", runAt, want)
	}
}

func assertAuctionEvent(t *testing.T, db *pgxpool.Pool, auctionID string, eventType string) {
	t.Helper()
	assertAuctionEventCount(t, db, auctionID, eventType, 1)
}

func assertAuctionEventCount(t *testing.T, db *pgxpool.Pool, auctionID string, eventType string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM auction_events WHERE auction_id = $1 AND event_type = $2`, auctionID, eventType).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != want {
		t.Fatalf("%s events = %d, want %d", eventType, count, want)
	}
}

func assertOutboxEvent(t *testing.T, db *pgxpool.Pool, auctionID string, eventType string) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE auction_id = $1 AND event_type = $2`, auctionID, eventType).Scan(&count); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if count == 0 {
		t.Fatalf("outbox event %s not found for auction %s", eventType, auctionID)
	}
}

func hasCode(err error, code apierrors.Code) bool {
	var apiErr apierrors.APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func TestEndAuctionSoldCallsFencer(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	row := createActiveAuction(t, repo, db)
	bid := auction.BidInput{ClientBidID: "fencer-sold-" + uuid.NewString(), AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, row.ID, "user_1", bid.ClientBidID, bid, "tr_fencer_sold"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	forceAuctionEndAt(t, db, row.ID, time.Now().UTC().Add(-time.Second))

	fencer := &mockFencer{}
	ok, err := NewRunner(db, "fencer-sold").WithFencer(fencer).ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected scheduler job to be processed")
	}

	got, err := repo.GetAuction(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetAuction: %v", err)
	}
	if got.Status != auction.StatusSold {
		t.Fatalf("status = %s, want SOLD", got.Status)
	}
	reasons := fencer.recordedReasons()
	if len(reasons) != 1 || reasons[0] != "SCHEDULER_SOLD" {
		t.Fatalf("fencer reasons = %v, want [SCHEDULER_SOLD]", reasons)
	}
}

func TestEndAuctionEndedCallsFencer(t *testing.T) {
	db := openDB(t)
	quiesceSchedulerJobs(t, db)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	row := createActiveAuction(t, repo, db)
	forceAuctionEndAt(t, db, row.ID, time.Now().UTC().Add(-time.Second))

	fencer := &mockFencer{}
	ok, err := NewRunner(db, "fencer-ended").WithFencer(fencer).ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected scheduler job to be processed")
	}

	got, err := repo.GetAuction(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetAuction: %v", err)
	}
	if got.Status != auction.StatusEnded {
		t.Fatalf("status = %s, want ENDED", got.Status)
	}
	reasons := fencer.recordedReasons()
	if len(reasons) != 1 || reasons[0] != "SCHEDULER_ENDED" {
		t.Fatalf("fencer reasons = %v, want [SCHEDULER_ENDED]", reasons)
	}
}
