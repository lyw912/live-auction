package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/redisx"
	"live-auction/backend/internal/storage"
)

type MonitorHandler struct {
	Deps *storage.Dependencies
}

type monitorAuctionRow struct {
	AuctionID        string     `json:"auction_id"`
	RoomID           string     `json:"room_id"`
	ItemTitle        string     `json:"item_title"`
	Status           string     `json:"status"`
	CurrentPrice     int64      `json:"current_price_cents"`
	CurrentWinnerID  *string    `json:"current_winner_id,omitempty"`
	EndAt            *time.Time `json:"end_at,omitempty"`
	Seq              int64      `json:"seq"`
	AcceptedBidCount int64      `json:"accepted_bid_count"`
	ExtendCount      int        `json:"extend_count"`
	LastEventAt      *time.Time `json:"last_event_at,omitempty"`
}

type monitorFlightRecorderSummary struct {
	AuctionID        string     `json:"auction_id"`
	RoomID           string     `json:"room_id"`
	ItemID           string     `json:"item_id"`
	ItemTitle        string     `json:"item_title"`
	Status           string     `json:"status"`
	IsNarrating      bool       `json:"is_narrating"`
	CurrentPrice     int64      `json:"current_price_cents"`
	CurrentWinnerID  *string    `json:"current_winner_id,omitempty"`
	StartPrice       int64      `json:"start_price_cents"`
	Increment        int64      `json:"increment_cents"`
	CapPrice         *int64     `json:"cap_price_cents,omitempty"`
	StartAt          *time.Time `json:"start_at,omitempty"`
	EndAt            *time.Time `json:"end_at,omitempty"`
	Version          int64      `json:"version"`
	Seq              int64      `json:"seq"`
	AcceptedBidCount int64      `json:"accepted_bid_count"`
	ExtendCount      int        `json:"extend_count"`
	RuleVersion      int        `json:"rule_version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type monitorFlightRecorderRule struct {
	RuleVersion             int        `json:"rule_version"`
	DurationSeconds         int        `json:"duration_seconds"`
	ExtendWindowSeconds     int        `json:"extend_window_seconds"`
	ExtendBySeconds         int        `json:"extend_by_seconds"`
	MaxExtendCount          int        `json:"max_extend_count"`
	FatFingerThresholdCents *int64     `json:"fat_finger_threshold_cents,omitempty"`
	DepositBPS              *int16     `json:"deposit_bps,omitempty"`
	DepositFloorCents       *int64     `json:"deposit_floor_cents,omitempty"`
	DepositCapCents         *int64     `json:"deposit_cap_cents,omitempty"`
	FrozenAt                *time.Time `json:"frozen_at,omitempty"`
}

type monitorFlightRecorderOrder struct {
	OrderID           string     `json:"order_id"`
	WinnerID          string     `json:"winner_id"`
	AmountCents       int64      `json:"amount_cents"`
	Status            string     `json:"status"`
	DepositCents      int64      `json:"deposit_cents"`
	DepositStatus     string     `json:"deposit_status"`
	ProviderPaymentID *string    `json:"provider_payment_id,omitempty"`
	ExpireAt          time.Time  `json:"expire_at"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type monitorFlightRecorderPaymentEvent struct {
	ID                int64          `json:"id"`
	Provider          string         `json:"provider"`
	ProviderEventID   string         `json:"provider_event_id"`
	ProviderPaymentID string         `json:"provider_payment_id"`
	OrderID           string         `json:"order_id"`
	EventType         string         `json:"event_type"`
	SignatureValid    bool           `json:"signature_valid"`
	ProcessedAt       *time.Time     `json:"processed_at,omitempty"`
	Payload           map[string]any `json:"payload"`
	TraceID           *string        `json:"trace_id,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

type monitorFlightRecorderTimelineRow struct {
	Time        time.Time      `json:"time"`
	Kind        string         `json:"kind"`
	AuctionID   string         `json:"auction_id"`
	Seq         *int64         `json:"seq,omitempty"`
	EventType   string         `json:"event_type"`
	RefID       string         `json:"ref_id"`
	UserID      *string        `json:"user_id,omitempty"`
	AmountCents *int64         `json:"amount_cents,omitempty"`
	Status      *string        `json:"status,omitempty"`
	TraceID     *string        `json:"trace_id,omitempty"`
	Payload     map[string]any `json:"payload"`
}

type monitorAnomalyRow struct {
	ID         int64          `json:"id"`
	Severity   string         `json:"severity"`
	Type       string         `json:"type"`
	AuctionID  *string        `json:"auction_id,omitempty"`
	Message    string         `json:"message"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"created_at"`
	ResolvedAt *time.Time     `json:"resolved_at,omitempty"`
}

type monitorOutboxRow struct {
	OutboxID           int64      `json:"outbox_id"`
	DeliveryMessageID  string     `json:"delivery_message_id"`
	AggregateType      string     `json:"aggregate_type"`
	AggregateID        string     `json:"aggregate_id"`
	AuctionID          *string    `json:"auction_id,omitempty"`
	Seq                *int64     `json:"seq,omitempty"`
	EventType          string     `json:"event_type"`
	SchemaVersion      int        `json:"event_schema_version"`
	EventKey           string     `json:"event_key"`
	PayloadHash        string     `json:"payload_sha256"`
	Status             string     `json:"status"`
	DeliveryState      string     `json:"delivery_state"`
	Attempts           int        `json:"attempts"`
	MaxAttempts        int        `json:"max_attempts"`
	RedeliveryCount    int        `json:"redelivery_count"`
	ShardID            *int       `json:"shard_id,omitempty"`
	LeaseOwner         *string    `json:"lease_owner,omitempty"`
	LeaseUntil         *time.Time `json:"lease_until,omitempty"`
	NextAttemptAt      time.Time  `json:"next_attempt_at"`
	AckDeadlineAt      *time.Time `json:"ack_deadline_at,omitempty"`
	LagMs              int64      `json:"lag_ms"`
	RetryAgeMs         int64      `json:"retry_age_ms"`
	LastError          *string    `json:"last_error,omitempty"`
	LastErrorClass     *string    `json:"last_error_class,omitempty"`
	LastErrorRetriable *bool      `json:"last_error_retriable,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	PublishedAt        *time.Time `json:"published_at,omitempty"`
}

type monitorOutboxWatermarkRow struct {
	ShardID                int        `json:"shard_id"`
	OwnerID                *string    `json:"owner_id,omitempty"`
	LastPublishedOutboxID  *int64     `json:"last_published_outbox_id,omitempty"`
	LastPublishedAuctionID *string    `json:"last_published_auction_id,omitempty"`
	LastPublishedSeq       *int64     `json:"last_published_seq,omitempty"`
	LastPublishedAt        *time.Time `json:"last_published_at,omitempty"`
	OldestReadyAgeMS       int64      `json:"oldest_ready_age_ms"`
	ReadyCount             int64      `json:"ready_count"`
	PublishingCount        int64      `json:"publishing_count"`
	DeadCount              int64      `json:"dead_count"`
	RetryingCount          int64      `json:"retrying_count"`
	AckPendingCount        int64      `json:"ack_pending_count"`
	RedeliveredCount       int64      `json:"redelivered_count"`
	OldestRetryAgeMS       int64      `json:"oldest_retry_age_ms"`
	MaxAttempts            int        `json:"max_attempts"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type monitorSchedulerRow struct {
	JobID         string     `json:"job_id"`
	JobType       string     `json:"job_type"`
	TargetType    string     `json:"target_type"`
	TargetID      string     `json:"target_id"`
	RunAt         time.Time  `json:"run_at"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	LockedUntil   *time.Time `json:"locked_until,omitempty"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LastError     *string    `json:"last_error,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type monitorRejectRow struct {
	Time              time.Time `json:"time"`
	AuctionID         string    `json:"auction_id"`
	UserID            string    `json:"user_id"`
	AmountCents       int64     `json:"amount_cents"`
	CurrentPriceCents int64     `json:"current_price_cents"`
	RejectReason      string    `json:"reject_reason"`
	TraceID           string    `json:"trace_id"`
}

type monitorRecoveryRow struct {
	RoomID                  string `json:"room_id"`
	ReconnectCountRecent    int64  `json:"reconnect_count_recent"`
	HistoryRecovered        int64  `json:"history_recovered"`
	SnapshotRecovered       int64  `json:"snapshot_recovered"`
	SnapshotFromDB          int64  `json:"snapshot_from_db"`
	SnapshotStale           int64  `json:"snapshot_stale"`
	SlowConsumerDisconnects int64  `json:"slow_consumer_disconnects"`
	SlowPendingBytes        int64  `json:"slow_pending_bytes"`
	SlowPendingMessages     int64  `json:"slow_pending_messages"`
	MaxQueueBytes           int64  `json:"max_queue_bytes"`
	MaxQueueDepth           int64  `json:"max_queue_depth"`
}

type monitorSnapshotRow struct {
	ID           int64     `json:"id"`
	AuctionID    string    `json:"auction_id"`
	RequestID    string    `json:"request_id"`
	Source       string    `json:"source"`
	Status       string    `json:"status"`
	Stale        bool      `json:"stale"`
	DurationMS   *int64    `json:"duration_ms,omitempty"`
	ErrorClass   *string   `json:"error_class,omitempty"`
	ErrorMessage *string   `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type monitorSignalRow struct {
	ID           int64          `json:"id"`
	SignalType   string         `json:"signal_type"`
	TargetType   string         `json:"target_type"`
	TargetID     string         `json:"target_id"`
	RequestedBy  string         `json:"requested_by"`
	Reason       string         `json:"reason"`
	Status       string         `json:"status"`
	Result       map[string]any `json:"result,omitempty"`
	ErrorMessage *string        `json:"error_message,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	ProcessedAt  *time.Time     `json:"processed_at,omitempty"`
}

type monitorRedisEngineRow struct {
	AuctionID             string     `json:"auction_id"`
	EngineMode            string     `json:"engine_mode"`
	Status                string     `json:"status"`
	CurrentPrice          int64      `json:"current_price_cents"`
	CurrentWinnerID       *string    `json:"current_winner_id,omitempty"`
	Seq                   int64      `json:"seq"`
	EngineEpoch           int64      `json:"engine_epoch"`
	EngineSeq             int64      `json:"engine_seq"`
	EnginePaused          bool       `json:"engine_paused"`
	EnginePauseReason     *string    `json:"engine_pause_reason,omitempty"`
	EnginePausedAt        *time.Time `json:"engine_paused_at,omitempty"`
	PendingSettlements    int64      `json:"pending_settlements"`
	FailedSettlements     int64      `json:"failed_settlements"`
	RedisPendingDecisions int64      `json:"redis_pending_decisions"`
	SettlementLagP50MS    int64      `json:"settlement_lag_p50_ms"`
	SettlementLagP95MS    int64      `json:"settlement_lag_p95_ms"`
	SettlementLagP99MS    int64      `json:"settlement_lag_p99_ms"`
	SettlementLagMaxMS    int64      `json:"settlement_lag_max_ms"`
	LastSettledAt         *time.Time `json:"last_settled_at,omitempty"`
	CheckpointTopic       *string    `json:"checkpoint_topic,omitempty"`
	CheckpointPartition   *int       `json:"checkpoint_partition,omitempty"`
	CheckpointNextOffset  *int64     `json:"checkpoint_next_offset,omitempty"`
	LastKafkaTopic        *string    `json:"last_kafka_topic,omitempty"`
	LastKafkaPartition    *int       `json:"last_kafka_partition,omitempty"`
	LastKafkaOffset       *int64     `json:"last_kafka_offset,omitempty"`
	LastKafkaSettlementID *string    `json:"last_kafka_settlement_id,omitempty"`
	LastKafkaSettledAt    *time.Time `json:"last_kafka_settled_at,omitempty"`
	LatestAppendTopic     *string    `json:"latest_append_topic,omitempty"`
	LatestAppendPartition *int       `json:"latest_append_partition,omitempty"`
	LatestAppendOffset    *int64     `json:"latest_append_offset,omitempty"`
	LatestAppendEngineSeq *int64     `json:"latest_append_engine_seq,omitempty"`
	LatestAppendStatus    *string    `json:"latest_append_status,omitempty"`
	LatestAppendClientBid *string    `json:"latest_append_client_bid_id,omitempty"`
	LatestAppendExpiresMS *int64     `json:"latest_append_expires_at_ms,omitempty"`
	AppendSuccessCount    int64      `json:"append_success_count"`
	AppendFailureCount    int64      `json:"append_failure_count"`
	AppendUnknownCount    int64      `json:"append_unknown_count"`
	AppendStatsStatus     *string    `json:"append_stats_last_status,omitempty"`
	AppendStatsEngineSeq  *int64     `json:"append_stats_last_engine_seq,omitempty"`
}

type createSignalRequest struct {
	SignalType string         `json:"signal_type"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Reason     string         `json:"reason"`
	Payload    map[string]any `json:"payload"`
}

func (h MonitorHandler) Auctions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT a.id, a.room_id, i.title, a.status, a.current_price_cents,
		       a.current_winner_id, a.end_at, a.seq, a.accepted_bid_count,
		       a.extend_count, max(ev.created_at) AS last_event_at
		FROM auctions a
		JOIN items i ON i.id = a.item_id
		LEFT JOIN auction_events ev ON ev.auction_id = a.id
		WHERE a.status IN ('ACTIVE','SCHEDULED','SOLD','ENDED','CANCELLED')
		GROUP BY a.id, i.title
		ORDER BY a.updated_at DESC
		LIMIT $1
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()

	result := []monitorAuctionRow{}
	for rows.Next() {
		var row monitorAuctionRow
		if err := rows.Scan(&row.AuctionID, &row.RoomID, &row.ItemTitle, &row.Status, &row.CurrentPrice, &row.CurrentWinnerID, &row.EndAt, &row.Seq, &row.AcceptedBidCount, &row.ExtendCount, &row.LastEventAt); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h MonitorHandler) FlightRecorder(w http.ResponseWriter, r *http.Request) {
	auctionID := strings.TrimSpace(r.PathValue("id"))
	if auctionID == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "auction id is required", http.StatusBadRequest))
		return
	}
	var summary monitorFlightRecorderSummary
	if err := h.Deps.Postgres.QueryRow(r.Context(), `
		SELECT a.id, a.room_id, a.item_id, i.title, a.status, a.is_narrating,
		       a.current_price_cents, a.current_winner_id, a.start_price_cents,
		       a.increment_cents, a.cap_price_cents, a.start_at, a.end_at,
		       a.version, a.seq, a.accepted_bid_count, a.extend_count,
		       a.rule_version, a.created_at, a.updated_at
		FROM auctions a
		JOIN items i ON i.id = a.item_id
		WHERE a.id = $1
	`, auctionID).Scan(
		&summary.AuctionID, &summary.RoomID, &summary.ItemID, &summary.ItemTitle, &summary.Status, &summary.IsNarrating,
		&summary.CurrentPrice, &summary.CurrentWinnerID, &summary.StartPrice, &summary.Increment, &summary.CapPrice,
		&summary.StartAt, &summary.EndAt, &summary.Version, &summary.Seq, &summary.AcceptedBidCount, &summary.ExtendCount,
		&summary.RuleVersion, &summary.CreatedAt, &summary.UpdatedAt,
	); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}

	rules, err := h.flightRecorderRules(r, auctionID)
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	orders, err := h.flightRecorderOrders(r, auctionID)
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	payments, err := h.flightRecorderPaymentEvents(r, auctionID)
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	anomalies, err := h.flightRecorderAnomalies(r, auctionID)
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	timeline, err := h.flightRecorderTimeline(r, auctionID, monitorTimelineLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":        summary,
		"rules":          rules,
		"orders":         orders,
		"payment_events": payments,
		"anomalies":      anomalies,
		"timeline":       timeline,
	})
}

func (h MonitorHandler) Anomalies(w http.ResponseWriter, r *http.Request) {
	query, args := anomalyFilterQuery(r)
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT id, severity, type, auction_id, message, payload_json, created_at, resolved_at
		FROM system_anomaly_events
		`+query+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+strconv.Itoa(len(args)+1)+`
	`, append(args, monitorLimit(r))...)
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()

	result := []monitorAnomalyRow{}
	for rows.Next() {
		var row monitorAnomalyRow
		if err := rows.Scan(&row.ID, &row.Severity, &row.Type, &row.AuctionID, &row.Message, &row.Payload, &row.CreatedAt, &row.ResolvedAt); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func anomalyFilterQuery(r *http.Request) (string, []any) {
	clauses := []string{}
	args := []any{}
	add := func(clause string, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		args = append(args, strings.TrimSpace(value))
		clauses = append(clauses, strings.ReplaceAll(clause, "?", "$"+strconv.Itoa(len(args))))
	}
	q := r.URL.Query()
	add("type = ?", q.Get("type"))
	add("auction_id = ?", q.Get("auction_id"))
	add("payload_json->>'room_id' = ?", q.Get("room_id"))
	add("payload_json->>'user_id' = ?", q.Get("user_id"))
	add("payload_json->>'trace_id' = ?", q.Get("trace_id"))
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func (h MonitorHandler) Outbox(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT e.id,
		       'outbox:' || e.id::text AS delivery_message_id,
		       e.aggregate_type, e.aggregate_id, e.auction_id, e.seq,
		       e.event_type, e.event_schema_version, e.event_key, e.payload_sha256,
		       d.status,
		       CASE
		         WHEN d.status = 'PUBLISHED' THEN 'ACKED'
		         WHEN d.status = 'PUBLISHING' THEN 'ACK_PENDING'
		         WHEN d.status = 'DEAD' THEN 'TERM'
		         WHEN d.status = 'FAILED' THEN 'NAK_RETRY_WAIT'
		         ELSE 'READY'
		       END AS delivery_state,
		       d.attempts, d.max_attempts, GREATEST(d.attempts - 1, 0) AS redelivery_count,
		       d.shard_id, l.owner_id, l.lease_until, d.next_attempt_at, d.locked_until AS ack_deadline_at,
		       (extract(epoch from (now() - e.created_at)) * 1000)::bigint AS lag_ms,
		       COALESCE((extract(epoch from (now() - d.last_error_at)) * 1000)::bigint, 0) AS retry_age_ms,
		       d.last_error, d.last_error_class, d.last_error_retriable, e.created_at, d.published_at
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		LEFT JOIN outbox_relay_shard_leases l ON l.shard_id = d.shard_id
		ORDER BY CASE WHEN d.status IN ('PENDING','FAILED','PUBLISHING') THEN 0 ELSE 1 END,
		         e.created_at DESC, e.id DESC
		LIMIT $1
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()

	result := []monitorOutboxRow{}
	for rows.Next() {
		var row monitorOutboxRow
		if err := rows.Scan(
			&row.OutboxID, &row.DeliveryMessageID, &row.AggregateType, &row.AggregateID, &row.AuctionID, &row.Seq,
			&row.EventType, &row.SchemaVersion, &row.EventKey, &row.PayloadHash,
			&row.Status, &row.DeliveryState, &row.Attempts, &row.MaxAttempts, &row.RedeliveryCount,
			&row.ShardID, &row.LeaseOwner, &row.LeaseUntil,
			&row.NextAttemptAt, &row.AckDeadlineAt, &row.LagMs, &row.RetryAgeMs, &row.LastError, &row.LastErrorClass,
			&row.LastErrorRetriable, &row.CreatedAt, &row.PublishedAt,
		); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h MonitorHandler) OutboxWatermarks(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT w.shard_id, w.owner_id, w.last_published_outbox_id, w.last_published_auction_id,
		       w.last_published_seq, w.last_published_at, w.oldest_ready_age_ms,
		       w.ready_count, w.publishing_count, w.dead_count,
		       COALESCE(s.retrying_count, 0) AS retrying_count,
		       COALESCE(s.ack_pending_count, 0) AS ack_pending_count,
		       COALESCE(s.redelivered_count, 0) AS redelivered_count,
		       COALESCE(s.oldest_retry_age_ms, 0) AS oldest_retry_age_ms,
		       COALESCE(s.max_attempts, 0) AS max_attempts,
		       w.updated_at
		FROM outbox_relay_watermarks w
		LEFT JOIN LATERAL (
		  SELECT count(*) FILTER (WHERE d.status = 'FAILED') AS retrying_count,
		         count(*) FILTER (WHERE d.status = 'PUBLISHING') AS ack_pending_count,
		         count(*) FILTER (WHERE d.attempts > 1) AS redelivered_count,
		         COALESCE(max((extract(epoch from (now() - d.last_error_at)) * 1000)::bigint)
		           FILTER (WHERE d.status = 'FAILED'), 0) AS oldest_retry_age_ms,
		         COALESCE(max(d.max_attempts), 0) AS max_attempts
		  FROM outbox_delivery d
		  WHERE d.shard_id = w.shard_id
		) s ON true
		ORDER BY w.shard_id
		LIMIT $1
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()
	result := []monitorOutboxWatermarkRow{}
	for rows.Next() {
		var row monitorOutboxWatermarkRow
		if err := rows.Scan(&row.ShardID, &row.OwnerID, &row.LastPublishedOutboxID, &row.LastPublishedAuctionID, &row.LastPublishedSeq, &row.LastPublishedAt, &row.OldestReadyAgeMS, &row.ReadyCount, &row.PublishingCount, &row.DeadCount, &row.RetryingCount, &row.AckPendingCount, &row.RedeliveredCount, &row.OldestRetryAgeMS, &row.MaxAttempts, &row.UpdatedAt); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h MonitorHandler) Scheduler(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT id, job_type, target_type, target_id, run_at, status, attempts,
		       locked_until, next_attempt_at, last_error, updated_at
		FROM scheduler_jobs
		ORDER BY run_at DESC, created_at DESC
		LIMIT $1
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()

	result := []monitorSchedulerRow{}
	for rows.Next() {
		var row monitorSchedulerRow
		if err := rows.Scan(&row.JobID, &row.JobType, &row.TargetType, &row.TargetID, &row.RunAt, &row.Status, &row.Attempts, &row.LockedUntil, &row.NextAttemptAt, &row.LastError, &row.UpdatedAt); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h MonitorHandler) Rejects(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT b.created_at, b.auction_id, b.user_id, b.amount_cents,
		       a.current_price_cents, COALESCE(b.reject_reason, ''), b.trace_id
		FROM bids b
		JOIN auctions a ON a.id = b.auction_id
		WHERE b.status = 'REJECTED'
		ORDER BY b.created_at DESC
		LIMIT $1
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()

	result := []monitorRejectRow{}
	for rows.Next() {
		var row monitorRejectRow
		if err := rows.Scan(&row.Time, &row.AuctionID, &row.UserID, &row.AmountCents, &row.CurrentPriceCents, &row.RejectReason, &row.TraceID); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h MonitorHandler) Recovery(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT room_id, reconnect_count_recent, history_recovered, snapshot_recovered,
		       snapshot_from_db, snapshot_stale, slow_consumer_disconnects,
		       slow_pending_bytes, slow_pending_messages, max_queue_bytes, max_queue_depth
		FROM (
		  SELECT COALESCE(room_id, '-') AS room_id,
		         count(*) FILTER (WHERE event_type = 'ws_reconnect') AS reconnect_count_recent,
		         count(*) FILTER (WHERE event_type = 'ws_recovered' AND payload_json->>'source' = 'history') AS history_recovered,
		         count(*) FILTER (WHERE event_type = 'ws_recovered' AND payload_json->>'source' IN ('snapshot','db','redis')) AS snapshot_recovered,
		         count(*) FILTER (WHERE event_type = 'ws_recovered' AND payload_json->>'source' = 'db') AS snapshot_from_db,
		         count(*) FILTER (WHERE event_type = 'ws_recovered' AND payload_json->>'stale' = 'true') AS snapshot_stale,
		         count(*) FILTER (WHERE event_type = 'ws_slow_consumer_closed') AS slow_consumer_disconnects,
		         count(*) FILTER (WHERE event_type = 'ws_slow_consumer_closed' AND payload_json->>'reason' = 'pending_bytes') AS slow_pending_bytes,
		         count(*) FILTER (WHERE event_type = 'ws_slow_consumer_closed' AND payload_json->>'reason' = 'pending_messages') AS slow_pending_messages,
		         COALESCE(max((payload_json->>'queue_bytes')::bigint) FILTER (WHERE event_type = 'ws_slow_consumer_closed' AND payload_json ? 'queue_bytes'), 0) AS max_queue_bytes,
		         COALESCE(max((payload_json->>'queue_depth')::bigint) FILTER (WHERE event_type = 'ws_slow_consumer_closed' AND payload_json ? 'queue_depth'), 0) AS max_queue_depth,
		         max(created_at) AS last_event_at
		  FROM user_activity_events
		  WHERE created_at >= now() - interval '10 minutes'
		    AND event_type IN ('ws_reconnect','ws_recovered','ws_slow_consumer_closed')
		  GROUP BY COALESCE(room_id, '-')
		) recovery
		ORDER BY last_event_at DESC, reconnect_count_recent DESC, room_id
		LIMIT $1
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()

	result := []monitorRecoveryRow{}
	for rows.Next() {
		var row monitorRecoveryRow
		if err := rows.Scan(&row.RoomID, &row.ReconnectCountRecent, &row.HistoryRecovered, &row.SnapshotRecovered, &row.SnapshotFromDB, &row.SnapshotStale, &row.SlowConsumerDisconnects, &row.SlowPendingBytes, &row.SlowPendingMessages, &row.MaxQueueBytes, &row.MaxQueueDepth); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h MonitorHandler) Snapshots(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT id, auction_id, request_id, source, status, stale,
		       duration_ms, error_class, error_message, created_at
		FROM snapshot_rebuild_events
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()
	result := []monitorSnapshotRow{}
	for rows.Next() {
		var row monitorSnapshotRow
		if err := rows.Scan(&row.ID, &row.AuctionID, &row.RequestID, &row.Source, &row.Status, &row.Stale, &row.DurationMS, &row.ErrorClass, &row.ErrorMessage, &row.CreatedAt); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h MonitorHandler) Signals(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT id, signal_type, target_type, target_id, requested_by, reason,
		       status, COALESCE(result_json, '{}'::jsonb), error_message, created_at, processed_at
		FROM system_control_signals
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()
	result := []monitorSignalRow{}
	for rows.Next() {
		var row monitorSignalRow
		if err := rows.Scan(&row.ID, &row.SignalType, &row.TargetType, &row.TargetID, &row.RequestedBy, &row.Reason, &row.Status, &row.Result, &row.ErrorMessage, &row.CreatedAt, &row.ProcessedAt); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h MonitorHandler) RedisEngine(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		WITH candidate_auctions AS (
		  SELECT id, status, current_price_cents, current_winner_id, seq,
		         engine_epoch, engine_seq, engine_paused, engine_pause_reason,
		         engine_paused_at, updated_at
		  FROM auctions
		  WHERE engine_seq > 0 OR engine_paused OR status = 'ACTIVE'
		  ORDER BY updated_at DESC
		  LIMIT $1
		),
		settlement_lag AS (
		  SELECT s.auction_id,
		         count(*) FILTER (WHERE s.status = 'PROCESSING') AS pending_settlements,
		         count(*) FILTER (WHERE s.status = 'FAILED') AS failed_settlements,
		         max(s.settled_at) AS last_settled_at,
		         percentile_cont(0.50) WITHIN GROUP (
		           ORDER BY EXTRACT(EPOCH FROM (COALESCE(s.settled_at, s.updated_at, now()) - s.created_at)) * 1000
		         ) FILTER (WHERE s.status IN ('PROCESSING','SETTLED','SKIPPED')) AS lag_p50_ms,
		         percentile_cont(0.95) WITHIN GROUP (
		           ORDER BY EXTRACT(EPOCH FROM (COALESCE(s.settled_at, s.updated_at, now()) - s.created_at)) * 1000
		         ) FILTER (WHERE s.status IN ('PROCESSING','SETTLED','SKIPPED')) AS lag_p95_ms,
		         percentile_cont(0.99) WITHIN GROUP (
		           ORDER BY EXTRACT(EPOCH FROM (COALESCE(s.settled_at, s.updated_at, now()) - s.created_at)) * 1000
		         ) FILTER (WHERE s.status IN ('PROCESSING','SETTLED','SKIPPED')) AS lag_p99_ms,
		         max(EXTRACT(EPOCH FROM (COALESCE(s.settled_at, s.updated_at, now()) - s.created_at)) * 1000)
		           FILTER (WHERE s.status IN ('PROCESSING','SETTLED','SKIPPED')) AS lag_max_ms
		  FROM redis_engine_settlements s
		  JOIN candidate_auctions ca ON ca.id = s.auction_id
		  GROUP BY s.auction_id
		),
		last_kafka AS (
		  SELECT DISTINCT ON (s.auction_id)
		         s.auction_id, s.stream_id, s.ledger_topic, s.ledger_partition, s.ledger_offset, s.settled_at
		  FROM redis_engine_settlements s
		  JOIN candidate_auctions ca ON ca.id = s.auction_id
		  WHERE s.ledger_source = 'kafka'
		  ORDER BY s.auction_id, s.settled_at DESC NULLS LAST, s.updated_at DESC
		)
		SELECT a.id,
		       CASE
		         WHEN a.engine_seq > 0 OR a.engine_paused OR sl.auction_id IS NOT NULL OR c.auction_id IS NOT NULL THEN 'redis_ledger'
		         ELSE 'postgres_lane_or_uninitialized'
		       END AS engine_mode,
		       a.status, a.current_price_cents, a.current_winner_id, a.seq,
		       a.engine_epoch, a.engine_seq, a.engine_paused, a.engine_pause_reason,
		       a.engine_paused_at,
		       COALESCE(sl.pending_settlements, 0) AS pending_settlements,
		       COALESCE(sl.failed_settlements, 0) AS failed_settlements,
		       0::bigint AS redis_pending_decisions,
		       COALESCE(sl.lag_p50_ms, 0)::bigint AS settlement_lag_p50_ms,
		       COALESCE(sl.lag_p95_ms, 0)::bigint AS settlement_lag_p95_ms,
		       COALESCE(sl.lag_p99_ms, 0)::bigint AS settlement_lag_p99_ms,
		       COALESCE(sl.lag_max_ms, 0)::bigint AS settlement_lag_max_ms,
		       sl.last_settled_at,
		       c.decision_topic, c.decision_partition, c.next_decision_offset,
		       lk.ledger_topic, lk.ledger_partition, lk.ledger_offset, lk.stream_id, lk.settled_at
		FROM candidate_auctions a
		LEFT JOIN settlement_lag sl ON sl.auction_id = a.id
		LEFT JOIN auction_engine_checkpoints c ON c.auction_id = a.id
		LEFT JOIN last_kafka lk ON lk.auction_id = a.id
		ORDER BY a.updated_at DESC
	`, monitorLimit(r))
	if err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	defer rows.Close()
	result := []monitorRedisEngineRow{}
	for rows.Next() {
		var row monitorRedisEngineRow
		if err := rows.Scan(&row.AuctionID, &row.EngineMode, &row.Status, &row.CurrentPrice, &row.CurrentWinnerID, &row.Seq, &row.EngineEpoch, &row.EngineSeq, &row.EnginePaused, &row.EnginePauseReason, &row.EnginePausedAt, &row.PendingSettlements, &row.FailedSettlements, &row.RedisPendingDecisions, &row.SettlementLagP50MS, &row.SettlementLagP95MS, &row.SettlementLagP99MS, &row.SettlementLagMaxMS, &row.LastSettledAt, &row.CheckpointTopic, &row.CheckpointPartition, &row.CheckpointNextOffset, &row.LastKafkaTopic, &row.LastKafkaPartition, &row.LastKafkaOffset, &row.LastKafkaSettlementID, &row.LastKafkaSettledAt); err != nil {
			writeError(w, r, internalMonitorError(err))
			return
		}
		enrichRedisEngineRuntime(r, h.Deps, &row)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func enrichRedisEngineRuntime(r *http.Request, deps *storage.Dependencies, row *monitorRedisEngineRow) {
	if deps == nil || deps.Redis == nil || row == nil || row.AuctionID == "" {
		return
	}
	ctx := r.Context()
	if pending, err := deps.Redis.HLen(ctx, redisx.BidEnginePendingKey(row.AuctionID)).Result(); err == nil {
		row.RedisPendingDecisions = pending
	}
	values, err := deps.Redis.HGetAll(ctx, redisx.BidEngineAppendMarkerKey(row.AuctionID)).Result()
	if err == nil && len(values) > 0 && values["kafka_append_status"] != "" {
		latest := redisAppendDiagnostic{
			status:      values["kafka_append_status"],
			topic:       values["kafka_topic"],
			clientBidID: values["client_bid_id"],
			partition:   parseOptionalInt(values["kafka_partition"]),
			offset:      parseOptionalInt64(values["kafka_offset"]),
			engineSeq:   parseOptionalInt64(values["engine_seq"]),
			expiresAtMS: parseOptionalInt64(values["expires_at_ms"]),
		}
		row.LatestAppendStatus = &latest.status
		if latest.topic != "" {
			row.LatestAppendTopic = &latest.topic
		}
		row.LatestAppendPartition = latest.partition
		row.LatestAppendOffset = latest.offset
		row.LatestAppendEngineSeq = latest.engineSeq
		if latest.clientBidID != "" {
			row.LatestAppendClientBid = &latest.clientBidID
		}
		row.LatestAppendExpiresMS = latest.expiresAtMS
	}

	stats, err := deps.Redis.HGetAll(ctx, redisx.BidEngineAppendStatsKey(row.AuctionID)).Result()
	if err != nil || len(stats) == 0 {
		return
	}
	row.AppendSuccessCount = parseOptionalInt64Value(stats["success_count"])
	row.AppendFailureCount = parseOptionalInt64Value(stats["failure_count"])
	row.AppendUnknownCount = parseOptionalInt64Value(stats["unknown_count"])
	if stats["last_status"] != "" {
		row.AppendStatsStatus = ptrMonitorString(stats["last_status"])
	}
	row.AppendStatsEngineSeq = parseOptionalInt64(stats["last_engine_seq"])
}

type redisAppendDiagnostic struct {
	status      string
	topic       string
	clientBidID string
	partition   *int
	offset      *int64
	engineSeq   *int64
	expiresAtMS *int64
}

func (h MonitorHandler) CreateSignal(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req createSignalRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	if strings.TrimSpace(req.SignalType) == "" || strings.TrimSpace(req.TargetType) == "" || strings.TrimSpace(req.TargetID) == "" || strings.TrimSpace(req.Reason) == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "signal_type, target_type, target_id, and reason are required", http.StatusBadRequest))
		return
	}
	if err := validateMonitorSignalRequest(req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, err.Error(), http.StatusBadRequest))
		return
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid signal payload", http.StatusBadRequest))
		return
	}
	var id int64
	if err := h.Deps.Postgres.QueryRow(r.Context(), `
		INSERT INTO system_control_signals (
		  signal_type, target_type, target_id, requested_by, reason, payload_json
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, req.SignalType, req.TargetType, req.TargetID, user.ID, req.Reason, payloadJSON).Scan(&id); err != nil {
		writeError(w, r, internalMonitorError(err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "PENDING"})
}

func validateMonitorSignalRequest(req createSignalRequest) error {
	switch req.SignalType {
	case "force_snapshot_rebuild":
		if req.TargetType != "auction" {
			return fmt.Errorf("force_snapshot_rebuild requires auction target")
		}
	case "retry_dead_outbox":
		if req.TargetType != "outbox" {
			return fmt.Errorf("retry_dead_outbox requires outbox target")
		}
		if id, err := strconv.ParseInt(req.TargetID, 10, 64); err != nil || id <= 0 {
			return fmt.Errorf("retry_dead_outbox target_id must be a positive outbox id")
		}
	case "pause_relay_shard", "resume_relay_shard":
		if req.TargetType != "relay_shard" {
			return fmt.Errorf("%s requires relay_shard target", req.SignalType)
		}
		shardID, err := strconv.Atoi(req.TargetID)
		if err != nil || shardID < 0 || shardID >= 16 {
			return fmt.Errorf("relay_shard target_id must be an integer from 0 to 15")
		}
	case "pause_redis_engine", "resume_redis_engine", "reconcile_redis_engine":
		if req.TargetType != "auction" {
			return fmt.Errorf("%s requires auction target", req.SignalType)
		}
		if strings.TrimSpace(req.TargetID) == "" {
			return fmt.Errorf("auction target_id is required")
		}
	default:
		return fmt.Errorf("unsupported signal_type %s", req.SignalType)
	}
	return nil
}

func (h MonitorHandler) flightRecorderRules(r *http.Request, auctionID string) ([]monitorFlightRecorderRule, error) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT rule_version, duration_seconds, extend_window_seconds, extend_by_seconds,
		       max_extend_count, fat_finger_threshold_cents, deposit_bps,
		       deposit_floor_cents, deposit_cap_cents, frozen_at
		FROM auction_rules
		WHERE auction_id = $1
		ORDER BY rule_version
	`, auctionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []monitorFlightRecorderRule{}
	for rows.Next() {
		var row monitorFlightRecorderRule
		if err := rows.Scan(&row.RuleVersion, &row.DurationSeconds, &row.ExtendWindowSeconds, &row.ExtendBySeconds, &row.MaxExtendCount, &row.FatFingerThresholdCents, &row.DepositBPS, &row.DepositFloorCents, &row.DepositCapCents, &row.FrozenAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (h MonitorHandler) flightRecorderOrders(r *http.Request, auctionID string) ([]monitorFlightRecorderOrder, error) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT id, winner_id, amount_cents, status, deposit_cents, deposit_status,
		       provider_payment_id, expire_at, paid_at, created_at
		FROM orders
		WHERE auction_id = $1
		ORDER BY created_at, id
	`, auctionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []monitorFlightRecorderOrder{}
	for rows.Next() {
		var row monitorFlightRecorderOrder
		if err := rows.Scan(&row.OrderID, &row.WinnerID, &row.AmountCents, &row.Status, &row.DepositCents, &row.DepositStatus, &row.ProviderPaymentID, &row.ExpireAt, &row.PaidAt, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (h MonitorHandler) flightRecorderPaymentEvents(r *http.Request, auctionID string) ([]monitorFlightRecorderPaymentEvent, error) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT pe.id, pe.provider, pe.provider_event_id, pe.provider_payment_id,
		       pe.order_id, pe.event_type, pe.signature_valid, pe.processed_at,
		       pe.payload_json, pe.trace_id, pe.created_at
		FROM payment_events pe
		JOIN orders o ON o.id = pe.order_id
		WHERE o.auction_id = $1
		ORDER BY pe.created_at, pe.id
	`, auctionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []monitorFlightRecorderPaymentEvent{}
	for rows.Next() {
		var row monitorFlightRecorderPaymentEvent
		if err := rows.Scan(&row.ID, &row.Provider, &row.ProviderEventID, &row.ProviderPaymentID, &row.OrderID, &row.EventType, &row.SignatureValid, &row.ProcessedAt, &row.Payload, &row.TraceID, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (h MonitorHandler) flightRecorderAnomalies(r *http.Request, auctionID string) ([]monitorAnomalyRow, error) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT id, severity, type, auction_id, message, payload_json, created_at, resolved_at
		FROM system_anomaly_events
		WHERE auction_id = $1 OR payload_json->>'auction_id' = $1
		ORDER BY created_at, id
		LIMIT $2
	`, auctionID, monitorLimit(r))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []monitorAnomalyRow{}
	for rows.Next() {
		var row monitorAnomalyRow
		if err := rows.Scan(&row.ID, &row.Severity, &row.Type, &row.AuctionID, &row.Message, &row.Payload, &row.CreatedAt, &row.ResolvedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (h MonitorHandler) flightRecorderTimeline(r *http.Request, auctionID string, limit int) ([]monitorFlightRecorderTimelineRow, error) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		WITH timeline AS (
		  SELECT e.created_at AS time, 'auction_event' AS kind, e.auction_id, e.seq,
		         e.event_type, e.id::text AS ref_id,
		         NULLIF(e.payload_json->>'user_id', '') AS user_id,
		         CASE WHEN e.payload_json ? 'amount_cents' THEN (e.payload_json->>'amount_cents')::bigint ELSE NULL END AS amount_cents,
		         NULL::text AS status, e.trace_id, e.payload_json AS payload
		  FROM auction_events e
		  WHERE e.auction_id = $1
		  UNION ALL
		  SELECT b.created_at AS time, 'bid' AS kind, b.auction_id, b.seq,
		         CASE WHEN b.status = 'ACCEPTED' THEN 'bid_accepted_row' ELSE 'bid_rejected_row' END AS event_type,
		         b.id AS ref_id, b.user_id, b.amount_cents, b.status, b.trace_id,
		         jsonb_build_object('client_bid_id', b.client_bid_id, 'reject_reason', b.reject_reason, 'request_hash', b.request_hash, 'response', b.response_json, 'source', b.source) AS payload
		  FROM bids b
		  WHERE b.auction_id = $1
		  UNION ALL
		  SELECT o.created_at AS time, 'outbox' AS kind, o.auction_id, o.seq,
		         o.event_type || ':' || d.status AS event_type,
		         o.id::text AS ref_id, NULL::text AS user_id, NULL::bigint AS amount_cents,
		         d.status, NULL::text AS trace_id,
		         jsonb_build_object(
		           'delivery_message_id', 'outbox:' || o.id::text,
		           'delivery_state', CASE
		             WHEN d.status = 'PUBLISHED' THEN 'ACKED'
		             WHEN d.status = 'PUBLISHING' THEN 'ACK_PENDING'
		             WHEN d.status = 'DEAD' THEN 'TERM'
		             WHEN d.status = 'FAILED' THEN 'NAK_RETRY_WAIT'
		             ELSE 'READY'
		           END,
		           'attempts', d.attempts,
		           'max_attempts', d.max_attempts,
		           'shard_id', d.shard_id,
		           'published_at', d.published_at,
		           'last_error_class', d.last_error_class,
		           'last_error', d.last_error,
		           'payload_sha256', o.payload_sha256
		         ) AS payload
		  FROM outbox_events o
		  JOIN outbox_delivery d ON d.outbox_id = o.id
		  WHERE o.auction_id = $1
		  UNION ALL
		  SELECT o.created_at AS time, 'order' AS kind, o.auction_id, NULL::bigint AS seq,
		         o.status AS event_type, o.id AS ref_id, o.winner_id AS user_id,
		         o.amount_cents, o.status, NULL::text AS trace_id,
		         jsonb_build_object('deposit_cents', o.deposit_cents, 'deposit_status', o.deposit_status, 'provider_payment_id', o.provider_payment_id, 'paid_at', o.paid_at, 'expire_at', o.expire_at) AS payload
		  FROM orders o
		  WHERE o.auction_id = $1
		  UNION ALL
		  SELECT pe.created_at AS time, 'payment_event' AS kind, o.auction_id, NULL::bigint AS seq,
		         pe.event_type, pe.id::text AS ref_id, o.winner_id AS user_id,
		         o.amount_cents, o.status, pe.trace_id,
		         pe.payload_json || jsonb_build_object('order_id', pe.order_id, 'provider_payment_id', pe.provider_payment_id, 'signature_valid', pe.signature_valid, 'processed_at', pe.processed_at) AS payload
		  FROM payment_events pe
		  JOIN orders o ON o.id = pe.order_id
		  WHERE o.auction_id = $1
		  UNION ALL
		  SELECT sae.created_at AS time, 'anomaly' AS kind, COALESCE(sae.auction_id, sae.payload_json->>'auction_id') AS auction_id,
		         NULL::bigint AS seq, sae.type AS event_type, sae.id::text AS ref_id,
		         NULLIF(sae.payload_json->>'user_id', '') AS user_id, NULL::bigint AS amount_cents,
		         sae.severity AS status, NULLIF(sae.payload_json->>'trace_id', '') AS trace_id,
		         sae.payload_json || jsonb_build_object('message', sae.message, 'resolved_at', sae.resolved_at) AS payload
		  FROM system_anomaly_events sae
		  WHERE sae.auction_id = $1 OR sae.payload_json->>'auction_id' = $1
		  UNION ALL
		  SELECT sre.created_at AS time, 'snapshot_rebuild' AS kind, sre.auction_id, NULL::bigint AS seq,
		         sre.status AS event_type, sre.id::text AS ref_id, NULL::text AS user_id,
		         NULL::bigint AS amount_cents, sre.status, NULL::text AS trace_id,
		         jsonb_build_object('request_id', sre.request_id, 'source', sre.source, 'stale', sre.stale, 'duration_ms', sre.duration_ms, 'error_class', sre.error_class, 'error_message', sre.error_message) AS payload
		  FROM snapshot_rebuild_events sre
		  WHERE sre.auction_id = $1
		)
		SELECT time, kind, auction_id, seq, event_type, ref_id, user_id,
		       amount_cents, status, trace_id, payload
		FROM timeline
		ORDER BY time, seq NULLS LAST, kind, ref_id
		LIMIT $2
	`, auctionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []monitorFlightRecorderTimelineRow{}
	for rows.Next() {
		var row monitorFlightRecorderTimelineRow
		if err := rows.Scan(&row.Time, &row.Kind, &row.AuctionID, &row.Seq, &row.EventType, &row.RefID, &row.UserID, &row.AmountCents, &row.Status, &row.TraceID, &row.Payload); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func monitorLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > 100 {
		return 50
	}
	return limit
}

func monitorTimelineLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("timeline_limit"))
	if err != nil || limit <= 0 || limit > 300 {
		return 100
	}
	return limit
}

func parseOptionalInt(value string) *int {
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseOptionalInt64(value string) *int64 {
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseOptionalInt64Value(value string) int64 {
	parsed := parseOptionalInt64(value)
	if parsed == nil {
		return 0
	}
	return *parsed
}

func ptrMonitorString(value string) *string {
	return &value
}

func internalMonitorError(err error) apierrors.APIError {
	_ = err
	return apierrors.New(apierrors.CodeInvalidArgument, "monitor query failed", http.StatusInternalServerError)
}
