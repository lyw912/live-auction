package storage

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/config"
)

type Dependencies struct {
	Postgres *pgxpool.Pool
	Redis    *redis.Client
	MinIO    *minio.Client
	Bucket   string
	log      *slog.Logger
}

type Health struct {
	PostgresOK bool              `json:"postgres_ok"`
	RedisOK    bool              `json:"redis_ok"`
	MinIOOK    bool              `json:"minio_ok"`
	Errors     map[string]string `json:"errors,omitempty"`
}

func Open(ctx context.Context, cfg config.Config, log *slog.Logger) (*Dependencies, error) {
	pgConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.DBMaxConns > 0 {
		pgConfig.MaxConns = cfg.DBMaxConns
	}
	if cfg.DBMinConns > 0 {
		pgConfig.MinConns = cfg.DBMinConns
	}
	if cfg.DBMaxConnLifetime > 0 {
		pgConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	}
	if cfg.DBMaxConnIdleTime > 0 {
		pgConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	}

	pg, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return nil, err
	}

	rdb, err := openRedis(cfg)
	if err != nil {
		pg.Close()
		return nil, err
	}

	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIORootUser, cfg.MinIORootPass, ""),
		Secure: cfg.S3UseSSL,
	})
	if err != nil {
		pg.Close()
		_ = rdb.Close()
		return nil, err
	}

	return &Dependencies{Postgres: pg, Redis: rdb, MinIO: minioClient, Bucket: cfg.S3Bucket, log: log}, nil
}

func openRedis(cfg config.Config) (*redis.Client, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.RedisMode)) {
	case "", "single":
		return redis.NewClient(&redis.Options{
			Addr:         cfg.RedisAddr,
			Password:     cfg.RedisPassword,
			DB:           cfg.RedisDB,
			PoolSize:     cfg.RedisPoolSize,     // 0 = go-redis default (10×GOMAXPROCS)
			MinIdleConns: cfg.RedisMinIdleConns, // pre-warm so burst requests never wait for new dials
		}), nil
	case "sentinel":
		addrs := splitCSV(cfg.RedisSentinelAddrs)
		if cfg.RedisSentinelMasterName == "" || len(addrs) == 0 {
			return nil, fmt.Errorf("REDIS_MODE=sentinel requires REDIS_SENTINEL_MASTER_NAME and REDIS_SENTINEL_ADDRS")
		}
		return redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       cfg.RedisSentinelMasterName,
			SentinelAddrs:    addrs,
			SentinelUsername: cfg.RedisSentinelUsername,
			SentinelPassword: cfg.RedisSentinelPassword,
			Password:         cfg.RedisPassword,
			DB:               cfg.RedisDB,
			PoolSize:         cfg.RedisPoolSize,
			MinIdleConns:     cfg.RedisMinIdleConns,
		}), nil
	case "cluster":
		return nil, fmt.Errorf("REDIS_MODE=cluster is not supported by the current hot-engine Lua key topology; use sentinel/managed failover or remove cross-slot global keys first")
	default:
		return nil, fmt.Errorf("unsupported REDIS_MODE %q; expected single or sentinel", cfg.RedisMode)
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (d *Dependencies) Close() {
	if d == nil {
		return
	}
	if d.Postgres != nil {
		d.Postgres.Close()
	}
	if d.Redis != nil {
		_ = d.Redis.Close()
	}
}

func (d *Dependencies) Health(ctx context.Context) Health {
	result := Health{
		Errors: map[string]string{},
	}

	if err := d.Postgres.Ping(ctx); err != nil {
		result.Errors["postgres"] = err.Error()
	} else {
		result.PostgresOK = true
	}

	if err := d.Redis.Ping(ctx).Err(); err != nil {
		result.Errors["redis"] = err.Error()
	} else {
		result.RedisOK = true
	}

	exists, err := d.MinIO.BucketExists(ctx, d.Bucket)
	if err != nil {
		result.Errors["minio"] = err.Error()
	} else if !exists {
		result.Errors["minio"] = "bucket does not exist"
	} else {
		result.MinIOOK = true
	}

	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	return result
}
