package realtime

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const TicketTTL = 60 * time.Second

type Ticket struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	RoomID    string `json:"room_id"`
	AuctionID string `json:"auction_id"`
}

type TicketStore struct {
	redis *redis.Client
}

func NewTicketStore(redisClient *redis.Client) *TicketStore {
	return &TicketStore{redis: redisClient}
}

func (s *TicketStore) Issue(ctx context.Context, ticket Ticket) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(ticket)
	if err != nil {
		return "", err
	}
	return token, s.redis.Set(ctx, ticketKey(token), payload, TicketTTL).Err()
}

func (s *TicketStore) Consume(ctx context.Context, token string) (Ticket, error) {
	key := ticketKey(token)
	value, err := s.redis.Eval(ctx, `
		local value = redis.call("GET", KEYS[1])
		if value then
			redis.call("DEL", KEYS[1])
		end
		return value
	`, []string{key}).Result()
	if errors.Is(err, redis.Nil) {
		return Ticket{}, ErrInvalidTicket
	}
	if err != nil {
		return Ticket{}, err
	}
	var payload []byte
	switch typed := value.(type) {
	case string:
		payload = []byte(typed)
	case []byte:
		payload = typed
	default:
		return Ticket{}, ErrInvalidTicket
	}
	var ticket Ticket
	if err := json.Unmarshal(payload, &ticket); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

var ErrInvalidTicket = fmt.Errorf("invalid ws ticket")

func ticketKey(token string) string {
	return "ws_ticket:" + token
}

func randomToken() (string, error) {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
