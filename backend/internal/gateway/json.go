package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"live-auction/backend/internal/auction"
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
		status := http.StatusOK
		if bid, ok := result.(auction.BidResponse); ok && bid.SettlementStatus == auction.SettlementStatusPending && bid.Result != auction.BidResultConfirmationPending {
			status = http.StatusAccepted
		}
		if bid, ok := result.(auction.BidResponse); ok && bid.Result == auction.BidResultConfirmationPending {
			status = http.StatusAccepted
		}
		writeResult(w, r, status, result, nil)
		return
	}
	var apiErr apierrors.APIError
	if errors.As(err, &apiErr) && (apiErr.Code == apierrors.CodeBidAuctionTooHot || apiErr.Code == apierrors.CodeRateLimited || apiErr.Code == apierrors.CodeBidRetryLater || apiErr.Code == apierrors.CodeEnginePaused || apiErr.Code == apierrors.CodeEngineReconciling) {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterFromError(apiErr)))
	}
	writeResult(w, r, http.StatusOK, result, err)
}
