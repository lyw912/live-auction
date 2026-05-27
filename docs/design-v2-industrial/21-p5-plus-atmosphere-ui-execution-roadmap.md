# 21 · P5+ Atmosphere And UI Execution Roadmap

> Status: execution plan for implementing `19-extreme-bidding-atmosphere.md` and `20-ui-ux-redesign.md`.
> Rule: every slice below should be implemented as one focused commit unless explicitly marked as docs-only or migration-only.

## Purpose

P0-P4 proved correctness, realtime recovery, diagnostics, and performance discipline. P5+ turns the two new product design documents into an executable engineering roadmap:

- `19-extreme-bidding-atmosphere.md`: event-driven bidding atmosphere, action-oriented ranking, host prompter, heat, trust, and advanced auction engagement.
- `20-ui-ux-redesign.md`: full UI/UX redesign across Viewer H5, Host Live Assist, Seller Studio, and Ops Diagnostics.
- `docs/references/official-brief-images/`: official brief screenshots that clarify expected PC product management, H5 live product list, bid panel, ended state, winner payment, and hammer result flows.

The roadmap is intentionally slice-based. Each slice has a small behavioral surface, a validation gate, and a suggested commit message. Follow the order unless a slice is explicitly independent.

## Non-Negotiables

1. No fake bids, fake viewers, fake heat, fake scarcity, or frontend-only success.
2. PostgreSQL remains the money truth; Redis/WebSocket/UI are projections.
3. Any strong visual effect must bind to a server event, snapshot, or documented aggregate with freshness.
4. H5 CTA must never be hidden or moved unpredictably by animation.
5. Weak network/recovery states always disable dangerous actions.
6. Sound/haptic are opt-in and must degrade cleanly.
7. Visual polish requires screenshot/longtask/text-fit gates, not manual taste alone.
8. PC host/seller UX must remain operationally efficient, not become a decorative dashboard.
9. Official screenshot semantics must be preserved or deliberately superseded with a stronger documented UX: product list half sheet, bid panel, self-leading hint, ended/unsold state, winner payment, hammer result, PC narrating and bid count controls.

## Phase Map

| Phase | Theme | Source docs | Outcome |
|---|---|---|---|
| P5 | UI foundation and visual system | 20 | Shared tokens, screenshot gates, baseline H5/PC structure ready for redesign. |
| P6 | Viewer H5 auction cockpit | 19, 20 | Live Stage, Bid Dock, RankStrip, sheets, recovery, result UX. |
| P7 | Atmosphere engine and action ranking | 19, 20 | Event scheduler, effect policy, leaderboard v2 fields, sound/haptic, longtask gates. |
| P8 | Host Live Assist and Seller Studio | 19, 20 | PC command center, prompter, heat pulse, seller setup wizard, preview. |
| P9 | Trust, advanced auction UX, and diagnostics | 19, 20 | Verified bidder hooks, Max Bid/Pre-bid design implementation path, risk UX, timeline diagnostics. |
| P10 | Evidence hardening and demo packaging | 19, 20 | Visual regression, accessibility, demo scripts, judge-facing evidence. |

## P5 · UI Foundation And Visual System

Goal: create the design-system base before moving screens. This prevents each later slice from inventing colors, spacing, and component behavior independently.

### P5-S1 · Add Design Tokens

Reference:

- `20-ui-ux-redesign.md` → `视觉方向`, `美学系统`, `Design Tokens`.

Scope:

- Add shared CSS tokens for colors, spacing, radii, z-index, typography, tabular numbers, safe area.
- Wire H5 and PC to use tokens without changing layouts materially.
- Keep colors semantically named: `bid-red`, `auction-gold`, `trust-green`, `live-cyan`, `risk-orange`.

Files:

- `frontend/mobile-h5/src/styles.css`
- `frontend/pc-console/src/styles.css` or current PC style entry
- optional `frontend/shared-design/tokens.css`

Validation:

