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

Current 2026-06-06 evidence:

- `TestAIListingDraftIsHostOnlyStructuredAndApplyIsAuditOnly` passed against local PostgreSQL/Redis after migration.
- PC Playwright MCP generated a real deterministic/local-template draft through `/api/host/ai/listing-drafts`; drawer showed `SUCCEEDED`, provider/model, title candidates, description, rule suggestion, and evidence flags.
- UI apply fills the local create/rule form only; publishing still requires existing create/save actions and backend validation.

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

Current 2026-06-06 evidence:

- `TestAICommentarySystemMessagesSentinelRecapAndProductQA` passed for commentary creation and system-message readback.
- PC Playwright MCP clicked `生成解说`; Live Assist showed a generated message with source seq and factual price.
- H5 Playwright MCP displayed that AI system message in the live chat overlay.

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

Current 2026-06-06 evidence:

- `TestAICommentarySystemMessagesSentinelRecapAndProductQA` seeded rejected-bid probing and verified sentinel alert creation.
- PC Live Assist has a real `检查风控` action and renders severity, score, explanation, and recommended action.
- Current sentinel is deterministic aggregate rules only; no LLM explanation and no automatic bid blocking.

## Recap And Product Q&A Tests

Current 2026-06-06 evidence:

- `TestAICommentarySystemMessagesSentinelRecapAndProductQA` verifies recap generation and product Q&A from auction facts.
- H5 Playwright MCP opened the real `问答` sheet and asked `起拍价是多少`; response was `起拍价 ¥100.00` with fact provenance and no hidden-bid leakage.

## Commands Run On 2026-06-06

```bash
make backend-migrate-up
/bin/bash -lc "GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestAI'"
pnpm build
pnpm test:frontend:domain
```

Notes:

- A non-escalated gateway test run failed because the sandbox blocked local Redis/PostgreSQL sockets; the same targeted AI tests passed with local service access.
- Browser console noise observed during MCP checks was limited to missing `favicon.ico`.

## Judge Evidence Packet

Before demoing this phase, prepare:

- screenshots or video of normal, critical countdown, extension, outbid, sold winner, sold loser, and recovering states;
- one AI Listing Copilot job record with prompt version and structured output;
- one AI Commentator message tied to an engine/outbox seq;
- one risk alert synthetic scenario if sentinel is included;
- a short "AI is not the auction judge" explanation with code path;
- a known-limits note listing any disabled or template-only AI features.
