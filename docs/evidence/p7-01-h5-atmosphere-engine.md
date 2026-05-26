# P7-01 H5 Atmosphere Engine

> Date: 2026-05-26 Asia/Shanghai  
> Classification: AUTHORITATIVE for P7-S1.

## Change

- Added a lightweight H5 atmosphere engine module with event normalization, user scope, priority, and truth metadata.
- Strong cues now carry `auction_id`, `cause_seq`, `event_type`, `user_scope`, and deterministic priority.
- The H5 room dedupes accepted effects by auction/event/seq/user/kind and suppresses effect playback during recovery or disconnection.
- The visible cue exposes metadata via `data-testid="atmosphere-cue"` and `data-*` fields so tests can prove the rendered effect is bound to a server event.

## Validation

| Command | Result |
|---|---|
| `pnpm --filter mobile-h5 build` | PASS |
| `pnpm exec playwright test --project=mobile-h5 -g "atmosphere engine|realtime leaderboard"` | PASS |
| `pnpm exec playwright test --project=mobile-h5 tests/e2e/atmosphere-engine.spec.ts` | PASS |
| `pnpm exec playwright test --project=mobile-h5` | PASS, 28 passed |
| `pnpm exec playwright test --project=visual-mobile-h5` | PASS, 7 passed |
| `pnpm build` | PASS |
| `pnpm test:e2e` | PASS, 40 passed |

## Review

ENGINEERING GATE: PASS.

- PostgreSQL truth and backend event contracts are unchanged.
- Effects are driven by accepted realtime/API outcomes, not local optimistic success.
- Reconnect snapshot recovery does not replay old strong effects.
- Priority order is explicitly tested: SOLD > RECOVERING > EXTENDED > OUTBID > LEADING > SOCIAL.

## Known Limits

- P7-S2 still owns the richer visual motion language: price tick, leading ring, outbid edge flash, extension stretch, and hammer/result moment.
- P7-S5 still owns the full opt-in sound and haptic policy; S1 only preserves the existing sound toggle behavior after a cue is accepted.
- P7-S3, P7-S4, P7-S4b, and P7-S6 remain pending.
