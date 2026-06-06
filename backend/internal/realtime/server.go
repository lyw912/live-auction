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
	admission         *Admission
	options           Options
	snapshotSemaphore chan struct{}
	snapshotGroup     *snapshotGroup
	rebuildSnapshotFn func(context.Context, string) ([]byte, error)
	activityQueue     chan wsActivityEvent
	leaderboardQueue  chan leaderboardProjectionEvent
}

type wsActivityEvent struct {
	roomID    string
	auctionID string
	userID    string
	eventType string
	payload   map[string]any
}

type leaderboardProjectionEvent struct {
	auctionID    string
	eventType    string
	seq          int64
	enqueuedTime time.Time
}

type Options struct {
	HubQueueMessages      int
	HubQueueBytes         int64
	RecoveryMaxEvents     int64
	SnapshotRebuildLimit  int
	HistoryTTL            time.Duration
	SnapshotTTL           time.Duration
	StreamEpochTTL        time.Duration
	RecoveryReadChunkSize int64
	HeartbeatInterval     time.Duration
	HeartbeatTimeout      time.Duration
	LeaderboardQueueSize  int
	LeaderboardWorkers    int
}

func defaultOptions() Options {
	return Options{
		HubQueueMessages:      defaultAuctionQueueSize,
		HubQueueBytes:         defaultAuctionQueueBytes,
		RecoveryMaxEvents:     300,
		SnapshotRebuildLimit:  4,
		HistoryTTL:            30 * time.Minute,
		SnapshotTTL:           30 * time.Minute,
		StreamEpochTTL:        24 * time.Hour,
		RecoveryReadChunkSize: 256,
		HeartbeatInterval:     20 * time.Second,
		HeartbeatTimeout:      5 * time.Second,
		LeaderboardQueueSize:  1024,
		LeaderboardWorkers:    1,
	}
}

func NewServer(db *pgxpool.Pool, redisClient *redis.Client) *Server {
	return NewServerWithOptions(db, redisClient, defaultOptions())
}

func NewServerWithHub(db *pgxpool.Pool, redisClient *redis.Client, hub *Hub) *Server {
	options := defaultOptions()
	if hub == nil {
		hub = NewHubWithOptions(HubOptions{QueueMessages: options.HubQueueMessages, QueueBytes: options.HubQueueBytes})
	}
	return newServer(db, redisClient, hub, options)
}

func NewServerWithOptions(db *pgxpool.Pool, redisClient *redis.Client, options Options) *Server {
	options = normalizeOptions(options)
	hub := NewHubWithOptions(HubOptions{QueueMessages: options.HubQueueMessages, QueueBytes: options.HubQueueBytes})
	return newServer(db, redisClient, hub, options)
}

func newServer(db *pgxpool.Pool, redisClient *redis.Client, hub *Hub, options Options) *Server {
	options = normalizeOptions(options)
	server := &Server{
		db:                db,
		redis:             redisClient,
		ticket:            NewTicketStore(redisClient),
		hub:               hub,
		admission:         NewAdmission(0, 0, time.Second),
		options:           options,
		snapshotSemaphore: make(chan struct{}, options.SnapshotRebuildLimit),
		snapshotGroup:     newSnapshotGroup(),
		activityQueue:     make(chan wsActivityEvent, 8192),
		leaderboardQueue:  make(chan leaderboardProjectionEvent, options.LeaderboardQueueSize),
	}
	server.rebuildSnapshotFn = server.rebuildSnapshotFromDB
	if db != nil {
		go server.runActivityWriter()
		for i := 0; i < options.LeaderboardWorkers; i++ {
			go server.runLeaderboardProjectionWorker()
		}
	}
	return server
}

