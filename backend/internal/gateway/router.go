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
	"live-auction/backend/internal/storage"
)

func NewRouter(cfg config.Config, deps *storage.Dependencies, log *slog.Logger) http.Handler {
	return NewRouterWithRealtime(cfg, deps, log, realtime.NewServer(deps.Postgres, deps.Redis))
}

func NewRouterWithRealtime(cfg config.Config, deps *storage.Dependencies, log *slog.Logger, rt *realtime.Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(traceMiddleware)
	r.Use(requestLogMiddleware(log))

	health := HealthHandler{Config: cfg, Deps: deps}
	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness)
	r.Get("/api/health", health.Readiness)
	r.Get("/metrics", observability.Handler(deps.Postgres).ServeHTTP)

	auctionHandler := AuctionHandler{
		Config: cfg,
		Deps:   deps,
		Repo:   auction.NewRepository(deps.Postgres),
		RT:     rt,
		ACL:    newRoomACL(deps.Postgres),
	}
	authHandler := AuthHandler{Config: cfg, DB: deps.Postgres}
	monitorHandler := MonitorHandler{Deps: deps}
	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/logout", authHandler.Logout)
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
			r.Get("/orders", auctionHandler.ListOrders)
			r.Get("/users/me/bids", auctionHandler.ListBidHistory)
			r.Get("/users/me/orders", auctionHandler.ListUserOrders)
			r.Post("/orders/{id}/pay-mock", auctionHandler.PayMock)
			r.Post("/auth/ws-ticket", auctionHandler.CreateWSTicket)
			r.Get("/rooms/{room_id}/auctions", auctionHandler.ListRoomAuctions)
			r.Get("/rooms/{room_id}/chat", auctionHandler.ListChatMessages)
			r.Post("/rooms/{room_id}/chat", auctionHandler.CreateChatMessage)
			r.With(requireHost).Get("/monitor/auctions", monitorHandler.Auctions)
			r.With(requireHost).Get("/monitor/anomalies", monitorHandler.Anomalies)
			r.With(requireHost).Get("/monitor/outbox", monitorHandler.Outbox)
			r.With(requireHost).Get("/monitor/scheduler", monitorHandler.Scheduler)
			r.With(requireHost).Get("/monitor/rejects", monitorHandler.Rejects)
			r.With(requireHost).Get("/monitor/recovery", monitorHandler.Recovery)
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
