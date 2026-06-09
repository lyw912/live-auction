package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"nhooyr.io/websocket"

	"live-auction/backend/internal/auction"
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

func TestServeWSRejectsTicketAfterMembershipRevoked(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServer(db, rdb)
	server := httptest.NewServer(rtHandler(rt))
	t.Cleanup(server.Close)

	token := issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	if _, err := db.Exec(context.Background(), `
		UPDATE room_memberships
		SET status = 'BANNED'
		WHERE room_id = $1 AND user_id = 'user_1'
	`, auctionRow.RoomID); err != nil {
		t.Fatalf("ban membership: %v", err)
	}

	_, response, err := websocket.Dial(context.Background(), wsURL(server.URL, auctionRow.RoomID, auctionRow.ID, 0), &websocket.DialOptions{
		Subprotocols: []string{"auction.v1", "ticket." + token},
	})
	if err == nil {
		t.Fatalf("revoked ticket dial unexpectedly succeeded")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked ticket status = %v, want 403", responseStatus(response))
	}

	var count int
	if err := db.QueryRow(context.Background(), `
		SELECT count(*)
		FROM user_activity_events
		WHERE room_id = $1
		  AND auction_id = $2
		  AND user_id = 'user_1'
		  AND event_type = 'ws_ticket_access_revoked'
	`, auctionRow.RoomID, auctionRow.ID).Scan(&count); err != nil {
		t.Fatalf("count ws revocation event: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected ws_ticket_access_revoked recovery event")
	}
}