func normalizeOptions(options Options) Options {
	defaults := defaultOptions()
	if options.HubQueueMessages <= 0 {
		options.HubQueueMessages = defaults.HubQueueMessages
	}
	if options.HubQueueBytes <= 0 {
		options.HubQueueBytes = defaults.HubQueueBytes
	}
	if options.RecoveryMaxEvents <= 0 {
		options.RecoveryMaxEvents = defaults.RecoveryMaxEvents
	}
	if options.SnapshotRebuildLimit <= 0 {
		options.SnapshotRebuildLimit = defaults.SnapshotRebuildLimit
	}
	if options.HistoryTTL <= 0 {
		options.HistoryTTL = defaults.HistoryTTL
	}
	if options.SnapshotTTL <= 0 {
		options.SnapshotTTL = defaults.SnapshotTTL
	}
	if options.StreamEpochTTL <= 0 {
		options.StreamEpochTTL = defaults.StreamEpochTTL
	}
	if options.RecoveryReadChunkSize <= 0 {
		options.RecoveryReadChunkSize = defaults.RecoveryReadChunkSize
	}
	if options.RecoveryReadChunkSize > options.RecoveryMaxEvents {
		options.RecoveryReadChunkSize = options.RecoveryMaxEvents
	}
	if options.HeartbeatInterval <= 0 {
		options.HeartbeatInterval = defaults.HeartbeatInterval
	}
	if options.HeartbeatTimeout <= 0 {
		options.HeartbeatTimeout = defaults.HeartbeatTimeout
	}
	if options.LeaderboardQueueSize <= 0 {
		options.LeaderboardQueueSize = defaults.LeaderboardQueueSize
	}
	if options.LeaderboardWorkers <= 0 {
		options.LeaderboardWorkers = defaults.LeaderboardWorkers
	}
	return options
}

func (s *Server) TicketStore() *TicketStore {
	return s.ticket
}

func (s *Server) Admission() *Admission {
	return s.admission
}

func (s *Server) WithAdmission(admission *Admission) *Server {
	if admission == nil {
		admission = NewAdmission(0, 0, time.Second)
	}
	s.admission = admission
	return s
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

func (s *Server) ValidateTicketRoomAccess(ctx context.Context, ticket Ticket) error {
	var exists bool
	var err error
	if ticket.Role == "host" {
		err = s.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1
				FROM rooms
				WHERE id = $1
				  AND host_id = $2
				  AND status = 'OPEN'
			)
		`, ticket.RoomID, ticket.UserID).Scan(&exists)
	} else {
		err = s.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1
				FROM room_memberships
				WHERE room_id = $1
				  AND user_id = $2
				  AND role IN ('viewer','host')
				  AND status = 'ACTIVE'
			)
		`, ticket.RoomID, ticket.UserID).Scan(&exists)
	}
	if err != nil {
		return err
	}
	if !exists {
		_ = s.recordWSActivity(ctx, ticket.RoomID, ticket.AuctionID, ticket.UserID, "ws_ticket_access_revoked", map[string]any{
			"role": ticket.Role,
		})
		return apierrors.New(apierrors.CodeForbiddenRoom, "active room access required", http.StatusForbidden)
	}
	return nil
}

