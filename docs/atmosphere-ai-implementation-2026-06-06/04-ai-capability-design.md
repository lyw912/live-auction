# AI Capability Design

## Non-Negotiable Boundary

AI does not decide bid validity, winner, price, settlement, or payment. It can only:

- draft listing/rule content for human review;
- narrate facts already decided by the auction engine;
- flag suspicious patterns for host/platform review;
- summarize completed auctions;
- answer product/rule questions from approved data.

Every AI output must carry provenance and be safely discardable.

## Capability A: AI Listing Copilot

### User value

Sellers of collectibles, antiques, second-hand luxury goods, and similar non-standard items often struggle with listing quality and auction rule setup. The copilot reduces cold-start friction.

### Flow

1. Host uploads or selects product images and enters optional notes.
2. Backend creates an AI draft job.
3. AI returns structured JSON:
   - title candidates;
   - description;
   - condition/provenance questions to ask the seller;
   - category suggestion;
   - starting price, increment, cap, duration, extension suggestion;
   - compliance/risk flags;
   - confidence and rationale.
4. Host reviews and applies selected fields into existing create/rule forms.
5. Server validates normal auction rules before publish.

### API sketch

```http
POST /api/host/ai/listing-drafts
GET  /api/host/ai/listing-drafts/{job_id}
POST /api/host/ai/listing-drafts/{job_id}/apply
```

Request fields:

```json
{
  "room_id": "room_main",
  "image_urls": ["..."],
  "seller_notes": "清代风格瓷杯，有证书",
  "target_category": "collectibles"
}
```

Safety:

- output is a draft, not published truth;
- "authentic", "guaranteed", "certified", and age/material claims require seller-provided evidence fields;
- price/rule suggestions are labeled estimates;
- all applied values pass existing backend validation.

## Capability B: AI Auction Commentator

### User value

Small sellers may not have a skilled host. A commentator creates short, factual, high-energy system messages from real engine events.

### Input facts

- `auction_id`;
- `engine_seq` or outbox seq;
- event type: accepted bid, new leader, outbid, extension, cap reached, sold, ended;
- current price, minimum next bid, extension duration, active bidder count, accepted bids in 30s;
- item title and approved selling points.

### Output contract

```json
{
  "auction_id": "auc_live",
  "source_seq": 123,
  "style": "high_energy",
  "body": "最后5秒又被抬到¥650，当前领先已经换人。",
  "facts_used": ["current_price_cents", "source_seq", "current_winner_masked"],
  "safety_labels": []
}
```

Rules:

- max 40 Chinese characters for stage messages;
- no fabricated buyer count, stock, discount, endorsement, or urgency;
- no direct pressure such as "不买就亏";
- no mention of hidden max bids;
- if AI fails or times out, use deterministic templates.

Delivery:

- write as `SYSTEM_AI` or equivalent source into the same user-visible stream as chat/commentary;
- include provenance so tests and judges can distinguish AI-generated commentary from human chat.

## Capability C: Shill/Troll Sentinel

### User value

Auction trust is a product feature. Live auction platforms explicitly monitor shill bidding and troll bidding because fake bids destroy buyer/seller confidence.

### Scope

This is a sidecar risk scorer, not a bid blocker in P0.

Inputs:

- bid stream aggregates;
- user/room relation features available in the system;
- device/IP/session features only if already collected and privacy-approved;
- repeated high outlier bids, rapid withdraw/expire behavior, payment failure after extreme bid, seller-related account signals.

Outputs:

- host alert;
- monitor incident;
- recommended action: watch, verify deposit, pause auction, cancel abnormal auction;
- score explanation using aggregate features.

Implementation route:

1. P0/P1: rules and aggregate heuristics, no LLM required.
2. P2: AI judge on aggregate summaries for explanation quality, never raw secret leakage to the client.

## Capability D: AI Auction Recap

After terminal state and settlement evidence:

- host recap: demand strength, price velocity, bidder funnel, extension moments, next-rule suggestions;
- buyer recap: shareable highlight card without exposing private bidder identities;
- demo artifact: useful for judge walk-through.

This is batch/offline and low risk.

## Capability E: Buyer Product Q&A

Answer product/rule questions from approved listing data, rules, certificate fields, and seller notes.

Guardrails:

- if information is missing, answer "未提供";
- do not make authenticity, investment return, medical/legal, or platform policy claims;
- never reveal private max bids, hidden risk scores, or non-public user data.

## Provider Boundary

Do not couple code to one vendor in UI or business handlers. Add an internal provider interface:

```go
type Generator interface {
    GenerateStructured(ctx context.Context, req StructuredRequest) (StructuredResult, error)
}
```

Use structured JSON schemas for drafts/commentary/sentinel explanations. OpenAI's current public API documentation supports structured outputs and image inputs; use official docs when implementing exact request shapes, because model names and parameters are time-sensitive.
