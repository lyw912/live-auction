package gateway

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/storage"
)

const heatSummaryWindowSeconds = 30

type HeatSummaryHandler struct {
	Deps *storage.Dependencies
}

type heatSummary struct {
	AuctionID             string    `json:"auction_id"`
	RoomID                string    `json:"room_id"`
	Status                string    `json:"status"`
	GeneratedAt           time.Time `json:"generated_at"`
	WindowSeconds         int       `json:"window_seconds"`
	ActiveBidders30s      int64     `json:"active_bidders_30s"`
	AcceptedBids30s       int64     `json:"accepted_bids_30s"`
	RejectedBids30s       int64     `json:"rejected_bids_30s"`
	ChatMessages30s       int64     `json:"chat_messages_30s"`
	RecoveryEvents30s     int64     `json:"recovery_events_30s"`
	WatcherCountAvailable bool      `json:"watcher_count_available"`
	WatcherCount          *int64    `json:"watcher_count,omitempty"`
	Source                string    `json:"source"`
}

func (h HeatSummaryHandler) Summary(w http.ResponseWriter, r *http.Request) {
	auctionID := strings.TrimSpace(r.PathValue("id"))
	if auctionID == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "auction id is required", http.StatusBadRequest))
		return
	}
	summary, err := loadHeatSummary(r.Context(), h.Deps.Postgres, auctionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, r, apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound))
			return
		}
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "heat summary query failed", http.StatusInternalServerError))
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func loadHeatSummary(ctx context.Context, db *pgxpool.Pool, auctionID string) (heatSummary, error) {
	var summary heatSummary
	err := db.QueryRow(ctx, `
		WITH auction_row AS (
			SELECT id, room_id, status
			FROM auctions
			WHERE id = $1
		),
		bid_counts AS (
			SELECT
			  count(DISTINCT user_id) FILTER (WHERE created_at >= now() - interval '30 seconds') AS active_bidders_30s,
			  count(*) FILTER (WHERE status = 'ACCEPTED' AND created_at >= now() - interval '30 seconds') AS accepted_bids_30s,
			  count(*) FILTER (WHERE status = 'REJECTED' AND created_at >= now() - interval '30 seconds') AS rejected_bids_30s
			FROM bids
			WHERE auction_id = $1
		),
		chat_counts AS (
			SELECT count(*) AS chat_messages_30s
			FROM chat_messages c
			JOIN auction_row a ON a.room_id = c.room_id
			WHERE c.created_at >= now() - interval '30 seconds'
		),
		recovery_counts AS (
			SELECT count(*) AS recovery_events_30s
			FROM user_activity_events
			WHERE auction_id = $1
			  AND event_type IN ('ws_reconnect','ws_recovered','ws_slow_consumer_closed')
			  AND created_at >= now() - interval '30 seconds'
		)
		SELECT a.id, a.room_id, a.status, now(),
		       coalesce(b.active_bidders_30s, 0),
		       coalesce(b.accepted_bids_30s, 0),
		       coalesce(b.rejected_bids_30s, 0),
		       coalesce(c.chat_messages_30s, 0),
		       coalesce(r.recovery_events_30s, 0)
		FROM auction_row a
		CROSS JOIN bid_counts b
		CROSS JOIN chat_counts c
		CROSS JOIN recovery_counts r
	`, auctionID).Scan(
		&summary.AuctionID,
		&summary.RoomID,
		&summary.Status,
		&summary.GeneratedAt,
		&summary.ActiveBidders30s,
		&summary.AcceptedBids30s,
		&summary.RejectedBids30s,
		&summary.ChatMessages30s,
		&summary.RecoveryEvents30s,
	)
	if err != nil {
		return heatSummary{}, err
	}
	summary.WindowSeconds = heatSummaryWindowSeconds
	summary.WatcherCountAvailable = false
	summary.Source = "postgres:bids,chat_messages,user_activity_events"
	return summary, nil
}
