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
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"

	apptracing "live-auction/backend/internal/tracing"
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
	// Append writes a single decision to the durable ledger and returns the
	// ledger message metadata. Used for fallback/single-decision paths.
	Append(ctx context.Context, result engineResult) (LedgerMessage, error)

	// AppendBatch writes a batch of decisions in a single round-trip (group commit).
	// This is the primary path for the relay: one call amortises the fsync/ack cost
	// over all decisions in the batch, giving throughput proportional to batch size
	// instead of serialised 1-RTT-per-decision.
	AppendBatch(ctx context.Context, results []engineResult) ([]LedgerMessage, error)

	Fetch(ctx context.Context) (LedgerMessage, error)
	Commit(ctx context.Context, message LedgerMessage) error
	WriteDLQ(ctx context.Context, message LedgerMessage, err error) error
	Close() error
}

type BatchBidLedger interface {
	BidLedger
	FetchBatch(ctx context.Context, limit int) ([]LedgerMessage, error)
	CommitBatch(ctx context.Context, messages []LedgerMessage) error
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
	writer      *kafka.Writer
	dlq         *kafka.Writer
	reader      *kafka.Reader
	topic       string
	dlqTopic    string
	mu          sync.Mutex
	uncommitted []LedgerMessage
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
		Addr:     addr,
		Topic:    cfg.BidTopic,
		Balancer: &kafka.Hash{},
		// RequireAll (acks=-1) for durable commits; exactly-once via idempotent producer.
		RequiredAcks: kafka.RequireAll,
		// Async=false so WriteMessages blocks until acks=all, giving the relay
		// per-batch durability confirmation before advancing the cursor.
		Async: false,
		// Large batch ceiling: the relay passes the whole stream-read batch in one call.
		// kafka-go will split at broker-side limits automatically.
		BatchSize: 1000,
		// Low linger: relay loop fires every ~2ms anyway; no extra buffering needed.
		BatchTimeout:           5 * time.Millisecond,
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
	traceHeaders := map[string][]string{}
	apptracing.InjectHTTP(ctx, traceHeaders)
	for key, values := range traceHeaders {
		for _, value := range values {
			msg.Headers = append(msg.Headers, kafka.Header{Key: key, Value: []byte(value)})
		}
	}
	if err := l.writer.WriteMessages(ctx, msg); err != nil {
		return LedgerMessage{}, err
	}
	return LedgerMessage{ID: result.ledgerID(), Topic: l.topic, Partition: -1, Offset: -1, Key: key, Value: value}, nil
}

func (l *KafkaLedger) AppendBatch(ctx context.Context, results []engineResult) ([]LedgerMessage, error) {
	if l == nil || l.writer == nil {
		return nil, errors.New("kafka bid ledger is unavailable")
	}
	if len(results) == 0 {
		return nil, nil
	}
	msgs := make([]kafka.Message, 0, len(results))
	for _, result := range results {
		value, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		headers := []kafka.Header{
			{Key: "decision_id", Value: []byte(result.ledgerID())},
			{Key: "auction_id", Value: []byte(result.AuctionID)},
			{Key: "engine_epoch", Value: []byte(strconv.FormatInt(result.EngineEpoch, 10))},
			{Key: "engine_seq", Value: []byte(strconv.FormatInt(result.EngineSeq, 10))},
			{Key: "client_bid_id", Value: []byte(result.ClientBidID)},
			{Key: "request_hash", Value: []byte(result.RequestHash)},
			{Key: "trace_id", Value: []byte(result.TraceID)},
			{Key: "result", Value: []byte(result.Result)},
			{Key: "server_time_ms", Value: []byte(strconv.FormatInt(result.ServerTimeMS, 10))},
		}
		traceHeaders := map[string][]string{}
		apptracing.InjectHTTP(ctx, traceHeaders)
		for k, vals := range traceHeaders {
			for _, v := range vals {
				headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
			}
		}
		msgs = append(msgs, kafka.Message{
			Key:     []byte(result.AuctionID),
			Value:   value,
			Time:    time.UnixMilli(result.ServerTimeMS).UTC(),
			Headers: headers,
		})
	}
	// Single WriteMessages call — kafka-go batches all msgs in one broker round-trip
	// when RequiredAcks=all, Async=false. This is the group-commit path.
	if err := l.writer.WriteMessages(ctx, msgs...); err != nil {
		return nil, err
	}
	out := make([]LedgerMessage, len(results))
	for i, result := range results {
		out[i] = LedgerMessage{
			ID:        result.ledgerID(),
			Topic:     l.topic,
			Partition: -1,
			Offset:    -1,
			Key:       result.AuctionID,
			Value:     msgs[i].Value,
		}
	}
	return out, nil
}

func (l *KafkaLedger) Fetch(ctx context.Context) (LedgerMessage, error) {
	messages, err := l.FetchBatch(ctx, 1)
	if err != nil {
		return LedgerMessage{}, err
	}
	if len(messages) == 0 {
		return LedgerMessage{}, context.Canceled
	}
	return messages[0], nil
}

