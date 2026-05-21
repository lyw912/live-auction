package auction

import (
	apierrors "live-auction/backend/internal/platform/errors"
)

type RuleInput struct {
	StartPriceCents         int64
	IncrementCents          int64
	CapPriceCents           *int64
	DurationSeconds         int
	ExtendWindowSeconds     int
	ExtendBySeconds         int
	MaxExtendCount          int
	FatFingerThresholdCents *int64
}

type RuleViolation struct {
	Code          apierrors.Code `json:"code"`
	Message       string         `json:"message"`
	Violations    []string       `json:"violations,omitempty"`
	SuggestedCaps []int64        `json:"suggested_caps,omitempty"`
}

func ValidateRule(input RuleInput) []RuleViolation {
	var violations []RuleViolation

	add := func(message string) {
		violations = append(violations, RuleViolation{
			Code:    apierrors.CodeInvalidAuctionRule,
			Message: message,
		})
	}

	if input.StartPriceCents < 0 {
		add("start_price_cents must be >= 0")
	}
	if input.IncrementCents <= 0 {
		add("increment_cents must be > 0")
	}
	if input.DurationSeconds < 30 || input.DurationSeconds > 86400 {
		add("duration_seconds must be between 30 and 86400")
	}
	if input.ExtendWindowSeconds < 10 || input.ExtendWindowSeconds > 30 {
		add("extend_window_seconds must be between 10 and 30")
	}
	if input.ExtendBySeconds < 10 || input.ExtendBySeconds > 30 {
		add("extend_by_seconds must be between 10 and 30")
	}
	if input.MaxExtendCount < 1 || input.MaxExtendCount > 10 {
		add("max_extend_count must be between 1 and 10")
	}
	if input.FatFingerThresholdCents != nil && input.IncrementCents > 0 && *input.FatFingerThresholdCents <= input.IncrementCents {
		add("fat_finger_threshold_cents must be > increment_cents")
	}

	if input.CapPriceCents != nil && input.IncrementCents > 0 {
		minCap := input.StartPriceCents + input.IncrementCents
		if *input.CapPriceCents < minCap {
			violations = append(violations, RuleViolation{
				Code:       apierrors.CodeInvalidAuctionRuleCapUnreachable,
				Message:    "cap_price_cents must be >= start_price_cents + increment_cents",
				Violations: []string{"cap_price_cents must be reachable by at least one bid"},
				SuggestedCaps: []int64{
					minCap,
					minCap + input.IncrementCents,
				},
			})
		} else if (*input.CapPriceCents-input.StartPriceCents)%input.IncrementCents != 0 {
			lower := *input.CapPriceCents - ((*input.CapPriceCents - input.StartPriceCents) % input.IncrementCents)
			upper := lower + input.IncrementCents
			violations = append(violations, RuleViolation{
				Code:       apierrors.CodeInvalidAuctionRuleCapUnreachable,
				Message:    "cap_price_cents must align to increment grid",
				Violations: []string{"cap_price_cents must satisfy (cap-start) % increment == 0"},
				SuggestedCaps: []int64{
					lower,
					upper,
				},
			})
		}
	}

	return violations
}

type BidClass string

const (
	BidClassAccepted          BidClass = "ACCEPTED"
	BidClassAcceptedSold      BidClass = "ACCEPTED_SOLD"
	BidClassTooLow            BidClass = "BID_TOO_LOW"
	BidClassIncrementMismatch BidClass = "BID_INCREMENT_MISMATCH"
	BidClassAboveCap          BidClass = "BID_ABOVE_CAP"
)

func ClassifyBidAmount(startPriceCents, currentPriceCents, incrementCents int64, capPriceCents *int64, amountCents int64, hasAcceptedBid bool) BidClass {
	if capPriceCents != nil && amountCents > *capPriceCents {
		return BidClassAboveCap
	}

	base := currentPriceCents
	if !hasAcceptedBid {
		base = startPriceCents
	}
	minimum := base + incrementCents
	if amountCents < minimum {
		return BidClassTooLow
	}
	if (amountCents-base)%incrementCents != 0 {
		return BidClassIncrementMismatch
	}
	if capPriceCents != nil && amountCents == *capPriceCents {
		return BidClassAcceptedSold
	}
	return BidClassAccepted
}

func CalculateExtension(currentEndAtUnix int64, serverNowUnix int64, extendWindowSeconds int, extendBySeconds int, extendCount int, maxExtendCount int) int64 {
	if extendCount >= maxExtendCount {
		return currentEndAtUnix
	}
	if currentEndAtUnix-serverNowUnix > int64(extendWindowSeconds) {
		return currentEndAtUnix
	}
	candidate := serverNowUnix + int64(extendBySeconds)
	if candidate > currentEndAtUnix {
		return candidate
	}
	return currentEndAtUnix
}
