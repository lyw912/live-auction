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

- `go test ./internal/ai` passed for the relay Chat Completions adapter request shape, strict `json_schema`, HTTPS-only image forwarding, and malformed-content rejection.
- `TestAIListingDraftIsHostOnlyStructuredAndApplyIsAuditOnly` passed against local PostgreSQL/Redis after migration.
- PC drawer now accepts merchant notes, category, and product-image upload. Uploaded images are sent to the provider only when the resulting object URL is HTTPS/provider-fetchable; local HTTP MinIO URLs remain visible in the form and generate a text-only draft warning.
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
- The same test now verifies automatic commentary creation through `CreateAutoCommentary`, including source-seq dedupe and `auto_generated` safety marking.
- The same test now verifies host-only `GET/PATCH /api/host/auctions/{id}/ai-settings`; disabling `auto_commentary_enabled` stops `CreateAutoCommentary`, while manual host commentary remains available.
- PC Playwright MCP clicked `生成解说`; Live Assist showed a generated message with source seq and factual price.
- H5 Playwright MCP displayed that AI system message in the live chat overlay.
- Bid gateway auto-commentary remains non-blocking: accepted/sold bid responses are written first, then a bounded background task writes a system message. Provider-backed generation is used only when `API_KEY` is configured; deterministic fallback remains explicit.

## Compliant Live-Ops Tests

Current 2026-06-06 evidence:

- `pnpm --filter mobile-h5 build` passed after adding `LiveOpsPanel`.
- Browser test subset passed: `/bin/bash -lc "H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line -g 'honest heat|leaderboard sheet|realtime leaderboard'"`.
- The H5 test clicks warm-up task buttons, buyer-team PK controls, and entry/leader effect card. Each opens an existing real sheet or changes visible local state; no button is decorative.
- The live-ops copy states that warm-up has no lottery/promised reward and buyer PK does not affect price or winner.
- Real-browser testing exposed and fixed two UX defects: entry effect was initially hidden behind the fixed chat composer, and clicking an already-followed entry card toggled follow off instead of opening leaderboard.

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
- H5 now renders a buyer-safe result recap/share card from public state facts only. It does not call host-only recap APIs and does not expose buyer identity or private max-bid data.

## Commands Run On 2026-06-06

```bash
make backend-migrate-up
/bin/bash -lc "GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestAI'"
pnpm build
pnpm test:frontend:domain
pnpm --filter mobile-h5 build
pnpm --filter pc-console build
/bin/bash -lc "GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestAI|TestPlaceBid'"
/bin/bash -lc "H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/mobile-h5.spec.ts tests/e2e/atmosphere-engine.spec.ts --project=mobile-h5 --reporter=line -g 'honest heat|bottom sheets open close|official bid hints|countdown|result|leaderboard'"
/bin/bash -lc "H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/pc-console.spec.ts tests/e2e/pc-console-live.spec.ts --project=pc-console --reporter=line -g 'AI|Live Assist|sentinel|recap|commentary|draft'"
```

Notes:

- A non-escalated gateway test run failed because the sandbox blocked local Redis/PostgreSQL sockets; the same targeted AI tests passed with local service access.
- Browser console noise observed during MCP checks was limited to missing `favicon.ico`.
- Playwright MCP screenshot/text inspection on 2026-06-06 confirmed the PC AI 场控 panel renders `生成解说`, `检查风控`, `生成复盘`, and `隐藏自动` as usable controls with compact layout.
- H5 Playwright MCP inspected the real room surface; first-load `401 /api/auth/me` is the expected pre-login probe, and business controls were rendered without fake viewer count.

## Judge Evidence Packet

Before demoing this phase, prepare:

- screenshots or video of normal, critical countdown, extension, outbid, sold winner, sold loser, and recovering states;
- one AI Listing Copilot job record with prompt version and structured output;
- one AI Commentator message tied to an engine/outbox seq;
- one risk alert synthetic scenario if sentinel is included;
- a short "AI is not the auction judge" explanation with code path;
- a known-limits note listing any disabled or template-only AI features.
