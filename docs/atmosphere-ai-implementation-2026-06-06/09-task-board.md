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
| P0-05 | Done with limits: add final-countdown visual tension gated by state and reduced-motion. | H5 CSS/components | Countdown phase drives `data-countdown-phase`; hot/critical/hammer styles are state-gated and `prefers-reduced-motion` disables hammer pulse. This is visual/state tension only; it does not complete the full review-design P0-A heartbeat soundbed, "三二一/going once" cadence, or escalating haptics. |
| P0-06 | Done: render heat meter from real fields. | H5/API | `heatSnapshot` derives active bidders, accepted bids, accepted bidder count, total accepted bids, and price velocity from leaderboard/auction fields with fallback source. |
| P0-07 | Done: fix atmosphere cue ID and outbid cue event logic. | H5 state | `normalizeAtmosphere` uses monotonic cue IDs; outbid cue uses authoritative winner transition. Covered by `atmosphere-engine.spec.ts` and H5 event tests. |
| P0-08 | Done with limits: start AI provider boundary and deterministic provider test harness. | Backend | `backend/internal/ai` defines `Generator`; gateway test proves host-only AI endpoints and deterministic structured output. Production external-provider runtime is not wired as the active backend mode. |
| P0-09 | Partial: ship Listing Copilot backend draft endpoint with deterministic provider. | Backend/PC | `TestAIListingDraftIsHostOnlyStructuredAndApplyIsAuditOnly` proves host-only auth, persisted structured output, rule suggestions, and `no_auto_publish` safety. This is local-template/deterministic output, not full provider-backed multimodal Listing Copilot. |
| P0-10 | Partial: add PC Listing Copilot review/apply UI for deterministic drafts. | PC | Playwright MCP generated a real draft in the drawer and showed field-level apply; UI states that publish/rule save still use existing backend validation. It does not yet cover image upload/provider-backed multimodal drafting. |

### P0-01 To P0-07 Evidence Snapshot

- Build: `pnpm --filter mobile-h5 build` passed on 2026-06-06.
- Domain contract: `pnpm test:frontend:domain` passed on 2026-06-06.
- Browser suite: `H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/atmosphere-engine.spec.ts tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line` passed with 54/54 tests on 2026-06-06.
- Chrome executable was installed for Playwright. Playwright MCP initially worked after install in earlier manual checks, but later returned `Transport closed`; this is recorded as MCP transport unavailable for the final manual pass rather than silently downgraded. The equivalent real-browser interactions are covered by the Playwright suite above.
- External check: current product/accessibility guidance supports truthful social proof and respecting reduced motion; countdown tension remains tied to server-derived state instead of local fake urgency.

### P0-08 To P0-10 Evidence Snapshot And Limits

- Migration: `make backend-migrate-up` applied `202606060001_ai_atmosphere_capabilities.sql` on 2026-06-06.
- Backend: `/bin/bash -lc "GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestAI'"` passed on 2026-06-06.
- Frontend: `pnpm build` and `pnpm test:frontend:domain` passed on 2026-06-06.
- Playwright MCP: PC Copilot drawer generated a `SUCCEEDED` deterministic/local-template draft from merchant notes, with title, description, rule suggestion, evidence flags, and no auto-publish claim.
- Do not claim full B1 AI Listing Copilot yet: current runtime uses deterministic/local-template generation and does not ingest merchant product images through a provider-backed multimodal flow.

## P1: Ceremony, Ranking, And AI Commentary

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P1-01 | Done with limits: add hammer ceremony triggered only by server terminal event. | H5 | H5 now renders the full result ceremony only from terminal `selected` states (`sold_winner`/`sold_loser`/`ended`); local countdown zero remains `syncing` and cannot create the result card. Focused H5 Playwright result/countdown tests passed on 2026-06-06. |
| P1-02 | Done with limits: add winner/loser/unsold ceremony variants. | H5 | `ResultSheet` now has winner, loser, and unsold copy plus a share-style recap card in feed mode; cancelled remains a calm terminal state without celebration. Particle effect is capped CSS confetti and disabled by reduced motion CSS. |
| P1-03 | Done: add leaderboard bid count and rank status labels. | H5 | Leaderboard rows now show `榜一/榜二/榜三/第 N 名` plus `bid_count`; focused H5 Playwright leaderboard tests passed on 2026-06-06. |
| P1-04 | Done with limits: add rank transition animation. | H5 | Rows now use stable grid dimensions and transform/background transition for rank/current-user changes. This is a lightweight transition, not a full measured FLIP implementation. |
| P1-05 | Done with limits: add opt-in critical countdown sound/haptics. | H5 | `playCountdownTone`/`vibrateCountdownPhase` trigger only at phase/beat boundary while connected, active, and user sound is enabled; no sound/haptic during recovery, stale, hidden tab, or reduced-motion. It is synthesized tones, not a full sound design asset pack. |
| P1-06 | Done with limits: add system commentary message stream/provenance. | Backend/H5/PC | `auction_system_messages` stores source, seq, style, facts, safety; PC creates commentary; H5 displays AI system messages. The stream is not yet a full animated system-barrage channel. |
| P1-07 | Done with limits: add AI commentator generator with deterministic fallback and automatic decided-event commentary. | Backend | Bid gateway now creates non-blocking auto commentary after accepted/sold decisions; `CreateAutoCommentary` dedupes by auction/source seq and marks `auto_generated`. Targeted gateway AI tests passed on 2026-06-06. This remains deterministic/local-template, not external provider-backed event consumer. |
| P1-08 | Partial: host toggle for AI commentary visibility. | PC/Backend | PC Live Assist has a real local visibility toggle for auto commentary messages and manual generation remains available. Server-side per-auction auto-generation enable/disable storage/API remains future work. |

