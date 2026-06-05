# Current Code Audit

This is the implementation baseline for the atmosphere/AI phase. It intentionally avoids re-auditing PTS-1B internals except where experience features depend on them.

## Strong Foundations To Preserve

| Area | Code proof | Why it matters |
|---|---|---|
| Server-authoritative cue gating | `frontend/mobile-h5/src/main.tsx:176` suppresses cues during recovery and deduplicates by auction/event/seq/scope/kind. | Atmosphere already respects recovery honesty. Preserve this discipline. |
| Drift-safe countdown | `frontend/mobile-h5/src/domain.ts:299` derives remaining time from last synced server time plus local elapsed time. | Client clock skew does not invent auction truth. |
| Sub-second last 10s display | `frontend/mobile-h5/src/domain.ts:288` switches to tenths under 10 seconds. | A good base for final-countdown tension. |
| Leaderboard data model has rich fields | `frontend/mobile-h5/src/domain.ts:228` includes `bid_count`, `active_bidders_30s`, `accepted_bids_30s`, and `price_velocity_cents_per_min`. | Heat and social rank can be rendered without new hot-path logic. |
| Host heat summary already exists | `backend/internal/gateway/heat_summary_handlers.go:27` exposes real 30s aggregate fields for host. | Good proof that heat metrics are real, though H5 currently needs a bidder-safe path. |
| Reduced-motion handling exists | `frontend/mobile-h5/src/styles.css:975` disables animation in reduced-motion mode. | Extend this rather than bypassing it. |

## Visible Gaps That Must Be Fixed First

| Gap | Code proof | Required direction |
|---|---|---|
| Fake viewer count | `frontend/mobile-h5/src/components.tsx:103` renders `2333`. | Replace with real watcher count if available; otherwise render active bidder count or omit. Never fake it. |
| Dead action buttons | `frontend/mobile-h5/src/components.tsx:101`, `:157`, `:158`, `:159` render follow/like/gift/more without meaningful behavior. | Implement small honest behaviors or remove/disable with clear state. |
| Static hype labels | `frontend/mobile-h5/src/components.tsx:118`, `:119`, `:138` include fixed marketing/rank/lot copy. | Bind to item/auction metadata or remove. |
| Unconditional trust claim | `frontend/mobile-h5/src/components.tsx:50` always includes `保证金锁定`. | Only show if deposit or guarantee state is present. |
| Heat fields not rendered in leaderboard rows | `frontend/mobile-h5/src/components.tsx:764` renders amount but not `bid_count` or heat metrics. | Add bid count, active bidder count, accepted bids, and price velocity. |
| Outbid cue depends on stale leaderboard | `frontend/mobile-h5/src/main.tsx:886` triggers outbid only if `previousLeading` from current REST leaderboard was true. | Move to event-authoritative winner transition logic. |
| Cue ID collision risk | `frontend/mobile-h5/src/main.tsx:228` clears active cue by `Date.now()` generated id. | Use monotonic local cue sequence. |
| 10Hz app-wide tick | `frontend/mobile-h5/src/main.tsx:158` updates `nowMS` every 100ms at app root. | Before adding richer DOM, move countdown-only ticking closer to leaf components or memoize aggressively. |

## Current API And Data Shape Implications

### H5 heat rendering

The H5 leaderboard type already expects heat data, but the code path must be verified before UI claims are made:

- if leaderboard API already returns `active_bidders_30s`, `accepted_bids_30s`, and `price_velocity_cents_per_min`, render it directly;
- if not, extend the existing leaderboard response or add a bidder-safe heat endpoint;
- do not call host-only `/api/host/auctions/{id}/heat-summary` from H5.

### AI insertion points

There is no AI provider boundary today. `rg` for `openai`, `llm`, `gpt`, and related terms finds only review prose. The implementation should add a new backend boundary rather than mixing provider calls into gateway handlers.

Recommended package boundaries:

- `backend/internal/ai`: provider client, prompt templates, safety filters, structured schemas;
- `backend/internal/auctionai`: use cases tied to auction/listing/commentary/sentinel;
- `backend/internal/gateway/ai_handlers.go`: host endpoints;
- optional workers under `backend/internal/scheduler` or a new worker package for commentary/sentinel jobs.

## Demo Honesty Notes

The fixed demo video is acceptable because the official brief allows fixed video/open-source simulation. The static viewer count and inert buttons are not acceptable because they create visible product claims that are false or untestable.

## Development Priority From Audit

1. Remove fake/static/inert UI leaks.
2. Make the existing facts visible.
3. Add richer atmosphere only after stale/recovering gates and 10Hz rendering risk are controlled.
4. Add AI behind explicit backend boundaries and flags.
