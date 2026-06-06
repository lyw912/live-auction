# Task Board

Use this as the execution board for the atmosphere/AI phase. Keep tasks small enough that each one can be reviewed, tested, and defended independently.

## Status Rules

- `Done`: implemented end to end with code, UI/API behavior, and stated test/manual evidence.
- `Done with limits`: implemented for the narrower task, but explicitly not the full review-design ambition.
- `Partial`: usable slice exists, but a required user/business path, automation mode, provider integration, or frontend surface is missing.
- `Not done`: no defensible implementation yet.
- `Deferred`: intentionally not started until a policy/compliance/product gate is written.

## P0: Visible Honesty And Core Tension

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P0-01 | Done: remove fake viewer count and replace with real/fallback heat label. | H5/API | `tests/e2e/mobile-h5.spec.ts` proves `2333` absent and renders `近30秒 2 人`; `HeatMeter` falls back to honest sync/waiting copy. |
| P0-02 | Done: implement or remove follow/like/gift/more controls. | H5 | Gift was removed; follow toggles, like increments, more opens a real settings sheet. Covered by `H5 renders honest heat and all visible live actions are interactive`. |
| P0-03 | Done: remove or data-bind static hype labels and unconditional proof chips. | H5/API | Static hype labels and unconditional deposit chip removed; proof chips render only item-backed certificate/condition/shipping values. |
| P0-04 | Done: add countdown phase derivation from server-time countdown. | H5 domain | `deriveCountdownPhase` covers normal/hot/critical/hammer/syncing/stale/terminal; `tests/frontend/domain-contract-tests.mjs` covers hot/critical/hammer/stale. |
| P0-05 | Done with limits: add final-countdown visual/audio tension gated by state and reduced-motion. | H5 CSS/components | Countdown phase drives `data-countdown-phase`; hot/critical/hammer styles are state-gated, hammer beat shows `第一次/第二次/最后一次` with server-result copy, and opt-in layered tones/haptics fire only in active connected state. This still uses synthesized/browser audio, not a licensed soundbed. |
| P0-06 | Done: render heat meter from real fields. | H5/API | `heatSnapshot` derives active bidders, accepted bids, accepted bidder count, total accepted bids, and price velocity from leaderboard/auction fields with fallback source. |
| P0-07 | Done: fix atmosphere cue ID and outbid cue event logic. | H5 state | `normalizeAtmosphere` uses monotonic cue IDs; outbid cue uses authoritative winner transition. Covered by `atmosphere-engine.spec.ts` and H5 event tests. |
| P0-08 | Done: wire AI provider boundary with provider-backed Chat Completions adapter and deterministic fallback. | Backend | `backend/internal/ai` defines `Generator`; `ChatCompletionsGenerator` uses strict `json_schema` Chat Completions when `API_KEY` is configured, otherwise local deterministic templates. `go test ./internal/ai` covers request shape, HTTPS image filtering, and malformed JSON rejection. |
| P0-09 | Done with limits: ship Listing Copilot backend draft endpoint with provider-backed mode and deterministic fallback. | Backend/PC | Host-only endpoint persists prompt version, provider/model, input/output JSON, safety flags, and `no_auto_publish`; provider mode supports strict structured output and HTTPS image URLs. Local default remains deterministic unless `API_KEY` is set. |
| P0-10 | Done with limits: add PC Listing Copilot review/apply UI with image upload. | PC | Drawer supports merchant notes, category, product-image upload, HTTPS provider-image status, structured draft review, field-level apply, and no auto-publish. Multimodal provider use requires a provider-fetchable HTTPS object URL; local MinIO HTTP images fall back to text draft with visible warning. |

### P0-01 To P0-07 Evidence Snapshot

- Build: `pnpm --filter mobile-h5 build` passed on 2026-06-06.
- Domain contract: `pnpm test:frontend:domain` passed on 2026-06-06.
- Browser suite: `H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/atmosphere-engine.spec.ts tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line` passed with 54/54 tests on 2026-06-06.
- Chrome executable was installed for Playwright. Playwright MCP initially worked after install in earlier manual checks, but later returned `Transport closed`; this is recorded as MCP transport unavailable for the final manual pass rather than silently downgraded. The equivalent real-browser interactions are covered by the Playwright suite above.
- External check: current product/accessibility guidance supports truthful social proof and respecting reduced motion; countdown tension remains tied to server-derived state instead of local fake urgency.

### P0-08 To P0-10 Evidence Snapshot And Limits

