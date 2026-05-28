package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int
	KafkaBrokers  string
	KafkaBidTopic string
	KafkaDLQTopic string

	MinIOEndpoint  string
	MinIORootUser  string
	MinIORootPass  string
	S3Bucket       string
	S3UseSSL       bool
	MockHostUserID string
	MockUserID     string
	AllowMockAuth  bool
	SessionTTL     time.Duration

	BidUserLimitPerSecond     int
	BidIPLimitPerSecond       int
	BidAuctionLimitPerSecond  int
	BidAuctionMaxInFlight     int
	BidLimitWindow            time.Duration
	BidLimitRedisTimeout      time.Duration
	BidEngineMode             string
	BidLaneWorkers            int
	BidLaneQueueSize          int
	BidLaneQueueTimeout       time.Duration
	BidRedisGuardMaxStaleness time.Duration
	BidRedisGuardTimeout      time.Duration
	FakePaymentWebhookSecret  string

	AdmissionEnabled       bool
	WSTicketMaxInFlight    int
	WSConnectMaxInFlight   int
	WSRetryAfter           time.Duration
	WSQueueMessages        int
	WSQueueBytes           int64
	WSRecoveryMaxEvents    int64
	WSSnapshotRebuildMax   int
	WSHeartbeatInterval    time.Duration
	WSHeartbeatTimeout     time.Duration
	RealtimeHistoryTTL     time.Duration
	RealtimeSnapshotTTL    time.Duration
	RealtimeStreamEpochTTL time.Duration
	OutboxNotifyEnabled    bool

	DBPingTimeout     time.Duration
	RedisPingTimeout  time.Duration
	DBMaxConns        int32
	DBMinConns        int32
	DBMaxConnLifetime time.Duration
	DBMaxConnIdleTime time.Duration
}

func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "local"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6380"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),
		KafkaBrokers:  getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaBidTopic: getEnv("KAFKA_BID_TOPIC", "auction.bid-events"),
		KafkaDLQTopic: getEnv("KAFKA_DLQ_TOPIC", "auction.dlq"),

		MinIOEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIORootUser:  getEnv("MINIO_ROOT_USER", "liveauction"),
		MinIORootPass:  getEnv("MINIO_ROOT_PASSWORD", "liveauction123"),
		S3Bucket:       getEnv("S3_BUCKET", "live-auction-items"),
		S3UseSSL:       getEnvBool("S3_USE_SSL", false),
		MockHostUserID: getEnv("MOCK_HOST_USER_ID", "host_1"),
		MockUserID:     getEnv("MOCK_USER_ID", "user_1"),
		AllowMockAuth:  getEnvBool("ALLOW_MOCK_AUTH", false),
		SessionTTL:     getEnvDuration("SESSION_TTL", 12*time.Hour),

		BidUserLimitPerSecond:     getEnvInt("BID_USER_LIMIT_PER_SECOND", 3),
		BidIPLimitPerSecond:       getEnvInt("BID_IP_LIMIT_PER_SECOND", 10),
		BidAuctionLimitPerSecond:  getEnvInt("BID_AUCTION_LIMIT_PER_SECOND", 80),
		BidAuctionMaxInFlight:     getEnvInt("BID_AUCTION_MAX_IN_FLIGHT", 32),
		BidLimitWindow:            getEnvDuration("BID_LIMIT_WINDOW", time.Second),
		BidLimitRedisTimeout:      getEnvDuration("BID_LIMIT_REDIS_TIMEOUT", 50*time.Millisecond),
		BidEngineMode:             getEnv("BID_ENGINE_MODE", "redis_ledger"),
		BidLaneWorkers:            getEnvInt("BID_LANE_WORKERS", 1),
		BidLaneQueueSize:          getEnvInt("BID_LANE_QUEUE_SIZE", 128),
		BidLaneQueueTimeout:       getEnvDuration("BID_LANE_QUEUE_TIMEOUT", 750*time.Millisecond),
		BidRedisGuardMaxStaleness: getEnvDuration("BID_REDIS_GUARD_MAX_STALENESS", 1500*time.Millisecond),
		BidRedisGuardTimeout:      getEnvDuration("BID_REDIS_GUARD_TIMEOUT", 30*time.Millisecond),
		FakePaymentWebhookSecret:  getEnv("FAKE_PAYMENT_WEBHOOK_SECRET", "local_fake_payment_secret"),

		AdmissionEnabled:       getEnvBool("ADMISSION_ENABLED", true),
		WSTicketMaxInFlight:    getEnvInt("WS_TICKET_MAX_IN_FLIGHT", 256),
		WSConnectMaxInFlight:   getEnvInt("WS_CONNECT_MAX_IN_FLIGHT", 512),
		WSRetryAfter:           getEnvDuration("WS_RETRY_AFTER", time.Second),
		WSQueueMessages:        getEnvInt("WS_QUEUE_MESSAGES", 256),
		WSQueueBytes:           getEnvInt64("WS_QUEUE_BYTES", 1<<20),
		WSRecoveryMaxEvents:    getEnvInt64("WS_RECOVERY_MAX_EVENTS", 300),
		WSSnapshotRebuildMax:   getEnvInt("WS_SNAPSHOT_REBUILD_MAX_IN_FLIGHT", 4),
		WSHeartbeatInterval:    getEnvDuration("WS_HEARTBEAT_INTERVAL", 20*time.Second),
		WSHeartbeatTimeout:     getEnvDuration("WS_HEARTBEAT_TIMEOUT", 5*time.Second),
		RealtimeHistoryTTL:     getEnvDuration("REALTIME_HISTORY_TTL", 30*time.Minute),
		RealtimeSnapshotTTL:    getEnvDuration("REALTIME_SNAPSHOT_TTL", 30*time.Minute),
		RealtimeStreamEpochTTL: getEnvDuration("REALTIME_STREAM_EPOCH_TTL", 24*time.Hour),
		OutboxNotifyEnabled:    getEnvBool("OUTBOX_NOTIFY_ENABLED", true),

		DBPingTimeout:     getEnvDuration("DB_PING_TIMEOUT", 2*time.Second),
		RedisPingTimeout:  getEnvDuration("REDIS_PING_TIMEOUT", 2*time.Second),
		DBMaxConns:        int32(getEnvInt("DB_MAX_CONNS", 8)),
		DBMinConns:        int32(getEnvInt("DB_MIN_CONNS", 0)),
		DBMaxConnLifetime: getEnvDuration("DB_MAX_CONN_LIFETIME", time.Hour),
		DBMaxConnIdleTime: getEnvDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
