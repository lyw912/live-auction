package gateway

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	"live-auction/backend/internal/observability"
	"live-auction/backend/internal/realtime"
	"live-auction/backend/internal/redisengine"
	"live-auction/backend/internal/storage"
)

func NewRouter(cfg config.Config, deps *storage.Dependencies, log *slog.Logger) http.Handler {
	cfg = normalizeBidLaneConfig(cfg)
	rt := realtime.NewServerWithOptions(deps.Postgres, deps.Redis, realtimeOptions(cfg)).WithAdmission(newRealtimeAdmission(cfg))
	var ledger redisengine.BidLedger
	if cfg.BidEngineMode != bidEngineModePostgresLane && cfg.BidEngineMode != bidEngineModeRedisGuard {
		kafkaLedger, err := redisengine.NewKafkaLedgerFromEnv(cfg.KafkaBrokers, cfg.KafkaBidTopic, cfg.KafkaDLQTopic, "settlement-workers", "gateway")
		if err == nil {
			ledger = kafkaLedger
		} else if log != nil {
			log.Error("open kafka bid ledger", slog.String("error", err.Error()))
		}
	}
	return NewRouterWithRealtimeAndLedger(cfg, deps, log, rt, ledger)
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

func newRealtimeAdmission(cfg config.Config) *realtime.Admission {
	if !cfg.AdmissionEnabled {
		return realtime.NewAdmission(0, 0, cfg.WSRetryAfter)
	}
	return realtime.NewAdmission(
		cfg.WSTicketMaxInFlight,
		cfg.WSConnectMaxInFlight,
		cfg.WSRetryAfter,
	)
}

func NewRouterWithRealtime(cfg config.Config, deps *storage.Dependencies, log *slog.Logger, rt *realtime.Server) http.Handler {
	return NewRouterWithRealtimeAndLedger(cfg, deps, log, rt, nil)
}

func NewRouterWithRealtimeAndLedger(cfg config.Config, deps *storage.Dependencies, log *slog.Logger, rt *realtime.Server, ledger redisengine.BidLedger) http.Handler {
	bidLaneCfg := normalizeBidLaneConfig(cfg)
	cfg = bidLaneCfg
	observability.SetAdmissionConfig(observability.AdmissionConfig{
		Enabled:               cfg.AdmissionEnabled,
		BidUserLimit:          cfg.BidUserLimitPerSecond,
		BidIPLimit:            cfg.BidIPLimitPerSecond,
		BidAuctionLimit:       cfg.BidAuctionLimitPerSecond,
		BidAuctionMaxInFlight: cfg.BidAuctionMaxInFlight,
		BidLaneWorkers:        bidLaneCfg.BidLaneWorkers,
		BidLaneQueueSize:      bidLaneCfg.BidLaneQueueSize,
		BidLaneQueueTimeout:   bidLaneCfg.BidLaneQueueTimeout,
		WSTicketMaxInFlight:   cfg.WSTicketMaxInFlight,
		WSConnectMaxInFlight:  cfg.WSConnectMaxInFlight,
		WSQueueMessages:       cfg.WSQueueMessages,
		WSQueueBytes:          cfg.WSQueueBytes,
		WSRecoveryMaxEvents:   cfg.WSRecoveryMaxEvents,
		WSSnapshotRebuildMax:  cfg.WSSnapshotRebuildMax,
		WSHeartbeatInterval:   cfg.WSHeartbeatInterval,
		WSHeartbeatTimeout:    cfg.WSHeartbeatTimeout,
	})

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(traceMiddleware)
	r.Use(requestLogMiddleware(log))

	health := HealthHandler{Config: cfg, Deps: deps}
	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness)
	r.Get("/api/health", health.Readiness)
	r.Get("/metrics", observability.Handler(deps.Postgres).ServeHTTP)

	var engine *redisengine.Engine
	if cfg.BidEngineMode != bidEngineModePostgresLane && cfg.BidEngineMode != bidEngineModeRedisGuard {
		engine = redisengine.New(deps.Postgres, deps.Redis, ledger)
	}
	auctionHandler := AuctionHandler{
		Config: cfg,
		Deps:   deps,
		Repo:   auction.NewRepository(deps.Postgres),
		RT:     rt,
		ACL:    newRoomACL(deps.Postgres),
		Bids:   newBidAdmission(cfg, deps.Postgres, deps.Redis),
		Lanes:  newBidLaneManager(cfg, deps.Postgres),
		Engine: engine,
	}
	authHandler := AuthHandler{Config: cfg, DB: deps.Postgres}
	monitorHandler := MonitorHandler{Deps: deps}
	hostPrompterHandler := HostPrompterHandler{Deps: deps}
	heatSummaryHandler := HeatSummaryHandler{Deps: deps}
	maxBidSummaryHandler := MaxBidSummaryHandler{Deps: deps}
	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/logout", authHandler.Logout)
		r.Post("/payments/fake-provider/webhook", auctionHandler.FakePaymentWebhook)
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware(cfg, deps.Postgres))
			r.Get("/auth/me", authHandler.Me)
			r.Get("/rooms", auctionHandler.ListRooms)
			r.With(requireHost).Post("/items/upload-url", auctionHandler.CreateUploadURL)
			r.With(requireHost).Post("/items", auctionHandler.CreateItem)
			r.With(requireHost).Post("/auctions", auctionHandler.CreateAuction)
			r.Get("/auctions", auctionHandler.ListAuctions)
			r.Get("/auctions/{id}", auctionHandler.GetAuction)
			r.With(requireHost).Patch("/auctions/{id}/rules", auctionHandler.UpdateRules)
			r.With(requireHost).Post("/auctions/{id}/schedule", auctionHandler.Schedule)
			r.With(requireHost).Post("/auctions/{id}/unschedule", auctionHandler.Unschedule)
			r.With(requireHost).Post("/auctions/{id}/start", auctionHandler.Start)
			r.With(requireHost).Post("/auctions/{id}/cancel", auctionHandler.Cancel)
			r.With(requireHost).Post("/auctions/{id}/narrate-start", auctionHandler.NarrateStart)
			r.With(requireHost).Post("/auctions/{id}/narrate-stop", auctionHandler.NarrateStop)
			r.Post("/auctions/{id}/bids", auctionHandler.PlaceBid)
			r.Post("/auctions/{id}/bids/confirm", auctionHandler.ConfirmBid)
			r.With(requireHost).Post("/demo/auctions/{id}/competing-bid", auctionHandler.DemoCompetingBid)
			r.Get("/auctions/{id}/leaderboard", auctionHandler.GetLeaderboard)
			r.Get("/auctions/{id}/max-bid-intent", auctionHandler.GetMaxBidIntent)
			r.Put("/auctions/{id}/max-bid-intent", auctionHandler.PutMaxBidIntent)
			r.Delete("/auctions/{id}/max-bid-intent", auctionHandler.DeleteMaxBidIntent)
			r.Get("/orders", auctionHandler.ListOrders)
			r.Get("/users/me/bids", auctionHandler.ListBidHistory)
			r.Get("/users/me/orders", auctionHandler.ListUserOrders)
			r.Post("/orders/{id}/pay-mock", auctionHandler.PayMock)
			r.Post("/auth/ws-ticket", auctionHandler.CreateWSTicket)
			r.Get("/rooms/{room_id}/auctions", auctionHandler.ListRoomAuctions)
			r.Get("/rooms/{room_id}/chat", auctionHandler.ListChatMessages)
			r.Post("/rooms/{room_id}/chat", auctionHandler.CreateChatMessage)
			r.With(requireHost).Get("/host/auctions/{id}/prompts", hostPrompterHandler.Prompts)
			r.With(requireHost).Get("/host/auctions/{id}/heat-summary", heatSummaryHandler.Summary)
			r.With(requireHost).Get("/host/auctions/{id}/max-bid-summary", maxBidSummaryHandler.Summary)
			r.With(requireHost).Get("/monitor/auctions", monitorHandler.Auctions)
			r.With(requireHost).Get("/monitor/auctions/{id}/flight-recorder", monitorHandler.FlightRecorder)
			r.With(requireHost).Get("/monitor/anomalies", monitorHandler.Anomalies)
			r.With(requireHost).Get("/monitor/outbox", monitorHandler.Outbox)
			r.With(requireHost).Get("/monitor/outbox/watermarks", monitorHandler.OutboxWatermarks)
			r.With(requireHost).Get("/monitor/scheduler", monitorHandler.Scheduler)
			r.With(requireHost).Get("/monitor/rejects", monitorHandler.Rejects)
			r.With(requireHost).Get("/monitor/recovery", monitorHandler.Recovery)
			r.With(requireHost).Get("/monitor/snapshots", monitorHandler.Snapshots)
			r.With(requireHost).Get("/monitor/signals", monitorHandler.Signals)
			r.With(requireHost).Get("/monitor/redis-engine", monitorHandler.RedisEngine)
			r.With(requireHost).Post("/monitor/signals", monitorHandler.CreateSignal)
			r.With(requireHost).Post("/test/rooms", auctionHandler.TestSetupRoom)
		})
	})
	r.Get("/ws", rt.ServeWS)

	return r
}

func requestLogMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(rec, r)
			status := rec.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = r.URL.Path
			}
			observability.Inc("http_request_total", map[string]string{
				"method": r.Method,
				"path":   route,
				"status": strconv.Itoa(status),
			})
			observability.Observe("http_request_latency_seconds", time.Since(start).Seconds(), map[string]string{
				"method": r.Method,
				"path":   route,
			}, observability.DefaultLatencyBuckets)
			log.Info("http_request",
				slog.String("trace_id", traceID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}
