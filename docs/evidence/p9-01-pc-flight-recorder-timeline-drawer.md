# P9-S1 PC Flight Recorder Timeline Drawer Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S1 Timeline Diagnostics Redesign<br>
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`, `docs/design-v2-industrial/08-observability-and-ops.md`

## Changed

PC diagnostics now opens an in-app Flight Recorder drawer from auction-scoped diagnostic rows instead of sending the host to a raw JSON tab.

The drawer consumes the existing backend endpoint:

```text
GET /api/monitor/auctions/{auction_id}/flight-recorder?limit=80&timeline_limit=120
```

It renders:

- auction summary: auction id, item title, status/seq, current price;
- evidence counts for rules, orders, payment events, anomalies, and timeline rows;
- one ordered timeline from backend rows covering auction events, bid rows, outbox delivery, orders, payment events, snapshot rebuilds, and anomalies;
- per-row `Impact` and `Next action` copy derived from the row kind/status/event type.

The Live Assist recent event area now opens the same drawer. Monitor table rows with an auction id open the drawer directly.

## Validation

```text
pnpm --filter pc-console exec tsc --noEmit
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console --reporter=line --workers=1
```

Result: PASS, 9 passed. Added coverage that a diagnostics row opens the drawer and displays auction event, bid, outbox, order, payment, snapshot, anomaly, impact, and next-action content from the flight-recorder payload.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-pc-console --update-snapshots --reporter=line --workers=1
```

Result: PASS, 1 passed. PC visual baseline regenerated after replacing raw flight-recorder links with drawer open controls.

## Review

- No backend mutation path changed.
- No auction truth, bid, order, payment, outbox, WebSocket, or scheduler behavior changed.
- The drawer does not fabricate rows. It renders only the returned backend flight recorder payload.
- Impact/next-action copy is explanatory guidance based on row type and status; it does not claim new backend evidence.
- Rows without an auction id do not invent an auction-scoped drilldown.

## Known Limits

- The drawer is a diagnostic view, not a timeline editor or incident workflow.
- Route-mocked Playwright coverage proves UI contract only. Live/no-mock evidence remains owned by P10 demo smoke.
