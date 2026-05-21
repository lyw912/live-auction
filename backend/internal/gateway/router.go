package gateway

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"live-auction/backend/internal/config"
	"live-auction/backend/internal/storage"
)

func NewRouter(cfg config.Config, deps *storage.Dependencies, log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(traceMiddleware)
	r.Use(requestLogMiddleware(log))

	health := HealthHandler{Config: cfg, Deps: deps}
	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness)
	r.Get("/api/health", health.Readiness)

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
