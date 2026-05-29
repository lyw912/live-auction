package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
)

func TestBidLaneQueueFullReturnsRetryableTooHot(t *testing.T) {
	observability.Default = observability.NewRegistry()
	cfg := config.Config{
		BidEngineMode:       bidEngineModePostgresLane,
		BidLaneWorkers:      1,
		BidLaneQueueSize:    1,
		BidLaneQueueTimeout: time.Second,
	}
	manager := &bidLaneManager{cfg: normalizeBidLaneConfig(cfg)}
	lane := &bidLane{auctionID: "auc_lane_full", tasks: make(chan bidLaneTask, 1)}
	manager.lanes.Store("auc_lane_full", lane)
	lane.tasks <- bidLaneTask{
		ctx:       context.Background(),
		queuedAt:  time.Now(),
		expiresAt: time.Now().Add(time.Second),
		started:   make(chan struct{}),
		done:      make(chan bidLaneResult, 1),
		run: func(context.Context) (auction.BidResponse, error) {
			return auction.BidResponse{}, nil
		},
	}
	lane.depth.Add(1)

	_, err := manager.Execute(context.Background(), "auc_lane_full", "user_1", "tr_lane_full", func(context.Context) (auction.BidResponse, error) {
		t.Fatal("full queue must not run bid function")
		return auction.BidResponse{}, nil
	})
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierrors.CodeBidAuctionTooHot || apiErr.Status != 429 {
		t.Fatalf("err = %#v, want BID_AUCTION_TOO_HOT/429", err)
	}
	if apiErr.Details["retry_after_ms"] == nil {
		t.Fatalf("missing retry_after_ms in %#v", apiErr.Details)
	}
	metrics := string(observability.Default.Render(context.Background()))
	if !strings.Contains(metrics, `auction_bid_queue_rejected_total{reason="BID_AUCTION_TOO_HOT"} 1`) {
		t.Fatalf("queue reject metric missing in:\n%s", metrics)
	}
}

func TestBidLaneWaitTimeoutReturnsRetryLaterBeforeExecution(t *testing.T) {
	observability.Default = observability.NewRegistry()
	cfg := config.Config{
		BidEngineMode:       bidEngineModePostgresLane,
		BidLaneWorkers:      1,
		BidLaneQueueSize:    4,
		BidLaneQueueTimeout: 20 * time.Millisecond,
	}
	manager := newBidLaneManager(cfg, nil)
	block := make(chan struct{})
	firstStarted := make(chan struct{})
	go func() {
		_, _ = manager.Execute(context.Background(), "auc_lane_timeout", "user_1", "tr_lane_first", func(context.Context) (auction.BidResponse, error) {
			close(firstStarted)
			<-block
			return auction.BidResponse{Result: auction.BidResultAccepted, AuctionID: "auc_lane_timeout"}, nil
		})
	}()
	<-firstStarted
	defer close(block)

	_, err := manager.Execute(context.Background(), "auc_lane_timeout", "user_2", "tr_lane_timeout", func(context.Context) (auction.BidResponse, error) {
		t.Fatal("timed out queued request must not run after caller received retry")
		return auction.BidResponse{}, nil
	})
	var apiErr apierrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierrors.CodeBidRetryLater || apiErr.Status != 409 {
		t.Fatalf("err = %#v, want BID_RETRY_LATER/409", err)
	}
	metrics := string(observability.Default.Render(context.Background()))
	for _, want := range []string{
		`auction_bid_queue_rejected_total{reason="BID_RETRY_LATER"} 1`,
		`auction_bid_queue_depth{auction_id="auc_lane_timeout"}`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics missing %q in:\n%s", want, metrics)
		}
	}
}

func TestBidLaneStartedRequestReturnsDBTruthPastWaitBudget(t *testing.T) {
	cfg := config.Config{
		BidEngineMode:       bidEngineModePostgresLane,
		BidLaneWorkers:      1,
		BidLaneQueueSize:    1,
		BidLaneQueueTimeout: time.Second,
	}
	manager := newBidLaneManager(cfg, nil)
	resp, err := manager.Execute(context.Background(), "auc_lane_started", "user_1", "tr_lane_started", func(context.Context) (auction.BidResponse, error) {
		time.Sleep(10 * time.Millisecond)
		return auction.BidResponse{Result: auction.BidResultAccepted, AuctionID: "auc_lane_started", Seq: 7}, nil
	})
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}
	if resp.Seq != 7 || resp.Result != auction.BidResultAccepted {
		t.Fatalf("response = %#v, want DB truth", resp)
	}
}

func TestBidLaneNonPostgresModeBypassesQueue(t *testing.T) {
	cfg := config.Config{
		BidEngineMode:       "redis_guard",
		BidLaneWorkers:      1,
		BidLaneQueueSize:    1,
		BidLaneQueueTimeout: time.Hour,
	}
	manager := newBidLaneManager(cfg, nil)
	called := false
	_, err := manager.Execute(context.Background(), "auc_lane_bypass", "user_1", "tr_lane_bypass", func(context.Context) (auction.BidResponse, error) {
		called = true
		return auction.BidResponse{Result: auction.BidResultAccepted}, nil
	})
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}
	if !called {
		t.Fatal("non-postgres mode did not call bid function")
	}
}
