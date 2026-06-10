package outbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/observability"
	"live-auction/backend/internal/redisx"
)

const (
	StatusPending    = "PENDING"
	StatusPublishing = "PUBLISHING"
	StatusPublished  = "PUBLISHED"
	StatusFailed     = "FAILED"
	StatusDead       = "DEAD"

	DefaultHistoryLimit    = 4096
	DefaultHistoryTTL      = 30 * time.Minute
	DefaultDrainBatch      = 256
	DefaultLeaseTTL        = 5 * time.Second
	MaxRetriableBackoff    = 30 * time.Second
	EventSchemaVersion     = 1
	GuardProjectionTTL     = 30 * time.Minute
	guardProjectionTimeout = 25 * time.Millisecond

	ErrorClassRedisUnavailable = "REDIS_UNAVAILABLE"
	ErrorClassPayloadInvalid   = "PAYLOAD_INVALID"
	ErrorClassPublishTimeout   = "PUBLISH_TIMEOUT"
	ErrorClassUnknown          = "UNKNOWN"

	SignalForceSnapshotRebuild = "force_snapshot_rebuild"
	SignalRetryDeadOutbox      = "retry_dead_outbox"
	SignalPauseRelayShard      = "pause_relay_shard"
	SignalResumeRelayShard     = "resume_relay_shard"
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
		if _, err := r.ProcessSignals(ctx, 16); err != nil {
			log.Warn("outbox_signal_process_failed", slog.String("error", err.Error()))
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
	OutboxID           int64           `json:"outbox_id"`
	AggregateType      string          `json:"aggregate_type"`
	AggregateID        string          `json:"aggregate_id"`
	AuctionID          string          `json:"auction_id"`
	Seq                int64           `json:"seq"`
	EventType          string          `json:"event_type"`
	EventSchemaVersion int             `json:"event_schema_version"`
	EventKey           string          `json:"event_key"`
	Payload            json.RawMessage `json:"payload"`
	PayloadSHA256      string          `json:"payload_sha256"`
	CreatedAt          time.Time       `json:"created_at"`
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

type guardProjection struct {
	Status            string
	CurrentPriceCents int64
	CurrentWinnerID   *string
	StartPriceCents   int64
	IncrementCents    int64
	CapPriceCents     *int64
	EndAt             *time.Time
	Seq               int64
	AcceptedBidCount  int64
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
	ok, _, err := r.processOne(ctx, true)
	return ok, err
}

func (r *Relay) ProcessBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = r.drainBatch
	}
	if err := r.ensureShardLease(ctx); err != nil {
		return 0, err
	}
	processed := 0
	touchedShards := map[int]struct{}{}
	for processed < limit {
		ok, shardID, err := r.processOneAfterLease(ctx, false)
		if err != nil {
			return processed, err
		}
		if !ok {
			break
		}
		touchedShards[shardID] = struct{}{}
		processed++
	}
	for shardID := range touchedShards {
		if err := r.refreshWatermark(ctx, shardID); err != nil {
			return processed, err
		}
	}
	return processed, nil
}

func (r *Relay) processOne(ctx context.Context, refreshWatermark bool) (bool, int, error) {
	if err := r.ensureShardLease(ctx); err != nil {
		return false, 0, err
	}
	return r.processOneAfterLease(ctx, refreshWatermark)
}

func (r *Relay) processOneAfterLease(ctx context.Context, refreshWatermark bool) (bool, int, error) {
	start := time.Now()
	event, shardID, ok, err := r.claimOne(ctx)
	if err != nil || !ok {
		return ok, 0, err
	}
	if err := r.publish(ctx, event); err != nil {
		if markErr := r.markFailure(ctx, event, err, refreshWatermark); markErr != nil {
			return true, shardID, markErr
		}
		return true, shardID, err
	}
	if err := r.markPublished(ctx, event.OutboxID, refreshWatermark); err != nil {
		return true, shardID, err
	}
	observability.Observe("auction_outbox_lag_seconds", time.Since(event.CreatedAt).Seconds(), nil, observability.DefaultLatencyBuckets)
	observability.Observe("auction_fanout_latency_seconds", time.Since(start).Seconds(), nil, observability.DefaultLatencyBuckets)
	return true, shardID, nil
}

