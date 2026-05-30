package redisengine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/redisx"
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

func TestKafkaWorkerPumpsRedisPendingAfterInitialEmptyKafkaPoll(t *testing.T) {
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
		ClientID:               "redisengine-worker-test-" + suffix,
		AllowAutoTopicCreation: true,
	})
	if err != nil {
		t.Fatalf("NewKafkaLedger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := openTestDB(t)
	rdb := openStreamsRedis(t)
	auctionID := createEngineAuction(t, db, 20_000)
	worker := NewWorker(db, rdb, ledger, "test-"+uuid.NewString())
	engine := New(db, rdb, ledger)

	if processed, err := worker.ProcessKafka(ctx, 1); err != nil || processed != 0 {
		t.Fatalf("initial empty kafka poll processed=%d err=%v", processed, err)
	}
	response, err := engine.PlaceBid(ctx, auctionID, "user_1", "kafka-pump-"+suffix, auction.BidInput{
		ClientBidID:   "kafka-pump-" + suffix,
		AmountCents:   15_000,
		ClientSeenSeq: 0,
	}, "tr_"+suffix)
	if err != nil {
		t.Fatalf("place bid: %v", err)
	}
	if response.EngineSeq != 1 {
		t.Fatalf("engine seq = %d, want 1", response.EngineSeq)
	}
	if indexed, err := rdb.SIsMember(ctx, redisx.BidEnginePendingAuctionsKey(), auctionID).Result(); err != nil || !indexed {
		t.Fatalf("pending index member=%v err=%v", indexed, err)
	}

	appended, err := worker.ProcessPendingAppends(ctx, 100)
	if err != nil {
		t.Fatalf("ProcessPendingAppends: %v", err)
	}
	if appended != 1 {
		t.Fatalf("pending appended=%d, want 1", appended)
	}
	if pending, err := rdb.HLen(ctx, redisx.BidEnginePendingKey(auctionID)).Result(); err != nil || pending != 0 {
		t.Fatalf("pending hlen=%d err=%v, want 0", pending, err)
	}

	settled, err := worker.ProcessKafka(ctx, 1)
	if err != nil {
		t.Fatalf("ProcessKafka settle: %v", err)
	}
	if settled != 1 {
		t.Fatalf("settled=%d, want 1", settled)
	}
	assertAuctionEngineSeq(t, db, auctionID, 1, 15_000, "ACTIVE")
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
