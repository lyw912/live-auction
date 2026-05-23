package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-auction/backend/internal/observability"
)

const (
	JobTypeEndAuction  = "END_AUCTION"
	JobTypeExpireOrder = "EXPIRE_ORDER"

	StatusPending   = "PENDING"
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"
	StatusDead      = "DEAD"
)

type Runner struct {
	db       *pgxpool.Pool
	workerID string
	now      func() time.Time
	lastNow  time.Time
}

type Job struct {
	ID          string
	JobType     string
	TargetType  string
	TargetID    string
	Attempts    int
	MaxAttempts int
	RunAt       time.Time
}

func NewRunner(db *pgxpool.Pool, workerID string) *Runner {
	if workerID == "" {
		workerID = "scheduler-" + uuid.NewString()
	}
	return &Runner{
		db:       db,
		workerID: workerID,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (r *Runner) Run(ctx context.Context, log *slog.Logger, interval time.Duration) {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		processed, err := r.ProcessOne(ctx)
		if err != nil && log != nil {
			log.Warn("scheduler_process_failed", slog.String("error", err.Error()))
		}
		if !processed {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}
}

func (r *Runner) ProcessOne(ctx context.Context) (bool, error) {
	if paused, err := r.pauseOnClockRollback(ctx); err != nil || paused {
		return false, err
	}
	job, ok, err := r.claimOne(ctx)
	if err != nil || !ok {
		return ok, err
	}
	if err := r.processClaimed(ctx, job); err != nil {
		if markErr := r.markFailure(ctx, job, err); markErr != nil {
			return true, markErr
		}
		return true, err
	}
	return true, nil
}

func (r *Runner) pauseOnClockRollback(ctx context.Context) (bool, error) {
	now := r.now()
	if !r.lastNow.IsZero() && r.lastNow.Sub(now) > time.Second {
		if err := r.writeClockRollbackAnomaly(ctx, r.lastNow, now); err != nil {
			return true, err
		}
		r.lastNow = now
		return true, nil
	}
	r.lastNow = now
	return false, nil
}

func (r *Runner) writeClockRollbackAnomaly(ctx context.Context, previous time.Time, current time.Time) error {
	payload, err := json.Marshal(map[string]any{
		"worker_id":   r.workerID,
		"previous_at": previous,
		"current_at":  current,
		"rollback_ms": previous.Sub(current).Milliseconds(),
	})
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, message, payload_json)
		VALUES ('HIGH', 'CLOCK_STEP_BACKWARD', $1, $2)
	`, "scheduler detected clock step backward", payload)
	return err
}

func (r *Runner) claimOne(ctx context.Context) (Job, bool, error) {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return Job{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var job Job
	claimStart := time.Now()
	err = tx.QueryRow(ctx, `
		SELECT id, job_type, target_type, target_id, attempts, max_attempts, run_at
		FROM scheduler_jobs
		WHERE status IN ('PENDING','FAILED','RUNNING')
		  AND run_at <= now()
		  AND next_attempt_at <= now()
		  AND (status <> 'RUNNING' OR locked_until <= now())
		ORDER BY run_at, created_at, id
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(&job.ID, &job.JobType, &job.TargetType, &job.TargetID, &job.Attempts, &job.MaxAttempts, &job.RunAt)
	observability.Observe("db_query_latency_seconds", time.Since(claimStart).Seconds(), map[string]string{"query": "scheduler_claim_one"}, observability.DefaultLatencyBuckets)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE scheduler_jobs
		SET status = 'RUNNING',
		    locked_by = $2,
		    locked_until = now() + interval '5 seconds',
		    updated_at = now()
		WHERE id = $1
	`, job.ID, r.workerID); err != nil {
		return Job{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func (r *Runner) processClaimed(ctx context.Context, job Job) error {
	observability.Observe("auction_scheduler_drift_seconds", r.now().Sub(job.RunAt).Seconds(), map[string]string{"job_type": job.JobType}, observability.DefaultLatencyBuckets)
	switch job.JobType {
	case JobTypeEndAuction:
		return r.processEndAuction(ctx, job)
	case JobTypeExpireOrder:
		return r.processExpireOrder(ctx, job)
	default:
		return fmt.Errorf("unsupported scheduler job type %s", job.JobType)
	}
}

func (r *Runner) processEndAuction(ctx context.Context, job Job) error {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := r.now()
	var status string
	var endAt time.Time
	var winnerID *string
	var price int64
	var depositBPS int16
	var depositFloorCents int64
	var depositCapCents int64
	if err := tx.QueryRow(ctx, `
		SELECT a.status, a.end_at, a.current_winner_id, a.current_price_cents,
		       COALESCE(ar.deposit_bps, 1000),
		       COALESCE(ar.deposit_floor_cents, 10000),
		       COALESCE(ar.deposit_cap_cents, 100000000)
		FROM auctions a
		JOIN auction_rules ar ON ar.auction_id = a.id AND ar.rule_version = a.rule_version
		WHERE a.id = $1
		FOR UPDATE OF a
	`, job.TargetID).Scan(&status, &endAt, &winnerID, &price, &depositBPS, &depositFloorCents, &depositCapCents); err != nil {
		return err
	}
	if status != "ACTIVE" {
		return r.markSucceededTx(ctx, tx, job.ID)
	}
	if now.Before(endAt) {
		return r.rescheduleTx(ctx, tx, job.ID, endAt)
	}

	eventType := "auction_ended"
	payload := map[string]any{
		"ended_at": now,
		"reason":   "END_AT_REACHED",
	}
	if winnerID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE auctions
			SET status = 'SOLD', is_narrating = false, narrating_started_at = NULL, updated_at = now()
			WHERE id = $1
		`, job.TargetID); err != nil {
			return err
		}
		proposedOrderID := "ord_" + uuid.NewString()
		var orderID string
		expireAt := now.Add(15 * time.Minute)
		deposit := calculateDeposit(price, int64(depositBPS), depositFloorCents, depositCapCents)
		if err := tx.QueryRow(ctx, `
			INSERT INTO orders (id, auction_id, winner_id, amount_cents, status, deposit_cents, deposit_status, expire_at)
			VALUES ($1, $2, $3, $4, 'ORDER_PENDING', $5, 'HELD', $6)
			ON CONFLICT (auction_id) DO UPDATE SET auction_id = EXCLUDED.auction_id
			RETURNING id, expire_at
		`, proposedOrderID, job.TargetID, *winnerID, price, deposit, expireAt).Scan(&orderID, &expireAt); err != nil {
			return err
		}
		if err := upsertSchedulerJob(ctx, tx, JobTypeExpireOrder, "order", orderID, "expire:"+orderID, expireAt); err != nil {
			return err
		}
		eventType = "auction_sold"
		payload["winner_id"] = *winnerID
		payload["amount_cents"] = price
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE auctions
			SET status = 'ENDED', is_narrating = false, narrating_started_at = NULL, updated_at = now()
			WHERE id = $1
		`, job.TargetID); err != nil {
			return err
		}
	}
	if err := appendAuctionEvent(ctx, tx, job.TargetID, eventType, "scheduler:"+r.workerID, payload); err != nil {
		return err
	}
	return r.markSucceededTx(ctx, tx, job.ID)
}

func (r *Runner) processExpireOrder(ctx context.Context, job Job) error {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := r.now()
	var status string
	var expireAt time.Time
	var auctionID string
	var winnerID string
	if err := tx.QueryRow(ctx, `
		SELECT status, expire_at, auction_id, winner_id
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, job.TargetID).Scan(&status, &expireAt, &auctionID, &winnerID); err != nil {
		return err
	}
	if status != "ORDER_PENDING" && status != "PAYMENT_INITIATED" {
		return r.markSucceededTx(ctx, tx, job.ID)
	}
	if now.Before(expireAt) {
		return r.rescheduleTx(ctx, tx, job.ID, expireAt)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE orders
		SET status = 'ORDER_EXPIRED', deposit_status = 'FORFEITED'
		WHERE id = $1
	`, job.TargetID); err != nil {
		return err
	}
	if err := appendAuctionEvent(ctx, tx, auctionID, "order_expired", "scheduler:"+r.workerID, map[string]any{
		"order_id":       job.TargetID,
		"user_id":        winnerID,
		"order_status":   "ORDER_EXPIRED",
		"deposit_status": "FORFEITED",
		"expired_at":     now,
	}); err != nil {
		return err
	}
	return r.markSucceededTx(ctx, tx, job.ID)
}

func (r *Runner) markFailure(ctx context.Context, job Job, processErr error) error {
	tx, err := r.beginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	nextAttemptAt := r.now().Add(time.Duration(job.Attempts+1)*200*time.Millisecond + retryStagger(job.ID))
	if _, err := tx.Exec(ctx, `
		UPDATE scheduler_jobs
		SET attempts = attempts + 1,
		    status = CASE WHEN attempts + 1 >= max_attempts THEN 'DEAD' ELSE 'FAILED' END,
		    next_attempt_at = $3,
		    locked_by = NULL,
		    locked_until = NULL,
		    last_error = $2,
		    updated_at = now()
		WHERE id = $1
	`, job.ID, processErr.Error(), nextAttemptAt); err != nil {
		return err
	}
	if job.Attempts+1 >= job.MaxAttempts {
		payload, _ := json.Marshal(map[string]any{
			"job_id":      job.ID,
			"job_type":    job.JobType,
			"target_type": job.TargetType,
			"target_id":   job.TargetID,
			"error":       processErr.Error(),
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO system_anomaly_events (severity, type, message, payload_json)
			VALUES ('HIGH', 'SCHEDULER_JOB_DEAD', $1, $2)
		`, "scheduler job exhausted max attempts", payload); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Runner) markSucceededTx(ctx context.Context, tx pgx.Tx, jobID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE scheduler_jobs
		SET status = 'SUCCEEDED',
		    locked_by = NULL,
		    locked_until = NULL,
		    last_error = NULL,
		    updated_at = now()
		WHERE id = $1
	`, jobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Runner) rescheduleTx(ctx context.Context, tx pgx.Tx, jobID string, runAt time.Time) error {
	if _, err := tx.Exec(ctx, `
		UPDATE scheduler_jobs
		SET status = 'PENDING',
		    run_at = $2,
		    next_attempt_at = $2,
		    locked_by = NULL,
		    locked_until = NULL,
		    updated_at = now()
		WHERE id = $1
	`, jobID, runAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Runner) beginTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '500ms'`); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '3s'`); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func appendAuctionEvent(ctx context.Context, tx pgx.Tx, auctionID string, eventType string, traceID string, payload map[string]any) error {
	var seq int64
	var version int64
	if err := tx.QueryRow(ctx, `
		UPDATE auctions
		SET seq = seq + 1, version = version + 1, updated_at = now()
		WHERE id = $1
		RETURNING seq, version
	`, auctionID).Scan(&seq, &version); err != nil {
		return err
	}
	payload["state_version"] = version
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	serverTimeMS := time.Now().UTC().UnixMilli()
	if _, err := tx.Exec(ctx, `
		INSERT INTO auction_events (auction_id, seq, event_type, payload_json, server_time_ms, trace_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, auctionID, seq, eventType, payloadJSON, serverTimeMS, traceID); err != nil {
		return err
	}
	var outboxID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, auction_id, seq, event_type, payload_json)
		VALUES ('auction', $1, $1, $2, $3, $4)
		RETURNING id
	`, auctionID, seq, eventType, payloadJSON).Scan(&outboxID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_delivery (outbox_id, status)
		VALUES ($1, 'PENDING')
	`, outboxID)
	return err
}