## P2: Trust, Recap, And Optional Live-Ops

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P2-01 | Done: add deterministic shill/troll sentinel rules. | Backend/PC | `EvaluateSentinel` flags rejected-bid probing, single-bidder push, and sold-unpaid pressure; it never blocks bids automatically. |
| P2-02 | Done: add sentinel host alert UI. | PC/Backend | Live Assist can run checks and render severity, score, explanation, and recommended action. |
| P2-03 | Partial: add sentinel explanation. | Backend | Alerts use aggregate features only and expose no private max-bid/user-secret data. This is deterministic explanation, not LLM explanation or advanced shill model. |
| P2-04 | Done with limits: add auction recap/highlight generator. | Backend/PC/H5 | Backend/PC recap remains host-only; H5 now renders a buyer-safe result recap/share card from public current-state facts only (item, terminal price, masked winner, accepted bidders/bids, next action). It is not video/highlight generation. |
| P2-05 | Partial: add buyer product Q&A from approved listing facts. | Backend/H5 | H5 Q&A sheet answers from item/rule facts and returns "未提供" when facts are absent; Playwright MCP verified 起拍价 answer. This is fact-only deterministic Q&A, not a full provider-backed AI shopping assistant. |
| P2-06 | Deferred: explore warm-up/PK mechanics only after compliance review. | Product/H5/Backend | Written rule spec and explicit dark-pattern review are required before code. No warm-up lucky draw, buyer PK progress bar, entry effect, or榜一特效 is implemented. |

## Review Design Cross-Check

This board must not be read as "all P0-P2 review-design items are complete." Current gaps against `docs/reviews/extreme-bidding-atmosphere-and-ai-judge-review-and-design-2026-06-05.md`:

- P0-A is only partially covered by `P0-05`; heartbeat soundbed, "三二一/going once", and escalating haptics are missing.
- P1-E victory ceremony is implemented as a compact H5 result ceremony, but not as a full-screen cinematic/video highlight.
- P1-F leaderboard 2.0 now covers bid-count and rank labels plus lightweight transitions; WS incremental ranking remains future work.
- P1-G sound design 2.0 is partially implemented with opt-in synthesized critical/hammer tones and haptics; no asset sound pack/TTS.
- P1-H is improved with automatic decided-event AI system messages and PC visibility toggle; true barrage animation and server-side per-auction auto toggle remain missing.
- P2-I/P2-J/P2-K warm-up lucky draw, buyer PK progress bar, entry effects, and榜一 effects are not implemented.
- B1 Listing Copilot is deterministic/local-template only, not full provider-backed multimodal AI.
- B2 AI commentary is automatic for decided bid/sold events and host-triggered for manual prompts, but still deterministic/local-template rather than provider-backed.
- B3 Sentinel is deterministic aggregate rules only, not LLM explanation or advanced anomaly model.
- B4 Recap includes backend/PC host recap plus H5 buyer-safe share/highlight card; generated video/highlight clips remain missing.
- B5 Q&A is deterministic fact lookup, not full AI导拍客服.

## AI Provider Gate

- Done: new relay base URL, exact model name, API key presence, strict JSON schema output, plain text, and multimodal image understanding were probed on 2026-06-06. Evidence: `docs/atmosphere-ai-implementation-2026-06-06/evidence/ai-relay-probe-gptgod-latest.json`.
- Current backend mode: `chat_completions_adapter`, not direct `/v1/responses`, because `/v1/responses` returned provider `not implemented` errors while `/v1/chat/completions` with strict `json_schema` passed.
- Schema implementation note: `gemini-3.1-flash-image-preview` may spend hundreds of completion tokens on reasoning before emitting JSON; use a generous output-token cap and reject empty content.
- Product image flow: merchant uploads to our backend; backend stores the image and gives the AI provider a short-lived provider-fetchable HTTPS object URL. Do not expose HTTP-image input fields to merchants.
- Provider Files API is not a P0 dependency because the current relay upload probe did not pass.

## AI Implementation Decisions Made

- Persistence uses generic `ai_generation_jobs`, `auction_system_messages`, and `auction_risk_alerts`.
- System commentary is separate from `chat_messages` so provenance, source seq, and safety labels remain explicit.
- Current runtime provider is deterministic/local-template. External relay-backed provider remains behind the `Generator` boundary until production secret/config handling is added.
- AI never decides bids, winners, prices, settlement, orders, or automatic risk blocking.

## Do Not Start Yet

- TTS voice commentary before text commentary is stable.
- Direct `/v1/responses` implementation on the current relay before a future probe proves that endpoint works.
- Auto-blocking shill decisions before policy, appeal, and evidence thresholds exist.
- Warm-up lottery/surprise mechanics before compliance and rule copy are written.
- Any viewer-count feature that cannot prove its data source.
