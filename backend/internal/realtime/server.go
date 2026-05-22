package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"nhooyr.io/websocket"

	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
)

type Server struct {
	db                *pgxpool.Pool
	redis             *redis.Client
	ticket            *TicketStore
	hub               *Hub
	snapshotSemaphore chan struct{}
	snapshotGroup     *snapshotGroup
	rebuildSnapshotFn func(context.Context, string) ([]byte, error)
}

func NewServer(db *pgxpool.Pool, redisClient *redis.Client) *Server {
	return NewServerWithHub(db, redisClient, NewHub(defaultAuctionQueueSize))
}

func NewServerWithHub(db *pgxpool.Pool, redisClient *redis.Client, hub *Hub) *Server {
	if hub == nil {
		hub = NewHub(defaultAuctionQueueSize)
	}
	server := &Server{
		db:                db,
		redis:             redisClient,
		ticket:            NewTicketStore(redisClient),
		hub:               hub,
		snapshotSemaphore: make(chan struct{}, 4),
		snapshotGroup:     newSnapshotGroup(),
	}
	server.rebuildSnapshotFn = server.rebuildSnapshotFromDB
	return server
}

func (s *Server) TicketStore() *TicketStore {
	return s.ticket
}

func (s *Server) ValidateRoomAuction(ctx context.Context, roomID string, auctionID string) error {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auctions WHERE id = $1 AND room_id = $2)`, auctionID, roomID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return apierrors.New(apierrors.CodeForbiddenRoom, "auction does not belong to room", http.StatusForbidden)
	}
	return nil
}

func (s *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room_id")
	auctionID := r.URL.Query().Get("auction_id")
	lastSeq, _ := strconv.ParseInt(r.URL.Query().Get("last_seq"), 10, 64)

	token := ticketFromProtocols(r.Header.Values("Sec-WebSocket-Protocol"))
	if token == "" {
		http.Error(w, "missing ticket", http.StatusUnauthorized)
		return
	}
	ticket, err := s.ticket.Consume(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return
	}
	if ticket.RoomID != roomID || ticket.AuctionID != auctionID {
		http.Error(w, "ticket scope mismatch", http.StatusForbidden)
		return
	}
	if err := s.ValidateRoomAuction(r.Context(), roomID, auctionID); err != nil {
		http.Error(w, "forbidden room", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"auction.v1"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	slow := make(chan struct{})
	var closeSlow sync.Once
	sub := s.hub.Subscribe(auctionID, func() { closeSlow.Do(func() { close(slow) }) })
	defer s.hub.Unsubscribe(auctionID, sub)
	observability.AddGauge("auction_ws_connections", 1, map[string]string{"room": roomID})
	defer observability.AddGauge("auction_ws_connections", -1, map[string]string{"room": roomID})

	writeCtx, cancelWrite := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancelWrite()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	messages, recoverySource := s.recoveryMessages(ctx, auctionID, lastSeq)
	observability.Inc("auction_ws_recover_total", map[string]string{"result": metricRecoveryResult(recoverySource)})
	observability.Inc("auction_snapshot_source_total", map[string]string{"source": recoverySource})
	_ = s.recordWSActivity(ctx, roomID, auctionID, ticket.UserID, "ws_reconnect", map[string]any{
		"last_seq": lastSeq,
	})
	_ = s.recordWSActivity(ctx, roomID, auctionID, ticket.UserID, "ws_recovered", map[string]any{
		"source": recoverySource,
	})
	for _, message := range messages {
		if err := conn.Write(writeCtx, websocket.MessageText, message); err != nil {
			observability.Inc("auction_ws_slow_consumer_disconnect_total", nil)
			_ = s.recordWSActivity(ctx, roomID, auctionID, ticket.UserID, "ws_slow_consumer_closed", map[string]any{
				"phase": "recovery",
			})
			_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
			return
		}
	}

	for {
		select {
		case message, ok := <-sub.Messages():
			if !ok {
				observability.Inc("auction_ws_slow_consumer_disconnect_total", nil)
				_ = s.recordWSActivity(r.Context(), roomID, auctionID, ticket.UserID, "ws_slow_consumer_closed", map[string]any{
					"phase": "queue_closed",
				})
				_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			err := conn.Write(ctx, websocket.MessageText, message)
			cancel()
			if err != nil {
				observability.Inc("auction_ws_slow_consumer_disconnect_total", nil)
				_ = s.recordWSActivity(r.Context(), roomID, auctionID, ticket.UserID, "ws_slow_consumer_closed", map[string]any{
					"phase": "write",
				})
				_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
				return
			}
		case <-slow:
			observability.Inc("auction_ws_slow_consumer_disconnect_total", nil)
			_ = s.recordWSActivity(r.Context(), roomID, auctionID, ticket.UserID, "ws_slow_consumer_closed", map[string]any{
				"phase": "backpressure",
			})
			_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) PublishAuctionEvent(ctx context.Context, auctionID string, payload []byte) {
	s.hub.Publish(ctx, auctionID, payload)
}

func (s *Server) recoveryMessages(ctx context.Context, auctionID string, lastSeq int64) ([][]byte, string) {
	redisStart := time.Now()
	values, err := s.redis.LRange(ctx, "auction:"+auctionID+":events", 0, -1).Result()
	observability.Observe("redis_command_latency_seconds", time.Since(redisStart).Seconds(), map[string]string{"command": "lrange_recovery_events"}, observability.DefaultLatencyBuckets)
	if err == nil {
		var out [][]byte
		next := lastSeq + 1
		for _, value := range values {
			seq, ok := eventSeq([]byte(value))
			if !ok || seq <= lastSeq {
				continue
			}
			if seq != next {
				return s.snapshotMessage(ctx, auctionID)
			}
			out = append(out, []byte(value))
			next++
		}
		if len(out) > 0 {
			return out, "history"
		}
	}
	return s.snapshotMessage(ctx, auctionID)
}

func (s *Server) snapshotMessage(ctx context.Context, auctionID string) ([][]byte, string) {
	redisStart := time.Now()
	payload, err := s.redis.Get(ctx, "auction:"+auctionID+":snapshot").Bytes()
	observability.Observe("redis_command_latency_seconds", time.Since(redisStart).Seconds(), map[string]string{"command": "get_snapshot"}, observability.DefaultLatencyBuckets)
	if err == nil && len(payload) > 0 {
		return [][]byte{payload}, snapshotSource(payload, "redis")
	}
	snapshot, err := s.snapshotGroup.Do(ctx, auctionID, func() ([]byte, error) {
		return s.rebuildSnapshotBounded(ctx, auctionID)
	})
	if err != nil {
		if errors.Is(err, errSnapshotRebuildSaturated) {
			_ = s.recordSnapshotSaturated(ctx, auctionID)
		}
		if stale := s.staleSnapshot(ctx, auctionID); len(stale) > 0 {
			return [][]byte{stale}, "redis"
		}
		return [][]byte{snapshotUnavailable(auctionID)}, "snapshot_unavailable"
	}
	return [][]byte{snapshot}, snapshotSource(snapshot, "db")
}

func (s *Server) rebuildSnapshotBounded(ctx context.Context, auctionID string) ([]byte, error) {
	select {
	case s.snapshotSemaphore <- struct{}{}:
		defer func() { <-s.snapshotSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, errSnapshotRebuildSaturated
	}
	return s.rebuildSnapshotFn(ctx, auctionID)
}

func (s *Server) rebuildSnapshotFromDB(ctx context.Context, auctionID string) ([]byte, error) {
	var payload []byte
	err := s.db.QueryRow(ctx, `
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
	`, auctionID).Scan(&payload)
	if err != nil {
		return nil, err
	}
	_ = s.redis.Set(ctx, "auction:"+auctionID+":snapshot", payload, 30*time.Minute).Err()
	return payload, nil
}

func (s *Server) staleSnapshot(ctx context.Context, auctionID string) []byte {
	payload, err := s.redis.Get(ctx, "auction:"+auctionID+":snapshot").Bytes()
	if err != nil || len(payload) == 0 {
		return nil
	}
	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		return payload
	}
	message["stale"] = true
	data, err := json.Marshal(message)
	if err != nil {
		return payload
	}
	return data
}

func (s *Server) recordSnapshotSaturated(ctx context.Context, auctionID string) error {
	payload, err := json.Marshal(map[string]any{
		"auction_id":     auctionID,
		"retry_after_ms": 1000,
	})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('MED', 'SNAPSHOT_REBUILD_SATURATED', $1, $2, $3)
	`, auctionID, fmt.Sprintf("snapshot rebuild saturated for auction %s", auctionID), payload)
	return err
}

