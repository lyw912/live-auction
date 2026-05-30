package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"live-auction/backend/internal/auction"
	apierrors "live-auction/backend/internal/platform/errors"
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

func TestWriteBidAdmissionResultCanReturnExplicitPendingDurabilityPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auctions/auc_1/bids", nil)
	rec := httptest.NewRecorder()

	result := auction.BidResponse{
		Result:           string(apierrors.CodeProcessingRetryLater),
		AuctionID:        "auc_1",
		EngineSeq:        7,
		DecisionStatus:   auction.DecisionStatusPendingDurability,
		DurabilityStatus: auction.DurabilityStatusKafkaUnknown,
		SettlementStatus: auction.SettlementStatusPending,
	}
	err := apierrors.New(apierrors.CodeProcessingRetryLater, "bid decision is waiting for Kafka durability", http.StatusAccepted)
	writeBidAdmissionResult(rec, req, result, err)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var got auction.BidResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if got.Result != string(apierrors.CodeProcessingRetryLater) || got.DurabilityStatus != auction.DurabilityStatusKafkaUnknown {
		t.Fatalf("response = %#v", got)
	}
}
