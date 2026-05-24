package outbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/observability"
)

const (
	StatusPending    = "PENDING"
	StatusPublishing = "PUBLISHING"
	StatusPublished  = "PUBLISHED"
	StatusFailed     = "FAILED"
	StatusDead       = "DEAD"

	DefaultHistoryLimit = 4096
	DefaultHistoryTTL   = 30 * time.Minute
	DefaultDrainBatch   = 64
	DefaultLeaseTTL     = 5 * time.Second
)

type Relay struct {
	db           *pgxpool.Pool
	redis        *redis.Client
	workerID     string
	historyLimit int64
	historyTTL   time.Duration
	drainBatch   int
	leaseTTL     time.Duration
	publisher    func(context.Context, string, []byte)
	notify       bool
}

func (r *Relay) Run(ctx context.Context, log *slog.Logger, interval time.Duration) {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	notifyCh, closeNotify := r.startNotifyListener(ctx, log)
	defer closeNotify()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := r.renewShardLeases(ctx); err != nil {
			log.Warn("outbox_lease_renew_failed", slog.String("error", err.Error()))
		}
		processed, err := r.ProcessBatch(ctx, r.drainBatch)
		if err != nil {
			log.Warn("outbox_process_failed", slog.String("error", err.Error()))
		}
		if processed == 0 {
			select {
			case <-ctx.Done():
				return
			case <-notifyCh:
			case <-ticker.C:
			}
		}
	}
}

func (r *Relay) startNotifyListener(ctx context.Context, log *slog.Logger) (<-chan struct{}, func()) {
	if !r.notify {
		return nil, func() {}
	}
	notifyCh := make(chan struct{}, 1)
	listenCtx, cancel := context.WithCancel(ctx)
	go func() {
		backoff := 100 * time.Millisecond
		for {
			if listenCtx.Err() != nil {
				return
			}
			if err := r.listenForOutboxNotifications(listenCtx, notifyCh); err != nil && listenCtx.Err() == nil {
				log.Warn("outbox_notify_listener_failed", slog.String("error", err.Error()))
				select {
				case <-listenCtx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < 2*time.Second {
					backoff *= 2
				}
			} else {
				backoff = 100 * time.Millisecond
			}
		}
	}()
	return notifyCh, cancel
}

func (r *Relay) listenForOutboxNotifications(ctx context.Context, notifyCh chan<- struct{}) error {
	conn, err := r.db.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN outbox_delivery_ready"); err != nil {
		return err
	}
	for {
		_, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				return fmt.Errorf("listen outbox delivery ready: %s", pgErr.Message)
			}
			return err
		}
		select {
		case notifyCh <- struct{}{}:
		default:
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
		drainBatch:   DefaultDrainBatch,
		leaseTTL:     DefaultLeaseTTL,
		notify:       true,
	}
}

func (r *Relay) WithPublisher(publisher func(context.Context, string, []byte)) *Relay {
	r.publisher = publisher
	return r
}

func (r *Relay) WithNotify(enabled bool) *Relay {
	r.notify = enabled
	return r
}

func (r *Relay) ProcessOne(ctx context.Context) (bool, error) {
	start := time.Now()
	if err := r.ensureShardLease(ctx); err != nil {
		return false, err
	}
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
	observability.Observe("auction_outbox_lag_seconds", time.Since(event.CreatedAt).Seconds(), nil, observability.DefaultLatencyBuckets)
	observability.Observe("auction_fanout_latency_seconds", time.Since(start).Seconds(), nil, observability.DefaultLatencyBuckets)
	return true, nil
}

func (r *Relay) ProcessBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = r.drainBatch
	}
	processed := 0
	for processed < limit {
		ok, err := r.ProcessOne(ctx)
		if err != nil {
			return processed, err
		}
		if !ok {
			return processed, nil
		}
		processed++
	}
	return processed, nil
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
		FROM outbox_delivery d
		JOIN outbox_events e ON e.id = d.outbox_id
		JOIN outbox_relay_shard_leases l ON l.shard_id = d.shard_id
		WHERE d.auction_id IS NOT NULL
		  AND d.auction_seq IS NOT NULL
		  AND l.owner_id = $1
		  AND l.lease_until > now()
		  AND (
		    (d.status IN ('PENDING','FAILED') AND d.next_attempt_at <= now())
		    OR (d.status = 'PUBLISHING' AND d.locked_until < now())
		  )
		  AND NOT EXISTS (
		    SELECT 1
		    FROM outbox_delivery prior
		    WHERE prior.auction_id = d.auction_id
		      AND prior.shard_id = d.shard_id
		      AND prior.auction_seq < d.auction_seq
		      AND prior.status IN ('PENDING','FAILED','PUBLISHING')
		  )
		ORDER BY d.event_created_at, d.outbox_id
		LIMIT 1
		FOR UPDATE OF d SKIP LOCKED
	`, r.workerID).Scan(&event.OutboxID, &event.AggregateType, &event.AggregateID, &event.AuctionID, &event.Seq, &event.EventType, &event.Payload, &event.CreatedAt)
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

func (r *Relay) renewShardLeases(ctx context.Context) error {
	leaseTTL := r.leaseTTL
	if leaseTTL <= 0 {
		leaseTTL = DefaultLeaseTTL
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO outbox_relay_shard_leases (shard_id, owner_id, lease_until)
		SELECT DISTINCT d.shard_id, $1, now() + ($2::double precision * interval '1 second')
		FROM outbox_delivery d
		WHERE d.status IN ('PENDING','FAILED','PUBLISHING')
		  AND d.shard_id IS NOT NULL
		ON CONFLICT (shard_id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id,
		    lease_until = EXCLUDED.lease_until,
		    renewed_at = now()
		WHERE outbox_relay_shard_leases.owner_id = EXCLUDED.owner_id
		   OR outbox_relay_shard_leases.lease_until <= now()
	`, r.workerID, leaseTTL.Seconds())
	return err
}