- `pnpm build`
- Existing e2e must still pass.
- Manual screenshot check: no obvious color regression.

Commit:

```text
style: add auction studio design tokens
```

### P5-S2 · Add Visual Regression Harness

Reference:

- `20-ui-ux-redesign.md` → `Visual Regression System`, `测试门禁`.
- `docs/references/official-brief-images/` → official PC/H5 state screenshots.

Scope:

- Add Playwright screenshot fixtures for H5 key states: active, self-leading, outbid, recovering, sold winner, sold loser.
- Add official-alignment screenshot cases: product list sheet, bid sheet, unsold ended state, winner payment/result state.
- Add PC screenshot fixtures for command panel/diagnostics initial state.
- Store screenshots under a predictable evidence or test snapshot path.
- Do not redesign UI yet; create gates first.

Validation:

- `pnpm exec playwright test ...` targeted screenshot tests.
- Baseline review in `docs/evidence` or test snapshots.

Commit:

```text
test: add visual regression gates for auction UI
```

### P5-S3 · H5 Component Boundary Refactor

Reference:

- `20-ui-ux-redesign.md` → `H5 组件`, `H5 重构顺序`.

Scope:

- Extract current H5 sections into component boundaries without major visual change:
  - `LiveStage`
  - `AuctionStatePanel`
  - `LeaderboardPanel`
  - `HistoryPanel`
  - `ChatPanel`
- Preserve current behavior and tests.

Validation:

- `pnpm --filter mobile-h5 exec tsc --noEmit`
- `pnpm test:e2e`

Commit:

```text
refactor: split H5 auction room into UI components
```

### P5-S4 · PC Component Boundary Refactor

Reference:

- `20-ui-ux-redesign.md` → `PC 组件`, `PC 控制台重构方案`.

Scope:

- Extract PC surfaces without major visual change:
  - `AuctionCommandPanel`
  - `AuctionQueue`
  - `RuleEditor`
  - `OrdersPanel`
  - `DiagnosticsPanel`
  - `EventTimeline`

Validation:

- `pnpm --filter pc-console exec tsc --noEmit`
- `pnpm test:e2e`

Commit:

```text
refactor: split PC console into command components
```

## P6 · Viewer H5 Auction Cockpit

Goal: replace the current stacked H5 page with a live-auction cockpit optimized for one-hand bidding and real product trust.

### P6-S1 · Implement Live Stage With Real Product Visuals

Reference:

- `20-ui-ux-redesign.md` → `H5 Live Stage`, `图像规范`.
- `19-extreme-bidding-atmosphere.md` → `3 秒停留钩子`.

Scope:

- Replace decorative gradient product background with item image/video poster when available.
- Add top live bar: LIVE, room, connection, sound toggle.
- Add proof chips placeholder: certificate/condition/shipping/deposit.
- Keep chat inside the stage safe zone.

Validation:

- H5 screenshot: first screen reads as live auction, not static page.
- Text-fit gate at 360px.
- No CTA overlap.

Commit:

```text
feat: redesign H5 live stage with product visuals
```

### P6-S2 · Implement Sticky Bid Dock

Reference:

- `20-ui-ux-redesign.md` → `Bid Dock`, `H5 全新界面蓝图`.

Scope:

- Move price, countdown, rank summary, stepper, and CTA into sticky bottom dock.
- Define dock state variants: ACTIVE, SELF_LEADING, OUTBID, PENDING, RECOVERING, SOLD_WINNER, SOLD_LOSER.
- Preserve no-optimistic-success behavior.

Validation:

- H5 first-screen gate: price/countdown/rank/CTA visible at 390x844 and 360px width.
- Existing bid, recovery, sold tests pass.

Commit:

```text
feat: add sticky H5 bid dock
```

### P6-S3 · Add Bottom Sheet System

Reference:

- `20-ui-ux-redesign.md` → `Bottom Sheet 系统`.
- `docs/references/official-brief-images/official-brief-image-02.png` and `official-brief-image-03.png`.

