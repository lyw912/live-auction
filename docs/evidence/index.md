# Evidence Index

> Date: 2026-05-24 Asia/Shanghai  
> Status: authoritative evidence map for P3/P4 reset.

## Classification

- `AUTHORITATIVE`: current decision input.
- `PARTIAL`: valid for a specific conclusion, but bounded by a known caveat.
- `HARNESS_ONLY`: proves scripts, seed, auth, or instrumentation can run; not bottleneck evidence.
- `SUPERSEDED`: replaced by newer evidence or policy.
- `RAW_LOCAL`: raw output exists but should be opened only through compact analysis or a named investigation.

## Authoritative Evidence

| Evidence | Classification | Current use |
|---|---|---|
| `docs/evidence/p2-01-real-session-boundary.md` | AUTHORITATIVE | P2 real session shortcut removed. |
| `docs/evidence/p2-02-room-membership-acl.md` | AUTHORITATIVE | Room membership/ACL is first-class for REST and WS. |
| `docs/evidence/p2-03-room-context-routing.md` | AUTHORITATIVE | Fixed `room_main` path removed from product flow. |
| `docs/evidence/p2-04-bid-admission-control.md` | AUTHORITATIVE | Admission exists as product protection, not as performance exploration. |
| `docs/evidence/p2-05-payment-provider-boundary.md` | AUTHORITATIVE | Payment provider mock has webhook/idempotency/reconciliation semantics. |
| `docs/evidence/p2-06-security-abuse-diagnostics.md` | AUTHORITATIVE | Security/abuse diagnostics have real producers. |
| `docs/evidence/p2-07-release-baseline-harness.md` | AUTHORITATIVE | Local baseline harness and final Linux guardrails exist. |
| `docs/evidence/p3-01-outbox-claim-fix-2026-05-24.md` | AUTHORITATIVE | Outbox claim bottleneck fixed for the tested local profile. |
| `docs/evidence/p3-02-relay-shard-ownership-2026-05-24.md` | AUTHORITATIVE | Relay shard ownership/failover is implemented with Windows-local evidence. |
| `docs/evidence/p3-03-local-stress-harness-2026-05-24.md` | AUTHORITATIVE_FOR_HARNESS | P3 runner isolation, zero-check detection, and workload management are fixed. |
| `docs/reviews/p3-04-centrifugo-judge-origin-2026-05-25.md` | AUTHORITATIVE | Original hostile Centrifugo comparison that triggered P3 realtime hardening. |
| `docs/evidence/p3-05-centrifugo-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | Bounded recovery, byte backpressure, stream epoch, outbox notify wakeup, and metrics implemented with focused tests. |
| `docs/adr/p3-01-centrifugo-borrowing-decision.md` | AUTHORITATIVE | Decision to borrow Centrifugo mechanisms without adding a second runtime transport. |
| `docs/reviews/p3-06-debezium-borrowing-review-2026-05-25.md` | AUTHORITATIVE | Hostile Debezium comparison: borrow CDC/outbox design discipline, do not rebuild runtime around Debezium. |
| `docs/adr/p3-02-debezium-borrowing-decision.md` | AUTHORITATIVE | Decision to keep Debezium/CDC evidence-gated and map borrowed ideas to project-owned outbox/recovery semantics. |
| `docs/evidence/p3-06-debezium-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | Debezium-borrowed envelope validation, relay watermarks, snapshot audit, control signals, and error classification implemented with focused tests. |
| `docs/reviews/p3-07-nats-jetstream-borrowing-review-2026-05-25.md` | AUTHORITATIVE | Hostile NATS/JetStream comparison: borrow delivery-state, ack/redelivery, dedupe, slow-consumer, monitoring, and snapshot/catchup design logic; do not rebuild runtime around a broker without measured internal messaging need. |
| `docs/evidence/p3-07-nats-jetstream-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | NATS/JetStream-borrowed delivery diagnostics implemented in app-owned outbox/realtime code with full backend and frontend typecheck verification. |
| `docs/reviews/p3-08-redis-lua-borrowing-review-2026-05-25.md` | AUTHORITATIVE | Hostile Redis Lua comparison: accept existing Lua GCRA admission and one-time WS ticket borrowing; keep PostgreSQL as auction money truth; keep Lua reservation/full rewrite evidence-gated behind PG hot-row proof, reconciliation ADR, and invariant tests. |
| `docs/evidence/p3-08-redis-lua-borrowed-hardening-2026-05-25.md` | AUTHORITATIVE | Redis Lua borrowed hardening implemented in app-owned code: script runner, stable script names, `EVALSHA` fallback discipline, script metrics/error classes, hash-tagged admission keys, and focused tests. |
| `docs/evidence/p3-09-raw-artifact-retention-cleanup-2026-05-25.md` | AUTHORITATIVE_FOR_EVIDENCE_HYGIENE | `docs/perf/raw` cleanup applied the current retention policy: keep evidence-referenced compact artifacts and delete old full logs, duplicate failed attempts, and unreferenced ignored raw directories. |
| `docs/evidence/p3-10-admission-off-harness-proof-2026-05-25.md` | AUTHORITATIVE_FOR_HARNESS | P3-R1 closed: committed downstream workloads prove `ADMISSION_ENABLED=false`, `auction_admission_enabled 0/0`, and zero admission reject delta in compact reports. |
| `docs/evidence/p3-11-multi-room-hot-cold-stress-2026-05-25.md` | AUTHORITATIVE | P3-R2 found shared bid-path DB/lock pressure: hot-room bid load degraded cold-room bid p95 from about `25ms` to about `506ms` without cross-room leak, cold WS error, or admission pollution. |
| `docs/evidence/p3-12-realtime-fanout-drilldown-2026-05-25.md` | AUTHORITATIVE | P3-R3 clean realtime drilldown: self-hub passed 300 watcher fanout, healthy-vs-slow isolation, and 100-VU reconnect recovery; higher profiles expose PG/recovery ceilings, not fanout failure. |
| `docs/evidence/p3-13-pg-hot-row-drilldown-2026-05-25.md` | AUTHORITATIVE | P3-R4 confirmed PostgreSQL hot auction row contention under clean admission-off bid pressure and implemented a conservative transaction-work reduction that lowered same-profile local p99 and lock/pool wait. Outbox pending remains the next bottleneck input. |
| `docs/evidence/p3-14-outbox-second-order-pressure-2026-05-25.md` | AUTHORITATIVE | P3-R5 found outbox relay watermark refresh as a second-order drain bottleneck and optimized batch drain to refresh watermarks once per touched shard. Relay drain improved about 4.6x in the tested post-observe window, but backlog still remained. |
| `docs/evidence/p3-15-architecture-go-no-go-2026-05-25.md` | AUTHORITATIVE | P3-R6 keeps the current release-track architecture: PostgreSQL bid truth, app-owned DB outbox relay, Redis projection/history, and self-hub realtime. Redis Lua reservation, Debezium/CDC, and NATS/JetStream runtime remain no-go for this P3 cycle without new ADR/invariant evidence. |
| `docs/evidence/p3-16-final-local-ceiling-sweep-2026-05-26.md` | AUTHORITATIVE | P3-R7 records the final Windows-local downstream ceiling table after P3-R4/P3-R5. It applies a small relay drain optimization, confirms outbox drain improvement, and classifies bid escalation beyond the clean profile as k6 VU ceiling caused by DB row-lock latency growth. |
| `docs/evidence/p3-17-admission-calibration-2026-05-26.md` | AUTHORITATIVE | P3-R8 calibrates local admission-on protection below the R7 cliff: controlled `RATE_LIMITED` and `BID_AUCTION_TOO_HOT` 429 responses with `Retry-After`, no dropped iterations, no downstream backlog collapse, and local default auction limits of `80/s` and `32` in-flight. |
| `docs/evidence/p4-01-invariant-verifier-2026-05-26.md` | AUTHORITATIVE | P4-R1 verifier implemented and integrated into P3 runner: seq, terminal, winner/price/order, idempotency, outbox coverage/order, DEAD anomaly, and room isolation are machine-checkable after stress runs. |
| `docs/evidence/p4-02-auction-flight-recorder-2026-05-26.md` | AUTHORITATIVE | P4-R2 flight recorder implemented as a host-only monitor API: one auction's rules, bids, events, outbox delivery, orders, payment events, snapshots, and anomalies are available as a forensic timeline. |
| `docs/evidence/p4-03-risk-simulator-2026-05-26.md` | AUTHORITATIVE | P4-R3 risk simulator implemented: repeatable real-backend scenarios for bid idempotency abuse, flight-recorder ACL, cap SOLD order/payment double click, and per-scenario scoped invariants. |
| `docs/evidence/p5-01-design-tokens.md` | AUTHORITATIVE | P5-S1 shared Auction Studio design tokens added and wired into H5/PC style entries with `pnpm build` evidence. |
| `docs/evidence/p5-02-visual-regression-gates.md` | AUTHORITATIVE | P5-S2 route-mocked H5/PC visual regression harness added with committed Playwright screenshot baselines and passing targeted visual run. |
| `docs/evidence/p5-03-h5-component-boundaries.md` | AUTHORITATIVE | P5-S3 H5 rendering split into LiveStage, AuctionStatePanel, LeaderboardPanel, HistoryPanel, and ChatPanel boundaries with build, H5 e2e, visual, and full e2e evidence. |
| `docs/evidence/p5-04-pc-component-boundaries.md` | AUTHORITATIVE | P5-S4 PC console rendering split into AuctionCommandPanel, AuctionQueue, RuleEditor, OrdersPanel, DiagnosticsPanel, and EventTimeline boundaries with PC typecheck, E2E, and visual evidence. |
| `docs/evidence/p6-01-h5-live-stage-product-visuals.md` | AUTHORITATIVE | P6-S1 H5 live stage now uses item media/proof metadata, top live bar, and bounded stage chat overlay with H5 build, behavior, 360px safe-zone, and visual evidence. |
| `docs/evidence/p6-02-h5-sticky-bid-dock.md` | AUTHORITATIVE | P6-S2 H5 sticky BidDock keeps price/countdown/rank/CTA visible at 390x844 and 360px, preserves no-optimistic-success behavior, and updates H5 visual baselines. |
| `docs/evidence/p6-03-h5-bottom-sheet-navigation.md` | AUTHORITATIVE | P6-S3 H5 bottom sheet navigation moves product/rules/leaderboard/history/orders into sheet tabs while keeping the fixed BidDock singular and visible, with H5 build, behavior, visual, and full e2e evidence. |
| `docs/evidence/p6-04-h5-product-trust-sheet.md` | AUTHORITATIVE | P6-S4 H5 product trust sheet adds item media, proof details, deposit/payment, cap, extension, fat-finger, and after-sale explanations in bidder-facing language with H5 build, behavior, visual, and full e2e evidence. |
| `docs/evidence/p6-05-h5-auction-result-sheets.md` | AUTHORITATIVE | P6-S5 H5 winner/loser/unsold result sheets add payment handoff, final gap/result explanation, next-item continuation, and disabled dangerous actions with H5 build, behavior, visual, and full e2e evidence. |
| `docs/evidence/p7-01-h5-atmosphere-engine.md` | AUTHORITATIVE | P7-S1 H5 atmosphere engine normalizes event-driven cues, carries server-truth metadata, dedupes recovery replays, and tests priority ordering. |
| `docs/evidence/p7-02-h5-event-driven-effects.md` | AUTHORITATIVE | P7-S2 H5 event effects add bounded price tick, leading ring, outbid edge flash, extension stretch, sold mark, reduced-motion fallback, CTA overlap checks, longtask gate, and visual regression evidence. |
| `docs/evidence/p7-03-leaderboard-action-metrics.md` | AUTHORITATIVE | P7-S3 extends the leaderboard API with PostgreSQL-derived seq, server time, next valid bid, adjacent-rank gap, user state, and 30s action stats while preserving old fields. |
| `docs/evidence/p7-04-h5-rankstrip-leaderboard-sheet.md` | AUTHORITATIVE | P7-S4 adds action-oriented H5 RankStrip and leaderboard sheet using P7-S3 fields, updates visual baselines, and proves Bid Dock/result CTA are not blocked. |
| `docs/evidence/p7-04b-official-bid-hint-states.md` | AUTHORITATIVE | P7-S4b adds amount-adjacent H5 bid hints for self-leading, multi-step bids, and authoritative price changes while preserving server-authoritative bidding. |
| `docs/evidence/p7-05-sound-haptic-policy.md` | AUTHORITATIVE | P7-S5 adds opt-in AudioContext reuse, capability fallback, event-specific sound/haptic patterns, hidden-tab skip, reduced-motion haptic suppression, and playback tests. |
| `docs/evidence/p7-06-countdown-extension-ux.md` | AUTHORITATIVE | P7-S6 adds stable last-10-second tenths countdown, event-authoritative extension old/new end-time copy, local-zero syncing behavior, updated H5 visual baselines, and PC E2E/visual port verification notes. |
| `docs/evidence/p8-01-pc-command-center-layout.md` | AUTHORITATIVE | P8-S1 rebuilds the PC console into a top health ribbon, left auction queue, center command panel, right live assist rail, and secondary workspace while preserving existing host workflows. |
| `docs/evidence/p8-02-pc-auction-queue-active-pinning.md` | AUTHORITATIVE | P8-S2 groups the PC auction queue, pins ACTIVE lots, and makes ACTIVE/narrating constraints visible while preserving backend authority. |
| `docs/evidence/p8-03-host-prompter-backend.md` | AUTHORITATIVE | P8-S3 adds a host-only read-only prompter API from real auction, bid, event, and order data without mutating auction truth or auto-sending chat. |
| `docs/evidence/p8-04-pc-host-live-assist-panel.md` | AUTHORITATIVE | P8-S4 renders host prompter cards, talk points, risk hints, and disabled system-chat scope in the PC Live Assist rail without mutating auction state. |
| `docs/evidence/p8-05-seller-rule-wizard-preview.md` | AUTHORITATIVE | P8-S5 converts PC seller setup into product/price/time/trust/preview wizard sections with H5 rule preview while preserving backend DRAFT-only validation. |
| `docs/evidence/p8-06-live-auction-heat-summary.md` | AUTHORITATIVE | P8-S6 adds a host-only PostgreSQL-backed 30s heat summary for PC Live Assist and labels watcher count unavailable instead of fabricating presence. |
| `docs/evidence/p9-01-pc-flight-recorder-timeline-drawer.md` | AUTHORITATIVE | P9-S1 adds an in-app PC flight recorder drawer that renders backend timeline rows with impact and next-action guidance without fabricating diagnostics. |
| `docs/evidence/p9-02-verified-bidder-ux-states.md` | AUTHORITATIVE | P9-S2 adds H5 verified-bidder/deposit-required UX hooks from optional server payloads and a disabled PC placeholder without claiming backend enforcement. |
| `docs/evidence/p9-03-similar-auction-handoff.md` | AUTHORITATIVE | P9-S3 adds H5 loser/unsold room-list handoff cards that show the next visible auction without claiming recommendations or reserved inventory. |
| `docs/adr/p9-04-max-bid-pre-bid-decision.md` | AUTHORITATIVE | P9-S4 accepts Max Bid/Pre-Bid as private PostgreSQL intents with row-lock settlement, public/private event boundaries, and P9-S5 implementation gates. |
| `docs/evidence/p9-04-max-bid-pre-bid-adr.md` | AUTHORITATIVE | P9-S4 evidence records the ADR review against PostgreSQL truth, idempotency, outbox, privacy, and docs-only validation. |
| `docs/evidence/p9-05-1-max-bid-intent-storage.md` | AUTHORITATIVE | P9-S5-1 adds PostgreSQL private max-bid intent storage, repository validation, and explicit manual bid source without exposing APIs or automatic bidding. |
| `docs/evidence/p9-05-2-max-bid-intent-api.md` | AUTHORITATIVE | P9-S5-2 exposes current-user private intent GET/PUT/DELETE APIs with membership ACL and idempotency, without automatic bidding or public realtime events. |
| `docs/evidence/p9-05-3-max-bid-transaction-integration.md` | AUTHORITATIVE | P9-S5-3 executes automatic Max Bid/Pre-Bid settlement under the auction row lock with real bid/event/outbox/order rows and public payload privacy checks. |
| `docs/evidence/p9-05-4-max-bid-event-recovery-model.md` | AUTHORITATIVE | P9-S5-4 keeps public realtime/Redis snapshots free of private Max Bid data while exposing current-user intent state through authenticated REST snapshots. |
| `docs/evidence/p9-05-5-h5-max-bid-sheet.md` | AUTHORITATIVE | P9-S5-5 adds H5 Max Bid sheet controls with committed API responses, privacy disclosure, recovery disabling, and updated visual baselines. |
| `docs/evidence/p9-05-6-pc-max-bid-readiness-audit.md` | AUTHORITATIVE | P9-S5-6 adds a host-only PC Max Bid readiness aggregate and flight recorder source metadata without exposing private ceilings or ranking bidders by max amount. |
| `docs/evidence/p9-05-7-max-bid-abuse-boundaries.md` | AUTHORITATIVE | P9-S5-7 hardens Max Bid idempotency identity and proves unsafe amount, terminal state, stuck processing, churn, and H5 reject recovery boundaries. |
| `docs/evidence/p9-06-risk-and-abuse-ux.md` | AUTHORITATIVE | P9-S6 adds H5 actionable abuse/reject guidance and a PC host risk queue sourced only from real rejects, anomalies, and recovery diagnostics. |
| `docs/evidence/p10-01-accessibility-reduced-motion.md` | AUTHORITATIVE | P10-S1 adds targeted H5/PC accessibility and reduced-motion gates for live regions, non-color state cues, touch targets, dialog semantics, and status regions. |
| `docs/design-v2-industrial/17-local-stress-and-p3-execution-plan.md` | AUTHORITATIVE | P3/P4 pressure protocol and admission-off policy. |
| `docs/design-v2-industrial/18-p3-p4-roadmap-reset.md` | AUTHORITATIVE | Current P3/P4 execution order and decision gates. |
| `docs/p3-decision-log.md` | AUTHORITATIVE | Current decisions, superseded evidence, and go/no-go gates, including P9-D01 reopening Max Bid/Pre-Bid through ADR only. |

## Partial Evidence

| Evidence | Classification | Keep for | Caveat |
|---|---|---|---|
| `docs/evidence/p3-00-stress-attacker-round-1-2026-05-24.md` | PARTIAL | Discovery of outbox claim O(pending squared) bottleneck and PG hot-row direction. | Used raised admission ceilings; future downstream evidence must use `ADMISSION_ENABLED=false`. |
| `docs/evidence/p3-01-realtime-fanout-attack-2026-05-24.md` | PARTIAL | Realtime self-hub direction, connection-storm classification, slow-consumer harness. | Needs clean admission-off reruns and Linux calibration before final transport decision. |
| `docs/perf/p2-07-linux-baseline-round-1.md` | PARTIAL | Baseline harness guardrail. | Not final P5 3-run capacity evidence. |
| `docs/perf/windows-local-k6-smoke-2026-05-23.md` | PARTIAL | Early local workload smoke. | Not adversarial P3 bottleneck evidence. |

## Harness-Only Evidence

| Evidence | Classification | Reason |
|---|---|---|
| Early `docs/perf/raw/p3-00/` bundles | HARNESS_ONLY | Proved seed/auth/scripts could run; admission polluted downstream attribution. |
| `docs/perf/raw/p3-local-stress-202605240620/` | HARNESS_ONLY | Admission-on smoke, useful for protection behavior and runner sanity. |
| `docs/perf/raw/p3-local-stress-202605240623/` | HARNESS_ONLY | Downstream realtime/isolation smoke; not adversarial enough. |

## Raw Artifact Policy

Default behavior:

1. Run `pnpm exec node tests/load/analyze-p3-artifacts.mjs`.
2. Read `docs/perf/raw/p3-artifact-index.json` and the relevant `analysis-compact.*` files.
3. Open at most the raw files named by the compact report for the suspected bottleneck.

Do not bulk-read `docs/perf/raw/**`.

Keep:

- compact reports;
- summaries;
- evidence markdown;
- raw paths referenced by authoritative evidence.

Ignore unless investigating:

- old raw bundles not referenced by this index;
- full logs from smoke runs;
- raw files from runs classified as harness-only.

Historical cleanup:

- `docs/evidence/p3-09-raw-artifact-retention-cleanup-2026-05-25.md` records the 2026-05-25 cleanup of pre-policy raw artifacts.

Clean or archive later only after confirming no evidence document references the raw path.

## Evidence Still Missing

| Gap | Why it matters | Next evidence |
|---|---|---|
| PG hot-row attribution after outbox fix | Closed by P3-R4 for Windows-local direction evidence. | `docs/evidence/p3-13-pg-hot-row-drilldown-2026-05-25.md`; final Linux capacity still separate. |
| Outbox second-order pressure | Closed for current Windows-local direction evidence; optimized watermark refresh and later tuned R7 batch drain, but backlog still remains under high local input. | `docs/evidence/p3-14-outbox-second-order-pressure-2026-05-25.md`; `docs/evidence/p3-16-final-local-ceiling-sweep-2026-05-26.md`. |
| P4 invariant verifier | Closed for P4-R1. | `docs/evidence/p4-01-invariant-verifier-2026-05-26.md`; Redis/browser projection correctness remains covered by realtime tests and future P4-R2/R3 evidence. |
| P4 auction flight recorder | Closed for P4-R2. | `docs/evidence/p4-02-auction-flight-recorder-2026-05-26.md`; PC UI surfacing can be improved later, but the host-only API and integration proof exist. |
| P4 risk simulator | Closed for P4-R3. | `docs/evidence/p4-03-risk-simulator-2026-05-26.md`; extend scenarios as new incident classes are discovered, but current money/ACL/payment smoke gate exists. |
| Final Linux 3-run capacity baseline | Required before any public capacity claim. | P5 Linux native baseline with environment and raw output. |