func (s *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	joinStart := time.Now()
	roomID := r.URL.Query().Get("room_id")
	auctionID := r.URL.Query().Get("auction_id")
	lastSeq, _ := strconv.ParseInt(r.URL.Query().Get("last_seq"), 10, 64)

	stageStart := time.Now()
	releaseConnect, ok := s.admission.TryConnect()
	if !ok {
		observeWSJoinStage("connect_admission", "rejected", stageStart)
		observeWSJoinStage("total", "rejected", joinStart)
		s.admission.WriteRejected(w)
		return
	}
	observeWSJoinStage("connect_admission", "ok", stageStart)
	defer releaseConnect()

	token := ticketFromRequest(r)
	if token == "" {
		observeWSJoinStage("ticket_consume", "missing", time.Now())
		observeWSJoinStage("total", "unauthorized", joinStart)
		http.Error(w, "missing ticket", http.StatusUnauthorized)
		return
	}
	stageStart = time.Now()
	ticket, err := s.ticket.Consume(r.Context(), token)
	if err != nil {
		observeWSJoinStage("ticket_consume", "invalid", stageStart)
		observeWSJoinStage("total", "unauthorized", joinStart)
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return
	}
	observeWSJoinStage("ticket_consume", "ok", stageStart)
	if ticket.RoomID != roomID || ticket.AuctionID != auctionID {
		observeWSJoinStage("scope_validate", "mismatch", time.Now())
		observeWSJoinStage("total", "forbidden", joinStart)
		http.Error(w, "ticket scope mismatch", http.StatusForbidden)
		return
	}
	observeWSJoinStage("scope_validate", "ok", time.Now())
	stageStart = time.Now()
	if err := s.ValidateRoomAuction(r.Context(), roomID, auctionID); err != nil {
		observeWSJoinStage("room_validate", "forbidden", stageStart)
		observeWSJoinStage("total", "forbidden", joinStart)
		http.Error(w, "forbidden room", http.StatusForbidden)
		return
	}
	observeWSJoinStage("room_validate", "ok", stageStart)
	stageStart = time.Now()
	if err := s.ValidateTicketRoomAccess(r.Context(), ticket); err != nil {
		observeWSJoinStage("access_validate", "forbidden", stageStart)
		observeWSJoinStage("total", "forbidden", joinStart)
		http.Error(w, "forbidden room", http.StatusForbidden)
		return
	}
	observeWSJoinStage("access_validate", "ok", stageStart)

	stageStart = time.Now()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"auction.v1"},
	})
	if err != nil {
		observeWSJoinStage("accept", "error", stageStart)
		observeWSJoinStage("total", "accept_error", joinStart)
		return
	}
	observeWSJoinStage("accept", "ok", stageStart)
	defer conn.Close(websocket.StatusNormalClosure, "")
	connCtx, cancelConn := context.WithCancel(r.Context())
	defer cancelConn()
	connCtx = conn.CloseRead(connCtx)
	writeMu := &sync.Mutex{}
	go s.keepAlive(connCtx, cancelConn, conn, roomID, auctionID, ticket.UserID)

	stageStart = time.Now()
	slow := make(chan SlowConsumerInfo, 1)
	var closeSlow sync.Once
	sub := s.hub.Subscribe(auctionID, func(info SlowConsumerInfo) {
		closeSlow.Do(func() {
			slow <- info
			close(slow)
		})
	})
	defer s.hub.Unsubscribe(auctionID, sub)
	observeWSJoinStage("subscribe", "ok", stageStart)
	observability.AddGauge("auction_ws_connections", 1, map[string]string{"room": roomID})
	defer observability.AddGauge("auction_ws_connections", -1, map[string]string{"room": roomID})

	writeCtx, cancelWrite := context.WithTimeout(connCtx, 10*time.Second)
	defer cancelWrite()
	ctx, cancel := context.WithTimeout(connCtx, 10*time.Second)
	defer cancel()
	stageStart = time.Now()
	messages, recoverySource := s.recoveryMessages(ctx, auctionID, lastSeq)
	observeWSJoinStage("recovery", metricRecoveryResult(recoverySource), stageStart)
	observability.Inc("auction_ws_recover_total", map[string]string{"result": metricRecoveryResult(recoverySource)})
	observability.Inc("auction_snapshot_source_total", map[string]string{"source": recoverySource})
	s.recordWSActivityAsync(roomID, auctionID, ticket.UserID, "ws_reconnect", map[string]any{
		"last_seq": lastSeq,
	})
	s.recordWSActivityAsync(roomID, auctionID, ticket.UserID, "ws_recovered", map[string]any{
		"source": recoverySource,
		"stale":  recoverySource == "redis_stale",
	})
	stageStart = time.Now()
	for _, message := range messages {
		if err := writeWS(writeCtx, conn, writeMu, websocket.MessageText, message); err != nil {
			observeWSJoinStage("first_write", "error", stageStart)
			observeWSJoinStage("total", "slow_consumer", joinStart)
			observability.Inc("auction_ws_slow_consumer_disconnect_total", nil)
			_ = s.recordWSActivity(ctx, roomID, auctionID, ticket.UserID, "ws_slow_consumer_closed", map[string]any{
				"phase": "recovery",
			})
			_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
			return
		}
	}
	observeWSJoinStage("first_write", "ok", stageStart)
	observeWSJoinStage("total", metricRecoveryResult(recoverySource), joinStart)

	for {
		select {
		case queued, ok := <-sub.Messages():
			if !ok {
				observability.Inc("auction_ws_slow_consumer_disconnect_total", nil)
				_ = s.recordWSActivity(context.Background(), roomID, auctionID, ticket.UserID, "ws_slow_consumer_closed", map[string]any{
					"phase": "queue_closed",
				})
				_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
				return
			}
			message := queued.data
			ctx, cancel := context.WithTimeout(connCtx, 5*time.Second)
			err := writeWS(ctx, conn, writeMu, websocket.MessageText, message)
			cancel()
			sub.Ack(queued)
			if err != nil {
				observability.Inc("auction_ws_slow_consumer_disconnect_total", nil)
				_ = s.recordWSActivity(context.Background(), roomID, auctionID, ticket.UserID, "ws_slow_consumer_closed", map[string]any{
					"phase": "write",
				})
				_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
				return
			}
		case info := <-slow:
			observability.Inc("auction_ws_slow_consumer_disconnect_total", nil)
			payload := map[string]any{
				"phase": "backpressure",
			}
			if info.Reason != "" {
				payload["reason"] = info.Reason
				payload["queue_depth"] = info.QueueDepth
				payload["queue_bytes"] = info.QueueBytes
				payload["queue_messages_limit"] = info.QueueMessagesLimit
				payload["queue_bytes_limit"] = info.QueueBytesLimit
				payload["payload_bytes"] = info.PayloadBytes
			}
			_ = s.recordWSActivity(context.Background(), roomID, auctionID, ticket.UserID, "ws_slow_consumer_closed", payload)
			_ = conn.Close(websocket.StatusPolicyViolation, string(apierrors.CodeSlowConsumer))
			return
		case <-connCtx.Done():
			return
		}
	}
}

