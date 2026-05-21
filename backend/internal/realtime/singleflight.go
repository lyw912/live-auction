package realtime

import (
	"context"
	"sync"
)

type snapshotGroup struct {
	mu       sync.Mutex
	inflight map[string]*snapshotCall
}

type snapshotCall struct {
	done chan struct{}
	data []byte
	err  error
}

func newSnapshotGroup() *snapshotGroup {
	return &snapshotGroup{inflight: make(map[string]*snapshotCall)}
}

func (g *snapshotGroup) Do(ctx context.Context, key string, fn func() ([]byte, error)) ([]byte, error) {
	g.mu.Lock()
	if call, ok := g.inflight[key]; ok {
		g.mu.Unlock()
		select {
		case <-call.done:
			return call.data, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &snapshotCall{done: make(chan struct{})}
	g.inflight[key] = call
	g.mu.Unlock()

	call.data, call.err = fn()
	close(call.done)

	g.mu.Lock()
	delete(g.inflight, key)
	g.mu.Unlock()
	return call.data, call.err
}
