package storage

import (
	"context"
	"log/slog"

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

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		PoolSize:     cfg.RedisPoolSize,     // 0 = go-redis default (10×GOMAXPROCS)
		MinIdleConns: cfg.RedisMinIdleConns, // pre-warm so burst requests never wait for new dials
	})

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