func (r *Relay) claimOne(ctx context.Context) (Event, int, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Event{}, 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var event Event
	var shardID int
	err = tx.QueryRow(ctx, `
		SELECT e.id, d.shard_id, e.aggregate_type, e.aggregate_id, e.auction_id, e.seq, e.event_type,
		       e.event_schema_version, e.event_key, e.payload_json, e.payload_sha256, e.created_at
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
	`, r.workerID).Scan(
		&event.OutboxID, &shardID, &event.AggregateType, &event.AggregateID, &event.AuctionID, &event.Seq, &event.EventType,
		&event.EventSchemaVersion, &event.EventKey, &event.Payload, &event.PayloadSHA256, &event.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, 0, false, nil
	}
	if err != nil {
		return Event{}, 0, false, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHING', locked_by = $2, locked_until = now() + interval '30 seconds'
		WHERE outbox_id = $1
	`, event.OutboxID, r.workerID)
	if err != nil {
		return Event{}, 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, 0, false, err
	}
	return event, shardID, true, nil
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
	if err := validateEventEnvelope(event); err != nil {
		return err
	}
	epoch, err := r.ensureStreamEpoch(ctx, event.AuctionID)
	if err != nil {
		return err
	}
	envelope := map[string]any{
		"auction_id":           event.AuctionID,
		"seq":                  event.Seq,
		"stream_epoch":         epoch,
		"snapshot_version":     event.Seq,
		"event_type":           event.EventType,
		"event_schema_version": event.EventSchemaVersion,
		"event_key":            event.EventKey,
		"payload":              json.RawMessage(event.Payload),
		"payload_sha256":       event.PayloadSHA256,
		"outbox_id":            event.OutboxID,
		"published_at_ms":      time.Now().UTC().UnixMilli(),
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
	if cmds, err := pipe.Exec(ctx); err != nil {
		for _, cmd := range cmds {
			if cmdErr := cmd.Err(); cmdErr != nil {
				return cmdErr
			}
		}
		return err
	}
	observability.Observe("redis_command_latency_seconds", time.Since(redisStart).Seconds(), map[string]string{"command": "outbox_publish_pipeline"}, observability.DefaultLatencyBuckets)
	if r.publisher != nil {
		r.publisher(ctx, event.AuctionID, data)
	}
	r.refreshGuardProjectionBestEffort(ctx, event.AuctionID, event.EventType)
	return nil
}

func (r *Relay) refreshGuardProjectionBestEffort(ctx context.Context, auctionID string, eventType string) {
	projectionCtx, cancel := context.WithTimeout(ctx, guardProjectionTimeout)
	defer cancel()
	if projection, ok, err := r.guardProjectionFromEvent(projectionCtx, Event{AuctionID: auctionID, EventType: eventType}); err == nil && ok {
		if err := r.writeGuardProjection(projectionCtx, auctionID, projection); err != nil {
			observability.Inc("auction_bid_redis_guard_projection_update_total", map[string]string{"outcome": "error"})
		} else {
			observability.Inc("auction_bid_redis_guard_projection_update_total", map[string]string{"outcome": "updated"})
		}
	} else if err != nil {
		observability.Inc("auction_bid_redis_guard_projection_update_total", map[string]string{"outcome": "error"})
	}
}

func (r *Relay) guardProjectionFromEvent(ctx context.Context, event Event) (guardProjection, bool, error) {
	switch event.EventType {
	case "bid_accepted", "auction_extended", "auction_sold", "auction_started", "auction_cancelled", "auction_ended", "rules_updated", "snapshot":
	default:
		return guardProjection{}, false, nil
	}
	var projection guardProjection
	err := r.db.QueryRow(ctx, `
		SELECT status, current_price_cents, current_winner_id, start_price_cents,
		       increment_cents, cap_price_cents, end_at, seq, accepted_bid_count
		FROM auctions
		WHERE id = $1
	`, event.AuctionID).Scan(
		&projection.Status,
		&projection.CurrentPriceCents,
		&projection.CurrentWinnerID,
		&projection.StartPriceCents,
		&projection.IncrementCents,
		&projection.CapPriceCents,
		&projection.EndAt,
		&projection.Seq,
		&projection.AcceptedBidCount,
	)
	if err != nil {
		return guardProjection{}, false, err
	}
	return projection, true, nil
}

func (r *Relay) writeGuardProjection(ctx context.Context, auctionID string, projection guardProjection) error {
	values := map[string]any{
		"status":              projection.Status,
		"current_price_cents": projection.CurrentPriceCents,
		"start_price_cents":   projection.StartPriceCents,
		"increment_cents":     projection.IncrementCents,
		"cap_price_cents":     0,
		"end_at_ms":           0,
		"seq":                 projection.Seq,
		"accepted_bid_count":  projection.AcceptedBidCount,
		"current_winner_id":   "",
		"projected_at_ms":     time.Now().UTC().UnixMilli(),
	}
	if projection.CurrentWinnerID != nil {
		values["current_winner_id"] = *projection.CurrentWinnerID
	}
	if projection.CapPriceCents != nil {
		values["cap_price_cents"] = *projection.CapPriceCents
	}
	if projection.EndAt != nil {
		values["end_at_ms"] = projection.EndAt.UTC().UnixMilli()
	}
	key := redisx.BidGuardProjectionKey(auctionID)
	pipe := r.redis.TxPipeline()
	for field, value := range values {
		pipe.HSet(ctx, key, field, value)
	}
	pipe.Expire(ctx, key, GuardProjectionTTL)
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		for _, cmd := range cmds {
			if cmdErr := cmd.Err(); cmdErr != nil {
				return cmdErr
			}
		}
	}
	return err
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

func (r *Relay) markPublished(ctx context.Context, outboxID int64, refreshWatermark bool) error {
	_, err := r.db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED',
		    published_at = now(),
		    locked_by = NULL,
		    locked_until = NULL,
		    last_error = NULL,
		    last_error_class = NULL,
		    last_error_retriable = NULL,
		    last_error_at = NULL,
		    last_published_watermark = jsonb_build_object(
		      'outbox_id', outbox_id,
		      'auction_id', auction_id,
		      'seq', auction_seq,
		      'published_at', now()
		    )
		WHERE outbox_id = $1
	`, outboxID)
	if err != nil {
		return err
	}
	if !refreshWatermark {
		return nil
	}
	return r.refreshWatermarkForOutbox(ctx, outboxID)
}