func (s *Server) keepAlive(ctx context.Context, cancelConn context.CancelFunc, conn *websocket.Conn, roomID string, auctionID string, userID string) {
	ticker := time.NewTicker(s.options.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, s.options.HeartbeatTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				observability.Inc("auction_ws_heartbeat_timeout_total", nil)
				_ = s.recordWSActivity(context.Background(), roomID, auctionID, userID, "ws_heartbeat_timeout", map[string]any{
					"interval_ms": int64(s.options.HeartbeatInterval / time.Millisecond),
					"timeout_ms":  int64(s.options.HeartbeatTimeout / time.Millisecond),
				})
				_ = conn.Close(websocket.StatusPolicyViolation, "heartbeat timeout")
				cancelConn()
				return
			}
			observability.Inc("auction_ws_heartbeat_ping_total", nil)
		case <-ctx.Done():
			return
		}
	}
}

func writeWS(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, typ websocket.MessageType, data []byte) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return conn.Write(ctx, typ, data)
}

func (s *Server) recoveryMessages(ctx context.Context, auctionID string, lastSeq int64) ([][]byte, string) {
	if lastSeq <= 0 {
		return s.snapshotMessage(ctx, auctionID)
	}
	redisStart := time.Now()
	values, err := s.redis.LRange(ctx, "auction:"+auctionID+":events", -s.options.RecoveryMaxEvents, -1).Result()
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
			observability.Observe("auction_ws_recovery_publications", float64(len(out)), map[string]string{"source": "history"}, []float64{1, 10, 50, 100, 300, 1000})
			return out, "history"
		}
	}
	return s.snapshotMessage(ctx, auctionID)
}

