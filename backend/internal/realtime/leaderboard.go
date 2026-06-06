package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"live-auction/backend/internal/observability"
)

const leaderboardDeltaTopLimit = 5

type leaderboardDelta struct {
	AuctionID           string                   `json:"auction_id"`
	EventType           string                   `json:"event_type"`
	Seq                 int64                    `json:"seq"`
	ServerTimeMS        int64                    `json:"server_time_ms"`
	CurrentPriceCents   int64                    `json:"current_price_cents"`
	CurrentWinnerID     *string                  `json:"current_winner_id,omitempty"`
	LeaderAmountCents   int64                    `json:"leader_amount_cents"`
	NextValidBidCents   int64                    `json:"next_valid_bid_cents"`
	AcceptedBidderCount int64                    `json:"accepted_bidder_count"`
	ActiveBidders30s    int64                    `json:"active_bidders_30s,omitempty"`
	AcceptedBids30s     int64                    `json:"accepted_bids_30s,omitempty"`
	PriceVelocityCPM    int64                    `json:"price_velocity_cents_per_min,omitempty"`
	Entries             []leaderboardDeltaEntry  `json:"entries"`
	Projection          leaderboardDeltaMetadata `json:"projection"`
}

type leaderboardDeltaEntry struct {
	Rank        int       `json:"rank"`
	UserID      string    `json:"user_id"`
	UserMasked  string    `json:"user_masked"`
	AmountCents int64     `json:"amount_cents"`
	BidCount    int64     `json:"bid_count"`
	LastBidAt   time.Time `json:"last_bid_at"`
}

type leaderboardDeltaMetadata struct {
	Source      string `json:"source"`
	TopLimit    int    `json:"top_limit"`
	GeneratedMS int64  `json:"generated_ms"`
}

func (s *Server) PublishAuctionEvent(ctx context.Context, auctionID string, payload []byte) {
	stats := s.hub.Publish(ctx, auctionID, payload)
	if stats.Subscribers > 0 {
		observePublishStats("auction_event", stats)
	}
	s.enqueueLeaderboardDeltaBestEffort(auctionID, payload)
}

func (s *Server) enqueueLeaderboardDeltaBestEffort(auctionID string, eventPayload []byte) {
	if s == nil || s.db == nil || s.leaderboardQueue == nil {
		return
	}
	var envelope struct {
		EventType string `json:"event_type"`
		Seq       int64  `json:"seq"`
	}
	if err := json.Unmarshal(eventPayload, &envelope); err != nil {
		return
	}
	switch envelope.EventType {
	case "bid_accepted", "auction_sold", "auction_ended":
	default:
		return
	}
	event := leaderboardProjectionEvent{
		auctionID:    auctionID,
		eventType:    envelope.EventType,
		seq:          envelope.Seq,
		enqueuedTime: time.Now(),
	}
	select {
	case s.leaderboardQueue <- event:
		observability.Inc("auction_leaderboard_delta_enqueue_total", map[string]string{"result": "ok"})
	default:
		observability.Inc("auction_leaderboard_delta_enqueue_total", map[string]string{"result": "dropped_queue_full"})
	}
}

func (s *Server) runLeaderboardProjectionWorker() {
	for event := range s.leaderboardQueue {
		s.publishLeaderboardDeltaBestEffort(context.Background(), event)
	}
}

func (s *Server) publishLeaderboardDeltaBestEffort(ctx context.Context, event leaderboardProjectionEvent) {
	if s == nil || s.db == nil {
		return
	}
	projectionCtx, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	delta, err := s.buildLeaderboardDelta(projectionCtx, event.auctionID, event.eventType, event.seq)
	if err != nil {
		observability.Inc("auction_leaderboard_delta_build_total", map[string]string{"result": "error"})
		return
	}
	data, err := json.Marshal(delta)
	if err != nil {
		observability.Inc("auction_leaderboard_delta_build_total", map[string]string{"result": "marshal_error"})
		return
	}
	stats := s.hub.Publish(ctx, event.auctionID, data)
	observability.Inc("auction_leaderboard_delta_build_total", map[string]string{"result": "ok"})
	observability.Observe("auction_leaderboard_delta_build_seconds", time.Since(start).Seconds(), nil, observability.DefaultLatencyBuckets)
	observability.Observe("auction_leaderboard_delta_queue_lag_seconds", start.Sub(event.enqueuedTime).Seconds(), nil, observability.DefaultLatencyBuckets)
	if stats.Subscribers > 0 {
		observePublishStats("leaderboard_delta", stats)
	}
}

