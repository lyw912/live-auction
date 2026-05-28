package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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

func writeBidAdmissionResult(w http.ResponseWriter, r *http.Request, result any, err error) {
	if err == nil {
		writeResult(w, r, http.StatusOK, result, nil)
		return
	}
	var apiErr apierrors.APIError
	if errors.As(err, &apiErr) && (apiErr.Code == apierrors.CodeBidAuctionTooHot || apiErr.Code == apierrors.CodeRateLimited) {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterFromError(apiErr)))
	}
	writeResult(w, r, http.StatusOK, result, err)
}
