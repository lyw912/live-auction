package auction

import (
	"testing"

	apierrors "live-auction/backend/internal/platform/errors"
)

func TestValidateRuleRejectsUnreachableCap(t *testing.T) {
	capPrice := int64(35_000)
	violations := ValidateRule(RuleInput{
		StartPriceCents:     10_000,
		IncrementCents:      10_000,
		CapPriceCents:       &capPrice,
		DurationSeconds:     60,
		ExtendWindowSeconds: 10,
		ExtendBySeconds:     10,
		MaxExtendCount:      1,
	})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %#v", len(violations), violations)
	}
	if violations[0].Code != apierrors.CodeInvalidAuctionRuleCapUnreachable {
		t.Fatalf("expected unreachable cap code, got %s", violations[0].Code)
	}
	if len(violations[0].SuggestedCaps) != 2 || violations[0].SuggestedCaps[0] != 30_000 || violations[0].SuggestedCaps[1] != 40_000 {
		t.Fatalf("unexpected suggestions: %#v", violations[0].SuggestedCaps)
	}
}

func TestValidateRuleAcceptsReachableZeroStartCap(t *testing.T) {
	capPrice := int64(30_000)
	violations := ValidateRule(RuleInput{
		StartPriceCents:     0,
		IncrementCents:      10_000,
		CapPriceCents:       &capPrice,
		DurationSeconds:     60,
		ExtendWindowSeconds: 10,
		ExtendBySeconds:     10,
		MaxExtendCount:      1,
	})
	if len(violations) != 0 {
		t.Fatalf("expected no violation, got %#v", violations)
	}
}

func TestClassifyBidAmountFirstBidUsesStartPlusIncrement(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		want   BidClass
	}{
		{name: "too low", amount: 0, want: BidClassTooLow},
		{name: "accepted", amount: 10_000, want: BidClassAccepted},
		{name: "mismatch", amount: 15_000, want: BidClassIncrementMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyBidAmount(0, 0, 10_000, nil, tt.amount, false)
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestClassifyBidAmountCapSoldAndAboveCap(t *testing.T) {
	capPrice := int64(30_000)
	if got := ClassifyBidAmount(0, 20_000, 10_000, &capPrice, 30_000, true); got != BidClassAcceptedSold {
		t.Fatalf("got %s, want %s", got, BidClassAcceptedSold)
	}
	if got := ClassifyBidAmount(0, 20_000, 10_000, &capPrice, 40_000, true); got != BidClassAboveCap {
		t.Fatalf("got %s, want %s", got, BidClassAboveCap)
	}
}

func TestCalculateExtensionNeverDecreasesEndAt(t *testing.T) {
	got := CalculateExtension(100, 95, 10, 10, 0, 3)
	if got != 105 {
		t.Fatalf("got %d, want 105", got)
	}
	got = CalculateExtension(100, 80, 10, 10, 0, 3)
	if got != 100 {
		t.Fatalf("got %d, want 100", got)
	}
	got = CalculateExtension(100, 95, 10, 10, 3, 3)
	if got != 100 {
		t.Fatalf("got %d, want 100", got)
	}
}
