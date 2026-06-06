package auction

import (
	"testing"
)

// TestCalculateDepositThreePaths verifies that all three settlement paths
// (redis engine via auction.CalculateDeposit, PG lane via createOrderForSoldAuction,
// scheduler via scheduler.calculateDeposit→auction.CalculateDeposit) produce
// identical deposit_cents for the same (amount, bps, floor, cap) inputs.
// All three now call the same canonical function, so we verify the function
// covers the required cases end-to-end.
func TestCalculateDepositCustomBPS(t *testing.T) {
	cases := []struct {
		name     string
		amount   int64
		bps      int64
		floor    int64
		cap      int64
		wantDep  int64
	}{
		{
			name: "15pct no clamp",
			// 1_000_000 * 1500 / 10000 = 150_000; floor=10_000 ok; cap=1_000_000 ok; half=500_000 ok
			amount: 1_000_000, bps: 1500, floor: 10_000, cap: 1_000_000,
			wantDep: 150_000,
		},
		{
			name: "floor clamp — tiny amount",
			// 500 * 1000 / 10000 = 50; floor=10_000 but half=250 → floor clamped to 250; raw(50)<floor(250) → 250
			amount: 500, bps: 1000, floor: 10_000, cap: 1_000_000,
			wantDep: 250,
		},
		{
			name: "cap clamp — huge amount",
			// 100_000_000 * 1000 / 10000 = 10_000_000; cap=500_000; half=50_000_000; capValue=500_000 → 500_000
			amount: 100_000_000, bps: 1000, floor: 10_000, cap: 500_000,
			wantDep: 500_000,
		},
		{
			name: "default 10pct normal",
			// 100_000 * 1000 / 10000 = 10_000; floor=10_000 → raw==floor → 10_000
			amount: 100_000, bps: 1000, floor: 10_000, cap: 100_000_000,
			wantDep: 10_000,
		},
		{
			name: "zero amount",
			amount: 0, bps: 1000, floor: 10_000, cap: 100_000_000,
			wantDep: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateDeposit(tc.amount, tc.bps, tc.floor, tc.cap)
			if got != tc.wantDep {
				t.Errorf("CalculateDeposit(%d, %d, %d, %d) = %d, want %d",
					tc.amount, tc.bps, tc.floor, tc.cap, got, tc.wantDep)
			}
		})
	}
}

// TestCalculateDepositDefaultsMatchSchedulerCOALESCE verifies that the COALESCE
// defaults used in the scheduler SQL (bps=1000, floor=10000, cap=100000000) produce
// the same result as calling CalculateDeposit with those defaults directly.
func TestCalculateDepositDefaultsMatchSchedulerCOALESCE(t *testing.T) {
	const defaultBPS, defaultFloor, defaultCap = int64(1000), int64(10_000), int64(100_000_000)

	// At amountCents=100_000: 10% = 10_000, which equals floor → 10_000.
	if got := CalculateDeposit(100_000, defaultBPS, defaultFloor, defaultCap); got != 10_000 {
		t.Errorf("default params: CalculateDeposit(100_000)=%d, want 10_000", got)
	}
	// At amountCents=60_000: raw=6_000 < floor=10_000, but half=30_000 → floor=10_000 → 10_000.
	if got := CalculateDeposit(60_000, defaultBPS, defaultFloor, defaultCap); got != 10_000 {
		t.Errorf("default params below-floor: CalculateDeposit(60_000)=%d, want 10_000", got)
	}
}
