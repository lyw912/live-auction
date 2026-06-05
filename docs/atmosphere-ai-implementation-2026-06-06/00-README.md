# Extreme Bidding Atmosphere And AI Implementation Pack

> Status: implementation guidance, 2026-06-06.
> Scope: focused follow-up to `docs/reviews/extreme-bidding-atmosphere-and-ai-judge-review-and-design-2026-06-05.md`.
> Non-goal: do not reopen PTS-1B hot-path architecture unless a proposed experience/AI feature violates the current contract.

## Why This Pack Exists

The current system already has a serious hot-bidding architecture, recovery semantics, and PTS evidence discipline under `docs/current/`. The remaining competitive gap is narrower and more visible:

- the live auction feels correct but not extreme;
- several social/heat signals exist but are not rendered;
- some visible controls are static or hardcoded;
- AI is absent even though it can solve real seller, host, and trust problems.

This pack turns the 2026-06-05 review into a development system: what to build, what to avoid, how to stage it, and how to prove it to judges.

## Document Map

| File | Purpose |
|---|---|
| `01-scope-and-principles.md` | Phase boundary, product principles, and non-negotiable engineering constraints. |
| `02-current-code-audit.md` | Current code facts that shape the plan; includes concrete file/line references. |
| `03-experience-system-design.md` | Bid atmosphere 2.0 design: countdown tension, heat meter, social proof, leaderboard, sounds, ceremony. |
| `04-ai-capability-design.md` | AI Listing Copilot, AI auction commentator, shill/troll sentinel, recap, and buyer Q&A. |
| `05-engineering-implementation-plan.md` | Phased implementation plan with APIs, data model, frontend tasks, feature flags, and rollback. |
| `06-test-evidence-and-acceptance.md` | Test matrix, visual/performance gates, abuse cases, and evidence required before claiming done. |
| `07-risk-compliance-and-judge-defense.md` | Dark-pattern boundary, AI safety, compliance posture, and expected judge questions. |
| `08-research-source-index.md` | External sources used to calibrate industry practice and API feasibility. |
| `09-task-board.md` | Concrete P0/P1/P2 engineering tasks and acceptance evidence. |

## Operating Rule

Every new feature in this phase must satisfy all four checks:

1. It improves the live auction atmosphere, seller workflow, host control, or trust/governance.
2. It is anchored to server-authoritative auction facts; the client and AI never invent bid truth.
3. It has a graceful degraded state when AI, audio, animation, or realtime delivery fails.
4. It ships with evidence: tests, screenshots/traces where relevant, and a judge-defense note.

## Minimum Competitive Package

If time is constrained, build this first:

1. Remove visible fake/static trust leaks: `2333`, dead buttons, unconditional claims.
2. Add a true final-countdown tension system gated by connected/recovered state.
3. Render existing heat signals: active bidders, accepted bids, price velocity, bid count.
4. Add a victory/loser ceremony before pushing payment.
5. Add AI Listing Copilot for host-side listing/rule draft generation.
6. Add AI auction commentator as text/system-barrage first; TTS is optional.
