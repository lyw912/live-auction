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
| P0-08 | Start AI provider boundary and fake provider test harness. | Backend | Tests prove disabled flag, fake provider success, timeout, malformed JSON. |
| P0-09 | Ship AI Listing Copilot backend draft endpoint with fake provider. | Backend/PC | Host-only endpoint tests; persisted job with structured output and safety flags. |
| P0-10 | Add PC Listing Copilot review/apply UI. | PC | Screenshot and Playwright test for field-level apply; no auto-publish. |

### P0-01 To P0-07 Evidence Snapshot

- Build: `pnpm --filter mobile-h5 build` passed on 2026-06-06.
- Domain contract: `pnpm test:frontend:domain` passed on 2026-06-06.
- Browser suite: `H5_PORT=5288 PC_PORT=5289 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 pnpm exec playwright test tests/e2e/atmosphere-engine.spec.ts tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line` passed with 54/54 tests on 2026-06-06.
- Chrome executable was installed for Playwright. Playwright MCP initially worked after install in earlier manual checks, but later returned `Transport closed`; this is recorded as MCP transport unavailable for the final manual pass rather than silently downgraded. The equivalent real-browser interactions are covered by the Playwright suite above.
- External check: current product/accessibility guidance supports truthful social proof and respecting reduced motion; countdown tension remains tied to server-derived state instead of local fake urgency.

## P1: Ceremony, Ranking, And AI Commentary

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P1-01 | Add hammer ceremony triggered only by server terminal event. | H5 | Local countdown-zero test does not celebrate; sold event does. |
| P1-02 | Add winner/loser/unsold/cancelled ceremony variants. | H5 | Visual snapshots for each variant. |
| P1-03 | Add leaderboard bid count and rank status labels. | H5 | Component test and mobile screenshot. |
| P1-04 | Add FLIP/rank transition animation. | H5 | Visual/manual trace; no layout shift after animation. |
| P1-05 | Add opt-in critical countdown sound/haptics. | H5 | Browser-blocked and sound-off states tested; no autoplay. |
| P1-06 | Add system commentary message stream/provenance. | Backend/H5/PC | Message has source, seq, and style; reconnect does not duplicate. |
| P1-07 | Add AI commentator generator with deterministic fallback. | Backend | Timeout/failure tests prove bid path unaffected and fallback message appears. |
| P1-08 | Add host toggle for AI commentary. | PC/Backend | Disabled flag and per-auction toggle tests. |

## P2: Trust, Recap, And Optional Live-Ops

| ID | Task | Owner area | Acceptance evidence |
|---|---|---|---|
| P2-01 | Add deterministic shill/troll sentinel rules. | Backend/PC | Synthetic suspicious pattern alerts; normal storm does not. |
| P2-02 | Add sentinel monitor/host alert UI. | PC/Backend | Alert includes explanation and recommended action. |
| P2-03 | Add optional AI explanation for aggregate sentinel alerts. | Backend | Aggregate-only input test; no private data in output. |
| P2-04 | Add auction recap/highlight generator. | Backend/PC/H5 | Completed auction recap job; share card hides private identities. |
| P2-05 | Add buyer product Q&A from approved listing facts. | Backend/H5 | Missing facts answer "未提供"; no private bid/risk leakage. |
| P2-06 | Explore warm-up/PK mechanics only after compliance review. | Product/H5/Backend | Written rule spec and explicit dark-pattern review before code. |

## AI Provider Gate

- Done: new relay base URL, exact model name, API key presence, strict JSON schema output, plain text, and multimodal image understanding were probed on 2026-06-06. Evidence: `docs/atmosphere-ai-implementation-2026-06-06/evidence/ai-relay-probe-gptgod-latest.json`.
- Current backend mode: `chat_completions_adapter`, not direct `/v1/responses`, because `/v1/responses` returned provider `not implemented` errors while `/v1/chat/completions` with strict `json_schema` passed.
- Schema implementation note: `gemini-3.1-flash-image-preview` may spend hundreds of completion tokens on reasoning before emitting JSON; use a generous output-token cap and reject empty content.
- Product image flow: merchant uploads to our backend; backend stores the image and gives the AI provider a short-lived provider-fetchable HTTPS object URL. Do not expose HTTP-image input fields to merchants.
- Provider Files API is not a P0 dependency because the current relay upload probe did not pass.

## Blockers To Resolve Before Coding AI

- Choose provider configuration names and secret handling.
- Decide persistence table shape: generic `ai_generation_jobs` vs per-capability tables.
- Decide whether system commentary reuses `chat_messages` or needs a distinct table plus outbox event.
- Verify exact OpenAI-compatible Chat Completions `json_schema` request shapes during implementation.

## Do Not Start Yet

- TTS voice commentary before text commentary is stable.
- Direct `/v1/responses` implementation on the current relay before a future probe proves that endpoint works.
- Auto-blocking shill decisions before policy, appeal, and evidence thresholds exist.
- Warm-up lottery/surprise mechanics before compliance and rule copy are written.
- Any viewer-count feature that cannot prove its data source.
