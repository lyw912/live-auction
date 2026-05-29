package redisengine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

func TestKafkaLedgerIntegration(t *testing.T) {
	if os.Getenv("KAFKA_INTEGRATION") != "1" {
		t.Skip("set KAFKA_INTEGRATION=1 to run against local Kafka")
	}
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	suffix := uuid.NewString()
	bidTopic := "auction.bid-events.test." + suffix
	dlqTopic := "auction.dlq.test." + suffix
	ensureKafkaTopic(t, brokers, bidTopic)
	ensureKafkaTopic(t, brokers, dlqTopic)
	ledger, err := NewKafkaLedger(KafkaLedgerConfig{
		Brokers:                parseKafkaBrokers(brokers),
		BidTopic:               bidTopic,
		DLQTopic:               dlqTopic,
		ConsumerGroup:          "settlement-workers-test-" + suffix,
		ClientID:               "redisengine-test-" + suffix,
		AllowAutoTopicCreation: true,
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

func ensureKafkaTopic(t *testing.T, brokers string, topic string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(ctx, "tcp", parseKafkaBrokers(brokers)[0])
	if err != nil {
		t.Fatalf("dial kafka broker: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}); err != nil && !isKafkaTopicAlreadyExists(err) {
		t.Fatalf("create kafka topic %s: %v", topic, err)
	}
}

func isKafkaTopicAlreadyExists(err error) bool {
	return err != nil && (err == kafka.TopicAlreadyExists || err.Error() == kafka.TopicAlreadyExists.Error())
}
