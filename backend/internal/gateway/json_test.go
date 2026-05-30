package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"live-auction/backend/internal/auction"
)

func TestWriteBidAdmissionResultUsesAcceptedForPendingSettlement(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auctions/auc_1/bids", nil)
	rec := httptest.NewRecorder()

	writeBidAdmissionResult(rec, req, auction.BidResponse{
		Result:           auction.BidResultEngineRejected,
		AuctionID:        "auc_1",
		EngineSeq:        7,
		SettlementStatus: auction.SettlementStatusPending,
	}, nil)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
}
