# P0 Coverage Ledger

Feature/Gate: P0 gate evidence coverage and known gaps

Date: 2026-05-22

Commit: pending

Environment: Windows 11, Go 1.26.3, Node 24.9.0, pnpm 11.2.2, Docker Desktop infra for backend tests

Command: `pnpm build`; `pnpm test:e2e`; `go test ./...` from `backend`

Raw Output Path: terminal output in development session

## Setup

This ledger maps `docs/design-v2-industrial/10-test-gates.md` P0 gates to committed evidence. It is intentionally conservative:

- Backend correctness gates are only marked covered when backed by backend automated tests or committed evidence.
- Frontend gates backed by Playwright route mocks are marked as UI contract coverage, not live backend/WebSocket integration.
- Load gates are not claimed because no performance numbers are claimed.
- Missing or partially scoped gates stay visible instead of being implied by adjacent work.

## Expected Invariant

Every P0 claim must say whether it is implemented, tested, or future work. A demo or final submission must not present mocked UI coverage as live backend integration, and must not present correctness gates as green without evidence.

## Result

PASS for documentation honesty. This file does not complete the remaining gaps; it makes them explicit.

## Covered Evidence

| Area | Gate coverage | Evidence |
|---|---|---|
| Foundation | repo setup, env, migrations, minimal backend tests | `p0-foundation-backend-tests.md`, `p0-02-db-migrations.md` |
| Auth/ACL | host-only mutation and monitor APIs; user denial | `p0-03-auth-role-acl.md` |
| Auction rules/lifecycle | rule validation, cap reachability, create/schedule/start, PATCH vs START race | `p0-05-auction-rule-lifecycle.md`, `p0-19-pc-rule-validation.md`, `p0-23-pc-rule-save.md`, `p0-26-pc-full-rule-fields.md` |
| Bid truth path | accepted bid, rejected bid, cap SOLD order, payment double click, idempotency, backend fat-finger confirm, H5 live REST bid/confirm/payment/history smoke | `p0-06-p0-07-bid-truth-path.md`, `p0-21-h5-payment-double-click.md`, `p0-24-h5-fat-finger-confirm.md`, `p0-30-h5-live-backend-rest-smoke.md` |
| Outbox/projection | tx-to-outbox, Redis projection, relay ordering | `p0-08-p0-09-outbox-redis-projection.md`, `p0-11-websocket-completion.md` |
| WebSocket/recovery | ticket foundation, browser ticket/subprotocol connect contract, live backend H5 WS smoke, replay/snapshot recovery, bounded reconnect rebuild, Redis-down reconnect degradation, slow consumer/backpressure | `p0-10-websocket-ticket-recovery.md`, `p0-11-websocket-completion.md`, `p0-15-reconnect-storm-snapshot-bounding.md`, `p0-28-h5-websocket-ticket-connect.md`, `p0-29-h5-live-backend-ws-smoke.md`, `p0-31-redis-down-reconnect-evidence.md` |
| Scheduler | end auction, winner/no-winner close, order expiry, retry/lease behavior | `p0-12-scheduler-end-order-expire.md`, `p0-17-clock-step-backward.md` |
| Concurrency | final-second bid, cancel/cap race, narrate race, active race | `p0-13-concurrency-and-narration.md` |
| Diagnostics | real monitor diagnostic producers and host ACL | `p0-14-monitor-diagnostics-apis.md` |
| Degradation | DB lock timeout, idempotency timeout, clock rollback guard, Redis-down reconnect degradation, bid-rate-limit scope adjustment | `p0-16-degradation-db-idempotency.md`, `p0-17-clock-step-backward.md`, `p0-31-redis-down-reconnect-evidence.md`, `p0-32-bid-rate-limit-scope-adjustment.md` |
| H5 UI | state matrix, pending/rejected bid, recovering CTA disable, SOLD winner/loser, payment double-click, fat-finger confirm UI contract, bid/order history, live backend REST smoke, room auction selection and current-auction payment target | `p0-18-frontend-state-surfaces.md`, `p0-20-h5-bid-protocol.md`, `p0-21-h5-payment-double-click.md`, `p0-22-h5-snapshot-recovery.md`, `p0-24-h5-fat-finger-confirm.md`, `p0-25-h5-history-ui.md`, `p0-30-h5-live-backend-rest-smoke.md` |
| PC UI | rule cap validation, backend save error, full P0 rule fields, diagnostics tabs | `p0-19-pc-rule-validation.md`, `p0-23-pc-rule-save.md`, `p0-26-pc-full-rule-fields.md` |

## Mocked vs Live Coverage

The Playwright frontend suite validates browser behavior and API payload contracts by mocking REST responses and browser events. It proves the UI follows P0 rules such as no optimistic bid success, disabled CTA during recovery, and backend-shaped PATCH bodies.

It does not prove:

- PC/H5 run against the live backend for all mocked REST paths.
- Cross-tab or multi-client frontend race behavior under real network jitter.

Those remain separate integration slices.

## Remaining Gaps

| Gate | Current status | Required next evidence |
|---|---|---|
| PC full rule cross-field validation | UI validates cap reachability and field ranges; backend remains authoritative | Add focused UI cases for duration/extension/deposit invalid combinations if desired, without trusting frontend as final authority |
| Animation longtask | No heavy animation is implemented and no performance claim is made | Add browser longtask measurement before claiming animation performance |
| Load gates | Not claimed | Run baseline scripts and store raw outputs before any P99/QPS/fanout claim |

## Failure Interpretation

The remaining gaps are not money-correctness blockers for the backend path already covered by automated tests. They are integration and evidence gaps that affect demo honesty and final P0 completeness claims, especially WebSocket end-to-end frontend behavior.

## Known Limits

- This file is a coverage ledger, not a new executable test.
- The `Commit` field will be finalized by the commit containing this file.
- Frontend E2E evidence remains mock-backed unless a specific evidence file says live backend was used.
- H5 still enters the deterministic local room `room_main`; auction and payment order IDs are now selected from live API responses.

## Next Action

Add focused PC rule cross-field UI cases or animation longtask evidence.