func ticketFromProtocols(protocols []string) string {
	for _, protocol := range protocols {
		for _, part := range strings.Split(protocol, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "ticket.") {
				return strings.TrimPrefix(part, "ticket.")
			}
		}
	}
	return ""
}

func eventSeq(payload []byte) (int64, bool) {
	var event struct {
		Seq int64 `json:"seq"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return 0, false
	}
	return event.Seq, true
}

func snapshotSource(payload []byte, fallback string) string {
	var event struct {
		EventType string `json:"event_type"`
		Source    string `json:"source"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return fallback
	}
	if event.EventType == "snapshot_unavailable" {
		return "snapshot_unavailable"
	}
	if event.Source != "" {
		return event.Source
	}
	return fallback
}

func metricRecoveryResult(source string) string {
	switch source {
	case "history", "snapshot_unavailable":
		return source
	case "":
		return "unknown"
	default:
		return "snapshot_" + source
	}
}

func (s *Server) recordWSActivity(ctx context.Context, roomID string, auctionID string, userID string, eventType string, payload map[string]any) error {
	if s.db == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO user_activity_events (room_id, auction_id, user_id, event_type, source, payload_json)
		VALUES ($1, $2, $3, $4, 'ws', $5)
	`, roomID, auctionID, userID, eventType, data)
	return err
}

func IsInvalidTicket(err error) bool {
	return errors.Is(err, ErrInvalidTicket)
}

var errSnapshotRebuildSaturated = errors.New("snapshot rebuild saturated")

func snapshotUnavailable(auctionID string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"event_type":     "snapshot_unavailable",
		"auction_id":     auctionID,
		"retry_after_ms": 1000,
	})
	return payload
}
