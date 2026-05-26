# P7-05 Sound And Haptic Policy

> Date: 2026-05-26 Asia/Shanghai  
> Classification: AUTHORITATIVE for P7-S5.

## Change

- AudioContext is created only after the user clicks the sound toggle.
- Enabled sound reuses one AudioContext instead of creating one per cue.
- Unsupported or blocked audio degrades to visual-only state.
- Event-specific tone frequencies and haptic patterns were added for leading, outbid, extended, sold, recovering, and social cues.
- Sound/haptic playback is skipped for hidden tabs; vibration is skipped under `prefers-reduced-motion: reduce`.
- Added a performance-surface test hook so the longtask gate samples post-load interaction work instead of buffered page-load tasks.

## Validation

| Command | Result |
|---|---|
| `pnpm --filter mobile-h5 build` | PASS |
| `pnpm exec playwright test --project=mobile-h5 -g "sound and haptic\|sound policy\|longtask"` | PASS, 3 passed |
| `pnpm exec playwright test --project=mobile-h5` | PASS, 35 passed |
| `pnpm exec playwright test --project=visual-mobile-h5` | PASS, 7 passed |
| `pnpm build` | PASS |
| `pnpm test:e2e` | PASS, 47 passed |

## Review

DESIGN VERDICT: COMPETITIVE.

- No sound or vibration occurs before opt-in.
- AudioContext initialization is user gesture bound.
- Unsupported audio does not throw or block bidding.
- Reduced-motion users keep visual cue fallback without haptic stimulation.
- Hidden tabs do not play cue sound/haptic.

## Known Limits

- P7-S6 still owns last-10-second countdown tenths, extension explanation detail, and local-zero syncing refinement.
