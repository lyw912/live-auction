package auction

import "testing"

func TestCalculateDepositUsesIntegerBounds(t *testing.T) {
	got := CalculateDeposit(100_000, 1000, 10_000, 1_000_000)
	if got != 10_000 {
		t.Fatalf("got %d, want 10000", got)
	}

	got = CalculateDeposit(10_000, 1000, 10_000, 1_000_000)
	if got != 5_000 {
		t.Fatalf("deposit must not exceed half amount, got %d", got)
	}
}
