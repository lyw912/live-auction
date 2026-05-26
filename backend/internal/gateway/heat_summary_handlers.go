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

type MaxBidSummaryHandler struct {
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

type maxBidSummary struct {
	AuctionID          string    `json:"auction_id"`
	RoomID             string    `json:"room_id"`
	Status             string    `json:"status"`
	GeneratedAt        time.Time `json:"generated_at"`
	ActiveIntentCount  int64     `json:"active_intent_count"`
	PreBidCount        int64     `json:"pre_bid_count"`
	MaxBidCount        int64     `json:"max_bid_count"`
	AppliedIntentCount int64     `json:"applied_intent_count"`
	ExhaustedCount     int64     `json:"exhausted_count"`
	CancelledCount     int64     `json:"cancelled_count"`
	HasPrivatePressure bool      `json:"has_private_pressure"`
	Source             string    `json:"source"`
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

func (h MaxBidSummaryHandler) Summary(w http.ResponseWriter, r *http.Request) {
	auctionID := strings.TrimSpace(r.PathValue("id"))
	if auctionID == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "auction id is required", http.StatusBadRequest))
		return
	}
	summary, err := loadMaxBidSummary(r.Context(), h.Deps.Postgres, auctionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, r, apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound))
			return
		}
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "max bid summary query failed", http.StatusInternalServerError))
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

func loadMaxBidSummary(ctx context.Context, db *pgxpool.Pool, auctionID string) (maxBidSummary, error) {
	var summary maxBidSummary
	err := db.QueryRow(ctx, `
		WITH auction_row AS (
			SELECT id, room_id, status
			FROM auctions
			WHERE id = $1
		),
		intent_counts AS (
			SELECT
			  count(*) FILTER (WHERE status = 'ACTIVE') AS active_intent_count,
			  count(*) FILTER (WHERE status = 'ACTIVE' AND source = 'PRE_BID') AS pre_bid_count,
			  count(*) FILTER (WHERE status = 'ACTIVE' AND source = 'MAX_BID') AS max_bid_count,
			  count(*) FILTER (WHERE last_applied_seq IS NOT NULL) AS applied_intent_count,
			  count(*) FILTER (WHERE status = 'EXHAUSTED') AS exhausted_count,
			  count(*) FILTER (WHERE status = 'CANCELLED') AS cancelled_count
			FROM max_bid_intents
			WHERE auction_id = $1
		)
		SELECT a.id, a.room_id, a.status, now(),
		       coalesce(i.active_intent_count, 0),
		       coalesce(i.pre_bid_count, 0),
		       coalesce(i.max_bid_count, 0),
		       coalesce(i.applied_intent_count, 0),
		       coalesce(i.exhausted_count, 0),
		       coalesce(i.cancelled_count, 0)
		FROM auction_row a
		CROSS JOIN intent_counts i
	`, auctionID).Scan(
		&summary.AuctionID,
		&summary.RoomID,
		&summary.Status,
		&summary.GeneratedAt,
		&summary.ActiveIntentCount,
		&summary.PreBidCount,
		&summary.MaxBidCount,
		&summary.AppliedIntentCount,
		&summary.ExhaustedCount,
		&summary.CancelledCount,
	)
	if err != nil {
		return maxBidSummary{}, err
	}
	summary.HasPrivatePressure = summary.ActiveIntentCount > 0
	summary.Source = "postgres:max_bid_intents"
	return summary, nil
}