func (r *Relay) markFailure(ctx context.Context, event Event, publishErr error, refreshWatermark bool) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var attempts int
	var maxAttempts int
	errClass, retriable := classifyPublishError(publishErr)
	if retriable {
		if err := tx.QueryRow(ctx, `
			UPDATE outbox_delivery
			SET attempts = attempts + 1,
			    status = 'FAILED',
			    next_attempt_at = now() + LEAST(($5::double precision * interval '1 second'), interval '30 seconds'),
			    locked_by = NULL,
			    locked_until = NULL,
			    last_error = $2,
			    last_error_class = $3,
			    last_error_retriable = $4,
			    last_error_at = now()
			WHERE outbox_id = $1
			RETURNING attempts, max_attempts
		`, event.OutboxID, publishErr.Error(), errClass, retriable, retriableBackoffSeconds(errClass, attempts+1)).Scan(&attempts, &maxAttempts); err != nil {
			return err
		}
	} else {
		if err := tx.QueryRow(ctx, `
			UPDATE outbox_delivery
			SET attempts = attempts + 1,
			    status = 'DEAD',
			    next_attempt_at = now() + interval '1 second',
			    locked_by = NULL,
			    locked_until = NULL,
			    last_error = $2,
			    last_error_class = $3,
			    last_error_retriable = $4,
			    last_error_at = now()
			WHERE outbox_id = $1
			RETURNING attempts, max_attempts
		`, event.OutboxID, publishErr.Error(), errClass, retriable).Scan(&attempts, &maxAttempts); err != nil {
			return err
		}
	}
	dead := !retriable
	if dead {
		payload, err := json.Marshal(map[string]any{
			"outbox_id":    event.OutboxID,
			"seq":          event.Seq,
			"error":        publishErr.Error(),
			"error_class":  errClass,
			"retriable":    retriable,
			"attempts":     attempts,
			"max_attempts": maxAttempts,
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
	if refreshWatermark {
		if refreshErr := r.refreshWatermarkForOutbox(ctx, event.OutboxID); refreshErr != nil {
			return refreshErr
		}
	}
	if dead {
		observability.Inc("auction_outbox_dead_total", nil)
		r.publishGapNotice(ctx, event)
	}
	return nil
}

func retriableBackoffSeconds(errClass string, attempts int) float64 {
	if attempts < 1 {
		attempts = 1
	}
	base := 1.0
	switch errClass {
	case ErrorClassRedisUnavailable:
		base = 2.0
	case ErrorClassPublishTimeout:
		base = 1.0
	default:
		base = 1.0
	}
	backoff := base
	for i := 1; i < attempts && backoff < MaxRetriableBackoff.Seconds(); i++ {
		backoff *= 2
	}
	if max := MaxRetriableBackoff.Seconds(); backoff > max {
		return max
	}
	return backoff
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
	requestID := "snapshot_" + time.Now().UTC().Format("20060102150405.000000000")
	start := time.Now()
	_ = r.recordSnapshotEvent(ctx, auctionID, requestID, "db", "REQUESTED", false, nil, nil, nil)
	_ = r.recordSnapshotEvent(ctx, auctionID, requestID, "db", "STARTED", false, nil, nil, nil)
	epoch, err := r.ensureStreamEpoch(ctx, auctionID)
	if err != nil {
		duration := time.Since(start).Milliseconds()
		errClass := classifySnapshotError(err)
		_ = r.recordSnapshotEvent(ctx, auctionID, requestID, "db", "FAILED", false, &duration, &errClass, err)
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
		duration := time.Since(start).Milliseconds()
		errClass := classifySnapshotError(err)
		_ = r.recordSnapshotEvent(ctx, auctionID, requestID, "db", "FAILED", false, &duration, &errClass, err)
		return nil, err
	}
	if err := r.redis.Set(ctx, fmt.Sprintf("auction:%s:snapshot", auctionID), payload, r.historyTTL).Err(); err != nil {
		duration := time.Since(start).Milliseconds()
		errClass := classifySnapshotError(err)
		_ = r.recordSnapshotEvent(ctx, auctionID, requestID, "db", "FAILED", false, &duration, &errClass, err)
		return nil, err
	}
	r.refreshGuardProjectionBestEffort(ctx, auctionID, "snapshot")
	duration := time.Since(start).Milliseconds()
	_ = r.recordSnapshotEvent(ctx, auctionID, requestID, "db", "COMPLETED", false, &duration, nil, nil)
	return payload, nil
}

func validateEventEnvelope(event Event) error {
	if event.EventSchemaVersion != EventSchemaVersion {
		return fmt.Errorf("%s: unsupported event schema version %d", ErrorClassPayloadInvalid, event.EventSchemaVersion)
	}
	if strings.TrimSpace(event.EventKey) == "" {
		return fmt.Errorf("%s: empty event key", ErrorClassPayloadInvalid)
	}
	if strings.TrimSpace(event.PayloadSHA256) == "" {
		return fmt.Errorf("%s: empty payload hash", ErrorClassPayloadInvalid)
	}
	actual := sha256.Sum256(event.Payload)
	if hex.EncodeToString(actual[:]) != event.PayloadSHA256 {
		return fmt.Errorf("%s: payload hash mismatch for outbox %d", ErrorClassPayloadInvalid, event.OutboxID)
	}
	if !json.Valid(event.Payload) {
		return fmt.Errorf("%s: invalid JSON payload for outbox %d", ErrorClassPayloadInvalid, event.OutboxID)
	}
	return nil
}

func classifyPublishError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, strings.ToLower(ErrorClassPayloadInvalid)), strings.Contains(message, "invalid json"), strings.Contains(message, "payload hash"):
		return ErrorClassPayloadInvalid, false
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(message, "timeout"), strings.Contains(message, "deadline"):
		return ErrorClassPublishTimeout, true
	case strings.Contains(message, "redis"), strings.Contains(message, "connection refused"), strings.Contains(message, "i/o timeout"), strings.Contains(message, "connectex"):
		return ErrorClassRedisUnavailable, true
	default:
		return ErrorClassUnknown, true
	}
}

