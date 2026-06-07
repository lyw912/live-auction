package redisengine

import (
	"testing"

	"live-auction/backend/internal/auction"
)

func TestMergeHotProxyResponseUsesFinalDecisionDurability(t *testing.T) {
	winner := "proxy_user"
	base := auction.BidResponse{
		Result:            auction.BidResultEngineAccepted,
		Seq:               1,
		EngineSeq:         1,
		DecisionStatus:    auction.DecisionStatusDecided,
		DurabilityStatus:  auction.DurabilityStatusKafkaAcked,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 15_000,
	}
	auto := auction.BidResponse{
		Result:            auction.BidResultEngineAccepted,
		Seq:               2,
		EngineSeq:         2,
		DecisionStatus:    auction.DecisionStatusDecided,
		DurabilityStatus:  auction.DurabilityStatusEngineDurable,
		SettlementStatus:  auction.SettlementStatusPending,
		CurrentPriceCents: 20_000,
		CurrentWinnerID:   &winner,
	}

	merged := mergeHotProxyResponse(base, auto)
	if merged.Seq != 2 || merged.EngineSeq != 2 || merged.CurrentPriceCents != 20_000 || merged.CurrentWinnerID == nil || *merged.CurrentWinnerID != winner {
		t.Fatalf("merged hot proxy state = %#v, want final auto state", merged)
	}
	if merged.DurabilityStatus != auction.DurabilityStatusEngineDurable {
		t.Fatalf("durability_status = %q, want final auto decision durability %q", merged.DurabilityStatus, auction.DurabilityStatusEngineDurable)
	}
}
