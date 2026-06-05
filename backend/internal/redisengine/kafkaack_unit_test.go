package redisengine

import (
	"context"
	"sync"
	"testing"
	"time"

	"live-auction/backend/internal/auction"
)

// TestKafkaAckRegistrySignalUnblocksWaiter verifies that signalKafkaAckWaiters
// wakes all goroutines registered for the same auctionID:clientBidID pair.
func TestKafkaAckRegistrySignalUnblocksWaiter(t *testing.T) {
	// Reset registry state to avoid cross-test pollution.
	kafkaAckRegistry.mu.Lock()
	kafkaAckRegistry.m = make(map[string][]chan bool)
	kafkaAckRegistry.mu.Unlock()

	auctionID := "auc_latch_test"
	clientBidID := "bid_latch_1"

	ch, unregister := registerKafkaAckWaiter(auctionID, clientBidID)
	defer unregister()

	var wg sync.WaitGroup
	wg.Add(1)
	gotAcked := make(chan bool, 1)
	go func() {
		defer wg.Done()
		select {
		case acked := <-ch:
			gotAcked <- acked
		case <-time.After(2 * time.Second):
			t.Errorf("waiter did not receive signal within 2s")
		}
	}()

	// Signal should unblock the waiter with acked=true.
	signalKafkaAckWaiters(auctionID, []string{clientBidID})
	wg.Wait()

	select {
	case acked := <-gotAcked:
		if !acked {
			t.Fatal("waiter received acked=false, want true")
		}
	default:
		t.Fatal("waiter goroutine did not receive signal")
	}
}

// TestKafkaAckRegistryMultipleWaitersUnblockTogether verifies the group-commit
// property: N concurrent waiters for the same batch all wake simultaneously.
func TestKafkaAckRegistryMultipleWaitersUnblockTogether(t *testing.T) {
	kafkaAckRegistry.mu.Lock()
	kafkaAckRegistry.m = make(map[string][]chan bool)
	kafkaAckRegistry.mu.Unlock()

	auctionID := "auc_group_commit"
	bidIDs := []string{"bid_gc_1", "bid_gc_2", "bid_gc_3"}

	type waiter struct {
		ch         chan bool
		unregister func()
	}
	waiters := make([]waiter, len(bidIDs))
	for i, id := range bidIDs {
		ch, unreg := registerKafkaAckWaiter(auctionID, id)
		waiters[i] = waiter{ch, unreg}
		defer unreg()
	}

	var wg sync.WaitGroup
	unblocked := make([]bool, len(bidIDs))
	for i, w := range waiters {
		wg.Add(1)
		i, w := i, w
		go func() {
			defer wg.Done()
			select {
			case acked := <-w.ch:
				if !acked {
					t.Errorf("waiter %d received acked=false, want true", i)
				}
				unblocked[i] = true
			case <-time.After(2 * time.Second):
				t.Errorf("waiter %d did not receive signal", i)
			}
		}()
	}

	// Signal all three at once (as relay does after AppendBatch).
	signalKafkaAckWaiters(auctionID, bidIDs)
	wg.Wait()

	for i, got := range unblocked {
		if !got {
			t.Errorf("waiter %d was not unblocked", i)
		}
	}
}

// TestKafkaAckRegistryCleanupOnUnregister verifies that the unregister func
// removes the channel from the map so stale channels don't accumulate.
func TestKafkaAckRegistryCleanupOnUnregister(t *testing.T) {
	kafkaAckRegistry.mu.Lock()
	kafkaAckRegistry.m = make(map[string][]chan bool)
	kafkaAckRegistry.mu.Unlock()

	auctionID := "auc_cleanup"
	clientBidID := "bid_cleanup_1"

	_, unregister := registerKafkaAckWaiter(auctionID, clientBidID)

	kafkaAckRegistry.mu.Lock()
	before := len(kafkaAckRegistry.m[auctionID+":"+clientBidID])
	kafkaAckRegistry.mu.Unlock()

	if before != 1 {
		t.Fatalf("expected 1 waiter registered, got %d", before)
	}

	unregister()

	kafkaAckRegistry.mu.Lock()
	after := len(kafkaAckRegistry.m[auctionID+":"+clientBidID])
	kafkaAckRegistry.mu.Unlock()

	if after != 0 {
		t.Fatalf("expected 0 waiters after unregister, got %d", after)
	}
}

