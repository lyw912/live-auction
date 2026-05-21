package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"nhooyr.io/websocket"

	apierrors "live-auction/backend/internal/platform/errors"
)

type Server struct {
	db     *pgxpool.Pool
	redis  *redis.Client
	ticket *TicketStore
	hub    *Hub
}

func NewServer(db *pgxpool.Pool, redisClient *redis.Client) *Server {
	return NewServerWithHub(db, redisClient, NewHub(defaultAuctionQueueSize))
}

func NewServerWithHub(db *pgxpool.Pool, redisClient *redis.Client, hub *Hub) *Server {
	if hub == nil {
		hub = NewHub(defaultAuctionQueueSize)
	}
	return &Server{db: db, redis: redisClient, ticket: NewTicketStore(redisClient), hub: hub}
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

	writeCtx, cancelWrite := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancelWrite()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	for _, message := range s.recoveryMessages(ctx, auctionID, lastSeq) {
		if err := conn.Write(writeCtx, websocket.MessageText, message); err != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
			return
		}
	}

	for {
		select {
		case message, ok := <-sub.Messages():
			if !ok {
				_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			err := conn.Write(ctx, websocket.MessageText, message)
			cancel()
			if err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
				return
			}
		case <-slow:
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

func (s *Server) recoveryMessages(ctx context.Context, auctionID string, lastSeq int64) [][]byte {
	values, err := s.redis.LRange(ctx, "auction:"+auctionID+":events", 0, -1).Result()
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
			return out
		}
	}
	return s.snapshotMessage(ctx, auctionID)
}

func (s *Server) snapshotMessage(ctx context.Context, auctionID string) [][]byte {
	payload, err := s.redis.Get(ctx, "auction:"+auctionID+":snapshot").Bytes()
	if err == nil && len(payload) > 0 {
		return [][]byte{payload}
	}
	snapshot, err := s.rebuildSnapshot(ctx, auctionID)
	if err != nil {
		return nil
	}
	return [][]byte{snapshot}
}

func (s *Server) rebuildSnapshot(ctx context.Context, auctionID string) ([]byte, error) {
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
				'accepted_bid_count', a.accepted_bid_count
			)
		)
		FROM auctions a
		WHERE a.id = $1
	`, auctionID).Scan(&payload)
	if err != nil {
		return nil, err
	}
	_ = s.redis.Set(ctx, "auction:"+auctionID+":snapshot", payload, 30*time.Minute).Err()
	return payload, nil
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

func IsInvalidTicket(err error) bool {
	return errors.Is(err, ErrInvalidTicket)
}