func (s *Server) snapshotMessage(ctx context.Context, auctionID string) ([][]byte, string) {
	redisStart := time.Now()
	payload, err := s.redis.Get(ctx, "auction:"+auctionID+":snapshot").Bytes()
	observability.Observe("redis_command_latency_seconds", time.Since(redisStart).Seconds(), map[string]string{"command": "get_snapshot"}, observability.DefaultLatencyBuckets)
	if err == nil && isSnapshotPayload(payload) {
		return [][]byte{payload}, snapshotSource(payload, "redis")
	}
	snapshot, err := s.snapshotGroup.Do(ctx, auctionID, func() ([]byte, error) {
		return s.rebuildSnapshotBounded(ctx, auctionID)
	})
	if err != nil {
		if errors.Is(err, errSnapshotRebuildSaturated) {
			_ = s.recordSnapshotSaturated(ctx, auctionID)
		}
		if stale := s.staleSnapshot(ctx, auctionID); len(stale) > 0 && isSnapshotPayload(stale) {
			return [][]byte{stale}, snapshotSource(stale, "redis")
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
			'stream_epoch', COALESCE(epoch.value, ''),
			'snapshot_version', a.version,
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
		LEFT JOIN LATERAL (
			SELECT value
			FROM realtime_stream_epochs
			WHERE auction_id = a.id
		) epoch ON true
		WHERE a.id = $1
	`, auctionID).Scan(&payload)
	if err != nil {
		return nil, err
	}
	_ = s.redis.Set(ctx, "auction:"+auctionID+":snapshot", payload, s.options.SnapshotTTL).Err()
	return payload, nil
}

func (s *Server) staleSnapshot(ctx context.Context, auctionID string) []byte {
	payload, err := s.redis.Get(ctx, "auction:"+auctionID+":snapshot").Bytes()
	if err != nil || len(payload) == 0 {
		return nil
	}
	if !isSnapshotPayload(payload) {
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

func ticketFromRequest(r *http.Request) string {
	if token := strings.TrimSpace(r.Header.Get("X-Auction-WS-Ticket")); token != "" {
		return token
	}
	return ticketFromProtocols(r.Header.Values("Sec-WebSocket-Protocol"))
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
	var staleEvent struct {
		Stale bool `json:"stale"`
	}
	_ = json.Unmarshal(payload, &staleEvent)
	if staleEvent.Stale {
		return fallback + "_stale"
	}
	if event.Source != "" {
		return event.Source
	}
	return fallback
}

func isSnapshotPayload(payload []byte) bool {
	var event struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return false
	}
	return event.EventType == "snapshot"
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

func (s *Server) recordWSActivityAsync(roomID string, auctionID string, userID string, eventType string, payload map[string]any) {
	if s.db == nil || s.activityQueue == nil {
		return
	}
	event := wsActivityEvent{
		roomID:    roomID,
		auctionID: auctionID,
		userID:    userID,
		eventType: eventType,
		payload:   payload,
	}
	select {
	case s.activityQueue <- event:
	default:
		observability.Inc("auction_ws_activity_dropped_total", map[string]string{"event_type": eventType})
	}
}

func (s *Server) runActivityWriter() {
	for event := range s.activityQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := s.recordWSActivity(ctx, event.roomID, event.auctionID, event.userID, event.eventType, event.payload)
		cancel()
		if err != nil {
			observability.Inc("auction_ws_activity_write_failed_total", map[string]string{"event_type": event.eventType})
		}
	}
}

func observeWSJoinStage(stage string, result string, started time.Time) {
	observability.Observe("auction_ws_join_stage_seconds", time.Since(started).Seconds(), map[string]string{
		"stage":  stage,
		"result": result,
	}, observability.DefaultLatencyBuckets)
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