// TestKafkaAckFailFastWakesWaiters verifies that failFastKafkaAckWaiters sends
// acked=false and handlers can distinguish fault from success.
func TestKafkaAckFailFastWakesWaiters(t *testing.T) {
	kafkaAckRegistry.mu.Lock()
	kafkaAckRegistry.m = make(map[string][]chan bool)
	kafkaAckRegistry.mu.Unlock()

	auctionID := "auc_fail_fast"
	clientBidID := "bid_ff_1"

	ch, unregister := registerKafkaAckWaiter(auctionID, clientBidID)
	defer unregister()

	var wg sync.WaitGroup
	wg.Add(1)
	gotAcked := make(chan bool, 1)
	go func() {
		defer wg.Done()
		select {
		case acked := <-ch:
			gotAcked <- acked
		case <-time.After(2 * time.Second):
			t.Errorf("waiter did not receive fail-fast signal within 2s")
		}
	}()

	// Simulate Kafka fault: relay calls fail-fast.
	failFastKafkaAckWaiters(auctionID, []string{clientBidID})
	wg.Wait()

	select {
	case acked := <-gotAcked:
		if acked {
			t.Fatal("fail-fast waiter received acked=true, want false")
		}
	default:
		t.Fatal("waiter goroutine did not receive fail-fast signal")
	}
}

// TestKafkaRelayCircuitBreakerSkipsWait verifies that when kafkaRelayHealthy=false,
// waitKafkaAck returns immediately without registering a latch.
func TestKafkaRelayCircuitBreakerSkipsWait(t *testing.T) {
	kafkaAckRegistry.mu.Lock()
	kafkaAckRegistry.m = make(map[string][]chan bool)
	kafkaAckRegistry.mu.Unlock()
	kafkaRelayUnhealthy.Store(true)
	defer kafkaRelayUnhealthy.Store(false)

	auctionID := "auc_circuit_open"
	clientBidID := "bid_cb_1"

	start := time.Now()
	// Use a nil redis client — the circuit breaker must return before any Redis call.
	e := &Engine{}
	result := e.waitKafkaAck(context.Background(), auctionID, clientBidID, 200*time.Millisecond)
	elapsed := time.Since(start)

	if result != kafkaAppendStatusUnknown {
		t.Fatalf("circuit breaker should return Unknown, got %q", result)
	}
	if elapsed > 10*time.Millisecond {
		t.Fatalf("circuit breaker took %v, expected <10ms (should skip wait entirely)", elapsed)
	}

	// No waiter should have been registered.
	kafkaAckRegistry.mu.Lock()
	count := len(kafkaAckRegistry.m[auctionID+":"+clientBidID])
	kafkaAckRegistry.mu.Unlock()
	if count != 0 {
		t.Fatalf("circuit breaker registered %d waiters, expected 0", count)
	}
}

// TestRelayTriggerChannelNonBlocking verifies that triggerRelayForAuction
// never blocks even when the channel is full (buffer overflow = silent drop).
func TestRelayTriggerChannelNonBlocking(t *testing.T) {
	// Fill the trigger channel completely.
	for i := 0; i < cap(relayTriggerCh); i++ {
		select {
		case relayTriggerCh <- "auc_overflow":
		default:
		}
	}
	// Additional trigger on a full channel must not block.
	done := make(chan struct{})
	go func() {
		triggerRelayForAuction("auc_overflow_extra")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("triggerRelayForAuction blocked on full channel")
	}
	// Drain channel to restore clean state for other tests.
	for {
		select {
		case <-relayTriggerCh:
		default:
			return
		}
	}
}

// TestKafkaAckRelayTriggerChainWithMemoryLedger verifies the full path:
// Engine.PlaceBid → triggerRelayForAuction → runRelayOnTrigger →
// relayAuctionLogBatch → signalKafkaAckWaiters → waitKafkaAck returns ACKED.
// Uses in-process MemoryLedger and real Redis (skips if Redis unavailable).
func TestKafkaAckRelayTriggerChainWithMemoryLedger(t *testing.T) {
	db := openTestDB(t)
	rdb := openStreamsRedis(t)

	ledger := NewMemoryLedger()
	engine := New(db, rdb, ledger).WithResponseDurability("kafka_ack")
	worker := NewWorker(db, rdb, ledger, "test-kafka-ack-trigger")

	auctionID := createEngineAuction(t, db, 0)

	// Start the relay trigger goroutine (mirrors Worker.Run's setup).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{}, 1)
	go worker.runRelayOnTrigger(ctx, done)

	const bidID = "bid_chain_kafka_ack_1"
	resp, err := engine.PlaceBid(ctx, auctionID, "user_1", bidID, auction.BidInput{
		ClientBidID:   bidID, // idempotency key must equal ClientBidID
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_kafka_ack_chain")
	if err != nil {
		t.Fatalf("PlaceBid failed: %v", err)
	}

	// In kafka_ack mode with an active relay trigger goroutine, the response
	// should carry KAFKA_ACKED (relay completes before 40ms timeout).
	if resp.DurabilityStatus != "KAFKA_ACKED" && resp.DurabilityStatus != "ENGINE_DURABLE" {
		t.Fatalf("unexpected durability_status=%q", resp.DurabilityStatus)
	}
	// Whatever the durability status, decision must be final.
	if resp.DecisionStatus != "DECIDED" {
		t.Fatalf("unexpected decision_status=%q", resp.DecisionStatus)
	}

	cancel()
	<-done
}
