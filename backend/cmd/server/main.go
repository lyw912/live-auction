package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"live-auction/backend/internal/config"
	"live-auction/backend/internal/gateway"
	"live-auction/backend/internal/outbox"
	"live-auction/backend/internal/platform/logger"
	"live-auction/backend/internal/realtime"
	"live-auction/backend/internal/redisengine"
	"live-auction/backend/internal/scheduler"
	"live-auction/backend/internal/storage"
	"live-auction/backend/internal/tracing"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	log := logger.New(cfg.AppEnv)
	shutdownTracing := tracing.Init(ctx, log)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			log.Error("shutdown tracing", slog.String("error", err.Error()))
		}
	}()

	deps, err := storage.Open(ctx, cfg, log)
	if err != nil {
		log.Error("open dependencies", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer deps.Close()

	rt := realtime.NewServerWithOptions(deps.Postgres, deps.Redis, realtimeOptions(cfg)).WithAdmission(realtimeAdmission(cfg))
	workerID := envOrDefault("OUTBOX_WORKER_ID", "server-main")
	schedulerWorkerID := envOrDefault("SCHEDULER_WORKER_ID", workerID)
	if envFlag("DISABLE_EMBEDDED_OUTBOX_RELAY") {
		log.Info("embedded outbox relay disabled", slog.String("worker_id", workerID))
	} else {
		go outbox.NewRelay(deps.Postgres, deps.Redis, workerID).
			WithNotify(cfg.OutboxNotifyEnabled).
			WithPublisher(rt.PublishAuctionEvent).
			Run(ctx, log, 500*time.Millisecond)
	}
	go scheduler.NewRunner(deps.Postgres, schedulerWorkerID).Run(ctx, log, 500*time.Millisecond)
	settlementWorkerID := envOrDefault("REDIS_ENGINE_SETTLEMENT_WORKER_ID", workerID)
	var bidLedger redisengine.BidLedger
	if cfg.BidEngineMode != "postgres_lane" && cfg.BidEngineMode != "redis_guard" {
		ledger, err := redisengine.NewKafkaLedgerFromEnv(cfg.KafkaBrokers, cfg.KafkaBidTopic, cfg.KafkaDLQTopic, "settlement-workers", settlementWorkerID)
		if err != nil {
			log.Error("open kafka bid ledger", slog.String("error", err.Error()))
			os.Exit(1)
		} else {
			bidLedger = ledger
			defer bidLedger.Close()
			go redisengine.NewWorker(deps.Postgres, deps.Redis, bidLedger, settlementWorkerID).WithLogger(log).Run(ctx, 200*time.Millisecond)
		}
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           gateway.NewRouterWithRealtimeAndLedger(cfg, deps, log, rt, bidLedger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("server starting", slog.String("addr", cfg.HTTPAddr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", slog.String("error", err.Error()))
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown failed", slog.String("error", err.Error()))
	}
}

func realtimeOptions(cfg config.Config) realtime.Options {
	return realtime.Options{
		HubQueueMessages:     cfg.WSQueueMessages,
		HubQueueBytes:        cfg.WSQueueBytes,
		RecoveryMaxEvents:    cfg.WSRecoveryMaxEvents,
		SnapshotRebuildLimit: cfg.WSSnapshotRebuildMax,
		HistoryTTL:           cfg.RealtimeHistoryTTL,
		SnapshotTTL:          cfg.RealtimeSnapshotTTL,
		StreamEpochTTL:       cfg.RealtimeStreamEpochTTL,
		HeartbeatInterval:    cfg.WSHeartbeatInterval,
		HeartbeatTimeout:     cfg.WSHeartbeatTimeout,
	}
}

func realtimeAdmission(cfg config.Config) *realtime.Admission {
	if !cfg.AdmissionEnabled {
		return realtime.NewAdmission(0, 0, cfg.WSRetryAfter)
	}
	return realtime.NewAdmission(
		cfg.WSTicketMaxInFlight,
		cfg.WSConnectMaxInFlight,
		cfg.WSRetryAfter,
	)
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envFlag(key string) bool {
	switch os.Getenv(key) {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	default:
		return false
	}
}
