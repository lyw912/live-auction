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

	MinIOEndpoint  string
	MinIORootUser  string
	MinIORootPass  string
	S3Bucket       string
	S3UseSSL       bool
	MockHostUserID string
	MockUserID     string

	DBPingTimeout    time.Duration
	RedisPingTimeout time.Duration
}

func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "local"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		MinIOEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIORootUser:  getEnv("MINIO_ROOT_USER", "liveauction"),
		MinIORootPass:  getEnv("MINIO_ROOT_PASSWORD", "liveauction123"),
		S3Bucket:       getEnv("S3_BUCKET", "live-auction-items"),
		S3UseSSL:       getEnvBool("S3_USE_SSL", false),
		MockHostUserID: getEnv("MOCK_HOST_USER_ID", "host_1"),
		MockUserID:     getEnv("MOCK_USER_ID", "user_1"),

		DBPingTimeout:    getEnvDuration("DB_PING_TIMEOUT", 2*time.Second),
		RedisPingTimeout: getEnvDuration("REDIS_PING_TIMEOUT", 2*time.Second),
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
