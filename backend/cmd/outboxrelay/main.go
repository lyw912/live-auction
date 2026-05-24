package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"live-auction/backend/internal/config"
	"live-auction/backend/internal/outbox"
	"live-auction/backend/internal/platform/logger"
	"live-auction/backend/internal/storage"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	log := logger.New(cfg.AppEnv)
	deps, err := storage.Open(ctx, cfg, log)
	if err != nil {
		log.Error("open dependencies", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer deps.Close()

	workerID := os.Getenv("OUTBOX_WORKER_ID")
	if workerID == "" {
		workerID = "outbox-relay"
	}
	log.Info("outbox relay starting", slog.String("worker_id", workerID))
	outbox.NewRelay(deps.Postgres, deps.Redis, workerID).Run(ctx, log, 500*time.Millisecond)
}
