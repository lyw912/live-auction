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

- `go test ./internal/ai` passed for the relay Chat Completions adapter request shape, strict `json_schema`, HTTPS image forwarding, safe local `data:image/*;base64` forwarding, text-payload image-data redaction, and malformed-content rejection.
- `TestAIListingDraftIsHostOnlyStructuredAndApplyIsAuditOnly` passed against local PostgreSQL/Redis after migration.
- PC drawer now accepts merchant notes, category, and product-image upload. Small local uploads are sent to the provider as safe image data URLs while still being saved to the item form; HTTPS image URLs are also forwarded. Oversized local images are not silently sent and require merchant notes or an HTTPS image URL.
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
- The same test now verifies provider-backed automatic commentary: a fake provider result is used for `auction_commentary`, persisted as the job provider/model, and still marked `auto_generated`.
- The same test now verifies host-only `GET/PATCH /api/host/auctions/{id}/ai-settings`; disabling `auto_commentary_enabled` stops `CreateAutoCommentary`, while manual host commentary remains available.
- PC Playwright MCP clicked `生成解说`; Live Assist showed a generated message with source seq and factual price.
- H5 Playwright MCP displayed that AI system message in the live chat overlay.
- Bid gateway auto-commentary remains non-blocking: accepted/sold bid responses are written first, then a bounded background task writes a system message. Provider-backed generation is used when `API_KEY` is configured; deterministic fallback remains explicit.

## Compliant Live-Ops Tests

Current 2026-06-06 evidence:

- Migrations `202606060003_liveops_campaigns.sql` and `202606060004_liveops_team_choices.sql` applied with `make backend-migrate-up`.
- Backend integration: `GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestLiveOps'` passed against local Postgres/Redis. It covers GET auto-create, persisted task completion, idempotent duplicate completion, persisted buyer-team selection/counts, invalid task/team rejection, and room ACL denial.
- Regression integration: `GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestLiveOps|TestAICommentarySystemMessagesSentinelRecapAndProductQA|TestSentinelAndProductQAUseProviderWithFactGuards'` passed.
- `pnpm --filter mobile-h5 build` and `pnpm test:frontend:domain` passed after replacing local warm-up state with server progress.
- Real-browser subset passed: `H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line -g 'live actions|product QA completes liveops'`.
- The H5 test clicks warm-up task buttons, buyer-team PK controls, entry/leader effect card, and product Q&A. Watch/follow/leaderboard/ask completion is asserted through `/api/rooms/room_main/liveops/tasks/{task_key}` calls and server-returned progress; buyer-team PK is asserted through `/api/rooms/room_main/liveops/team`; no button is decorative.
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
- `TestSentinelAndProductQAUseProviderWithFactGuards` verifies provider-backed sentinel explanation is used when returned risk types match deterministic candidates, writes a `sentinel_explanation` job, and marks alert features with the linked AI job id.
- PC Live Assist has a real `检查风控` action and renders severity, score, explanation, and recommended action.
- Real browser check on 2026-06-06 clicked PC `检查风控`; `/api/host/auctions/auc_live/sentinel-evaluate` returned `200` with a provider-backed `sentinel_explanation` job using `chat_completions_adapter`. The checked live auction had no candidate anomalies, so `items` was correctly empty.
- Sentinel still uses deterministic aggregate rules as the candidate generator, but explanation/scoring copy can now be provider-backed. It does not use hidden max-bid data and does not automatically block bids.

## Recap And Product Q&A Tests

Current 2026-06-06 evidence:

