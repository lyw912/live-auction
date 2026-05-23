package realtime

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestTicketStoreIssueConsumeOneTime(t *testing.T) {
	store := NewTicketStore(openRedisForRealtime(t))
	ctx := context.Background()
	token, err := store.Issue(ctx, Ticket{
		UserID:    "user_1",
		Role:      "user",
		RoomID:    "room_1",
		AuctionID: "auction_1",
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	ticket, err := store.Consume(ctx, token)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if ticket.UserID != "user_1" || ticket.RoomID != "room_1" || ticket.AuctionID != "auction_1" {
		t.Fatalf("unexpected ticket: %#v", ticket)
	}
	_, err = store.Consume(ctx, token)
	if !IsInvalidTicket(err) {
		t.Fatalf("second consume err = %v, want invalid ticket", err)
	}
}

func TestTicketFromProtocolsParsesHeaderList(t *testing.T) {
	got := ticketFromProtocols([]string{"auction.v1, ticket.abc123"})
	if got != "abc123" {
		t.Fatalf("ticket = %q, want abc123", got)
	}
}

func TestTicketFromRequestPrefersExplicitHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("X-Auction-WS-Ticket", "header-token")
	req.Header.Set("Sec-WebSocket-Protocol", "auction.v1, ticket.protocol-token")

	got := ticketFromRequest(req)
	if got != "header-token" {
		t.Fatalf("ticket = %q, want header-token", got)
	}
}

func TestTicketStoreRedisDownFailsClosed(t *testing.T) {
	store := NewTicketStore(redis.NewClient(&redis.Options{
		Addr:            "127.0.0.1:1",
		MaxRetries:      0,
		DialTimeout:     50 * time.Millisecond,
		ReadTimeout:     50 * time.Millisecond,
		WriteTimeout:    50 * time.Millisecond,
		MinRetryBackoff: 50 * time.Millisecond,
		MaxRetryBackoff: 50 * time.Millisecond,
	}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := store.Issue(ctx, Ticket{UserID: "user_1", RoomID: "room_1", AuctionID: "auction_1"}); err == nil {
		t.Fatalf("Issue succeeded while Redis is unavailable")
	}
	if _, err := store.Consume(ctx, "missing"); err == nil || IsInvalidTicket(err) {
		t.Fatalf("Consume err = %v, want Redis availability error", err)
	}
}

func openRedisForRealtime(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
