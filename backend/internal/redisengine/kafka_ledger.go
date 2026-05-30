package redisengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	defaultBidEventsTopic = "auction.bid-events"
	defaultBidDLQTopic    = "auction.dlq"
	defaultConsumerGroup  = "settlement-workers"
)

type LedgerMessage struct {
	ID        string
	Topic     string
	Partition int
	Offset    int64
	Key       string
	Value     []byte
}

type BidLedger interface {
	Append(ctx context.Context, result engineResult) (LedgerMessage, error)
	Fetch(ctx context.Context) (LedgerMessage, error)
	Commit(ctx context.Context, message LedgerMessage) error
	WriteDLQ(ctx context.Context, message LedgerMessage, err error) error
	Close() error
}

type KafkaLedgerConfig struct {
	Brokers                []string
	BidTopic               string
	DLQTopic               string
	ConsumerGroup          string
	ClientID               string
	AllowAutoTopicCreation bool
}

type KafkaLedger struct {
	writer   *kafka.Writer
	dlq      *kafka.Writer
	reader   *kafka.Reader
	topic    string
	dlqTopic string
}

func NewKafkaLedger(cfg KafkaLedgerConfig) (*KafkaLedger, error) {
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	if cfg.BidTopic == "" {
		cfg.BidTopic = defaultBidEventsTopic
	}
	if cfg.DLQTopic == "" {
		cfg.DLQTopic = defaultBidDLQTopic
	}
	if cfg.ConsumerGroup == "" {
		cfg.ConsumerGroup = defaultConsumerGroup
	}
	if cfg.ClientID == "" {
		cfg.ClientID = "live-auction"
	}
	addr := kafka.TCP(cfg.Brokers...)
	writer := &kafka.Writer{
		Addr:                   addr,
		Topic:                  cfg.BidTopic,
		Balancer:               &kafka.Hash{},
		RequiredAcks:           kafka.RequireAll,
		Async:                  false,
		BatchSize:              1,
		BatchTimeout:           10 * time.Millisecond,
		WriteTimeout:           10 * time.Second,
		ReadTimeout:            3 * time.Second,
		MaxAttempts:            10,
		AllowAutoTopicCreation: cfg.AllowAutoTopicCreation,
	}
	dlq := &kafka.Writer{
		Addr:                   addr,
		Topic:                  cfg.DLQTopic,
		Balancer:               &kafka.Hash{},
		RequiredAcks:           kafka.RequireAll,
		Async:                  false,
		BatchSize:              1,
		BatchTimeout:           10 * time.Millisecond,
		WriteTimeout:           10 * time.Second,
		ReadTimeout:            3 * time.Second,
		MaxAttempts:            10,
		AllowAutoTopicCreation: cfg.AllowAutoTopicCreation,
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  cfg.Brokers,
		Topic:    cfg.BidTopic,
		GroupID:  cfg.ConsumerGroup,
		MinBytes: 1,
		MaxBytes: 1 << 20,
		MaxWait:  250 * time.Millisecond,
		Dialer: &kafka.Dialer{
			Timeout:  3 * time.Second,
			ClientID: cfg.ClientID,
		},
	})
	return &KafkaLedger{writer: writer, dlq: dlq, reader: reader, topic: cfg.BidTopic, dlqTopic: cfg.DLQTopic}, nil
}

func (l *KafkaLedger) Append(ctx context.Context, result engineResult) (LedgerMessage, error) {
	if l == nil || l.writer == nil {
		return LedgerMessage{}, errors.New("kafka bid ledger is unavailable")
	}
	value, err := json.Marshal(result)
	if err != nil {
		return LedgerMessage{}, err
	}
	key := result.AuctionID
	msg := kafka.Message{
		Key:   []byte(key),
		Value: value,
		Time:  time.UnixMilli(result.ServerTimeMS).UTC(),
		Headers: []kafka.Header{
			{Key: "decision_id", Value: []byte(result.ledgerID())},
			{Key: "auction_id", Value: []byte(result.AuctionID)},
			{Key: "engine_epoch", Value: []byte(strconv.FormatInt(result.EngineEpoch, 10))},
			{Key: "engine_seq", Value: []byte(strconv.FormatInt(result.EngineSeq, 10))},
			{Key: "client_bid_id", Value: []byte(result.ClientBidID)},
			{Key: "request_hash", Value: []byte(result.RequestHash)},
			{Key: "trace_id", Value: []byte(result.TraceID)},
			{Key: "result", Value: []byte(result.Result)},
			{Key: "server_time_ms", Value: []byte(strconv.FormatInt(result.ServerTimeMS, 10))},
		},
	}
	if err := l.writer.WriteMessages(ctx, msg); err != nil {
		return LedgerMessage{}, err
	}
	return LedgerMessage{ID: result.ledgerID(), Topic: l.topic, Partition: -1, Offset: -1, Key: key, Value: value}, nil
}