func (l *KafkaLedger) FetchBatch(ctx context.Context, limit int) ([]LedgerMessage, error) {
	if l == nil || l.reader == nil {
		return nil, errors.New("kafka bid ledger reader is unavailable")
	}
	if limit <= 0 {
		limit = 1
	}
	l.mu.Lock()
	if len(l.uncommitted) > 0 {
		messages := append([]LedgerMessage(nil), l.uncommitted...)
		l.mu.Unlock()
		if len(messages) > limit {
			messages = messages[:limit]
		}
		return messages, nil
	}
	l.mu.Unlock()

	msg, err := l.reader.FetchMessage(ctx)
	if err != nil {
		return nil, err
	}
	messages := []LedgerMessage{kafkaMessageToLedgerMessage(msg)}
	for len(messages) < limit {
		nextCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		next, err := l.reader.FetchMessage(nextCtx)
		cancel()
		if err != nil {
			break
		}
		messages = append(messages, kafkaMessageToLedgerMessage(next))
	}
	l.mu.Lock()
	l.uncommitted = append([]LedgerMessage(nil), messages...)
	l.mu.Unlock()
	return messages, nil
}

func (l *KafkaLedger) Commit(ctx context.Context, message LedgerMessage) error {
	return l.CommitBatch(ctx, []LedgerMessage{message})
}

func (l *KafkaLedger) CommitBatch(ctx context.Context, messages []LedgerMessage) error {
	if l == nil || l.reader == nil {
		return nil
	}
	if len(messages) == 0 {
		return nil
	}
	kafkaMessages := make([]kafka.Message, 0, len(messages))
	for _, message := range messages {
		kafkaMessages = append(kafkaMessages, kafka.Message{
			Topic:     message.Topic,
			Partition: message.Partition,
			Offset:    message.Offset,
		})
	}
	if err := l.reader.CommitMessages(ctx, kafkaMessages...); err != nil {
		return err
	}
	l.mu.Lock()
	if len(l.uncommitted) > 0 {
		committed := make(map[string]struct{}, len(messages))
		for _, message := range messages {
			committed[ledgerCommitKey(message)] = struct{}{}
		}
		remaining := l.uncommitted[:0]
		for _, message := range l.uncommitted {
			if _, ok := committed[ledgerCommitKey(message)]; !ok {
				remaining = append(remaining, message)
			}
		}
		l.uncommitted = remaining
	}
	l.mu.Unlock()
	return nil
}

func kafkaMessageToLedgerMessage(msg kafka.Message) LedgerMessage {
	return LedgerMessage{
		ID:        kafkaLedgerID(msg.Topic, msg.Partition, msg.Offset),
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Key:       string(msg.Key),
		Value:     msg.Value,
	}
}

func ledgerCommitKey(message LedgerMessage) string {
	return message.Topic + ":" + strconv.Itoa(message.Partition) + ":" + strconv.FormatInt(message.Offset, 10)
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
	mu        sync.Mutex
	messages  []LedgerMessage
	next      int
	dlq       []LedgerMessage
	partition int
}

var nextMemoryLedgerPartition = time.Now().UnixNano() % 1_000_000_000

func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{partition: int(atomic.AddInt64(&nextMemoryLedgerPartition, 1))}
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
		Partition: l.partition,
		Offset:    int64(len(l.messages)),
		Key:       result.AuctionID,
		Value:     value,
	}
	l.messages = append(l.messages, msg)
	return msg, nil
}

func (l *MemoryLedger) AppendBatch(_ context.Context, results []engineResult) ([]LedgerMessage, error) {
	out := make([]LedgerMessage, 0, len(results))
	for i := range results {
		value, err := json.Marshal(results[i])
		if err != nil {
			return out, err
		}
		l.mu.Lock()
		msg := LedgerMessage{
			ID:        results[i].ledgerID(),
			Topic:     defaultBidEventsTopic,
			Partition: l.partition,
			Offset:    int64(len(l.messages)),
			Key:       results[i].AuctionID,
			Value:     value,
		}
		l.messages = append(l.messages, msg)
		l.mu.Unlock()
		out = append(out, msg)
	}
	return out, nil
}

func (l *MemoryLedger) Fetch(ctx context.Context) (LedgerMessage, error) {
	messages, err := l.FetchBatch(ctx, 1)
	if err != nil {
		return LedgerMessage{}, err
	}
	if len(messages) == 0 {
		return LedgerMessage{}, context.Canceled
	}
	return messages[0], nil
}

func (l *MemoryLedger) FetchBatch(ctx context.Context, limit int) ([]LedgerMessage, error) {
	if limit <= 0 {
		limit = 1
	}
	for {
		l.mu.Lock()
		if l.next < len(l.messages) {
			end := l.next + limit
			if end > len(l.messages) {
				end = len(l.messages)
			}
			messages := append([]LedgerMessage(nil), l.messages[l.next:end]...)
			l.mu.Unlock()
			return messages, nil
		}
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (l *MemoryLedger) Commit(_ context.Context, _ LedgerMessage) error {
	return l.CommitBatch(context.Background(), []LedgerMessage{{}})
}

func (l *MemoryLedger) CommitBatch(_ context.Context, messages []LedgerMessage) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	advance := len(messages)
	if advance <= 0 {
		return nil
	}
	if l.next+advance > len(l.messages) {
		advance = len(l.messages) - l.next
	}
	if advance > 0 {
		l.next += advance
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
