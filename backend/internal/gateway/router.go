package gateway

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
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

	auctionHandler := AuctionHandler{
		Config: cfg,
		Deps:   deps,
		Repo:   auction.NewRepository(deps.Postgres),
		RT:     rt,
	}
	r.Route("/api", func(r chi.Router) {
		r.Use(mockAuthMiddleware(cfg))
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
		r.Get("/orders", auctionHandler.ListOrders)
		r.Get("/users/me/orders", auctionHandler.ListOrders)
		r.Post("/orders/{id}/pay-mock", auctionHandler.PayMock)
		r.Post("/auth/ws-ticket", auctionHandler.CreateWSTicket)
		r.Get("/rooms/{room_id}/auctions", auctionHandler.ListRoomAuctions)
	})
	r.Get("/ws", rt.ServeWS)

	return r
}

func requestLogMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			log.Info("http_request",
				slog.String("trace_id", traceID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}
