package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/config"
	"live-auction/backend/internal/platform/logger"
	"live-auction/backend/internal/reconcile"
)

func main() {
	var limit int
	var auctionIDs string
	var writeAnomalies bool
	var failOnDrift bool
	flag.IntVar(&limit, "limit", 100, "maximum auctions to check when --auction-id is not set")
	flag.StringVar(&auctionIDs, "auction-id", "", "comma-separated auction IDs to check")
	flag.BoolVar(&writeAnomalies, "write-anomalies", false, "write REDIS_DB_RECONCILIATION_DRIFT anomaly rows for detected drift")
	flag.BoolVar(&failOnDrift, "fail-on-drift", false, "exit non-zero when drift is detected")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg := config.Load()
	logSink := logger.New(cfg.AppEnv)
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func() { _ = rdb.Close() }()

	opts := reconcile.Options{
		Limit:          limit,
		AuctionIDs:     splitIDs(auctionIDs),
		WriteAnomalies: writeAnomalies,
	}
	report, err := reconcile.NewChecker(db, rdb).Check(ctx, opts)
	if err != nil {
		logSink.Error("reconciliation check failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("marshal report: %v", err)
	}
	fmt.Println(string(data))
	if failOnDrift && report.DriftCount > 0 {
		os.Exit(2)
	}
}

func splitIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
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
