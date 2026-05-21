package auction

func CalculateDeposit(amountCents int64, bps int64, floorCents int64, capCents int64) int64 {
	if amountCents <= 0 {
		return 0
	}
	raw := amountCents * bps / 10_000
	half := amountCents / 2
	floor := minInt64(floorCents, half)
	capValue := minInt64(capCents, half)
	return minInt64(maxInt64(raw, floor), capValue)
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