Scope:

- Add reusable bottom sheet component.
- Move product detail, rules, leaderboard, history, orders into sheets or sheet tabs.
- Add product list sheet behavior matching the official live-room expectation: bidding/upcoming/ended/sold/cutoff cards with state-specific CTA.
- Keep primary bid CTA singular and stable.

Validation:

- Playwright: sheets open/close, do not hide dock unexpectedly.
- `h5-safe-zone` gate passes.

Commit:

```text
feat: add H5 bottom sheet navigation
```

### P6-S4 · Product Trust Detail Sheet

Reference:

- `20-ui-ux-redesign.md` → `商品信任详情`.
- `19-extreme-bidding-atmosphere.md` → `3 秒停留钩子`.

Scope:

- Add product trust sheet with item media, rules, proof chips, deposit/shipping/cap/extension explanation.
- Use user-language explanations, not raw engineering field names.

Validation:

- Screenshot: high-value product trust details visible.
- E2E: sheet includes rule summary and does not alter bid flow.

Commit:

```text
feat: add H5 product trust sheet
```

### P6-S5 · Winner And Loser Result Sheets

Reference:

- `19-extreme-bidding-atmosphere.md` → `成交/输家承接`.
- `20-ui-ux-redesign.md` → `支付与成交 UX`.
- `docs/references/official-brief-images/official-brief-image-05.png` and `official-brief-image-06.png`.

Scope:

- Winner sheet: sold price, order lock countdown, payment CTA, deposit/payment status.
- Loser sheet: final gap, winner masked, next/similar auction placeholder.
- Unsold/ended sheet: disabled bid CTA, explanation, return-to-live or next-item countdown.
- Keep payment mock path intact.

Validation:

- H5 sold winner/loser state tests.
- Payment double-click test still passes.

Commit:

```text
feat: add H5 auction result sheets
```

## P7 · Atmosphere Engine And Action Ranking

Goal: move from scattered event UI to a proper atmosphere engine with event priority, dedupe, freshness, sound/haptic policy, and action-oriented ranking.

### P7-S1 · Add Atmosphere Engine

Reference:

- `19-extreme-bidding-atmosphere.md` → `氛围体验是事件编排系统`, `事件与数据模型`.
- `20-ui-ux-redesign.md` → `动效美学`.

Scope:

- Implement client-side event normalizer, user context enricher, priority scheduler, and effect policy.
- Strong effects carry `cause_seq`, `event_type`, `auction_id`, `user_scope`.
- Dedupe effects on reconnect/snapshot recovery.

Validation:

- Playwright: reconnect does not replay old strong effects.
- Unit tests for priority: SOLD > RECOVERING > OUTBID > LEADING > SOCIAL.

Commit:

```text
feat: add H5 atmosphere event engine
```

### P7-S2 · Upgrade Visual Effects

Reference:

- `20-ui-ux-redesign.md` → `动效语言`, `动效美学`.
- `19-extreme-bidding-atmosphere.md` → `情绪反馈要分层`.

Scope:

- Replace generic toast cue with motion language:
  - price tick
  - gold leading ring
  - red outbid edge flash
  - extension countdown stretch
  - hammer/result moment
- Add reduced-motion fallback.

Validation:

- Longtask gate.
- WCAG flash threshold review.
- CTA overlap test.

Commit:

```text
feat: upgrade H5 event-driven auction effects
```

### P7-S3 · Leaderboard V2 API

Reference:

- `19-extreme-bidding-atmosphere.md` → `实时排行榜：从 Top N 到行动型排名`.
- `20-ui-ux-redesign.md` → `行动型排行榜`.

Scope:

- Extend leaderboard response with:
  - `seq`
  - `server_time_ms`
  - `gap_to_next_rank_cents`
  - `next_valid_bid_cents`
  - optional `active_bidders_30s`, `accepted_bids_30s`, `price_velocity_cents_per_min`
- Keep old fields backward-compatible.

Validation:

