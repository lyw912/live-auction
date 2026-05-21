package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"nhooyr.io/websocket"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/outbox"
)

func TestServeWSBrowserTicketAuthAndReuse(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServer(db, rdb)
	server := httptest.NewServer(rtHandler(rt))
	t.Cleanup(server.Close)

	token := issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	conn := dialRealtime(t, server.URL, auctionRow.RoomID, auctionRow.ID, 0, token)
	defer conn.Close(websocket.StatusNormalClosure, "")
	assertWSMessageType(t, conn, "snapshot")

	_, response, err := websocket.Dial(context.Background(), wsURL(server.URL, auctionRow.RoomID, auctionRow.ID, 0), &websocket.DialOptions{
		Subprotocols: []string{"auction.v1", "ticket." + token},
	})
	if err == nil {
		t.Fatalf("reused ticket dial unexpectedly succeeded")
	}
	if response == nil || response.StatusCode != 401 {
		t.Fatalf("reused ticket status = %v, want 401", responseStatus(response))
	}
}

func TestServeWSRejectsTicketForForgedRoom(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServer(db, rdb)
	server := httptest.NewServer(rtHandler(rt))
	t.Cleanup(server.Close)

	token := issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	_, response, err := websocket.Dial(context.Background(), wsURL(server.URL, "room_forged", auctionRow.ID, 0), &websocket.DialOptions{
		Subprotocols: []string{"auction.v1", "ticket." + token},
	})
	if err == nil {
		t.Fatalf("forged room dial unexpectedly succeeded")
	}
	if response == nil || response.StatusCode != 403 {
		t.Fatalf("forged room status = %v, want 403", responseStatus(response))
	}
}

func TestServeWSHistoryRecoveryAndSnapshotFallback(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServer(db, rdb)
	server := httptest.NewServer(rtHandler(rt))
	t.Cleanup(server.Close)

	eventsKey := "auction:" + auctionRow.ID + ":events"
	if err := rdb.Del(context.Background(), eventsKey, "auction:"+auctionRow.ID+":snapshot").Err(); err != nil {
		t.Fatalf("redis cleanup: %v", err)
	}
	pushRealtimeEvent(t, rdb, auctionRow.ID, 1, "auction_started")
	pushRealtimeEvent(t, rdb, auctionRow.ID, 2, "bid_accepted")

	token := issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	conn := dialRealtime(t, server.URL, auctionRow.RoomID, auctionRow.ID, 0, token)
	assertWSMessageType(t, conn, "auction_started")
	assertWSMessageType(t, conn, "bid_accepted")
	_ = conn.Close(websocket.StatusNormalClosure, "")

	if err := rdb.Del(context.Background(), eventsKey, "auction:"+auctionRow.ID+":snapshot").Err(); err != nil {
		t.Fatalf("redis cleanup: %v", err)
	}
	pushRealtimeEvent(t, rdb, auctionRow.ID, 3, "bid_accepted")
	token = issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	conn = dialRealtime(t, server.URL, auctionRow.RoomID, auctionRow.ID, 1, token)
	defer conn.Close(websocket.StatusNormalClosure, "")
	assertWSMessageType(t, conn, "snapshot")
}

func TestServeWSReceivesOutboxFanoutWhileConnected(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	quiesceRealtimeOutbox(t, db)
	rt := NewServer(db, rdb)
	server := httptest.NewServer(rtHandler(rt))
	t.Cleanup(server.Close)

	token := issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	conn := dialRealtime(t, server.URL, auctionRow.RoomID, auctionRow.ID, auctionRow.Seq, token)
	defer conn.Close(websocket.StatusNormalClosure, "")
	assertWSMessageType(t, conn, "snapshot")

	bid := auction.BidInput{ClientBidID: "ws-fanout-" + uuid.NewString(), AmountCents: 15_000}
	if _, err := repo.PlaceBid(ctx, auctionRow.ID, "user_1", bid.ClientBidID, bid, "tr_ws_fanout"); err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	ok, err := outbox.NewRelay(db, rdb, "ws-fanout-worker").WithPublisher(rt.PublishAuctionEvent).ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !ok {
		t.Fatalf("expected one outbox event")
	}
	assertWSMessageType(t, conn, "bid_accepted")
}

