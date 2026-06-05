# Engineering Implementation Plan

## Phase 0: Safety Rails And Visible Honesty

Goal: remove demo-breaking issues before adding spectacle.

Frontend:

- replace `2333` with a real field or remove it;
- bind static labels to item/auction metadata or remove them;
- remove unconditional trust chips;
- implement or remove follow/like/gift/more buttons;
- add a monotonic cue ID generator;
- change outbid cue logic to compare event winner transition against current user rather than stale leaderboard-only state.

Backend/API:

- verify whether H5 leaderboard response already includes heat fields;
- if not, add bidder-safe heat fields to the existing leaderboard/snapshot response rather than exposing host-only heat endpoints.

Tests:

- Playwright checks for no hardcoded `2333`;
- route-mocked and live-backend H5 tests for connected/recovering/disconnected atmosphere suppression;
- unit/domain test for cue ID monotonicity and countdown phase derivation.

## Phase 1: Atmosphere V2 Core

Goal: make real tension visible.

Frontend:

- add `deriveCountdownPhase()` returning phase and remaining ms;
- set `data-countdown-phase` on `LiveStage`, floating product card, and bid dock;
- add `HeatMeter` component using active bidders, accepted bids, velocity, and total bids;
- show `bid_count` in leaderboard rows;
- add opt-in critical countdown sound/haptics;
- keep reduced-motion and recovery gates.

Performance:

- memoize heavy child components;
- consider moving 100ms ticking to countdown/phase components;
- ensure particles/heart bursts are capped.

Tests:

- visual regression for normal/hot/critical/recovering/sold states;
- reduced-motion screenshot;
- small local performance trace before claiming demo-ready.

## Phase 2: Ceremony And Realtime Social Proof

Goal: make the hammer moment memorable and rank changes legible.

Frontend:

- add `HammerCeremony` overlay triggered only by server terminal events;
- add winner/loser/unsold/cancelled variants;
- add confetti canvas with particle cap and reduced-motion fallback;
- add FLIP leaderboard transitions;
- add chat/system-message stage lane with provenance labels.

Backend/API:

- include enough terminal event metadata for ceremony: winner scope, terminal price, accepted bidder count if real, item image/title;
- if necessary, add a low-risk event type/source for system commentary.

Tests:

- local countdown zero must not trigger ceremony without server event;
- sold winner and sold loser visual snapshots;
- no animation during recovering.

## Phase 3: AI Listing Copilot

Goal: ship the highest-ROI AI feature first.

Backend:

- add `internal/ai` provider boundary;
- add structured schemas for listing drafts;
- add `ai_listing_draft_jobs` table or reuse a generic `ai_generation_jobs` table;
- add host-only endpoints;
- add timeout, retry, cache/dedupe by image hash and notes hash;
- store prompt version, model/provider, input references, output JSON, safety flags, and reviewer/apply status.

Frontend PC:

- add "AI Draft" action in item creation/rule setup;
- show draft fields side-by-side with current form values;
- host explicitly applies fields;
- show safety flags and missing-evidence questions.

Tests:

- provider fake returns structured draft;
- invalid output is rejected;
- host-only auth enforced;
- applying draft still passes normal rule validation;
- no generated authenticity claim can be applied without evidence.

## Phase 4: AI Auction Commentator

Goal: add live AI wow without hot-path risk.

Backend:

- event consumer reads decided outbox/engine events after decision;
- rate-limit by auction and event type;
- generate short structured message or deterministic fallback;
- store output with source event seq and facts used;
- publish as system message.

Frontend:

- render AI/system messages with distinct style;
- allow host to enable/disable;
- allow user to mute AI voice if TTS is later added.

Tests:

- no commentary for stale/recovered replay if already generated for seq;
- timeout falls back to template;
- generated text cannot contain hidden max-bid values or unsupported claims;
- disabling flag stops generation without breaking chat.

## Phase 5: Sentinel And Recap

Goal: improve platform trust and judge defense.

Backend:

- add aggregate risk scorer from bid/order/activity events;
- surface host alerts and monitor incidents;
- start with deterministic rules;
- optional LLM explanation runs only on aggregate summaries.

Frontend PC:

- show risk alert with explanation and recommended operator actions;
- link to cancel/pause/reconcile flows where already supported.

Tests:

- synthetic shill/troll patterns produce alert;
- normal final-second competition does not alert;
- risk alert cannot block a bid unless an explicit host/platform action is taken.

## Data Model Candidates

Keep tables generic enough to avoid future migration churn:

```sql
ai_generation_jobs(
  id,
  room_id,
  auction_id,
  kind,
  status,
  input_hash,
  prompt_version,
  provider,
  model,
  input_json,
  output_json,
  safety_json,
  reviewed_by,
  applied_at,
  created_at,
  updated_at
)
```

```sql
auction_system_messages(
  id,
  room_id,
  auction_id,
  source,
  source_seq,
  body,
  facts_json,
  safety_json,
  created_at
)
```

Use existing chat/outbox paths if they already satisfy ordering, recovery, and provenance needs. Do not add a parallel realtime delivery system unless necessary.

## Rollback

Every phase must be independently disableable:

- UI flags hide atmosphere/AI surfaces;
- backend flags return `404` or disabled response for AI endpoints;
- workers can be stopped without bid-path impact;
- deterministic templates cover commentator AI failures.