func (s *Server) buildLeaderboardDelta(ctx context.Context, auctionID string, eventType string, seq int64) (leaderboardDelta, error) {
	var delta leaderboardDelta
	delta.AuctionID = auctionID
	delta.EventType = "leaderboard_delta"
	delta.Seq = seq
	delta.Projection = leaderboardDeltaMetadata{
		Source:      "outbox_relay_projection",
		TopLimit:    leaderboardDeltaTopLimit,
		GeneratedMS: time.Now().UTC().UnixMilli(),
	}
	err := s.db.QueryRow(ctx, `
		SELECT current_price_cents, current_winner_id, seq, increment_cents,
		       floor(extract(epoch from clock_timestamp()) * 1000)::bigint
		FROM auctions
		WHERE id = $1
	`, auctionID).Scan(&delta.CurrentPriceCents, &delta.CurrentWinnerID, &delta.Seq, &delta.NextValidBidCents, &delta.ServerTimeMS)
	if err != nil {
		return leaderboardDelta{}, err
	}
	if seq > delta.Seq {
		delta.Seq = seq
	}
	delta.NextValidBidCents += delta.CurrentPriceCents
	rows, err := s.db.Query(ctx, `
		WITH best AS (
			SELECT user_id, max(amount_cents) AS amount_cents, count(*) AS bid_count, max(created_at) AS last_bid_at
			FROM bids
			WHERE auction_id = $1 AND status = 'ACCEPTED'
			GROUP BY user_id
		),
		ranked AS (
			SELECT user_id, amount_cents, bid_count, last_bid_at,
			       row_number() OVER (ORDER BY amount_cents DESC, last_bid_at ASC, user_id ASC) AS rank
			FROM best
		)
		SELECT rank, user_id, amount_cents, bid_count, last_bid_at
		FROM ranked
		WHERE rank <= $2
		ORDER BY rank
	`, auctionID, leaderboardDeltaTopLimit)
	if err != nil {
		return leaderboardDelta{}, err
	}
	defer rows.Close()
	delta.Entries = make([]leaderboardDeltaEntry, 0, leaderboardDeltaTopLimit)
	for rows.Next() {
		var entry leaderboardDeltaEntry
		if err := rows.Scan(&entry.Rank, &entry.UserID, &entry.AmountCents, &entry.BidCount, &entry.LastBidAt); err != nil {
			return leaderboardDelta{}, err
		}
		entry.UserMasked = maskLeaderboardUserID(entry.UserID)
		delta.Entries = append(delta.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return leaderboardDelta{}, err
	}
	if len(delta.Entries) > 0 {
		delta.LeaderAmountCents = delta.Entries[0].AmountCents
	}
	if err := s.db.QueryRow(ctx, `
		SELECT count(DISTINCT user_id)
		FROM bids
		WHERE auction_id = $1 AND status = 'ACCEPTED'
	`, auctionID).Scan(&delta.AcceptedBidderCount); err != nil && err != pgx.ErrNoRows {
		return leaderboardDelta{}, err
	}
	if err := s.db.QueryRow(ctx, `
		SELECT
			COALESCE(count(DISTINCT user_id), 0),
			COALESCE(count(*), 0),
			COALESCE(max(amount_cents) - min(amount_cents), 0)
		FROM bids
		WHERE auction_id = $1
		  AND status = 'ACCEPTED'
		  AND created_at >= now() - interval '30 seconds'
	`, auctionID).Scan(&delta.ActiveBidders30s, &delta.AcceptedBids30s, &delta.PriceVelocityCPM); err != nil {
		return leaderboardDelta{}, err
	}
	delta.PriceVelocityCPM *= 2
	_ = eventType
	return delta, nil
}

func observePublishStats(kind string, stats PublishStats) {
	observability.Observe("auction_ws_publish_subscribers", float64(stats.Subscribers), map[string]string{"kind": kind}, []float64{1, 10, 50, 100, 300, 1000})
	observability.Observe("auction_ws_send_queue_depth", float64(stats.MaxQueueDepth), map[string]string{"kind": kind}, []float64{1, 16, 64, 128, 256, 512})
	observability.Observe("auction_ws_send_queue_bytes", float64(stats.MaxQueueBytes), map[string]string{"kind": kind}, []float64{1024, 16384, 65536, 262144, 1048576, 4194304})
	if stats.SlowClosed > 0 {
		observability.Add("auction_ws_slow_consumer_disconnect_total", float64(stats.SlowClosed), map[string]string{"kind": kind})
		observability.Observe("auction_ws_slow_consumer_queue_depth", float64(stats.SlowMaxDepth), map[string]string{"kind": kind}, []float64{1, 16, 64, 128, 256, 512})
		observability.Observe("auction_ws_slow_consumer_queue_bytes", float64(stats.SlowMaxBytes), map[string]string{"kind": kind}, []float64{1024, 16384, 65536, 262144, 1048576, 4194304})
	}
}

func maskLeaderboardUserID(userID string) string {
	if userID == "" {
		return "匿名"
	}
	if len(userID) <= 2 {
		return userID + "**"
	}
	return userID[:2] + "**"
}
