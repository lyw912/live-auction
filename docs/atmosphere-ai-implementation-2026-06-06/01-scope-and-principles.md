# Scope And Principles

## Current Phase Boundary

This phase is not a rewrite of the official brief or the PTS architecture. Treat these as governing background:

- official scope and scoring context: `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md`;
- hot-path and recovery authority: `docs/current/architecture.md`;
- latency/correctness/evidence authority: `docs/current/performance-correctness-contract.md`;
- product/AI gap review: `docs/reviews/extreme-bidding-atmosphere-and-ai-judge-review-and-design-2026-06-05.md`.

The work here focuses on the official "extreme bidding atmosphere" and optional AI innovation surfaces.

## Product North Star

Build a live auction that feels intense because the competition is real:

- real countdown, not fake urgency;
- real bidder activity, not fake social proof;
- real ranking and outbid feedback, not optimistic client guesses;
- real host tools and AI assistance, not decorative buttons;
- real trust controls for shill/troll bidding, not only visual hype.

The judge should be able to click any visible control, interrupt the network, ask "who decided this fact?", and get a defensible answer.

## Engineering Principles

1. AI is assistant, narrator, or sentinel. It is never the auction judge.
2. The Redis/Kafka/PG decision chain remains untouched by UI atmosphere features.
3. AI and generated commentary are asynchronous or pre-live. They must not add latency to manual bid decisions.
4. UI intensity is suppressed when state is stale, recovering, disconnected, paused, reconciling, cancelled, or terminal-uncertain.
5. Every cue must carry enough provenance to defend it: `auction_id`, `engine_seq` or source event seq, source type, and generated/rendered timestamp.
6. The frontend may animate facts, but it must not create facts.
7. Prefer small, shippable slices behind flags over one large "atmosphere refactor".

## Anti-Goals

- Do not add fake viewer counts, fake scarcity, fake "someone just bought" messages, or fake countdown extensions.
- Do not use AI to recommend a user bid more aggressively in a way that targets vulnerability or age.
- Do not generate item authenticity claims that the seller did not provide.
- Do not play sound by default; require user activation and an obvious mute state.
- Do not add DOM-heavy animation that breaks mobile frame rate.
- Do not create a second bid-state model in the client for atmosphere features.

## Feature Flags

Use flags so demo and fallback behavior are explicit:

| Flag | Default | Purpose |
|---|---:|---|
| `VITE_ATMOSPHERE_V2` | off until verified | Enables countdown tension, heat meter, ceremony, and action rail. |
| `VITE_AI_COMMENTARY` | off until backend ready | Enables AI/system commentary rendering. |
| `AI_LISTING_COPILOT_ENABLED` | off | Enables host-side AI listing draft endpoint. |
| `AI_COMMENTARY_ENABLED` | off | Enables commentary worker/generator. |
| `AI_SENTINEL_ENABLED` | off | Enables shill/troll risk scoring and host alerts. |

Flags are not a substitute for correctness. They are for staged rollout, demo recovery, and judge-visible honesty.

## Definition Of Done

A feature is done only when all are true:

- the happy path works against the live backend or is explicitly documented as UI-only;
- the real rendered UI has been inspected with Playwright MCP or Playwright screenshots/clicks when the change is user-facing;
- stale/recovering/disconnected behavior is tested;
- reduced-motion and sound-off modes are tested for visual coherence;
- there is no hardcoded social proof or invisible static demo data;
- every visible button is useful and interactive; disabled controls must explain the real unavailable state;
- the evidence is linked from `06-test-evidence-and-acceptance.md` or a follow-up evidence file.

## Frontend Development Reminders

- Use Playwright MCP during development for real rendering and click paths, not only API or unit tests.
- Keep merchant, bidder, and host-facing UI clean and non-technical. Diagnostics may expose operational terms, but normal users should not see implementation jargon.
- Use tabs, drawers, sheets, cards, and modals to keep dense features usable, but only when the grouping matches the workflow. Do not split pages or create cards just to make the UI look busy.
- Keep the visual language unified: consistent colors, spacing, typography, and motion. Avoid adding one-off styles for a single feature unless they are promoted into reusable tokens/patterns.
- Small surfaces should do one job well. If a card or drawer starts collecting unrelated controls, split the workflow or remove secondary actions.
- The implementation loop for each phase is: develop, self-review, test with real UI interaction, update docs/evidence, then commit when requested.