- Backend unit/integration tests for ranking and gaps.
- H5 mocked e2e updates expected payload.

Commit:

```text
feat: extend leaderboard with action metrics
```

### P7-S4 · H5 RankStrip And Leaderboard Sheet

Reference:

- `19-extreme-bidding-atmosphere.md` → `行动型榜单`.
- `20-ui-ux-redesign.md` → `RankStrip`.

Scope:

- Replace full-page leaderboard section with default RankStrip in Bid Dock.
- Add expanded leaderboard sheet with Top N, my rank, next action, freshness/stale marker.

Validation:

- H5 e2e: `第 2 名 · 差 ¥x · 下一口 ¥y`.
- Screenshot: leaderboard no longer pushes CTA down.

Commit:

```text
feat: add action-oriented H5 rank strip
```

### P7-S4b · Official Bid Hint States

Reference:

- `19-extreme-bidding-atmosphere.md` → `Official. 官方图片状态补齐`.
- `docs/references/official-brief-images/official-brief-image-04.png`.

Scope:

- Add explicit bid-adjacent hints for:
  - one-step/multi-step amount above current price;
  - self-leading guardrail;
  - stale price changed while user prepared to bid.
- Hints must be near the amount/CTA, not hidden in toast.

Validation:

- H5 e2e: self-leading state disables or changes CTA with clear copy.
- H5 e2e: multi-step bid hint shows how much above current price.

Commit:

```text
feat: add official bid hint states
```

### P7-S5 · Sound And Haptic Policy

Reference:

- `19-extreme-bidding-atmosphere.md` → `提示音与震动`.
- `20-ui-ux-redesign.md` → `动效与声音系统`.

Scope:

- Initialize AudioContext only after user enables sound.
- Add capability detection and visual fallback.
- Add event-specific sound/haptic patterns.
- Respect `prefers-reduced-motion` and hidden tab.

Validation:

- E2E: no sound before opt-in.
- E2E: toggle state works and does not trigger longtask.

Commit:

```text
feat: add opt-in auction sound and haptic policy
```

### P7-S6 · Countdown And Extension UX

Reference:

- `19-extreme-bidding-atmosphere.md` → `倒计时与延时`.
- `20-ui-ux-redesign.md` → `Bid Dock`.

Scope:

- Last 10 seconds use stable-width tenths display if server time is fresh.
- Extension explanation shows old/new end time and extend count.
- Local zero enters syncing/recovery, not SOLD.

Validation:

- H5 e2e for extension display and local-zero syncing.
- No layout shift in countdown row.

Commit:

```text
feat: refine H5 countdown and extension UX
```

## P8 · Host Live Assist And Seller Studio

Goal: make PC useful for live operation, not just CRUD and diagnostics.

### P8-S1 · PC Command Center Layout

Reference:

- `20-ui-ux-redesign.md` → `PC 全新界面蓝图`, `PC Command Center`.
- `docs/references/official-brief-images/official-brief-image-01.png`.

Scope:

- Rebuild PC shell into:
  - top health ribbon
  - left auction queue
  - center active command panel
  - right live assist/event rail
  - secondary workspace for rules/orders/diagnostics
- Keep all existing PC workflows.
- Preserve official PC list semantics: product thumbnail, tags, start price, increment, cap price, current/sold price, bid count, narrating state, auction status, countdown, cancel/off-shelf actions.

Validation:

- PC e2e workflows still pass.
- Screenshot: ACTIVE auction is controllable without scrolling.

Commit:

```text
feat: redesign PC console as auction command center
```

### P8-S2 · Auction Queue And Active Pinning

Reference:

- `20-ui-ux-redesign.md` → `PC Command Center`.

Scope:

- ACTIVE auction pinned at top.
- DRAFT/SCHEDULED grouped with status and product thumbnail.
- Narrating/ACTIVE constraints visible.

Validation:

- PC e2e: room switch, active auction selection, narrate state.

Commit:

```text
feat: add PC auction queue with active pinning
```

