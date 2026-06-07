package ai

import "testing"

func TestRecapRuleSuggestionUsesServerFactsAndRequiresReview(t *testing.T) {
	state := auctionState{
		Status:            "SOLD",
		StartPriceCents:   10_000,
		IncrementCents:    5_000,
		CapPriceCents:     55_000,
		CurrentPriceCents: 60_000,
	}

	suggestion := recapRuleSuggestion(state, 4)
	if suggestion == nil {
		t.Fatal("expected recap rule suggestion")
	}
	if suggestion.StartPriceCents != 25_000 {
		t.Fatalf("start price = %d, want 25000", suggestion.StartPriceCents)
	}
	if suggestion.IncrementCents != 5_000 {
		t.Fatalf("increment = %d, want 5000", suggestion.IncrementCents)
	}
	if suggestion.CapPriceCents != 60_000 {
		t.Fatalf("cap price = %d, want 60000", suggestion.CapPriceCents)
	}
	if suggestion.Source != "auction_recap:server_facts" {
		t.Fatalf("source = %q", suggestion.Source)
	}
	if !suggestion.HumanReviewRequired {
		t.Fatal("suggestion must require human review")
	}
}

func TestRecapRuleSuggestionDoesNotInventDemandForThinAuctions(t *testing.T) {
	state := auctionState{
		Status:            "SOLD",
		StartPriceCents:   10_000,
		IncrementCents:    5_000,
		CapPriceCents:     60_000,
		CurrentPriceCents: 60_000,
	}

	suggestion := recapRuleSuggestion(state, 1)
	if suggestion == nil {
		t.Fatal("expected conservative recap rule suggestion")
	}
	if suggestion.StartPriceCents != 10_000 {
		t.Fatalf("thin auction start price = %d, want original 10000", suggestion.StartPriceCents)
	}
	if !suggestion.HumanReviewRequired {
		t.Fatal("thin auction suggestion must require human review")
	}
}
