# Task Board

Use this as the execution board for the atmosphere/AI phase. Keep tasks small enough that each one can be reviewed, tested, and defended independently.

## P0: Visible Honesty And Core Tension

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P0-01 | Done: remove fake viewer count and replace with real/fallback heat label. | H5/API | `tests/e2e/mobile-h5.spec.ts` proves `2333` absent and renders `近30秒 2 人`; `HeatMeter` falls back to honest sync/waiting copy. |
| P0-02 | Done: implement or remove follow/like/gift/more controls. | H5 | Gift was removed; follow toggles, like increments, more opens a real settings sheet. Covered by `H5 renders honest heat and all visible live actions are interactive`. |
| P0-03 | Done: remove or data-bind static hype labels and unconditional proof chips. | H5/API | Static hype labels and unconditional deposit chip removed; proof chips render only item-backed certificate/condition/shipping values. |
| P0-04 | Done: add countdown phase derivation from server-time countdown. | H5 domain | `deriveCountdownPhase` covers normal/hot/critical/hammer/syncing/stale/terminal; `tests/frontend/domain-contract-tests.mjs` covers hot/critical/hammer/stale. |
| P0-05 | Done: add final-countdown visual tension gated by state and reduced-motion. | H5 CSS/components | Countdown phase drives `data-countdown-phase`; hot/critical/hammer styles are state-gated and `prefers-reduced-motion` disables hammer pulse. |
| P0-06 | Done: render heat meter from real fields. | H5/API | `heatSnapshot` derives active bidders, accepted bids, accepted bidder count, total accepted bids, and price velocity from leaderboard/auction fields with fallback source. |
| P0-07 | Done: fix atmosphere cue ID and outbid cue event logic. | H5 state | `normalizeAtmosphere` uses monotonic cue IDs; outbid cue uses authoritative winner transition. Covered by `atmosphere-engine.spec.ts` and H5 event tests. |
| P0-08 | Done: start AI provider boundary and deterministic provider test harness. | Backend | `backend/internal/ai` defines `Generator`; gateway test proves host-only AI endpoints and deterministic structured output. |
| P0-09 | Done: ship AI Listing Copilot backend draft endpoint with deterministic provider. | Backend/PC | `TestAIListingDraftIsHostOnlyStructuredAndApplyIsAuditOnly` proves host-only auth, persisted structured output, rule suggestions, and `no_auto_publish` safety. |
| P0-10 | Done: add PC Listing Copilot review/apply UI. | PC | Playwright MCP generated a real draft in the drawer and showed field-level apply; UI states that publish/rule save still use existing backend validation. |

### P0-01 To P0-07 Evidence Snapshot

- Build: `pnpm --filter mobile-h5 build` passed on 2026-06-06.
- Domain contract: `pnpm test:frontend:domain` passed on 2026-06-06.
- Browser suite: `H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/atmosphere-engine.spec.ts tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line` passed with 54/54 tests on 2026-06-06.
- Chrome executable was installed for Playwright. Playwright MCP initially worked after install in earlier manual checks, but later returned `Transport closed`; this is recorded as MCP transport unavailable for the final manual pass rather than silently downgraded. The equivalent real-browser interactions are covered by the Playwright suite above.
- External check: current product/accessibility guidance supports truthful social proof and respecting reduced motion; countdown tension remains tied to server-derived state instead of local fake urgency.

### P0-08 To P0-10 Evidence Snapshot

- Migration: `make backend-migrate-up` applied `202606060001_ai_atmosphere_capabilities.sql` on 2026-06-06.
- Backend: `/bin/bash -lc "GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestAI'"` passed on 2026-06-06.
- Frontend: `pnpm build` and `pnpm test:frontend:domain` passed on 2026-06-06.
- Playwright MCP: PC Copilot drawer generated a `SUCCEEDED` deterministic/local-template draft from merchant notes, with title, description, rule suggestion, evidence flags, and no auto-publish claim.

## P1: Ceremony, Ranking, And AI Commentary

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P1-01 | Add hammer ceremony triggered only by server terminal event. | H5 | Local countdown-zero test does not celebrate; sold event does. |
| P1-02 | Add winner/loser/unsold/cancelled ceremony variants. | H5 | Visual snapshots for each variant. |
| P1-03 | Add leaderboard bid count and rank status labels. | H5 | Component test and mobile screenshot. |
| P1-04 | Add FLIP/rank transition animation. | H5 | Visual/manual trace; no layout shift after animation. |
| P1-05 | Add opt-in critical countdown sound/haptics. | H5 | Browser-blocked and sound-off states tested; no autoplay. |
| P1-06 | Done: add system commentary message stream/provenance. | Backend/H5/PC | `auction_system_messages` stores source, seq, style, facts, safety; PC creates commentary; H5 displays AI system messages. |
| P1-07 | Done: add AI commentator generator with deterministic fallback. | Backend | `TestAICommentarySystemMessagesSentinelRecapAndProductQA` proves message creation with source seq and no hidden max-bid safety. |
| P1-08 | Deferred: host toggle for AI commentary. | PC/Backend | Current implementation is host-triggered, not automatic; per-auction auto-generation toggle remains future work. |

## P2: Trust, Recap, And Optional Live-Ops

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P2-01 | Done: add deterministic shill/troll sentinel rules. | Backend/PC | `EvaluateSentinel` flags rejected-bid probing, single-bidder push, and sold-unpaid pressure; it never blocks bids automatically. |
| P2-02 | Done: add sentinel host alert UI. | PC/Backend | Live Assist can run checks and render severity, score, explanation, and recommended action. |
| P2-03 | Done as deterministic explanation, not LLM explanation. | Backend | Alerts use aggregate features only and expose no private max-bid/user-secret data. |
| P2-04 | Done: add auction recap/highlight generator. | Backend/PC | Recap job hides buyer identities and appears in PC Live Assist; H5 share-card rendering remains future. |
| P2-05 | Done: add buyer product Q&A from approved listing facts. | Backend/H5 | H5 Q&A sheet answers from item/rule facts and returns "未提供" when facts are absent; Playwright MCP verified 起拍价 answer. |
| P2-06 | Explore warm-up/PK mechanics only after compliance review. | Product/H5/Backend | Written rule spec and explicit dark-pattern review before code. |

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
