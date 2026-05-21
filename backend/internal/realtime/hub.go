package realtime

import (
	"context"
	"sync"
)

const defaultAuctionQueueSize = 256

type Hub struct {
	mu        sync.RWMutex
	rooms     map[string]map[*subscription]struct{}
	queueSize int
}

type subscription struct {
	mu     sync.Mutex
	ch     chan []byte
	closed bool
	onSlow func()
}

func NewHub(queueSize int) *Hub {
	if queueSize <= 0 {
		queueSize = defaultAuctionQueueSize
	}
	return &Hub{
		rooms:     make(map[string]map[*subscription]struct{}),
		queueSize: queueSize,
	}
}

func (h *Hub) Subscribe(auctionID string, onSlow func()) *subscription {
	sub := &subscription{
		ch:     make(chan []byte, h.queueSize),
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

func (h *Hub) Publish(_ context.Context, auctionID string, payload []byte) {
	h.mu.RLock()
	subs := make([]*subscription, 0, len(h.rooms[auctionID]))
	for sub := range h.rooms[auctionID] {
		subs = append(subs, sub)
	}
	h.mu.RUnlock()

	for _, sub := range subs {
		message := append([]byte(nil), payload...)
		if !sub.trySend(message) {
			sub.closeSlow()
		}
	}
}

func (s *subscription) Messages() <-chan []byte {
	return s.ch
}

func (s *subscription) trySend(message []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return true
	}
	select {
	case s.ch <- message:
		return true
	default:
		return false
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