- `TestAICommentarySystemMessagesSentinelRecapAndProductQA` verifies recap generation and product Q&A from auction facts.
- `TestSentinelAndProductQAUseProviderWithFactGuards` verifies provider-backed product Q&A uses only approved fact keys, persists a `product_qa` job, carries `thread_id` and `recent_turns` for follow-up questions, records `context_turn_count`, and falls back when the provider returns unsafe authenticity/investment claims or unapproved fact references.
- H5 Playwright MCP opened the real `问答` sheet and asked `起拍价是多少`; response was `起拍价 ¥100.00` with fact provenance and no hidden-bid leakage.
- Real browser check on 2026-06-06 clicked H5 `问拍品`, asked `起拍价和加价是多少`, and verified the visible answer `起拍价是¥100.00，加价幅度是¥50.00。` with `auction.start_price_display` and `auction.increment_display` provenance. The backend returned a `SUCCEEDED` `product_qa` job from `chat_completions_adapter`.
- Real Chromium check on 2026-06-06 opened H5 at `5298`, clicked `问拍品`, asked `起拍价和加价是多少`, clicked the generated follow-up `有封顶价吗？`, and verified 2 visible Q&A turns. The second backend response carried `recent_turns` with the first question/answer, `context_turn_count: 1`, and a provider-backed `chat_completions_adapter` `product_qa` job. Screenshot evidence: `docs/atmosphere-ai-implementation-2026-06-06/evidence/h5-multiturn-product-qa-2026-06-06.png`.
- H5 now renders a buyer-safe result recap/share card from public state facts only, with real copy, downloadable SVG highlight-card, and browser-generated WebM highlight-video actions. It does not call host-only recap APIs and does not expose buyer identity or private max-bid data.
- `H5 winner result sheet locks order and shares the single payment path` now clicks `复制`, `高光卡`, and `短视频`, verifies copy/download feedback, and waits for real browser downloads ending in `highlight.svg` and `highlight.webm`.
- Real-browser subset passed after WebM generation: `H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line -g 'winner result sheet locks order'`.

## H5 Atmosphere Interaction Evidence

Current 2026-06-06 evidence:

- `pnpm --filter mobile-h5 build` passed after adding measured FLIP leaderboard rows, system barrage, generated WAV sound assets, layered cue audio, heartbeat bed loop, and browser TTS helpers.
- Real browser check on 2026-06-06 loaded H5 at `5298`, found the `.system-barrage-layer`, and saved screenshot evidence to `docs/atmosphere-ai-implementation-2026-06-06/evidence/h5-barrage-flip-2026-06-06.png`.
- The barrage layer renders actual `auction_system_messages` content only; it does not synthesize fake heat, fake viewers, or decorative inactive controls.
- FLIP row animation uses measured DOM positions and Web Animations; `prefers-reduced-motion` disables barrage animation and leaderboard row transitions.
- Generated H5 sound assets are project-owned deterministic WAV files under `frontend/mobile-h5/public/audio/auction`; `MANIFEST.md` documents regeneration and asset purpose. Real Chromium check on 2026-06-06 clicked `开启提示音` and observed 200 responses for `heartbeat-bed.wav`, `hammer-hit.wav`, `whoosh-rank.wav`, `coin-leading.wav`, and `system-chime.wav`. Screenshot evidence: `docs/atmosphere-ai-implementation-2026-06-06/evidence/h5-sound-asset-pack-2026-06-06.png`.
- Real browser check on 2026-06-06 closed the bid panel, selected the `成交` terminal state, and verified the feed-mode result ceremony covers the full 393x851 H5 viewport. Screenshot evidence: `docs/atmosphere-ai-implementation-2026-06-06/evidence/h5-cinematic-result-2026-06-06.png`.
- Hammer countdown copy now renders `第一次/第二次/最后一次` plus server-authority copy; local countdown zero still enters syncing rather than creating a terminal result.

## Commands Run On 2026-06-06

```bash
make backend-migrate-up
/bin/bash -lc "GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestAI'"
pnpm build
pnpm test:frontend:domain
pnpm --filter mobile-h5 build
pnpm --filter pc-console build
node scripts/generate-h5-sound-assets.mjs
/bin/bash -lc "GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestAI|TestPlaceBid'"
/bin/bash -lc "GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestAI|TestSentinelAndProductQA'"
/bin/bash -lc "H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line -g 'winner result sheet'"
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
