package gateway

import (
	"net/http"
	"strconv"
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
	OutboxID      int64      `json:"outbox_id"`
	AggregateType string     `json:"aggregate_type"`
	AggregateID   string     `json:"aggregate_id"`
	AuctionID     *string    `json:"auction_id,omitempty"`
	Seq           *int64     `json:"seq,omitempty"`
	EventType     string     `json:"event_type"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LagMs         int64      `json:"lag_ms"`
	LastError     *string    `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
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
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT id, severity, type, auction_id, message, payload_json, created_at, resolved_at
		FROM system_anomaly_events
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, monitorLimit(r))
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

func (h MonitorHandler) Outbox(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Deps.Postgres.Query(r.Context(), `
		SELECT e.id, e.aggregate_type, e.aggregate_id, e.auction_id, e.seq,
		       e.event_type, d.status, d.attempts, d.next_attempt_at,
		       (extract(epoch from (now() - e.created_at)) * 1000)::bigint AS lag_ms,
		       d.last_error, e.created_at, d.published_at
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
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
		if err := rows.Scan(&row.OutboxID, &row.AggregateType, &row.AggregateID, &row.AuctionID, &row.Seq, &row.EventType, &row.Status, &row.Attempts, &row.NextAttemptAt, &row.LagMs, &row.LastError, &row.CreatedAt, &row.PublishedAt); err != nil {
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