func classifySnapshotError(err error) string {
	class, _ := classifyPublishError(err)
	if class == "" {
		return ErrorClassUnknown
	}
	return class
}

func (r *Relay) refreshWatermarkForOutbox(ctx context.Context, outboxID int64) error {
	shardID, err := r.shardIDForOutbox(ctx, outboxID)
	if err != nil {
		return err
	}
	return r.refreshWatermark(ctx, shardID)
}

func (r *Relay) shardIDForOutbox(ctx context.Context, outboxID int64) (int, error) {
	var shardID int
	if err := r.db.QueryRow(ctx, `SELECT shard_id FROM outbox_delivery WHERE outbox_id = $1`, outboxID).Scan(&shardID); err != nil {
		return 0, err
	}
	return shardID, nil
}

func (r *Relay) refreshWatermark(ctx context.Context, shardID int) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO outbox_relay_watermarks (
		  shard_id, owner_id, last_published_outbox_id, last_published_auction_id,
		  last_published_seq, last_published_at, oldest_ready_age_ms, ready_count,
		  publishing_count, dead_count, updated_at
		)
		SELECT shard_id,
		       max(l.owner_id) AS owner_id,
		       (array_agg(outbox_id ORDER BY published_at DESC NULLS LAST, outbox_id DESC)
		          FILTER (WHERE status = 'PUBLISHED'))[1] AS last_published_outbox_id,
		       (array_agg(auction_id ORDER BY published_at DESC NULLS LAST, outbox_id DESC)
		          FILTER (WHERE status = 'PUBLISHED'))[1] AS last_published_auction_id,
		       (array_agg(auction_seq ORDER BY published_at DESC NULLS LAST, outbox_id DESC)
		          FILTER (WHERE status = 'PUBLISHED'))[1] AS last_published_seq,
		       max(published_at) FILTER (WHERE status = 'PUBLISHED') AS last_published_at,
		       COALESCE(max((extract(epoch from (now() - event_created_at)) * 1000)::bigint)
		          FILTER (WHERE status IN ('PENDING','FAILED') AND next_attempt_at <= now()), 0) AS oldest_ready_age_ms,
		       count(*) FILTER (WHERE status IN ('PENDING','FAILED') AND next_attempt_at <= now()) AS ready_count,
		       count(*) FILTER (WHERE status = 'PUBLISHING') AS publishing_count,
		       count(*) FILTER (WHERE status = 'DEAD') AS dead_count,
		       now()
		FROM outbox_delivery d
		LEFT JOIN outbox_relay_shard_leases l USING (shard_id)
		WHERE d.shard_id = $1
		GROUP BY shard_id
		ON CONFLICT (shard_id) DO UPDATE
		SET owner_id = EXCLUDED.owner_id,
		    last_published_outbox_id = EXCLUDED.last_published_outbox_id,
		    last_published_auction_id = EXCLUDED.last_published_auction_id,
		    last_published_seq = EXCLUDED.last_published_seq,
		    last_published_at = EXCLUDED.last_published_at,
		    oldest_ready_age_ms = EXCLUDED.oldest_ready_age_ms,
		    ready_count = EXCLUDED.ready_count,
		    publishing_count = EXCLUDED.publishing_count,
		    dead_count = EXCLUDED.dead_count,
		    updated_at = EXCLUDED.updated_at
	`, shardID)
	if err == nil {
		var retrying, ackPending, redelivered float64
		var oldestRetryAge float64
		scanErr := r.db.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE status = 'FAILED')::double precision,
			       count(*) FILTER (WHERE status = 'PUBLISHING')::double precision,
			       count(*) FILTER (WHERE attempts > 1)::double precision,
			       COALESCE(max(extract(epoch from (now() - last_error_at)))
			         FILTER (WHERE status = 'FAILED'), 0)::double precision
			FROM outbox_delivery
			WHERE shard_id = $1
		`, shardID).Scan(&retrying, &ackPending, &redelivered, &oldestRetryAge)
		if scanErr == nil {
			labels := map[string]string{"shard": strconv.Itoa(shardID)}
			observability.Set("auction_outbox_retrying", retrying, labels)
			observability.Set("auction_outbox_ack_pending", ackPending, labels)
			observability.Set("auction_outbox_redelivered", redelivered, labels)
			observability.Set("auction_outbox_oldest_retry_age_seconds", oldestRetryAge, labels)
		}
	}
	return err
}

