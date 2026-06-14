package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string

	RedisAddr               string
	RedisMode               string
	RedisPassword           string
	RedisDB                 int
	RedisPoolSize           int
	RedisMinIdleConns       int
	RedisSentinelMasterName string
	RedisSentinelAddrs      string
	RedisSentinelUsername   string
	RedisSentinelPassword   string
	KafkaBrokers            string
	KafkaBidTopic           string
	KafkaDLQTopic           string

	MinIOEndpoint  string
	MinIORootUser  string
	MinIORootPass  string
	S3Bucket       string
	S3UseSSL       bool
	MockHostUserID string
	MockUserID     string
	AllowMockAuth  bool
	SessionTTL     time.Duration

	LiveDemoMediaProtocol string
	LiveDemoMediaURL      string
	LiveDemoMimeType      string
	LiveDemoIsLive        bool
	LiveDemoLatencyMs     int
	LiveFallbackMP4URL    string

	BidUserLimitPerSecond        int
	BidIPLimitPerSecond          int
	BidAuctionLimitPerSecond     int
	BidAuctionMaxInFlight        int
	BidLimitWindow               time.Duration
	BidLimitRedisTimeout         time.Duration
	BidEngineMode                string
	BidEngineResponseDurability  string // current runtime is kafka_ack only
	BidLaneWorkers               int
	BidLaneQueueSize             int
	BidLaneQueueTimeout          time.Duration
	BidRedisGuardMaxStaleness    time.Duration
	BidRedisGuardTimeout         time.Duration
	RedisEngineSettlementWorkers int
	FakePaymentWebhookSecret     string
	AlipaySandboxEnabled         bool
	AlipayAppID                  string
	AlipayPrivateKey             string
	AlipayPrivateKeyPath         string
	AlipayPublicKey              string
	AlipayPublicKeyPath          string
	AlipayGatewayURL             string
	AlipayNotifyURL              string
	AlipayIncludeNotifyURL       bool
	AlipayReturnURL              string
	AlipayIncludeReturnURL       bool
	AlipayPayMethod              string
	AlipayProductCode            string

	AIProviderMode               string
	AIRelayBaseURL               string
	AIRelayModel                 string
	AIAPIKey                     string
	AIRelayTimeout               time.Duration
	AIRelayMaxTokens             int
	AITextBaseURL                string
	AITextModel                  string
	AITextAPIKey                 string
	AITextTimeout                time.Duration
	AITextMaxTokens              int
	AIVisionBaseURL              string
	AIVisionModel                string
	AIVisionAPIKey               string
	AIVisionTimeout              time.Duration
	AIVisionMaxTokens            int
	AICommentaryBackfillLookback time.Duration
	AICommentaryBatchSize        int
	AICommentaryTaskTimeout      time.Duration

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
	LeaderboardQueueSize   int
	LeaderboardWorkers     int
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
	loadDotEnvIfPresent()
	cfg := Config{
		AppEnv:      getEnv("APP_ENV", "local"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":18080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"),

		RedisAddr:               getEnv("REDIS_ADDR", "localhost:6380"),
		RedisMode:               getEnv("REDIS_MODE", "single"),
		RedisPassword:           getEnv("REDIS_PASSWORD", ""),
		RedisDB:                 getEnvInt("REDIS_DB", 0),
		RedisPoolSize:           getEnvInt("REDIS_POOL_SIZE", 0),       // 0 = go-redis default (10×GOMAXPROCS)
		RedisMinIdleConns:       getEnvInt("REDIS_MIN_IDLE_CONNS", 50), // pre-warmed connections
		RedisSentinelMasterName: getEnv("REDIS_SENTINEL_MASTER_NAME", ""),
		RedisSentinelAddrs:      getEnv("REDIS_SENTINEL_ADDRS", ""),
		RedisSentinelUsername:   getEnv("REDIS_SENTINEL_USERNAME", ""),
		RedisSentinelPassword:   getEnv("REDIS_SENTINEL_PASSWORD", ""),
		KafkaBrokers:            getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaBidTopic:           getEnv("KAFKA_BID_TOPIC", "auction.bid-events"),
		KafkaDLQTopic:           getEnv("KAFKA_DLQ_TOPIC", "auction.dlq"),

		MinIOEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIORootUser:  getEnv("MINIO_ROOT_USER", "liveauction"),
		MinIORootPass:  getEnv("MINIO_ROOT_PASSWORD", "liveauction123"),
		S3Bucket:       getEnv("S3_BUCKET", "live-auction-items"),
		S3UseSSL:       getEnvBool("S3_USE_SSL", false),
		MockHostUserID: getEnv("MOCK_HOST_USER_ID", "host_1"),
		MockUserID:     getEnv("MOCK_USER_ID", "user_1"),
		AllowMockAuth:  getEnvBool("ALLOW_MOCK_AUTH", false),
		SessionTTL:     getEnvDuration("SESSION_TTL", 12*time.Hour),

		LiveDemoMediaProtocol: getEnv("LIVE_DEMO_MEDIA_PROTOCOL", "whep"),
		LiveDemoMediaURL:      getEnv("LIVE_DEMO_MEDIA_URL", "/mtx/auction-live/whep"),
		LiveDemoMimeType:      getEnv("LIVE_DEMO_MIME_TYPE", "application/sdp"),
		LiveDemoIsLive:        getEnvBool("LIVE_DEMO_IS_LIVE", true),
		LiveDemoLatencyMs:     getEnvInt("LIVE_DEMO_LATENCY_MS", 800),
		LiveFallbackMP4URL:    getEnv("LIVE_MEDIA_FALLBACK_MP4_URL", ""),

		BidUserLimitPerSecond:        getEnvInt("BID_USER_LIMIT_PER_SECOND", 3),
		BidIPLimitPerSecond:          getEnvInt("BID_IP_LIMIT_PER_SECOND", 10),
		BidAuctionLimitPerSecond:     getEnvInt("BID_AUCTION_LIMIT_PER_SECOND", 80),
		BidAuctionMaxInFlight:        getEnvInt("BID_AUCTION_MAX_IN_FLIGHT", 32),
		BidLimitWindow:               getEnvDuration("BID_LIMIT_WINDOW", time.Second),
		BidLimitRedisTimeout:         getEnvDuration("BID_LIMIT_REDIS_TIMEOUT", 50*time.Millisecond),
		BidEngineMode:                "redis_ledger",
		BidEngineResponseDurability:  "kafka_ack",
		BidLaneWorkers:               getEnvInt("BID_LANE_WORKERS", 1),
		BidLaneQueueSize:             getEnvInt("BID_LANE_QUEUE_SIZE", 128),
		BidLaneQueueTimeout:          getEnvDuration("BID_LANE_QUEUE_TIMEOUT", 750*time.Millisecond),
		BidRedisGuardMaxStaleness:    getEnvDuration("BID_REDIS_GUARD_MAX_STALENESS", 1500*time.Millisecond),
		BidRedisGuardTimeout:         getEnvDuration("BID_REDIS_GUARD_TIMEOUT", 30*time.Millisecond),
		RedisEngineSettlementWorkers: getEnvInt("REDIS_ENGINE_SETTLEMENT_WORKERS", 1),
		FakePaymentWebhookSecret:     getEnv("FAKE_PAYMENT_WEBHOOK_SECRET", "local_fake_payment_secret"),
		AlipaySandboxEnabled:         getEnvBool("ALIPAY_SANDBOX_ENABLED", false),
		AlipayAppID:                  getEnv("ALIPAY_APP_ID", ""),
		AlipayPrivateKey:             getEnv("ALIPAY_PRIVATE_KEY", ""),
		AlipayPrivateKeyPath:         getEnv("ALIPAY_PRIVATE_KEY_PATH", ""),
		AlipayPublicKey:              getEnv("ALIPAY_PUBLIC_KEY", ""),
		AlipayPublicKeyPath:          getEnv("ALIPAY_PUBLIC_KEY_PATH", ""),
		AlipayGatewayURL:             getEnv("ALIPAY_GATEWAY_URL", "https://openapi-sandbox.dl.alipaydev.com/gateway.do"),
		AlipayNotifyURL:              getEnv("ALIPAY_NOTIFY_URL", ""),
		AlipayIncludeNotifyURL:       getEnvBool("ALIPAY_INCLUDE_NOTIFY_URL", false),
		AlipayReturnURL:              getEnv("ALIPAY_RETURN_URL", ""),
		AlipayIncludeReturnURL:       getEnvBool("ALIPAY_INCLUDE_RETURN_URL", false),
		AlipayPayMethod:              getEnv("ALIPAY_PAY_METHOD", "alipay.trade.page.pay"),
		AlipayProductCode:            getEnv("ALIPAY_PRODUCT_CODE", "FAST_INSTANT_TRADE_PAY"),

		AIProviderMode:               getEnv("AI_PROVIDER_MODE", "provider"),
		AIRelayBaseURL:               getEnv("AI_RELAY_BASE_URL", "https://api.gptgod.online/v1"),
		AIRelayModel:                 getEnv("AI_RELAY_MODEL", "gemini-3.1-flash-image-preview"),
		AIAPIKey:                     getEnv("API_KEY", ""),
		AIRelayTimeout:               getEnvDuration("AI_RELAY_TIMEOUT", getEnvMillisDuration("AI_RELAY_TIMEOUT_MS", 45*time.Second)),
		AIRelayMaxTokens:             getEnvInt("AI_RELAY_MAX_TOKENS", 2048),
		AITextBaseURL:                getEnv("AI_TEXT_BASE_URL", "https://api.deepseek.com"),
		AITextModel:                  getEnv("AI_TEXT_MODEL", "deepseek-v4-flash"),
		AITextAPIKey:                 getEnvFirst([]string{"AI_TEXT_API_KEY", "DEEPSEEK_API_KEY"}, ""),
		AITextTimeout:                getEnvDuration("AI_TEXT_TIMEOUT", getEnvMillisDuration("AI_TEXT_TIMEOUT_MS", 20*time.Second)),
		AITextMaxTokens:              getEnvInt("AI_TEXT_MAX_TOKENS", 1024),
		AIVisionBaseURL:              getEnv("AI_VISION_BASE_URL", getEnv("AI_RELAY_BASE_URL", "https://api.gptgod.online/v1")),
		AIVisionModel:                getEnv("AI_VISION_MODEL", getEnv("AI_RELAY_MODEL", "gemini-3.1-flash-image-preview")),
		AIVisionAPIKey:               getEnv("AI_VISION_API_KEY", getEnv("API_KEY", "")),
		AIVisionTimeout:              getEnvDuration("AI_VISION_TIMEOUT", getEnvDuration("AI_RELAY_TIMEOUT", getEnvMillisDuration("AI_RELAY_TIMEOUT_MS", 45*time.Second))),
		AIVisionMaxTokens:            getEnvInt("AI_VISION_MAX_TOKENS", getEnvInt("AI_RELAY_MAX_TOKENS", 2048)),
		AICommentaryBackfillLookback: getEnvDuration("AI_COMMENTARY_BACKFILL_LOOKBACK", 24*time.Hour),
		AICommentaryBatchSize:        getEnvInt("AI_COMMENTARY_BATCH_SIZE", 4),
		AICommentaryTaskTimeout:      getEnvDuration("AI_COMMENTARY_TASK_TIMEOUT", 20*time.Second),

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
		LeaderboardQueueSize:   getEnvInt("LEADERBOARD_PROJECTION_QUEUE_SIZE", 1024),
		LeaderboardWorkers:     getEnvInt("LEADERBOARD_PROJECTION_WORKERS", 1),
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
	if cfg.RedisEngineSettlementWorkers < 1 {
		cfg.RedisEngineSettlementWorkers = 1
	}
	if cfg.AICommentaryBatchSize < 1 {
		cfg.AICommentaryBatchSize = 1
	}
	if cfg.LeaderboardQueueSize < 1 {
		cfg.LeaderboardQueueSize = 1
	}
	if cfg.LeaderboardWorkers < 1 {
		cfg.LeaderboardWorkers = 1
	}
	return cfg
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvFirst(keys []string, fallback string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return fallback
}

func loadDotEnvIfPresent() {
	for _, path := range dotenvCandidates() {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
			if key == "" || os.Getenv(key) != "" {
				continue
			}
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"'`)
			_ = os.Setenv(key, value)
		}
		_ = file.Close()
		return
	}
}

func dotenvCandidates() []string {
	wd, err := os.Getwd()
	if err != nil {
		return []string{".env"}
	}
	return []string{
		filepath.Join(wd, ".env"),
		filepath.Join(wd, "..", ".env"),
		filepath.Join(wd, "..", "..", ".env"),
	}
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

func getEnvMillisDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return time.Duration(parsed) * time.Millisecond
}
