package ai

import (
	"context"
	"encoding/json"
)

type DeterministicGenerator struct{}

func (DeterministicGenerator) GenerateStructured(ctx context.Context, req StructuredRequest) (StructuredResult, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	select {
	case <-ctx.Done():
		return StructuredResult{}, ctx.Err()
	default:
	}
	switch req.Kind {
	case "listing_draft":
		listingReq := ListingDraftRequest{
			RoomID:         stringFromInput(req.Input, "room_id"),
			ImageURLs:      stringSlice(req.Input["image_urls"]),
			SellerNotes:    stringFromInput(req.Input, "seller_notes"),
			TargetCategory: stringFromInput(req.Input, "target_category"),
		}
		draft := BuildFallbackListingDraft(listingReq)
		return resultFromStruct("deterministic", "local-template", draft, map[string]any{
			"provider_mode":         "deterministic",
			"human_review_required": true,
			"no_auto_publish":       true,
		}), nil
	case "auction_commentary":
		commentReq := CommentaryRequest{
			RoomID:              stringFromInput(req.Input, "room_id"),
			AuctionID:           stringFromInput(req.Input, "auction_id"),
			SourceSeq:           int64Value(req.Input["source_seq"]),
			EventType:           stringFromInput(req.Input, "event_type"),
			ItemTitle:           stringFromInput(req.Input, "item_title"),
			CurrentPriceCents:   int64Value(req.Input["current_price_cents"]),
			CurrentWinnerMasked: stringFromInput(req.Input, "current_winner_masked"),
			ActiveBidders30s:    int64Value(req.Input["active_bidders_30s"]),
			AcceptedBids30s:     int64Value(req.Input["accepted_bids_30s"]),
		}
		body, style, safety := BuildCommentary(commentReq)
		return StructuredResult{
			Provider: "deterministic",
			Model:    "local-template",
			Output: map[string]any{
				"auction_id":    commentReq.AuctionID,
				"source_seq":    commentReq.SourceSeq,
				"style":         style,
				"body":          body,
				"facts_used":    safety["facts_used"],
				"safety_labels": []string{},
			},
			Safety: safety,
		}, nil
	case "sentinel_explanation":
		input := SentinelEvaluationInput{
			RoomID:     stringFromInput(req.Input, "room_id"),
			AuctionID:  stringFromInput(req.Input, "auction_id"),
			ItemTitle:  stringFromInput(req.Input, "item_title"),
			Status:     stringFromInput(req.Input, "status"),
			Features:   mapFromInput(req.Input, "features"),
			Candidates: sentinelCandidatesFromInput(req.Input["candidates"]),
		}
		alerts := NormalizeSentinelAlerts(map[string]any{}, input)
		return StructuredResult{
			Provider: "deterministic",
			Model:    "local-template",
			Output: map[string]any{
				"alerts": structToAnySlice(alerts),
			},
			Safety: map[string]any{
				"provider_mode":        "deterministic",
				"aggregate_facts_only": true,
				"no_auto_block":        true,
			},
		}, nil
	case "product_qa":
		facts := mapFromInput(req.Input, "facts")
		allowedFacts := map[string]string{}
		for key := range facts {
			allowedFacts[key] = key
		}
		answer := AnswerFromFacts(
			stringFromInput(req.Input, "auction_id"),
			stringFromInput(req.Input, "question"),
			allowedFacts["item.title"],
			allowedFacts["item.description"],
			map[string]any{
				"start_price_cents": int64Value(facts["auction.start_price_cents"]),
				"increment_cents":   int64Value(facts["auction.increment_cents"]),
				"cap_price_cents":   int64Value(facts["auction.cap_price_cents"]),
			},
		)
		return resultFromStruct("deterministic", "local-template", answer, map[string]any{
			"provider_mode":          "deterministic",
			"approved_facts_only":    true,
			"no_private_bid_data":    true,
			"no_authenticity_claims": true,
		}), nil
	default:
		return StructuredResult{
			Provider: "deterministic",
			Model:    "local-template",
			Output:   map[string]any{},
			Safety: map[string]any{
				"provider_mode": "deterministic",
			},
		}, nil
	}
}

type DisabledGenerator struct{}

func (DisabledGenerator) GenerateStructured(context.Context, StructuredRequest) (StructuredResult, error) {
	return StructuredResult{}, ErrDisabled
}

func resultFromStruct(provider string, model string, value any, safety map[string]any) StructuredResult {
	return StructuredResult{
		Provider: provider,
		Model:    model,
		Output:   structToMap(value),
		Safety:   safety,
	}
}

func structToMap(value any) map[string]any {
	raw, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func mapToStruct(input map[string]any, out any) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func stringFromInput(input map[string]any, key string) string {
	return cleanText(stringValue(input[key]), 240)
}

func mapFromInput(input map[string]any, key string) map[string]any {
	value, ok := input[key].(map[string]any)
	if !ok || value == nil {
		return map[string]any{}
	}
	return value
}

func sentinelCandidatesFromInput(value any) []SentinelAlert {
	raw, _ := json.Marshal(value)
	var out []SentinelAlert
	_ = json.Unmarshal(raw, &out)
	return out
}

func structToAnySlice(value any) []any {
	raw, _ := json.Marshal(value)
	var out []any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		return []any{}
	}
	return out
}
