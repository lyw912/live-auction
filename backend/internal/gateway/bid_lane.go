package gateway

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
)

const (
	bidEngineModePostgresLane = "postgres_lane"
	bidEngineModeRedisLedger  = "redis_ledger"
	bidLaneRetryAfterSeconds  = 1
)

type bidLaneFunc func(context.Context) (auction.BidResponse, error)

type bidLaneManager struct {
	cfg   config.Config
	db    *pgxpool.Pool
	lanes sync.Map
}

type bidLane struct {
	auctionID string
	tasks     chan *bidLaneTask
	depth     atomic.Int64
}

type bidLaneTask struct {
	ctx       context.Context
	queuedAt  time.Time
	expiresAt time.Time
	run       bidLaneFunc
	state     atomic.Int32
	started   chan struct{}
	done      chan bidLaneResult
}

type bidLaneResult struct {
	response auction.BidResponse
	err      error
}

func newBidLaneManager(cfg config.Config, db *pgxpool.Pool) *bidLaneManager {
	cfg = normalizeBidLaneConfig(cfg)
	return &bidLaneManager{cfg: cfg, db: db}
}

func normalizeBidLaneConfig(cfg config.Config) config.Config {
	if cfg.BidEngineMode == "" {
		cfg.BidEngineMode = bidEngineModeRedisLedger
	}
	if cfg.BidLaneWorkers <= 0 {
		cfg.BidLaneWorkers = 1
	}
	if cfg.BidLaneQueueSize <= 0 {
		cfg.BidLaneQueueSize = 128
	}
	if cfg.BidLaneQueueTimeout <= 0 {
		cfg.BidLaneQueueTimeout = 750 * time.Millisecond
	}
	return cfg
}

func (m *bidLaneManager) Execute(ctx context.Context, auctionID string, userID string, traceID string, fn bidLaneFunc) (auction.BidResponse, error) {
	return fn(ctx)
}

func (m *bidLaneManager) laneFor(auctionID string) *bidLane {
	if existing, ok := m.lanes.Load(auctionID); ok {
		return existing.(*bidLane)
	}
	created := &bidLane{
		auctionID: auctionID,
		tasks:     make(chan *bidLaneTask, m.cfg.BidLaneQueueSize),
	}
	actual, loaded := m.lanes.LoadOrStore(auctionID, created)
	lane := actual.(*bidLane)
	if !loaded {
		for i := 0; i < m.cfg.BidLaneWorkers; i++ {
			go lane.worker()
		}
	}
	return lane
}

func (l *bidLane) worker() {
	for task := range l.tasks {
		depth := l.depth.Add(-1)
		recordBidLaneDepth(l.auctionID, depth)
		wait := time.Since(task.queuedAt)
		outcome := "started"
		if task.ctx.Err() != nil || time.Now().After(task.expiresAt) {
			task.state.CompareAndSwap(0, 2)
			outcome = "expired"
			observability.Observe("auction_bid_queue_wait_seconds", wait.Seconds(), map[string]string{"auction_id": l.auctionID, "outcome": outcome}, observability.DefaultLatencyBuckets)
			continue
		}
		if !task.state.CompareAndSwap(0, 1) {
			outcome = "expired"
			observability.Observe("auction_bid_queue_wait_seconds", wait.Seconds(), map[string]string{"auction_id": l.auctionID, "outcome": outcome}, observability.DefaultLatencyBuckets)
			continue
		}
		observability.Observe("auction_bid_queue_wait_seconds", wait.Seconds(), map[string]string{"auction_id": l.auctionID, "outcome": outcome}, observability.DefaultLatencyBuckets)
		close(task.started)
		response, err := task.run(task.ctx)
		select {
		case task.done <- bidLaneResult{response: response, err: err}:
		default:
		}
	}
}

func (m *bidLaneManager) recordLaneReject(ctx context.Context, auctionID string, userID string, traceID string, code apierrors.Code, reason string, retryAfter time.Duration) error {
	return nil
}

func recordBidLaneDepth(auctionID string, depth int64) {
	observability.Set("auction_bid_queue_depth", float64(depth), map[string]string{"auction_id": auctionID})
}

func recordBidLaneReject(code apierrors.Code) {
	observability.Inc("auction_bid_queue_rejected_total", map[string]string{"reason": string(code)})
}

func bidLaneRetryAfter() time.Duration {
	return time.Duration(bidLaneRetryAfterSeconds) * time.Second
}
