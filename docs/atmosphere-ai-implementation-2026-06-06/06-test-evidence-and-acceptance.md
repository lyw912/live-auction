# Test Evidence And Acceptance

## Evidence Standard

Do not claim "extreme atmosphere" or "AI implemented" from screenshots alone. Each claim needs:

- code path;
- automated test or live smoke;
- stale/recovery behavior;
- visual or trace evidence when UI/animation is involved;
- explicit known limits.

## P0 Test Matrix

| Feature | Required proof |
|---|---|
| No fake viewer count | Playwright asserts `2333` is absent and real/fallback label is present. |
| Action rail honesty | Each visible button has a real behavior or is absent; Playwright clicks all visible controls. |
| Countdown phases | Unit tests for normal/hot/critical/hammer/syncing with server-time inputs. |
| Recovery suppression | H5 test forces recovering/disconnected state and verifies no tension class, sound trigger, or ceremony. |
| Heat meter | Contract test verifies fields render from leaderboard/heat payload and fallback is honest when absent. |
| Leaderboard bid count | Component test/Playwright verifies `bid_count` appears and does not break layout. |
| Cue ID monotonicity | Unit test produces multiple cues in one millisecond and verifies stable independent dismissal. |

## P1 UI And Performance Gates

Visual states:

- active normal;
- active hot countdown;
- active critical countdown;
- self leading;
- outbid;
- extended;
- recovering;
- sold winner ceremony;
- sold loser ceremony;
- reduced-motion variant.

Performance gates:

- no layout-shifting leaderboard rows after animation completes;
- no text overlap on mobile viewport;
- local UI trace shows no long animation task introduced by particles;
- canvas effects pause when hidden or reduced-motion.

Recommended commands, adjusted to actual project scripts:

```bash
pnpm test:e2e -- tests/e2e/visual-regression.spec.ts
node tests/perf/run-ui-performance-trace.mjs
pnpm build:h5
```

## AI Listing Copilot Tests

Backend:

- host-only auth;
- disabled flag response;
- provider timeout returns job failure, not server crash;
- malformed provider JSON rejected;
- generated draft cannot bypass auction rule validation;
- authenticity/condition claims require evidence fields;
- prompt version and output persisted.

Frontend:

- draft loading/error/success states;
- field-level apply;
- safety flags visible;
- manual edits after apply still work;
- no auto-publish.

Evidence:

- sample request/response with fake provider;
- screenshot of draft review UI;
- backend test output.

## AI Commentator Tests

Scenarios:

- accepted bid from another user;
- current user becomes leader;
- final-window extension;
- sold event;
- ended/no sale;
- duplicate event replay;
- AI timeout/fallback;
- disabled flag.

Assertions:

- message references source seq;
- no duplicate for same source seq;
- no hidden max-bid or private risk data;
- no fake viewer count, fake discount, or unsupported urgency;
- deterministic fallback appears within bounded time.

## Sentinel Tests

High-risk cases:

- seller-related account bids repeatedly but never pays;
- unrealistic high troll bid followed by payment expiration;
- multiple accounts from same risk cluster push price without purchase;
- normal final-second competition with many real bidders.

Assertions:

- suspicious scenarios create alert with explanation;
- normal competition does not create alert;
- alerts are visible in PC console/monitor;
- no automatic bid rejection unless explicitly designed and documented.

## Judge Evidence Packet

Before demoing this phase, prepare:

- screenshots or video of normal, critical countdown, extension, outbid, sold winner, sold loser, and recovering states;
- one AI Listing Copilot job record with prompt version and structured output;
- one AI Commentator message tied to an engine/outbox seq;
- one risk alert synthetic scenario if sentinel is included;
- a short "AI is not the auction judge" explanation with code path;
- a known-limits note listing any disabled or template-only AI features.
