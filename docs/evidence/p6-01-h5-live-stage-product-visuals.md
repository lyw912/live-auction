# P6-S1 H5 Live Stage Product Visuals Evidence

> Date: 2026-05-26 Asia/Shanghai  
> Slice: P6-S1 Implement Live Stage With Real Product Visuals  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

- `frontend/mobile-h5/src/main.tsx` now carries item media/proof metadata from room auction responses and auction snapshots into `LiveStage`.
- `LiveStage` uses item `image_url` or `video_poster_url` as the stage background when available, with a fallback only when no item media exists.
- Added a top live bar with `LIVE`, room, connection state, and sound toggle.
- Added product proof chips for certificate, condition, shipping, and deposit.
- Moved live chat messages into a bounded stage overlay safe zone; the composer remains available below the main auction surface and still calls the room chat API.
- No bid state machine, payment path, WebSocket ticket/connect, snapshot recovery, or money truth behavior changed.

## Validation

```text
pnpm --filter mobile-h5 build
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --reporter=line --workers=1
```

Result: PASS, 18 passed.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --grep "live stage uses product media" --reporter=line --workers=1
```

Result: PASS, confirms product media, proof chips, and stage chat safe zone at 360px.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --update-snapshots --reporter=line --workers=1
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --reporter=line --workers=1
```

Result: PASS, 7 updated and 7 passed.

## Review

DESIGN VERDICT: USABLE for P6-S1, not yet complete P6 cockpit.

- The first H5 screen now reads as a live auction stage with real product media, not a static decorative page.
- Price/countdown/status/CTA remain visible through the existing auction panel until P6-S2 moves them into a sticky Bid Dock.
- The chat overlay is bounded inside the stage and the new 360px test proves it does not overlap the CTA.
- Route-mocked screenshot baselines intentionally use deterministic inline product media so visual gates remain reproducible.

## Known Limits

- P6-S2 still owns sticky Bid Dock and first-screen price/countdown/rank/CTA optimization.
- P6-S3 still owns bottom sheets and product list behavior.
- P6-S4 still owns the fuller product trust detail sheet; S1 proof chips are a stage-level preview, not the full trust workflow.