- Migration: `make backend-migrate-up` applied `202606060001_ai_atmosphere_capabilities.sql` and `202606060002_auction_ai_settings.sql` on 2026-06-06.
- Backend: `go test ./internal/ai` and `go test ./internal/gateway -run 'TestAI'` passed on 2026-06-06 with writable Go cache and local service access.
- Frontend: `pnpm --filter pc-console build`, `pnpm --filter mobile-h5 build`, and `pnpm test:frontend:domain` passed on 2026-06-06.
- Playwright MCP: PC Copilot drawer generated a `SUCCEEDED` deterministic/local-template draft from merchant notes, with title, description, rule suggestion, evidence flags, and no auto-publish claim.
- Do not overclaim B1: provider-backed generation is wired but only active with `API_KEY`; product images are sent to the provider only when the uploaded object URL is HTTPS and provider-fetchable.

## P1: Ceremony, Ranking, And AI Commentary

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P1-01 | Done: add hammer/result ceremony triggered only by server terminal event. | H5 | H5 renders the full-screen result ceremony only from terminal `selected` states (`sold_winner`/`sold_loser`/`ended`); local countdown zero remains `syncing` and cannot create the result card. Real browser evidence confirms feed-mode result overlay covers the H5 viewport. |
| P1-02 | Done with limits: add winner/loser/unsold cinematic ceremony variants. | H5 | `ResultSheet` now has full-screen winner, loser, and unsold variants with cinematic background glow, capped confetti, recap ticket, next-action copy, and compact fallback inside the bid panel. Cancelled remains a calm terminal state without celebration. This is not generated video. |
| P1-03 | Done: add leaderboard bid count and rank status labels. | H5 | Leaderboard rows now show `榜一/榜二/榜三/第 N 名` plus `bid_count`; focused H5 Playwright leaderboard tests passed on 2026-06-06. |
| P1-04 | Done: add measured FLIP rank transition animation. | H5 | `LeaderboardRows` measures previous/current row positions and animates rank movement with Web Animations; reduced-motion disables transitions. Stable row dimensions prevent layout shift. |
| P1-05 | Done with limits: add opt-in layered sound, countdown haptics, and browser TTS. | H5 | `playCountdownTone`, `playLayeredCue`, `vibrateCountdownPhase`, and `speakSystemMessage` run only after the user enables sound, while visible and supported. It is still synthesized/browser audio, not a licensed professional sound asset pack. |
| P1-06 | Done: add animated system commentary barrage/provenance. | Backend/H5/PC | `auction_system_messages` stores source, seq, style, facts, safety; PC creates commentary; H5 displays AI system messages in chat and an animated stage barrage layer from real system messages. |
| P1-07 | Done with limits: add AI commentator generator with deterministic fallback and automatic decided-event commentary. | Backend | Bid gateway now creates non-blocking auto commentary after accepted/sold decisions; `CreateAutoCommentary` dedupes by auction/source seq and marks `auto_generated`. Targeted gateway AI tests passed on 2026-06-06. This remains deterministic/local-template, not external provider-backed event consumer. |
| P1-08 | Done: host toggle for per-auction AI commentary generation. | PC/Backend | `auction_ai_settings` stores `auto_commentary_enabled`; host-only GET/PATCH APIs drive PC Live Assist. `CreateAutoCommentary` checks the server-side setting before generating; manual host commentary remains available. |

## P2: Trust, Recap, And Optional Live-Ops

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P2-01 | Done: add deterministic shill/troll sentinel rules. | Backend/PC | `EvaluateSentinel` flags rejected-bid probing, single-bidder push, and sold-unpaid pressure; it never blocks bids automatically. |
| P2-02 | Done: add sentinel host alert UI. | PC/Backend | Live Assist can run checks and render severity, score, explanation, and recommended action. |
| P2-03 | Done: add provider-backed sentinel explanation with deterministic fallback. | Backend | `EvaluateSentinel` now sends only aggregate features and rule candidates through the AI provider boundary, validates returned risk types against the candidate set, persists a `sentinel_explanation` job, and never exposes private max-bid/user-secret data or auto-blocks bids. Targeted gateway tests cover provider output and fallback. |
| P2-04 | Done with limits: add auction recap/highlight generator. | Backend/PC/H5 | Backend/PC recap remains host-only; H5 now renders a buyer-safe result recap/share card from public current-state facts only (item, terminal price, masked winner, accepted bidders/bids, next action). It is not video/highlight generation. |
| P2-05 | Done: add provider-backed buyer product Q&A from approved listing facts. | Backend/H5 | H5 Q&A calls the backend Q&A endpoint; backend sends only whitelisted item/rule facts to the provider, validates `facts_used` against that whitelist, rejects unsafe authenticity/investment/private-bid claims back to fallback, persists a `product_qa` job, and returns buyer-safe answer copy. |
| P2-06 | Done with limits: add compliant warm-up, buyer PK, entry/leader effects. | Product/H5 | H5 `LiveOpsPanel` adds warm-up task buttons, buyer-team PK progress, and entry/leader effect cards. Buttons open real sheets or toggle real local state; copy explicitly says no lottery, no promised reward, no price/winner impact. This is not a random 福袋, reward campaign, or backend promotion engine. |

