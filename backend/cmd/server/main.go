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

	rt := realtime.NewServer(deps.Postgres, deps.Redis)
	go outbox.NewRelay(deps.Postgres, deps.Redis, "server-main").
		WithPublisher(rt.PublishAuctionEvent).
		Run(ctx, log, 500*time.Millisecond)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           gateway.NewRouterWithRealtime(cfg, deps, log, rt),
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
