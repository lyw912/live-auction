package redisengine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestKafkaLedgerRedpandaIntegration(t *testing.T) {
	if os.Getenv("KAFKA_INTEGRATION") != "1" {
		t.Skip("set KAFKA_INTEGRATION=1 to run against local Redpanda/Kafka")
	}
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	suffix := uuid.NewString()
	ledger, err := NewKafkaLedger(KafkaLedgerConfig{
		Brokers:       parseKafkaBrokers(brokers),
		BidTopic:      "auction.bid-events.test." + suffix,
		DLQTopic:      "auction.dlq.test." + suffix,
		ConsumerGroup: "settlement-workers-test-" + suffix,
		ClientID:      "redisengine-test-" + suffix,
	})
	if err != nil {
		t.Fatalf("NewKafkaLedger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result := engineResult{
		Result:            resultAccepted,
		BidID:             "bid_" + suffix,
		AuctionID:         "auc_" + suffix,
		UserID:            "user_1",
		ClientBidID:       "client_" + suffix,
		AmountCents:       15_000,
		EngineSeq:         1,
		EngineEpoch:       1,
		SettlementStatus:  "PENDING",
		CurrentPriceCents: 15_000,
		CurrentWinnerID:   "user_1",
		EndAtMS:           time.Now().UTC().Add(time.Minute).UnixMilli(),
		ServerTimeMS:      time.Now().UTC().UnixMilli(),
		TraceID:           "tr_" + suffix,
		RequestHash:       requestHash("auc_"+suffix, "user_1", "client_"+suffix, 15_000),
	}
	if _, err := ledger.Append(ctx, result); err != nil {
		t.Fatalf("Append: %v", err)
	}
	msg, err := ledger.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if msg.Key != result.AuctionID || msg.ID == "" || msg.Topic == "" {
		t.Fatalf("message = %#v", msg)
	}
	if err := ledger.WriteDLQ(ctx, msg, context.Canceled); err != nil {
		t.Fatalf("WriteDLQ: %v", err)
	}
	if err := ledger.Commit(ctx, msg); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}
