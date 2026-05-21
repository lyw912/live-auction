package gateway

import (
	"context"
	"net/http"
	"time"

	"live-auction/backend/internal/config"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/storage"
)

type HealthHandler struct {
	Config config.Config
	Deps   *storage.Dependencies
}

func (h HealthHandler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"app":    "live-auction",
		"env":    h.Config.AppEnv,
	})
}

func (h HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := h.Deps.Health(ctx)
	ready := checks.PostgresOK && checks.RedisOK && checks.MinIOOK
	if !ready {
		writeError(w, r, apierrors.APIError{
			Code:    apierrors.CodeInvalidArgument,
			Message: "dependency readiness check failed",
			Status:  http.StatusServiceUnavailable,
			Details: map[string]any{
				"postgres": checks.PostgresOK,
				"redis":    checks.RedisOK,
				"minio":    checks.MinIOOK,
				"errors":   checks.Errors,
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
		"checks": checks,
	})
}