### P8-S3 · Host Prompter Backend

Reference:

- `19-extreme-bidding-atmosphere.md` → `主播控场`.
- `20-ui-ux-redesign.md` → `Host Live Assist`.

Scope:

- Add host-only API for prompts based on recent auction events:
  - no bid for N seconds
  - last 10 seconds
  - extension triggered
  - high outbid frequency
  - sold/unpaid
- Prompts are advisory and never mutate auction truth.

Validation:

- Backend tests for prompt cases.
- ACL host-only.

Commit:

```text
feat: add host auction prompter API
```

### P8-S4 · Host Live Assist UI

Reference:

- `19-extreme-bidding-atmosphere.md` → `Auction Prompter`.
- `20-ui-ux-redesign.md` → `Host Live Assist`.

Scope:

- Add prompter cards, heat pulse placeholder, talk points, risk hints.
- Add manual system chat template UI if chat API supports it; otherwise keep disabled with clear scope.

Validation:

- PC e2e: prompter renders from API, dismiss/manual action does not mutate auction state.

Commit:

```text
feat: add PC host live assist panel
```

### P8-S5 · Seller Rule Wizard And Preview

Reference:

- `20-ui-ux-redesign.md` → `Seller Studio`, `Wizard + Preview`.
- `19-extreme-bidding-atmosphere.md` → `商家运营`.

Scope:

- Convert rule creation/editing into grouped wizard:
  - product
  - price
  - time/extension
  - trust/deposit
  - preview
- Preview H5 display of chips and rule explanation.
- Preserve backend validation and freeze rules.

Validation:

- PC e2e for create item/auction and rule save.
- Rule freeze state explains why disabled.

Commit:

```text
feat: add seller auction setup wizard
```

### P8-S6 · Heat Summary Aggregation

Reference:

- `19-extreme-bidding-atmosphere.md` → `auction_heat_updated`, `北极星指标`.
- `20-ui-ux-redesign.md` → `Host Live Assist`.

Scope:

- Add room/auction heat summary endpoint:
  - active bidders 30s
  - accepted bids 30s
  - chat count 30s
  - reconnect/recovery count if useful
- Do not fabricate watcher count; if not measured, omit or label unavailable.

Validation:

- Backend tests for aggregation.
- UI displays unavailable state honestly.

Commit:

```text
feat: add live auction heat summary
```

## P9 · Trust, Advanced Auction UX, And Diagnostics

Goal: add differentiated product depth that answers judge questions beyond UI polish.

### P9-S1 · Timeline Diagnostics Redesign

Reference:

- `20-ui-ux-redesign.md` → `Ops Diagnostics`.
- `19-extreme-bidding-atmosphere.md` → `可恢复一致性`, `事件与数据模型`.

Scope:

- Add flight recorder drawer/timeline in PC.
- Show auction events, outbox delivery, order/payment, recovery/snapshot, anomalies in one sequence.
- Rows show impact and next action.

Validation:

- PC e2e: diagnostic row opens drawer.
- Backend flight recorder already trusted; UI must not fake timeline.

Commit:

```text
feat: add PC flight recorder timeline drawer
```

### P9-S2 · Verified Bidder UX Hooks

Reference:

- `19-extreme-bidding-atmosphere.md` → `反作弊与信任氛围`, Whatnot verified buyer reference.
- `20-ui-ux-redesign.md` → `商品信任详情`.

Scope:

- Add UI and API placeholders for bidder verification/deposit-required states.
- H5 shows clear requirement before bid.
- PC Seller Studio can mark requirement if backend flag exists; otherwise docs and disabled UI only.

Validation:

- E2E: deposit/verification-required state disables bid with clear copy.

Commit:

```text
feat: add verified bidder UX states
```

### P9-S3 · Similar Auction Handoff

Reference:

- `19-extreme-bidding-atmosphere.md` → `输掉`, `Similar auction handoff`.
- `20-ui-ux-redesign.md` → `支付与成交 UX`.

