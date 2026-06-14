# Phase 2/3 Media Contract And Live Stream Plan

Goal: complete Phase 2 and Phase 3 from `planning/01-frontend-media-payment-refactor.md` and `planning/03-phase2-media-contract-execution-guide.md` without degrading auction, realtime, recovery, payment, or visual Phase 1 invariants.

## Current Status

| WP | Name | Status |
|---|---|---|
| P2-WP-1 | MediaPlayback contract and schema guards | complete |
| P2-WP-2 | Backend live session descriptor API | complete |
| P2-WP-3 | Frontend media adapter system | complete |
| P2-WP-4 | H5 LiveBackdrop wiring and fallback chain | complete |
| P2-WP-5 | Media/auction decoupling tests | in_progress |
| P2-WP-6 | Static HLS sample verification | pending |
| P2-Final | Review, tests, docs update, commit | pending |
| P3-WP-1 | MediaMTX LL-HLS config and runtime wiring | complete |
| P3-WP-2 | Backend/config descriptor switch to LL-HLS | complete |
| P3-WP-3 | Stream smoke tests and frontend playback verification | in_progress |
| P3-Final | Review, tests, docs update, commit | pending |

## Governing Documents

- `planning/01-frontend-media-payment-refactor.md`
- `planning/03-phase2-media-contract-execution-guide.md`
- `CLAUDE.md`

## Phase 2 Red-Line Invariants

| ID | Contract | Status |
|---|---|---|
| M1 | Live Session API carries only media descriptor fields, never auction truth | pending |
| M2 | Media API/source/player failure cannot break auction UI, bidding, recovery, or countdown | pending |
| M3 | Media state is never used as auction clock, price, winner, seq, or terminal truth | pending |
| M4 | Media query/player is independent from auction WS and error boundary | pending |

## Phase 1 Invariants To Keep Green

| ID | Contract | Status |
|---|---|---|
| I1 | Bid response interpretation uses server fields, not HTTP status | covered by existing tests |
| I2 | WebSocket entry uses one-time ticket and `auction.v1` + `ticket.*` subprotocols | covered by existing tests |
| I3 | Recovery triggers and sources remain complete | covered by existing tests |
| I4 | Countdown remains server-time anchored | covered by existing tests |
| I5 | Dangerous bid actions are disabled during unsafe states | covered by existing tests |
| I6 | Bid idempotency and 8s uncertain retry semantics remain intact | covered by existing tests |
| I7 | Payment success uses server `order_status === 'PAID'`; PC remains read-only | covered by existing tests |

## Hard Boundaries

- Do not put `price`, `winner`, `status`, `seq`, `endAt`, settlement, or rule truth into `GET /api/live/sessions/{id}`.
- Do not change bid, settlement, recovery, WS protocol, or payment core for Phase 2/3 media work.
- Do not add DB columns for media descriptors.
- Do not implement WHEP/WebRTC in Phase 2/3; keep only a disabled adapter seam.
- Do not use Motion+ paid APIs.
- Do not downgrade Phase 3 to static MP4. Phase 3 must add MediaMTX + LL-HLS simulated live stream with MP4 fallback through the same contract.

## Errors Encountered

| Time | Error | Attempt | Resolution |
|---|---|---|---|
| 2026-06-14 | Repo has unrelated dirty/untracked files after Phase 1 | Checked `git status --short` before Phase 2/3 | Leave unrelated `.codex/skills/USAGE.md`, `.claude/`, `phase1-h5-wp3-mobile.png`, `thirdparty/` untouched; use `planning/03...` because it governs this phase |
| 2026-06-14 | Frontend domain test failed importing React Query hook from Node bundle | Bundled `useLiveMediaSource.ts` directly after Phase 2 hook conversion | Split pure `select-source.ts` for query key/source selection tests; left hook module for runtime only |
| 2026-06-14 | Backend gateway tests failed because local Postgres/Redis were not running | Ran `go test ./internal/gateway` before starting infra | Start required docker compose services before rerunning backend tests |