func (r *Relay) ensureShardLease(ctx context.Context) error {
	var owned int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_relay_shard_leases
		WHERE owner_id = $1 AND lease_until > now()
	`, r.workerID).Scan(&owned); err != nil {
		return err
	}
	if owned > 0 {
		return nil
	}
	return r.renewShardLeases(ctx)
}

func (r *Relay) publish(ctx context.Context, event Event) error {
	epoch, err := r.ensureStreamEpoch(ctx, event.AuctionID)
	if err != nil {
		return err
	}
	envelope := map[string]any{
		"auction_id":       event.AuctionID,
		"seq":              event.Seq,
		"stream_epoch":     epoch,
		"snapshot_version": event.Seq,
		"event_type":       event.EventType,
		"payload":          json.RawMessage(event.Payload),
		"outbox_id":        event.OutboxID,
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
	redisStart := time.Now()
	if _, err = pipe.Exec(ctx); err != nil {
		return err
	}
	observability.Observe("redis_command_latency_seconds", time.Since(redisStart).Seconds(), map[string]string{"command": "outbox_publish_pipeline"}, observability.DefaultLatencyBuckets)
	if r.publisher != nil {
		r.publisher(ctx, event.AuctionID, data)
	}
	return nil
}

func (r *Relay) ensureStreamEpoch(ctx context.Context, auctionID string) (string, error) {
	generated, err := randomStreamEpoch()
	if err != nil {
		return "", err
	}
	var epoch string
	err = r.db.QueryRow(ctx, `
		INSERT INTO realtime_stream_epochs (auction_id, value, expires_at)
		VALUES ($1, $3, now() + ($2::double precision * interval '1 second'))
		ON CONFLICT (auction_id) DO UPDATE
		SET expires_at = GREATEST(realtime_stream_epochs.expires_at, now() + ($2::double precision * interval '1 second')),
		    updated_at = now()
		RETURNING value
	`, auctionID, r.historyTTL.Seconds(), generated).Scan(&epoch)
	return epoch, err
}

func randomStreamEpoch() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
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
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if attempts >= maxAttempts {
		observability.Inc("auction_outbox_dead_total", nil)
		r.publishGapNotice(ctx, event)
	}
	return nil
}

func (r *Relay) publishGapNotice(ctx context.Context, event Event) {
	if r.publisher == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"auction_id":     event.AuctionID,
		"event_type":     "outbox_gap_notice",
		"missing_seq":    []int64{event.Seq},
		"outbox_id":      event.OutboxID,
		"server_time_ms": time.Now().UTC().UnixMilli(),
	})
	if err != nil {
		return
	}
	r.publisher(ctx, event.AuctionID, payload)
}

func (r *Relay) RebuildSnapshot(ctx context.Context, auctionID string) ([]byte, error) {
	epoch, err := r.ensureStreamEpoch(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	var payload []byte
	err = r.db.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'event_type', 'snapshot',
			'auction_id', a.id,
			'seq', a.seq,
			'stream_epoch', $2::text,
			'snapshot_version', a.version,
			'source', 'db',
			'stale', false,
			'payload', jsonb_build_object(
				'status', a.status,
				'current_price_cents', a.current_price_cents,
				'current_winner_id', a.current_winner_id,
				'end_at', a.end_at,
				'accepted_bid_count', a.accepted_bid_count,
				'extend_count', a.extend_count,
				'reason', cancel_event.payload_json->>'reason'
			)
		)
		FROM auctions a
		LEFT JOIN LATERAL (
			SELECT ev.payload_json
			FROM auction_events ev
			WHERE ev.auction_id = a.id AND ev.event_type = 'auction_cancelled'
			ORDER BY ev.seq DESC
			LIMIT 1
		) cancel_event ON true
		WHERE a.id = $1
	`, auctionID, epoch).Scan(&payload)
	if err != nil {
		return nil, err
	}
	return payload, r.redis.Set(ctx, fmt.Sprintf("auction:%s:snapshot", auctionID), payload, r.historyTTL).Err()
}