Scope:

- After losing, show next/similar auction suggestions from room auction list.
- No recommendation algorithm claim; use room next scheduled/draft item if available.

Validation:

- H5 e2e: loser result shows next auction when available and does not imply reserved inventory.

Commit:

```text
feat: add loser handoff to next auction
```

### P9-S4 · Max Bid And Pre-Bid ADR

Reference:

- `19-extreme-bidding-atmosphere.md` → `代理出价 / Max Bid / Pre-bid`.
- `20-ui-ux-redesign.md` → `出价方式矩阵`.

Scope:

- Write ADR before implementation:
  - privacy model
  - transaction semantics
  - event model
  - conflict rules
  - UI disclosure
  - abuse/fat-finger behavior
- No runtime code in this slice unless ADR is approved.

Validation:

- ADR passes plan review against PG truth, idempotency, outbox, and privacy.

Commit:

```text
docs: add max bid and pre-bid architecture decision
```

### P9-S5 · Max Bid/Pre-Bid Implementation Slice Set

Reference:

- `19-extreme-bidding-atmosphere.md` → `Max Bid / Pre-bid`.

Prerequisite:

- P9-S4 accepted ADR.

Sub-slices:

1. DB schema and repository for private max bid intents.
2. API for create/update/cancel intent.
3. Auction transaction integration under row lock.
4. Public/private event model.
5. H5 Max Bid sheet and disclosure.
6. PC/Seller pre-bid visibility and audit.
7. Abuse/fat-finger tests.

Each sub-slice must be one commit and separately tested. Do not batch all of Max Bid into one large commit.

### P9-S6 · Risk And Abuse UX

Reference:

- `19-extreme-bidding-atmosphere.md` → `反作弊与信任氛围`.

Scope:

- Add risk flags to diagnostics for:
  - repeated unpaid wins
  - suspicious linked bidder pattern placeholder
  - repeated high bid then cancel/fail payment
- Keep user-facing copy neutral.

Validation:

- Backend test for risk event producer if implemented.
- PC diagnostics show risk without public shaming.

Commit:

```text
feat: add auction risk diagnostics UX
```

## P10 · Evidence, Accessibility, And Demo Packaging

Goal: make the redesign defensible in front of judges and safe to demo repeatedly.

### P10-S1 · Accessibility And Reduced Motion Gate

Reference:

- `19-extreme-bidding-atmosphere.md` → `Web 平台约束`.
- `20-ui-ux-redesign.md` → `可访问性`.

Scope:

- Add tests for:
  - reduced motion disables movement effects
  - aria-live for event cues
  - color not sole state indicator
  - touch target minimums where practical

Validation:

- Playwright/accessibility checks.

Commit:

```text
test: add accessibility gates for auction UI
```

### P10-S2 · UI Performance Gate

Reference:

- `19-extreme-bidding-atmosphere.md` → `测试门禁`.
- `20-ui-ux-redesign.md` → `动效系统`.

Scope:

- Expand longtask and layout-shift checks under rapid bid events.
- Validate Bid Dock stays stable.

Validation:

- `pnpm test:e2e` includes longtask scenario.

Commit:

```text
test: harden UI performance gates for atmosphere effects
```

### P10-S3 · No-Mock Auction Demo Script And Judge Walkthrough

Reference:

- `19-extreme-bidding-atmosphere.md` → `评委拷打口径`.
- `20-ui-ux-redesign.md` → `评委拷打口径`.

Scope:

- Define the judge-facing demo as a real backend auction path, not a route-mocked UI playback.
- Main trunk:
  - PC host uses the local demo room/session boundary; P10 does not add registration, OAuth, SMS, or password flows.
  - PC creates a product using demo product copy plus real media asset upload/URL.
  - PC creates auction rules, schedules/starts the auction, and optionally marks the item as narrating.
  - H5 enters the room, loads auctions/snapshot from the backend, obtains a real WS ticket, and places real bids.
  - Backend persists bids/events/outbox through PostgreSQL and delivers realtime updates through Redis/WebSocket.
  - H5 shows pending, accepted, outbid/rejected, extension, countdown, rank, atmosphere, and terminal result from server events.
  - PC diagnostics and flight recorder show the same auction's real rules, bids, events, outbox delivery, orders, snapshots, and anomalies.
