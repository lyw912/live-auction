# Phase 1 Frontend Rebuild Plan

Goal: implement Phase 1 from `planning/01-frontend-media-payment-refactor.md` and `planning/02-phase1-rebuild-execution-guide.md` without backend changes, without media/payment scope creep, and without degrading correctness.

## Current Status

| WP | Name | Status |
|---|---|---|
| WP-0 | Scaffold, dependencies, invariant tests | complete |
| WP-1 | Auction Terminal OKLCH tokens and shared UI package | complete |
| WP-2 | H5 state migration boundaries | complete |
| WP-3 | H5 component split and visual rebuild | complete |
| WP-4 | H5 live-media thin seam | complete |
| WP-5 | H5 signature interactions | complete |
| WP-6 | PC Arco to shadcn-equivalent migration | complete |
| WP-7 | PC command-center visualization and realtime UX | complete |
| WP-8 | Payment UI rebuild with mock behavior preserved | complete |
| Final | Review, tests, docs update, commit | complete |

## Governing Documents

- `planning/01-frontend-media-payment-refactor.md`
- `planning/02-phase1-rebuild-execution-guide.md`
- `CLAUDE.md`

## Red-Line Invariants

| ID | Contract | Status |
|---|---|---|
| I1 | Bid response interpretation uses server fields, not HTTP status | covered |
| I2 | WebSocket entry uses one-time ticket and `auction.v1` + `ticket.*` subprotocols | covered |
| I3 | Recovery triggers and sources remain complete | covered |
| I4 | Countdown remains server-time anchored | covered |
| I5 | Dangerous bid actions are disabled during unsafe states | covered |
| I6 | Bid idempotency and 8s uncertain retry semantics remain intact | covered |
| I7 | Payment success uses server `order_status === 'PAID'`; PC remains read-only | covered |

## Hard Boundaries

- Do not change backend code in Phase 1.
- Do not implement Phase 2/3 media: no full `MediaPlayback`, no live session API, no MediaMTX, no hls.js/WebRTC.
- Do not implement real payments: keep `pay-mock`, idempotency, order recheck, and server-truth UI.
- Do not use Motion+ paid APIs.
- Do not merge H5 and PC applications.
- Do not weaken bid, settlement, recovery, WebSocket, or payment correctness.

## Errors Encountered

| Time | Error | Attempt | Resolution |
|---|---|---|---|
| 2026-06-14 | `CLAUDE.md` required doc paths no longer exist | Tried to read `docs/design/01-architecture.md`, `docs/design/02-performance-correctness-contract.md`, `docs/s1-s5/00-overview.md` | Used current `docs/README.md` map and recorded drift in `findings.md` |
| 2026-06-14 | Initial payment idempotency-key test assumed an `h5-` prefix | Ran `pnpm test:frontend:domain` after adding red-line tests | Verified `createClientBidID()` returns UUID when available and changed test to assert non-empty key instead of a false prefix |
| 2026-06-14 | Full e2e initially failed on visual snapshots | Ran `pnpm test:e2e` after Phase 1 visual rebuild | Updated intentional H5 and PC visual baselines, then reran full e2e successfully |
| 2026-06-14 | Final review found PC still depended on Arco | Audited `@arco-design` imports after WP-6/WP-7 | Replaced remaining PC Arco usage with local shadcn-style console primitives, removed Arco dependency/config/CSS, and reran PC/full tests |