func (r *Relay) recordSnapshotEvent(ctx context.Context, auctionID string, requestID string, source string, status string, stale bool, durationMS *int64, errClass *string, eventErr error) error {
	var errorMessage *string
	if eventErr != nil {
		value := eventErr.Error()
		errorMessage = &value
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO snapshot_rebuild_events (
		  auction_id, request_id, source, status, stale, duration_ms, error_class, error_message
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, auctionID, requestID, source, status, stale, durationMS, errClass, errorMessage)
	return err
}

func (r *Relay) ProcessSignals(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 16
	}
	processed := 0
	for processed < limit {
		ok, err := r.processOneSignal(ctx)
		if err != nil || !ok {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (r *Relay) processOneSignal(ctx context.Context) (bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	var signalType string
	var targetType string
	var targetID string
	err = tx.QueryRow(ctx, `
		SELECT id, signal_type, target_type, target_id
		FROM system_control_signals
		WHERE status = 'PENDING'
		  AND signal_type = ANY($1)
		  AND (locked_until IS NULL OR locked_until < now())
		ORDER BY created_at, id
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, []string{
		SignalForceSnapshotRebuild,
		SignalRetryDeadOutbox,
		SignalPauseRelayShard,
		SignalResumeRelayShard,
	}).Scan(&id, &signalType, &targetType, &targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE system_control_signals
		SET status = 'PROCESSING', locked_by = $2, locked_until = now() + interval '30 seconds'
		WHERE id = $1
	`, id, r.workerID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	result, execErr := r.executeSignal(ctx, signalType, targetType, targetID)
	status := "SUCCEEDED"
	var errorMessage *string
	if execErr != nil {
		status = "FAILED"
		value := execErr.Error()
		errorMessage = &value
	}
	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return true, marshalErr
	}
	_, err = r.db.Exec(ctx, `
		UPDATE system_control_signals
		SET status = $2,
		    processed_at = now(),
		    locked_by = NULL,
		    locked_until = NULL,
		    result_json = $3,
		    error_message = $4
		WHERE id = $1
	`, id, status, resultJSON, errorMessage)
	if err != nil {
		return true, err
	}
	return true, execErr
}

func (r *Relay) executeSignal(ctx context.Context, signalType string, targetType string, targetID string) (map[string]any, error) {
	switch signalType {
	case SignalForceSnapshotRebuild:
		if targetType != "auction" {
			return nil, fmt.Errorf("force_snapshot_rebuild requires auction target")
		}
		payload, err := r.RebuildSnapshot(ctx, targetID)
		return map[string]any{"auction_id": targetID, "snapshot_bytes": len(payload)}, err
	case SignalRetryDeadOutbox:
		if targetType != "outbox" {
			return nil, fmt.Errorf("retry_dead_outbox requires outbox target")
		}
		outboxID, err := strconv.ParseInt(targetID, 10, 64)
		if err != nil || outboxID <= 0 {
			return nil, fmt.Errorf("retry_dead_outbox target_id must be a positive outbox id")
		}
		tag, err := r.db.Exec(ctx, `
			UPDATE outbox_delivery
			SET status = 'PENDING',
			    next_attempt_at = now(),
			    locked_by = NULL,
			    locked_until = NULL,
			    last_error = NULL,
			    last_error_class = NULL,
			    last_error_retriable = NULL,
			    last_error_at = NULL
			WHERE outbox_id = $1 AND status = 'DEAD'
		`, outboxID)
		return map[string]any{"outbox_id": outboxID, "rows": tag.RowsAffected()}, err
	case SignalPauseRelayShard:
		if targetType != "relay_shard" {
			return nil, fmt.Errorf("pause_relay_shard requires relay_shard target")
		}
		shardID, err := strconv.Atoi(targetID)
		if err != nil || shardID < 0 || shardID >= 16 {
			return nil, fmt.Errorf("relay_shard target_id must be an integer from 0 to 15")
		}
		tag, err := r.db.Exec(ctx, `
			INSERT INTO outbox_relay_shard_leases (shard_id, owner_id, lease_until)
			VALUES ($2, 'paused:' || $1, now() + interval '5 minutes')
			ON CONFLICT (shard_id) DO UPDATE
			SET owner_id = EXCLUDED.owner_id,
			    lease_until = EXCLUDED.lease_until,
			    renewed_at = now()
		`, r.workerID, shardID)
		return map[string]any{"shard_id": shardID, "rows": tag.RowsAffected()}, err
	case SignalResumeRelayShard:
		if targetType != "relay_shard" {
			return nil, fmt.Errorf("resume_relay_shard requires relay_shard target")
		}
		shardID, err := strconv.Atoi(targetID)
		if err != nil || shardID < 0 || shardID >= 16 {
			return nil, fmt.Errorf("relay_shard target_id must be an integer from 0 to 15")
		}
		tag, err := r.db.Exec(ctx, `
			DELETE FROM outbox_relay_shard_leases
			WHERE shard_id = $1 AND owner_id LIKE 'paused:%'
		`, shardID)
		return map[string]any{"shard_id": shardID, "rows": tag.RowsAffected()}, err
	default:
		return nil, fmt.Errorf("unsupported signal type %s", signalType)
	}
}