- Demo media:
  - Real live streaming is out of scope.
  - Use a local looping product video or product image as the live-stage visual asset; this is demo content, not mocked auction state.
- Payment:
  - Real external payment is out of scope.
  - The main P10 no-mock auction demo may stop at SOLD/order creation; local fake-provider payment can be shown only as a labeled optional extension.
- Extension branches:
  - cancel an active auction and show terminal UI plus flight-recorder evidence;
  - edit DRAFT rules before schedule and show backend validation for cap/increment;
  - inspect generated order rows after SOLD without claiming real payment;
  - show recovery/gap behavior using real snapshot/reconnect paths;
  - show P6/P7 high-pressure atmosphere: sticky Bid Dock, rank strip, authoritative bid hints, extension explanation, sound/haptic opt-in, and reduced-motion behavior.
- Include exact talking points and evidence links.
- Explicitly separate:
  - allowed demo sample data: product title, product description, image, and looping video asset;
  - allowed local identity setup: seeded demo host/user/room/session boundary;
  - forbidden demo shortcuts: Playwright `page.route` API mocks, local fake bid success, route-mocked diagnostics, or pre-seeded ACTIVE auction masquerading as a PC-created auction.

Validation:

- Manual dry run from README commands plus a no-route-mock smoke that creates item/auction/start through backend APIs, bids through H5/backend APIs, and verifies the flight recorder for the created auction.

Commit:

```text
docs: add no-mock auction demo walkthrough
```

### P10-S4 · Evidence Ledger Update

Reference:

- Both docs, all P5+ gates.

Scope:

- Add evidence records for visual regression, e2e, accessibility, performance, live smoke.
- Link to screenshots, commands, and known limits.
- Evidence ledger must label route-mocked Playwright tests as UI contract coverage only.
- Evidence ledger must separately list the no-route-mock P10 demo smoke and the exact created auction ID or captured flight-recorder path.
- Demo evidence must not claim real live streaming, real payment, registration, OAuth, SMS, or production capacity unless those systems have dedicated implementation and evidence.

Validation:

- Ship gate review can find evidence paths.

Commit:

```text
docs: record P5 UI atmosphere evidence
```

## Cross-Phase Dependency Rules

- P6 may start after P5-S1 and P5-S3.
- P7 may start after P6-S2 because effects need the Bid Dock target.
- P8 may start after P5-S4; it does not need P6/P7.
- P9-S4 ADR can start anytime, but runtime Max Bid work waits until P7 leaderboard/action UX is stable.
- P10 gates should be added incrementally, not saved until the end.

## Branch And Commit Discipline

Recommended branch naming:

```text
p5-ui-foundation
p6-h5-cockpit
p7-atmosphere-engine
p8-host-seller-studio
p9-trust-advanced-auction
p10-ui-evidence
```

Every slice commit should include:

- implementation files;
- focused tests or explicit reason if docs-only;
- no unrelated refactors;
- no fake data unless route-mocked tests clearly label it as UI contract coverage;
- updated docs/evidence when the slice changes a release/demo claim.

## Slice Exit Template

Use this at the end of each slice:

```text
Slice:
Reference:
Changed:
Validation:
Screenshots/evidence:
Known limits:
Next slice:
```

## First Implementation Recommendation

Start with P5-S1, P5-S2, and P5-S3:

1. Tokens prevent visual drift.
2. Screenshot gates make the redesign measurable.
3. H5 component boundaries reduce risk before moving layout.

Do not start with Max Bid, Prompter, or full PC redesign. Those are differentiators, but they are harder to validate if the visual foundation and H5 cockpit are still unstable.
