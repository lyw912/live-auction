package realtime

import (
	"context"
	"os"
	"testing"

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
