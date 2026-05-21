package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	StatusPending    = "PENDING"
	StatusPublishing = "PUBLISHING"
	StatusPublished  = "PUBLISHED"
	StatusFailed     = "FAILED"
	StatusDead       = "DEAD"

	DefaultHistoryLimit = 4096
	DefaultHistoryTTL   = 30 * time.Minute
)

type Relay struct {
	db           *pgxpool.Pool
	redis        *redis.Client
	workerID     string
	historyLimit int64
	historyTTL   time.Duration
	publisher    func(context.Context, string, []byte)
}

func (r *Relay) Run(ctx context.Context, log *slog.Logger, interval time.Duration) {
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
		if err != nil {
			log.Warn("outbox_process_failed", slog.String("error", err.Error()))
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

type Event struct {
	OutboxID      int64           `json:"outbox_id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	AuctionID     string          `json:"auction_id"`
	Seq           int64           `json:"seq"`
	EventType     string          `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     time.Time       `json:"created_at"`
}

func NewRelay(db *pgxpool.Pool, redisClient *redis.Client, workerID string) *Relay {
	return &Relay{
		db:           db,
		redis:        redisClient,
		workerID:     workerID,
		historyLimit: DefaultHistoryLimit,
		historyTTL:   DefaultHistoryTTL,
	}
}

func (r *Relay) WithPublisher(publisher func(context.Context, string, []byte)) *Relay {
	r.publisher = publisher
	return r
}

func (r *Relay) ProcessOne(ctx context.Context) (bool, error) {
	event, ok, err := r.claimOne(ctx)
	if err != nil || !ok {
		return ok, err
	}
	if err := r.publish(ctx, event); err != nil {
		if markErr := r.markFailure(ctx, event, err); markErr != nil {
			return true, markErr
		}
		return true, err
	}
	if err := r.markPublished(ctx, event.OutboxID); err != nil {
		return true, err
	}
	return true, nil
}

func (r *Relay) claimOne(ctx context.Context) (Event, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Event{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var event Event
	err = tx.QueryRow(ctx, `
		SELECT e.id, e.aggregate_type, e.aggregate_id, e.auction_id, e.seq, e.event_type, e.payload_json, e.created_at
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE d.status IN ('PENDING','FAILED')
		  AND d.next_attempt_at <= now()
		  AND NOT EXISTS (
		    SELECT 1
		    FROM outbox_events e2
		    JOIN outbox_delivery d2 ON d2.outbox_id = e2.id
		    WHERE e2.auction_id = e.auction_id
		      AND e2.seq < e.seq
		      AND d2.status NOT IN ('PUBLISHED','DEAD')
		  )
		ORDER BY e.created_at, e.id
		LIMIT 1
		FOR UPDATE OF d SKIP LOCKED
	`).Scan(&event.OutboxID, &event.AggregateType, &event.AggregateID, &event.AuctionID, &event.Seq, &event.EventType, &event.Payload, &event.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHING', locked_by = $2, locked_until = now() + interval '30 seconds'
		WHERE outbox_id = $1
	`, event.OutboxID, r.workerID)
	if err != nil {
		return Event{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, false, err
	}
	return event, true, nil
}

func (r *Relay) publish(ctx context.Context, event Event) error {
	envelope := map[string]any{
		"auction_id": event.AuctionID,
		"seq":        event.Seq,
		"event_type": event.EventType,
		"payload":    json.RawMessage(event.Payload),
		"outbox_id":  event.OutboxID,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	eventsKey := fmt.Sprintf("auction:%s:events", event.AuctionID)
	snapshotKey := fmt.Sprintf("auction:%s:snapshot", event.AuctionID)
	pipe := r.redis.TxPipeline()
	pipe.RPush(ctx, eventsKey, data)
	pipe.LTrim(ctx, eventsKey, -r.historyLimit, -1)
	pipe.Expire(ctx, eventsKey, r.historyTTL)
	pipe.Set(ctx, snapshotKey, data, r.historyTTL)
	if _, err = pipe.Exec(ctx); err != nil {
		return err
	}
	if r.publisher != nil {
		r.publisher(ctx, event.AuctionID, data)
	}
	return nil
}

func (r *Relay) markPublished(ctx context.Context, outboxID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED', published_at = now(), locked_by = NULL, locked_until = NULL, last_error = NULL
		WHERE outbox_id = $1
	`, outboxID)
	return err
}

func (r *Relay) markFailure(ctx context.Context, event Event, publishErr error) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var attempts int
	var maxAttempts int
	if err := tx.QueryRow(ctx, `
		UPDATE outbox_delivery
		SET attempts = attempts + 1,
		    status = CASE WHEN attempts + 1 >= max_attempts THEN 'DEAD' ELSE 'FAILED' END,
		    next_attempt_at = now() + interval '1 second',
		    locked_by = NULL,
		    locked_until = NULL,
		    last_error = $2
		WHERE outbox_id = $1
		RETURNING attempts, max_attempts
	`, event.OutboxID, publishErr.Error()).Scan(&attempts, &maxAttempts); err != nil {
		return err
	}
	if attempts >= maxAttempts {
		payload, err := json.Marshal(map[string]any{
			"outbox_id": event.OutboxID,
			"seq":       event.Seq,
			"error":     publishErr.Error(),
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
			VALUES ('CRITICAL', 'OUTBOX_DEAD_LETTER', $1, $2, $3)
		`, event.AuctionID, "outbox delivery exhausted max attempts", payload); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Relay) RebuildSnapshot(ctx context.Context, auctionID string) ([]byte, error) {
	var payload []byte
	err := r.db.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'event_type', 'snapshot',
			'auction_id', a.id,
			'seq', a.seq,
			'source', 'db',
			'stale', false,
			'payload', jsonb_build_object(
				'status', a.status,
				'current_price_cents', a.current_price_cents,
				'current_winner_id', a.current_winner_id,
				'end_at', a.end_at,
				'accepted_bid_count', a.accepted_bid_count,
				'extend_count', a.extend_count
			)
		)
		FROM auctions a
		WHERE a.id = $1
	`, auctionID).Scan(&payload)
	if err != nil {
		return nil, err
	}
	return payload, r.redis.Set(ctx, fmt.Sprintf("auction:%s:snapshot", auctionID), payload, r.historyTTL).Err()
}