func TestServeWSInitialJoinUsesSnapshotAndReconnectCanUseHistory(t *testing.T) {
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
	assertWSMessageType(t, conn, "snapshot")
	_ = conn.Close(websocket.StatusNormalClosure, "")

	token = issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	conn = dialRealtime(t, server.URL, auctionRow.RoomID, auctionRow.ID, 0, token)
	assertWSMessageType(t, conn, "snapshot")
	_ = conn.Close(websocket.StatusNormalClosure, "")

	token = issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	conn = dialRealtime(t, server.URL, auctionRow.RoomID, auctionRow.ID, 1, token)
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

func TestRecoveryMessagesReturnsCleanHistoryWhenNoGap(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	rt := NewServer(db, rdb)
	auctionID := "auc_history_" + uuid.NewString()

	eventsKey := "auction:" + auctionID + ":events"
	if err := rdb.Del(context.Background(), eventsKey, "auction:"+auctionID+":snapshot").Err(); err != nil {
		t.Fatalf("redis cleanup: %v", err)
	}
	pushRealtimeEvent(t, rdb, auctionID, 2, "bid_accepted")
	pushRealtimeEvent(t, rdb, auctionID, 3, "bid_accepted")

	messages, source := rt.recoveryMessages(context.Background(), auctionID, 1)
	if source != "history" {
		t.Fatalf("source = %q, want history", source)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if !messageEventTypeIs(messages[0], "bid_accepted") || !messageEventTypeIs(messages[1], "bid_accepted") {
		t.Fatalf("unexpected history messages: %q", messages)
	}
}

func TestServeWSRecoveryWindowGapFallsBackToSnapshot(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServerWithOptions(db, rdb, Options{RecoveryMaxEvents: 2})
	server := httptest.NewServer(rtHandler(rt))
	t.Cleanup(server.Close)

	eventsKey := "auction:" + auctionRow.ID + ":events"
	if err := rdb.Del(context.Background(), eventsKey, "auction:"+auctionRow.ID+":snapshot").Err(); err != nil {
		t.Fatalf("redis cleanup: %v", err)
	}
	pushRealtimeEvent(t, rdb, auctionRow.ID, 1, "auction_started")
	pushRealtimeEvent(t, rdb, auctionRow.ID, 2, "bid_accepted")
	pushRealtimeEvent(t, rdb, auctionRow.ID, 3, "bid_accepted")
	pushRealtimeEvent(t, rdb, auctionRow.ID, 4, "bid_accepted")

	token := issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	conn := dialRealtime(t, server.URL, auctionRow.RoomID, auctionRow.ID, 1, token)
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

	payload, err := json.Marshal(map[string]any{
		"event_type":      "bid_accepted",
		"auction_id":      auctionRow.ID,
		"seq":             auctionRow.Seq + 1,
		"published_at_ms": time.Now().UTC().UnixMilli(),
		"payload":         map[string]any{"current_price_cents": 15000},
	})
	if err != nil {
		t.Fatalf("marshal fanout payload: %v", err)
	}
	rt.PublishAuctionEvent(ctx, auctionRow.ID, payload)
	assertWSMessageType(t, conn, "bid_accepted")
}

func TestDBSnapshotDoesNotExposePrivateMaxBidIntent(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	ctx := context.Background()
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	userID := "user_snapshot_max_" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO users (id, role, display_name) VALUES ($1, 'user', $1)`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := repo.UpsertMaxBidIntent(ctx, auctionRow.ID, userID, auction.MaxBidIntentInput{MaxAmountCents: 25_000}); err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	rt := NewServer(db, rdb)
	payload, err := rt.rebuildSnapshotFromDB(ctx, auctionRow.ID)
	if err != nil {
		t.Fatalf("rebuildSnapshotFromDB: %v", err)
	}
	if bytes.Contains(payload, []byte("max_bid_intent")) || bytes.Contains(payload, []byte("max_amount_cents")) {
		t.Fatalf("DB realtime snapshot leaked private max bid intent: %s", payload)
	}
}

func TestServeWSAdmissionRejectsWhenConnectSaturated(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServer(db, rdb).WithAdmission(NewAdmission(0, 1, time.Second))
	server := httptest.NewServer(rtHandler(rt))
	t.Cleanup(server.Close)

	token := issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	conn := dialRealtime(t, server.URL, auctionRow.RoomID, auctionRow.ID, auctionRow.Seq, token)
	defer conn.Close(websocket.StatusNormalClosure, "")

	token = issueRealtimeTicket(t, rt, auctionRow.RoomID, auctionRow.ID)
	_, response, err := websocket.Dial(context.Background(), wsURL(server.URL, auctionRow.RoomID, auctionRow.ID, auctionRow.Seq), &websocket.DialOptions{
		Subprotocols: []string{"auction.v1", "ticket." + token},
	})
	if err == nil {
		t.Fatalf("saturated dial unexpectedly succeeded")
	}
	if response == nil || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("saturated dial status = %v, want 429", responseStatus(response))
	}
	if response.Header.Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, want 1", response.Header.Get("Retry-After"))
	}
}

func TestNormalizeOptionsKeepsHeartbeatConfig(t *testing.T) {
	options := normalizeOptions(Options{
		HeartbeatInterval: 3 * time.Second,
		HeartbeatTimeout:  750 * time.Millisecond,
	})
	if options.HeartbeatInterval != 3*time.Second {
		t.Fatalf("HeartbeatInterval = %s, want 3s", options.HeartbeatInterval)
	}
	if options.HeartbeatTimeout != 750*time.Millisecond {
		t.Fatalf("HeartbeatTimeout = %s, want 750ms", options.HeartbeatTimeout)
	}
}

func TestSnapshotSourceMarksStaleRedisRecovery(t *testing.T) {
	payload := []byte(`{"event_type":"snapshot","auction_id":"auc","seq":7,"source":"redis","stale":true,"payload":{}}`)
	if got := snapshotSource(payload, "redis"); got != "redis_stale" {
		t.Fatalf("snapshotSource = %q, want redis_stale", got)
	}
}

func TestSnapshotPayloadRejectsLatestEventEnvelope(t *testing.T) {
	if !isSnapshotPayload([]byte(`{"event_type":"snapshot","auction_id":"auc","seq":7,"payload":{}}`)) {
		t.Fatalf("snapshot payload was not accepted")
	}
	if isSnapshotPayload([]byte(`{"event_type":"bid_accepted","auction_id":"auc","seq":7,"payload":{}}`)) {
		t.Fatalf("latest event envelope must not be treated as a snapshot")
	}
}

func TestHubClosesSlowConsumerOnBoundedQueueOverflow(t *testing.T) {
	hub := NewHub(1)
	closed := make(chan struct{})
	sub := hub.Subscribe("auction_1", func(SlowConsumerInfo) { close(closed) })
	defer hub.Unsubscribe("auction_1", sub)

	hub.Publish(context.Background(), "auction_1", []byte(`{"seq":1}`))
	hub.Publish(context.Background(), "auction_1", []byte(`{"seq":2}`))

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatalf("slow consumer was not closed")
	}
}

func TestHubClosesSlowConsumerOnByteBudgetOverflow(t *testing.T) {
	hub := NewHubWithOptions(HubOptions{QueueMessages: 10, QueueBytes: 8})
	closed := make(chan struct{})
	sub := hub.Subscribe("auction_1", func(SlowConsumerInfo) { close(closed) })
	defer hub.Unsubscribe("auction_1", sub)

	stats := hub.Publish(context.Background(), "auction_1", []byte(`12345`))
	if stats.SlowClosed != 0 || stats.Enqueued != 1 {
		t.Fatalf("first publish stats = %#v", stats)
	}
	stats = hub.Publish(context.Background(), "auction_1", []byte(`67890`))
	if stats.SlowClosed != 1 {
		t.Fatalf("second publish stats = %#v, want slow close", stats)
	}

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatalf("byte-budget slow consumer was not closed")
	}
}

func TestPublishAuctionEventAlsoPublishesLeaderboardDelta(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServer(db, rdb)
	input := auction.BidInput{
		ClientBidID: "lb-delta-" + uuid.NewString(),
		AmountCents: 15_000,
	}
	bid, err := repo.PlaceBidPostgresLegacyForTests(context.Background(), auctionRow.ID, "user_1", input.ClientBidID, input, "tr_lb_delta")
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}

	sub := rt.hub.Subscribe(auctionRow.ID, nil)
	defer rt.hub.Unsubscribe(auctionRow.ID, sub)
	data, err := json.Marshal(map[string]any{
		"auction_id": auctionRow.ID,
		"event_type": "bid_accepted",
		"seq":        bid.Seq,
		"payload": map[string]any{
			"user_id":             "user_1",
			"current_price_cents": 15_000,
		},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	rt.PublishAuctionEvent(context.Background(), auctionRow.ID, data)

	first := <-sub.Messages()
	sub.Ack(first)
	if !strings.Contains(string(first.data), `"event_type":"bid_accepted"`) {
		t.Fatalf("first message = %s, want original auction event", string(first.data))
	}
	var second queuedMessage
	select {
	case second = <-sub.Messages():
	case <-time.After(time.Second):
		t.Fatalf("leaderboard delta was not published")
	}
	sub.Ack(second)
	var delta struct {
		EventType           string `json:"event_type"`
		AuctionID           string `json:"auction_id"`
		Seq                 int64  `json:"seq"`
		CurrentPriceCents   int64  `json:"current_price_cents"`
		AcceptedBidderCount int64  `json:"accepted_bidder_count"`
		Entries             []struct {
			Rank        int    `json:"rank"`
			UserID      string `json:"user_id"`
			UserMasked  string `json:"user_masked"`
			AmountCents int64  `json:"amount_cents"`
			BidCount    int64  `json:"bid_count"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(second.data, &delta); err != nil {
		t.Fatalf("unmarshal delta %s: %v", string(second.data), err)
	}
	if delta.EventType != "leaderboard_delta" || delta.AuctionID != auctionRow.ID || delta.Seq < bid.Seq {
		t.Fatalf("unexpected delta envelope: %#v", delta)
	}
	if delta.CurrentPriceCents != 15_000 || delta.AcceptedBidderCount != 1 || len(delta.Entries) != 1 {
		t.Fatalf("unexpected delta body: %#v", delta)
	}
	if got := delta.Entries[0]; got.Rank != 1 || got.UserID != "user_1" || got.UserMasked != "匿名用户 1" || got.AmountCents != 15_000 || got.BidCount != 1 {
		t.Fatalf("unexpected top entry: %#v", got)
	}
}

func TestPublishAuctionEventDropsLeaderboardProjectionWhenQueueFull(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServerWithOptions(db, rdb, Options{
		LeaderboardQueueSize: 1,
		LeaderboardWorkers:   1,
	})
	rt.leaderboardQueue = make(chan leaderboardProjectionEvent, 1)
	rt.leaderboardQueue <- leaderboardProjectionEvent{
		auctionID:    auctionRow.ID,
		eventType:    "bid_accepted",
		seq:          1,
		enqueuedTime: time.Now(),
	}

	sub := rt.hub.Subscribe(auctionRow.ID, nil)
	defer rt.hub.Unsubscribe(auctionRow.ID, sub)
	data := []byte(`{"auction_id":"` + auctionRow.ID + `","event_type":"bid_accepted","seq":2}`)
	start := time.Now()
	rt.PublishAuctionEvent(context.Background(), auctionRow.ID, data)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("PublishAuctionEvent blocked on full leaderboard queue for %s", elapsed)
	}

	first := <-sub.Messages()
	sub.Ack(first)
	if !strings.Contains(string(first.data), `"event_type":"bid_accepted"`) {
		t.Fatalf("first message = %s, want original auction event", string(first.data))
	}
	select {
	case extra := <-sub.Messages():
		t.Fatalf("unexpected leaderboard message while queue was full: %s", string(extra.data))
	default:
	}
}

func TestSnapshotRebuildSingleflightBoundsReconnectStorm(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServer(db, rdb)
	if err := rdb.Del(context.Background(), "auction:"+auctionRow.ID+":events", "auction:"+auctionRow.ID+":snapshot").Err(); err != nil {
		t.Fatalf("redis cleanup: %v", err)
	}

	var rebuilds atomic.Int32
	release := make(chan struct{})
	rt.rebuildSnapshotFn = func(_ context.Context, auctionID string) ([]byte, error) {
		rebuilds.Add(1)
		<-release
		return []byte(`{"event_type":"snapshot","auction_id":"` + auctionID + `","seq":1,"source":"db","stale":false,"payload":{}}`), nil
	}

	const clients = 12
	var wg sync.WaitGroup
	wg.Add(clients)
	results := make([]string, clients)
	for i := 0; i < clients; i++ {
		i := i
		go func() {
			defer wg.Done()
			messages, _ := rt.snapshotMessage(context.Background(), auctionRow.ID)
			if len(messages) != 1 {
				t.Errorf("messages len = %d, want 1", len(messages))
				return
			}
			results[i] = string(messages[0])
		}()
	}
	waitForCondition(t, func() bool { return rebuilds.Load() == 1 })
	close(release)
	wg.Wait()
	if rebuilds.Load() != 1 {
		t.Fatalf("rebuilds = %d, want 1", rebuilds.Load())
	}
	for _, got := range results {
		if !strings.Contains(got, `"event_type":"snapshot"`) {
			t.Fatalf("unexpected snapshot result: %s", got)
		}
	}
}

func TestSnapshotRebuildSaturationFallsBackToStaleOrUnavailable(t *testing.T) {
	db := openDBForRealtime(t)
	rdb := openRedisForRealtime(t)
	repo := auction.NewRepository(db)
	auctionRow := createActiveAuctionForRealtime(t, repo, db)
	rt := NewServer(db, rdb)
	rt.snapshotSemaphore = make(chan struct{}, 1)
	rt.snapshotSemaphore <- struct{}{}

	if err := rdb.Del(context.Background(), "auction:"+auctionRow.ID+":snapshot").Err(); err != nil {
		t.Fatalf("redis cleanup: %v", err)
	}
	messages, _ := rt.snapshotMessage(context.Background(), auctionRow.ID)
	if len(messages) != 1 || !strings.Contains(string(messages[0]), "snapshot_unavailable") {
		t.Fatalf("unavailable message = %q", messages)
	}
	var anomalies int
	if err := db.QueryRow(context.Background(), `
		SELECT count(*) FROM system_anomaly_events
		WHERE auction_id = $1 AND type = 'SNAPSHOT_REBUILD_SATURATED'
	`, auctionRow.ID).Scan(&anomalies); err != nil {
		t.Fatalf("count saturated anomalies: %v", err)
	}
	if anomalies == 0 {
		t.Fatalf("expected SNAPSHOT_REBUILD_SATURATED anomaly")
	}

	staleAuctionID := "auc_stale_" + uuid.NewString()
	stale := `{"event_type":"snapshot","auction_id":"` + staleAuctionID + `","seq":1,"source":"redis","stale":false,"payload":{}}`
	if err := rdb.Set(context.Background(), "auction:"+staleAuctionID+":snapshot", stale, time.Minute).Err(); err != nil {
		t.Fatalf("set stale snapshot: %v", err)
	}
	messages, _ = rt.snapshotMessage(context.Background(), staleAuctionID)
	if len(messages) != 1 || !messageEventTypeIs(messages[0], "snapshot") || !strings.Contains(string(messages[0]), `"stale":false`) {
		t.Fatalf("redis snapshot should be returned before rebuild, got %q", messages)
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
	got := nextWSMessageType(t, conn)
	if got != want {
		t.Fatalf("event_type = %q, want %q", got, want)
	}
}

func nextWSMessageType(t *testing.T, conn *websocket.Conn) string {
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
	return message.EventType
}

func messageEventTypeIs(data []byte, want string) bool {
	var message struct {
		EventType string `json:"event_type"`
	}
	return json.Unmarshal(data, &message) == nil && message.EventType == want
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
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, 'user_1', 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id) DO UPDATE
		SET role = EXCLUDED.role, status = EXCLUDED.status
	`, roomID); err != nil {
		t.Fatalf("insert room membership: %v", err)
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

func waitForCondition(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met before deadline")
}