func TestHubClosesSlowConsumerOnBoundedQueueOverflow(t *testing.T) {
	hub := NewHub(1)
	closed := make(chan struct{})
	sub := hub.Subscribe("auction_1", func() { close(closed) })
	defer hub.Unsubscribe("auction_1", sub)

	hub.Publish(context.Background(), "auction_1", []byte(`{"seq":1}`))
	hub.Publish(context.Background(), "auction_1", []byte(`{"seq":2}`))

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatalf("slow consumer was not closed")
	}
}

func rtHandler(rt *Server) http.Handler {
	return http.HandlerFunc(rt.ServeWS)
}

func dialRealtime(t *testing.T, serverURL string, roomID string, auctionID string, lastSeq int64, token string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, wsURL(serverURL, roomID, auctionID, lastSeq), &websocket.DialOptions{
		Subprotocols: []string{"auction.v1", "ticket." + token},
	})
	if err != nil {
		t.Fatalf("Dial status=%v err=%v", responseStatus(response), err)
	}
	if conn.Subprotocol() != "auction.v1" {
		t.Fatalf("subprotocol = %q, want auction.v1", conn.Subprotocol())
	}
	return conn
}

func wsURL(serverURL string, roomID string, auctionID string, lastSeq int64) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + "/ws?room_id=" + roomID + "&auction_id=" + auctionID + "&last_seq=" + strconv.FormatInt(lastSeq, 10)
}

func assertWSMessageType(t *testing.T, conn *websocket.Conn, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var message struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatalf("unmarshal ws message %s: %v", string(data), err)
	}
	if message.EventType != want {
		t.Fatalf("event_type = %q, want %q; payload=%s", message.EventType, want, string(data))
	}
}

func issueRealtimeTicket(t *testing.T, rt *Server, roomID string, auctionID string) string {
	t.Helper()
	token, err := rt.TicketStore().Issue(context.Background(), Ticket{
		UserID:    "user_1",
		Role:      "user",
		RoomID:    roomID,
		AuctionID: auctionID,
	})
	if err != nil {
		t.Fatalf("Issue ticket: %v", err)
	}
	return token
}

func pushRealtimeEvent(t *testing.T, rdb *redis.Client, auctionID string, seq int64, eventType string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"auction_id": auctionID,
		"seq":        seq,
		"event_type": eventType,
		"payload":    map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if err := rdb.RPush(context.Background(), "auction:"+auctionID+":events", data).Err(); err != nil {
		t.Fatalf("RPush event: %v", err)
	}
}

func openDBForRealtime(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func createActiveAuctionForRealtime(t *testing.T, repo *auction.Repository, db *pgxpool.Pool) auction.Auction {
	t.Helper()
	roomID := "room_ws_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN') ON CONFLICT DO NOTHING`, roomID); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	item, err := repo.CreateItem(context.Background(), auction.CreateItemInput{Title: "WS Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	created, err := repo.CreateAuction(context.Background(), auction.CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		Rule: auction.Rule{
			DurationSeconds:     60,
			ExtendWindowSeconds: 10,
			ExtendBySeconds:     10,
			MaxExtendCount:      3,
			DepositBPS:          1000,
			DepositFloorCents:   10_000,
			DepositCapCents:     100_000_000,
		},
	}, "tr_ws")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	if _, err := repo.Schedule(context.Background(), created.ID, nil, "tr_ws"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	started, err := repo.Start(context.Background(), created.ID, "tr_ws")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return started
}

func quiesceRealtimeOutbox(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED', published_at = COALESCE(published_at, now()), locked_by = NULL, locked_until = NULL
		WHERE status <> 'PUBLISHED'
	`); err != nil {
		t.Fatalf("quiesce outbox: %v", err)
	}
}

func responseStatus(response *http.Response) any {
	if response == nil {
		return nil
	}
	return response.StatusCode
}