## Review Design Cross-Check

This board must not be read as "all P0-P2 review-design items are complete." Current gaps against `docs/reviews/extreme-bidding-atmosphere-and-ai-judge-review-and-design-2026-06-05.md`:

- P0-A is now covered for state-gated hot/critical/hammer visuals, `第一次/第二次/最后一次` beat copy, opt-in synthesized tones, and haptics. It still does not include a licensed heartbeat soundbed.
- P1-E victory ceremony is now a full-screen H5 result ceremony with recap ticket and next action. It is still not a generated video highlight.
- P1-F leaderboard 2.0 now covers bid-count, rank labels, and measured FLIP transitions; WS incremental ranking remains future work.
- P1-G sound design 2.0 now includes opt-in synthesized countdown/cue layers, haptics, and browser TTS for system messages; no licensed asset sound pack.
- P1-H now includes automatic decided-event AI system messages, server-side per-auction auto commentary toggle, H5 chat display, and animated stage barrage.
- P2-I/P2-J/P2-K are implemented as a compliance-limited H5 live-ops panel. They are not random reward 福袋, cash/promotion mechanics, or a backend campaign engine.
- B1 Listing Copilot supports provider-backed strict structured output and HTTPS image URLs when configured; local default remains deterministic and HTTP/local object URLs are not sent as multimodal provider input.
- B2 AI commentary is automatic for decided bid/sold events and host-triggered for manual prompts; it can use the external AI provider when configured, with deterministic fallback on missing/failed provider.
- B3 Sentinel now uses provider-backed explanations over deterministic aggregate rule candidates, with deterministic fallback and audit jobs. It is still not an advanced cross-account anomaly model or automatic enforcement engine.
- B4 Recap includes backend/PC host recap plus H5 buyer-safe share/highlight card; generated video/highlight clips remain missing.
- B5 Q&A now uses provider-backed answers constrained to approved listing/rule facts, with deterministic fallback. It is still not a full multi-turn 导拍客服 or personalized shopping assistant.

## AI Provider Gate

- Done: new relay base URL, exact model name, API key presence, strict JSON schema output, plain text, and multimodal image understanding were probed on 2026-06-06. Evidence: `docs/atmosphere-ai-implementation-2026-06-06/evidence/ai-relay-probe-gptgod-latest.json`.
- Current backend mode: `chat_completions_adapter`, not direct `/v1/responses`, because `/v1/responses` returned provider `not implemented` errors while `/v1/chat/completions` with strict `json_schema` passed.
- Schema implementation note: `gemini-3.1-flash-image-preview` may spend hundreds of completion tokens on reasoning before emitting JSON; use a generous output-token cap and reject empty content.
- Product image flow: merchant uploads to our backend; backend stores the image and gives the AI provider a short-lived provider-fetchable HTTPS object URL. Do not expose HTTP-image input fields to merchants.
- Provider Files API is not a P0 dependency because the current relay upload probe did not pass.

## AI Implementation Decisions Made

- Persistence uses generic `ai_generation_jobs`, `auction_system_messages`, and `auction_risk_alerts`.
- System commentary is separate from `chat_messages` so provenance, source seq, and safety labels remain explicit.
- Current runtime provider mode is `auto`: provider-backed Chat Completions is used only when `API_KEY` is configured; otherwise deterministic/local-template fallback is explicit.
- AI never decides bids, winners, prices, settlement, orders, or automatic risk blocking.

## Do Not Start Yet

- TTS voice commentary before text commentary is stable.
- Direct `/v1/responses` implementation on the current relay before a future probe proves that endpoint works.
- Auto-blocking shill decisions before policy, appeal, and evidence thresholds exist.
- Random warm-up lottery/surprise reward mechanics before campaign rules, odds disclosure, anti-addiction copy, and platform compliance review are written.
- Any viewer-count feature that cannot prove its data source.
