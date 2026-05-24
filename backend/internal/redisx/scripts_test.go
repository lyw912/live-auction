package redisx

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestClassifyScriptError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "context deadline", err: context.DeadlineExceeded, want: OutcomeTimeout},
		{name: "redis nil", err: redis.Nil, want: OutcomeMissing},
		{name: "busy", err: errors.New("BUSY Redis is busy running a script"), want: OutcomeBusy},
		{name: "noscript", err: errors.New("NOSCRIPT No matching script"), want: OutcomeNoScript},
		{name: "connection refused", err: errors.New("dial tcp 127.0.0.1:1: connect: connection refused"), want: OutcomeUnavailable},
		{name: "generic", err: errors.New("ERR bad lua"), want: OutcomeError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyScriptError(tt.err); got != tt.want {
				t.Fatalf("ClassifyScriptError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBorrowedRedisKeyConventions(t *testing.T) {
	auctionID := "auction_123"
	keys := []string{
		BidLimitUserKey(auctionID, "user_1"),
		BidLimitIPKey(auctionID, "192.0.2.1"),
		BidLimitAuctionKey(auctionID),
	}
	for _, key := range keys {
		if !strings.Contains(key, "{"+auctionID+"}") {
			t.Fatalf("key %q does not contain auction hash tag", key)
		}
	}
	if got := WSTicketKey("tok"); got != "ws_ticket:tok" {
		t.Fatalf("WSTicketKey = %q, want ws_ticket:tok", got)
	}
}
