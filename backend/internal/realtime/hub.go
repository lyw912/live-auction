package realtime

import (
	"context"
	"sync"

	"live-auction/backend/internal/observability"
)

const defaultAuctionQueueSize = 256
const defaultAuctionQueueBytes int64 = 1 << 20

type HubOptions struct {
	QueueMessages int
	QueueBytes    int64
}

type PublishStats struct {
	Subscribers   int
	Enqueued      int
	SlowClosed    int
	PayloadBytes  int
	MaxQueueDepth int
	MaxQueueBytes int64
}

type Hub struct {
	mu            sync.RWMutex
	rooms         map[string]map[*subscription]struct{}
	queueMessages int
	queueBytes    int64
}

type subscription struct {
	mu          sync.Mutex
	ch          chan queuedMessage
	closed      bool
	queuedBytes int64
	onSlow      func()
}

type queuedMessage struct {
	data []byte
	size int64
}

func NewHub(queueSize int) *Hub {
	return NewHubWithOptions(HubOptions{QueueMessages: queueSize})
}

func NewHubWithOptions(options HubOptions) *Hub {
	if options.QueueMessages <= 0 {
		options.QueueMessages = defaultAuctionQueueSize
	}
	if options.QueueBytes <= 0 {
		options.QueueBytes = defaultAuctionQueueBytes
	}
	return &Hub{
		rooms:         make(map[string]map[*subscription]struct{}),
		queueMessages: options.QueueMessages,
		queueBytes:    options.QueueBytes,
	}
}

func (h *Hub) Subscribe(auctionID string, onSlow func()) *subscription {
	sub := &subscription{
		ch:     make(chan queuedMessage, h.queueMessages),
		onSlow: onSlow,
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[auctionID] == nil {
		h.rooms[auctionID] = make(map[*subscription]struct{})
	}
	h.rooms[auctionID][sub] = struct{}{}
	return sub
}

func (h *Hub) Unsubscribe(auctionID string, sub *subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[auctionID], sub)
	if len(h.rooms[auctionID]) == 0 {
		delete(h.rooms, auctionID)
	}
	sub.close()
}

func (h *Hub) Publish(_ context.Context, auctionID string, payload []byte) PublishStats {
	h.mu.RLock()
	subs := make([]*subscription, 0, len(h.rooms[auctionID]))
	for sub := range h.rooms[auctionID] {
		subs = append(subs, sub)
	}
	h.mu.RUnlock()

	stats := PublishStats{
		Subscribers:  len(subs),
		PayloadBytes: len(payload),
	}
	for _, sub := range subs {
		message := append([]byte(nil), payload...)
		depth, bytes, ok := sub.trySend(message, h.queueBytes)
		if depth > stats.MaxQueueDepth {
			stats.MaxQueueDepth = depth
		}
		if bytes > stats.MaxQueueBytes {
			stats.MaxQueueBytes = bytes
		}
		if ok {
			stats.Enqueued++
		} else {
			stats.SlowClosed++
			sub.closeSlow()
		}
	}
	observability.Observe("auction_ws_publish_payload_bytes", float64(len(payload)), nil, []float64{128, 512, 1024, 4096, 16384, 65536})
	return stats
}

func (s *subscription) Messages() <-chan queuedMessage {
	return s.ch
}

func (s *subscription) Ack(message queuedMessage) {
	s.release(message)
}

func (s *subscription) release(message queuedMessage) {
	if message.size <= 0 {
		return
	}
	s.mu.Lock()
	s.queuedBytes -= message.size
	if s.queuedBytes < 0 {
		s.queuedBytes = 0
	}
	s.mu.Unlock()
}

func (s *subscription) trySend(message []byte, maxBytes int64) (int, int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return len(s.ch), s.queuedBytes, true
	}
	size := int64(len(message))
	if maxBytes > 0 && s.queuedBytes+size > maxBytes {
		return len(s.ch), s.queuedBytes, false
	}
	select {
	case s.ch <- queuedMessage{data: message, size: size}:
		s.queuedBytes += size
		return len(s.ch), s.queuedBytes, true
	default:
		return len(s.ch), s.queuedBytes, false
	}
}

func (s *subscription) closeSlow() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.ch)
	onSlow := s.onSlow
	s.mu.Unlock()
	if onSlow != nil {
		onSlow()
	}
}

func (s *subscription) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}
