package gateway

import (
	"encoding/json"
	"net/http"

	apierrors "live-auction/backend/internal/platform/errors"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, r *http.Request, err apierrors.APIError) {
	err.TraceID = traceID(r.Context())
	writeJSON(w, err.Status, err)
}
