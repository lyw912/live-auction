# P10-S3 No-Mock Auction Demo Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P10-S3 No-Mock Auction Demo Script And Judge Walkthrough<br>
> Design: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/10-test-gates.md`

## Changed

Added the judge-facing no-mock auction demo policy:

- `docs/demo/p10-no-mock-auction-demo.md`
- P10 updates in `docs/demo/demo-flow.md`
- P10 scope notes in `docs/demo/known-limits.md`

Added a no-route-mock live smoke:

- `tests/e2e/p10-no-mock-live.spec.ts`
- `p10-no-mock-live` project in `tests/e2e/playwright-live.config.ts`
- evidence assertion in `tests/e2e/run-h5-live-backend-smoke.mjs`
- host-only `/api/test/rooms` setup helper, enabled only when `APP_ENV=test`, so the smoke can create an isolated room and remain repeatable after failed runs

The smoke creates a new room, item, and auction through backend APIs, schedules and starts the auction, opens H5 for the room, verifies the live feed product card and then the full bidding panel without browser route interception, sends real bid requests, drives the auction to SOLD, then reads the host-only flight recorder for the same created auction.

The route-mocked visual baselines were regenerated for the current H5 feed -> product card -> bid panel path and the current PC secondary workspace diagnostics tab. These remain UI contract evidence only; they are not no-mock demo proof.

## Evidence

Raw live-smoke output:

- `docs/perf/raw/p10-no-mock-live-smoke.json`

The raw record includes:

- `room_id`
- `smoke_item_id`
- `smoke_auction_id`
- `smoke_item_title`
- `flight_recorder_path`
- `no_browser_route_mocks: true`

Latest raw record:

- `room_id`: `room_p10_1779873267642`
- `smoke_item_id`: `item_f7d1d1e5-2b17-49cc-87ed-9d484c402df9`
- `smoke_auction_id`: `auc_edb3d6e6-73c6-4767-b822-6766f72b1ce1`
- `flight_recorder_path`: `/api/monitor/auctions/auc_edb3d6e6-73c6-4767-b822-6766f72b1ce1/flight-recorder?limit=80&timeline_limit=120`

## Validation

```text
pnpm test:e2e:h5-live
```

Expected result: PASS.

This command runs the existing H5 live backend smoke plus the P10 no-mock smoke. The runner fails if `docs/perf/raw/p10-no-mock-live-smoke.json` is missing, not PASS, lacks created IDs, or does not mark `no_browser_route_mocks: true`.

## Classification

`AUTHORITATIVE_FOR_DEMO_TRUNK`.

This evidence proves the local judge-facing auction trunk is not a route-mocked UI playback. It does not claim real live streaming, real external payment, public registration, OAuth, SMS, or production capacity.

## Known Limits

- Local seeded host/user/room setup is still used.
- `/api/test/rooms` is test-only setup plumbing gated by `APP_ENV=test` and host auth. It is not a production room-management surface.
- The second competing bidder uses local test mock-auth headers so the smoke can prove multi-user bidding without adding a public login/account product surface.
- The media assets are demo content, not proof of a real live streaming stack.
- The H5 default live feed intentionally shows the floating product card first; the full bid CTA, Max Bid, leaderboard, rules, history, orders, and result controls open after tapping that card.
- Payment remains outside the P10 no-mock auction trunk unless shown as the explicitly labeled local fake-provider branch.