func (l *KafkaLedger) Fetch(ctx context.Context) (LedgerMessage, error) {
	if l == nil || l.reader == nil {
		return LedgerMessage{}, errors.New("kafka bid ledger reader is unavailable")
	}
	msg, err := l.reader.FetchMessage(ctx)
	if err != nil {
		return LedgerMessage{}, err
	}
	return LedgerMessage{
		ID:        kafkaLedgerID(msg.Topic, msg.Partition, msg.Offset),
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Key:       string(msg.Key),
		Value:     msg.Value,
	}, nil
}

func (l *KafkaLedger) Commit(ctx context.Context, message LedgerMessage) error {
	if l == nil || l.reader == nil {
		return nil
	}
	return l.reader.CommitMessages(ctx, kafka.Message{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
	})
}

func (l *KafkaLedger) WriteDLQ(ctx context.Context, message LedgerMessage, eventErr error) error {
	if l == nil || l.dlq == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"source_topic":     message.Topic,
		"source_partition": message.Partition,
		"source_offset":    message.Offset,
		"source_key":       message.Key,
		"error":            eventErr.Error(),
		"payload":          json.RawMessage(message.Value),
		"created_at":       time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	return l.dlq.WriteMessages(ctx, kafka.Message{
		Key:   []byte(message.Key),
		Value: payload,
		Time:  time.Now().UTC(),
	})
}

func (l *KafkaLedger) Close() error {
	if l == nil {
		return nil
	}
	var errs []error
	if l.reader != nil {
		errs = append(errs, l.reader.Close())
	}
	if l.writer != nil {
		errs = append(errs, l.writer.Close())
	}
	if l.dlq != nil {
		errs = append(errs, l.dlq.Close())
	}
	return errors.Join(errs...)
}

func ledgerDLQTopic(ledger BidLedger) string {
	if kafkaLedger, ok := ledger.(*KafkaLedger); ok && kafkaLedger.dlqTopic != "" {
		return kafkaLedger.dlqTopic
	}
	return defaultBidDLQTopic
}

type MemoryLedger struct {
	mu       sync.Mutex
	messages []LedgerMessage
	next     int
	dlq      []LedgerMessage
}

func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{}
}

func (l *MemoryLedger) Append(_ context.Context, result engineResult) (LedgerMessage, error) {
	value, err := json.Marshal(result)
	if err != nil {
		return LedgerMessage{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	msg := LedgerMessage{
		ID:        result.ledgerID(),
		Topic:     defaultBidEventsTopic,
		Partition: 0,
		Offset:    int64(len(l.messages)),
		Key:       result.AuctionID,
		Value:     value,
	}
	l.messages = append(l.messages, msg)
	return msg, nil
}

func (l *MemoryLedger) Fetch(ctx context.Context) (LedgerMessage, error) {
	for {
		l.mu.Lock()
		if l.next < len(l.messages) {
			msg := l.messages[l.next]
			l.mu.Unlock()
			return msg, nil
		}
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return LedgerMessage{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (l *MemoryLedger) Commit(_ context.Context, _ LedgerMessage) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.next < len(l.messages) {
		l.next++
	}
	return nil
}

func (l *MemoryLedger) WriteDLQ(_ context.Context, message LedgerMessage, _ error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dlq = append(l.dlq, message)
	return nil
}

func (l *MemoryLedger) Close() error { return nil }

func (l *MemoryLedger) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.messages)
}

func (l *MemoryLedger) DLQLen() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.dlq)
}

func (l *MemoryLedger) Message(index int) (LedgerMessage, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if index < 0 || index >= len(l.messages) {
		return LedgerMessage{}, false
	}
	return l.messages[index], true
}

func kafkaLedgerID(topic string, partition int, offset int64) string {
	return fmt.Sprintf("kafka:%s:%d:%d", topic, partition, offset)
}

func (r engineResult) ledgerID() string {
	return fmt.Sprintf("engine:%s:%d:%d", r.AuctionID, r.EngineEpoch, r.EngineSeq)
}

func parseKafkaBrokers(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(trimmed); err != nil {
			out = append(out, trimmed+":9092")
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func NewKafkaLedgerFromEnv(brokers string, bidTopic string, dlqTopic string, consumerGroup string, clientID string) (*KafkaLedger, error) {
	return NewKafkaLedger(KafkaLedgerConfig{
		Brokers:       parseKafkaBrokers(brokers),
		BidTopic:      bidTopic,
		DLQTopic:      dlqTopic,
		ConsumerGroup: consumerGroup,
		ClientID:      clientID,
	})
}