func upsertSchedulerJob(ctx context.Context, tx pgx.Tx, jobType string, targetType string, targetID string, idempotencyKey string, runAt time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO scheduler_jobs (id, job_type, target_type, target_id, idempotency_key, run_at, status, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', $6)
		ON CONFLICT (job_type, target_type, target_id, idempotency_key)
		DO UPDATE SET run_at = EXCLUDED.run_at,
		              status = CASE WHEN scheduler_jobs.status IN ('SUCCEEDED','DEAD') THEN scheduler_jobs.status ELSE 'PENDING' END,
		              next_attempt_at = EXCLUDED.next_attempt_at,
		              locked_by = NULL,
		              locked_until = NULL,
		              updated_at = now()
	`, "job_"+uuid.NewString(), jobType, targetType, targetID, idempotencyKey, runAt)
	return err
}

func calculateDeposit(amountCents int64, depositBPS int64, floorCents int64, capCents int64) int64 {
	raw := amountCents * depositBPS / 10000
	half := amountCents / 2
	floor := minInt64(floorCents, half)
	capValue := minInt64(capCents, half)
	return minInt64(maxInt64(raw, floor), capValue)
}

func minInt64(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func retryStagger(jobID string) time.Duration {
	h := fnv.New32a()
	_, _ = h.Write([]byte(jobID))
	return time.Duration(h.Sum32()%200) * time.Millisecond
}
