package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	apierrors "live-auction/backend/internal/platform/errors"
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
	AggregateType      string     `json:"aggregate_type"`
	AggregateID        string     `json:"aggregate_id"`
	AuctionID          *string    `json:"auction_id,omitempty"`
	Seq                *int64     `json:"seq,omitempty"`
	EventType          string     `json:"event_type"`
	SchemaVersion      int        `json:"event_schema_version"`
	EventKey           string     `json:"event_key"`
	PayloadHash        string     `json:"payload_sha256"`
	Status             string     `json:"status"`
	Attempts           int        `json:"attempts"`
	ShardID            *int       `json:"shard_id,omitempty"`
	LeaseOwner         *string    `json:"lease_owner,omitempty"`
	LeaseUntil         *time.Time `json:"lease_until,omitempty"`
	NextAttemptAt      time.Time  `json:"next_attempt_at"`
	LagMs              int64      `json:"lag_ms"`
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
		SELECT e.id, e.aggregate_type, e.aggregate_id, e.auction_id, e.seq,
		       e.event_type, e.event_schema_version, e.event_key, e.payload_sha256,
		       d.status, d.attempts, d.shard_id, l.owner_id, l.lease_until, d.next_attempt_at,
		       (extract(epoch from (now() - e.created_at)) * 1000)::bigint AS lag_ms,
		       d.last_error, d.last_error_class, d.last_error_retriable, e.created_at, d.published_at
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		LEFT JOIN outbox_relay_shard_leases l ON l.shard_id = d.shard_id
		ORDER BY e.created_at DESC, e.id DESC
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
			&row.OutboxID, &row.AggregateType, &row.AggregateID, &row.AuctionID, &row.Seq,
			&row.EventType, &row.SchemaVersion, &row.EventKey, &row.PayloadHash,
			&row.Status, &row.Attempts, &row.ShardID, &row.LeaseOwner, &row.LeaseUntil,
			&row.NextAttemptAt, &row.LagMs, &row.LastError, &row.LastErrorClass,
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
		SELECT shard_id, owner_id, last_published_outbox_id, last_published_auction_id,
		       last_published_seq, last_published_at, oldest_ready_age_ms,
		       ready_count, publishing_count, dead_count, updated_at
		FROM outbox_relay_watermarks
		ORDER BY shard_id
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
		if err := rows.Scan(&row.ShardID, &row.OwnerID, &row.LastPublishedOutboxID, &row.LastPublishedAuctionID, &row.LastPublishedSeq, &row.LastPublishedAt, &row.OldestReadyAgeMS, &row.ReadyCount, &row.PublishingCount, &row.DeadCount, &row.UpdatedAt); err != nil {
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
		SELECT COALESCE(room_id, '-') AS room_id,
		       count(*) FILTER (WHERE event_type = 'ws_reconnect') AS reconnect_count_recent,
		       count(*) FILTER (WHERE event_type = 'ws_recovered' AND payload_json->>'source' = 'history') AS history_recovered,
		       count(*) FILTER (WHERE event_type = 'ws_recovered' AND payload_json->>'source' IN ('snapshot','db','redis')) AS snapshot_recovered,
		       count(*) FILTER (WHERE event_type = 'ws_recovered' AND payload_json->>'source' = 'db') AS snapshot_from_db,
		       count(*) FILTER (WHERE event_type = 'ws_recovered' AND payload_json->>'stale' = 'true') AS snapshot_stale,
		       count(*) FILTER (WHERE event_type = 'ws_slow_consumer_closed') AS slow_consumer_disconnects
		FROM user_activity_events
		WHERE created_at >= now() - interval '10 minutes'
		  AND event_type IN ('ws_reconnect','ws_recovered','ws_slow_consumer_closed')
		GROUP BY COALESCE(room_id, '-')
		ORDER BY reconnect_count_recent DESC, room_id
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
		if err := rows.Scan(&row.RoomID, &row.ReconnectCountRecent, &row.HistoryRecovered, &row.SnapshotRecovered, &row.SnapshotFromDB, &row.SnapshotStale, &row.SlowConsumerDisconnects); err != nil {
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
	default:
		return fmt.Errorf("unsupported signal_type %s", req.SignalType)
	}
	return nil
}

func monitorLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > 100 {
		return 50
	}
	return limit
}

func internalMonitorError(err error) apierrors.APIError {
	_ = err
	return apierrors.New(apierrors.CodeInvalidArgument, "monitor query failed", http.StatusInternalServerError)
}
