# Phase 1 Findings

## Initial Repository State

- Root workspace uses pnpm workspaces with `frontend/*`.
- `frontend/shared-design` exists but has no `package.json`; it is currently a shared folder, not a package.
- Root scripts include `build`, `test:e2e`, `test:e2e:h5-live`, and `test:frontend:domain`.
- Existing uncommitted/untracked changes at start: deleted `.codex/skills/USAGE.md`; untracked `.claude/`, `planning/`, `thirdparty/`.
- `git status` emits warnings for denied access to `C:\Users\yewen/.config/git/ignore`; this is outside the repo and not blocking.
- `CLAUDE.md` still points to older `docs/design/*` and `docs/s1-s5/*` paths. The current committed doc entrypoint is `docs/README.md`, which maps architecture to `docs/01-architecture/*`, frontend to `docs/05-frontend/*`, and PTS/evidence to `docs/07-performance-and-evidence/*`.

## Governing Scope Notes

- Phase 1 explicitly keeps backend unchanged.
- Media work in Phase 1 is only a thin `video-file` source seam around the existing demo video.
- Payment work in Phase 1 keeps mock behavior and server-truth status handling.

## Baseline Verification

- `pnpm test:frontend:domain` passed before Phase 1 edits.
- `pnpm build` passed before Phase 1 edits.
- Baseline build still emits a PC chunk warning for the Arco bundle; this is existing WP-6 scope.

## Final Verification Notes

- Phase 1 final verification passed:
  - `pnpm test:frontend:domain`
  - `pnpm build`
  - `pnpm test:e2e` (84/84)
- PC Arco migration was not complete until final review. The remaining Arco imports, Arco CSS, and Vite `arco` chunk were replaced with local shadcn-style console primitives and removed from dependencies.
- The final PC build no longer contains the Arco chunk. The remaining large chunk warning is `CommandVizStrip` (~783 kB minified), caused by ECharts/visx realtime visualization. It is a follow-up optimization target, not a Phase 1 correctness failure.
- E2E runs print repeated Vite proxy `ECONNREFUSED 127.0.0.1:18080` messages for optional unmocked endpoints such as heat summaries, system messages, AI settings, and orders. These are mocked-browser test environment noise when the backend is not running; they did not fail Playwright tests.
- H5 media scope remains Phase 1 only: `demoLiveVideoURL` is only consumed by the `video-file` seam (`useLiveMediaSource` + `LiveBackdrop`). No full MediaPlayback, hls.js, MediaMTX, WHEP, or live session API was implemented.
- Payment scope remains Phase 1 only: H5 still uses `pay-mock` through `features/pay-order/pay-mock-action.ts`, preserves `Idempotency-Key`, and only treats server `order_status === 'PAID'` as success. PC remains read-only for orders/payment.
