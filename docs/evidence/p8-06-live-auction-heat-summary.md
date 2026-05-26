# P8-S6 Live Auction Heat Summary Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P8-S6 Heat Summary Aggregation<br>
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/19-extreme-bidding-atmosphere.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

Added a host-only heat summary API:

```text
GET /api/host/auctions/{id}/heat-summary
```

The endpoint aggregates recent real backend signals from PostgreSQL:

- active bidders in the last 30 seconds from `bids`;
- accepted and rejected bids in the last 30 seconds from `bids`;
- room chat messages in the last 30 seconds from `chat_messages`;
- reconnect/recovery/slow-consumer events in the last 30 seconds from `user_activity_events`.

The response includes `source: postgres:bids,chat_messages,user_activity_events`. Watcher count is not fabricated; the API returns `watcher_count_available: false` until a measured producer exists.

PC Live Assist now renders the heat summary beside prompter cards, talk points, recovery/outbox/reject summaries, and risk hints. The UI labels watcher count as unavailable instead of estimating it.

## Validation

```text
go test ./internal/gateway -run TestHeatSummaryRequiresHostAndAggregatesRealThirtySecondSignals -count=1 -v
```

Result: PASS.

```text
pnpm --filter pc-console exec tsc --noEmit
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console --reporter=line --workers=1
```

Result: PASS, 8 passed. Added UI contract coverage for real heat summary fields and unavailable watcher count labeling. The route-mocked PC test is UI contract coverage only.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-pc-console --update-snapshots --reporter=line --workers=1
```

Result: PASS, 1 passed. PC visual baseline was regenerated for the heat summary panel.

## Review

- The API is guarded by the existing host middleware.
- The endpoint is read-only and does not write auction events, outbox rows, bids, orders, or payment state.
- PostgreSQL remains the only source for the displayed heat counters.
- No fake viewer/watcher count is introduced.
- The PC display has empty/unavailable wording for failed or missing heat data.

## Known Limits

- Watcher count is unavailable because the current system has no measured presence producer for this panel.
- Heat summary is a recent operational signal, not a performance or capacity claim.
- Route-mocked PC/visual tests prove rendering contracts only; no-mock demo evidence must use a backend-created auction and real API responses.
