# P0 Product Audit

Date: 2026-05-23

Skill: `live-auction-v2-tiktok-product-auditor`

Design baseline: `docs/design-v2-industrial`

## Verdict

PRODUCT VERDICT: COMPLETE WITH DISCLOSED LIMITS

The backend auction truth path and the H5 live REST/WebSocket demo slice are substantive. This is not a fake-only project: bids, idempotency, fat-finger confirm, outbox, WebSocket ticketing, chat, history, and mock payment all have executable backend or live-smoke coverage.

The product is now complete for P0 with disclosed limits. The prior blockers were fixed: the H5 test state matrix is hidden from normal entry, terminal/payment state is server/order-driven, live smoke proves cap SOLD creates the payable order before payment, `pay-mock` validates the contract body, and PC has a browser-to-live-backend host workflow smoke.

## Verification Run

Commands run during audit:

```text
go test ./...
pnpm test:e2e
pnpm test:e2e:h5-live
```

Result:

- Backend Go tests passed.
- Mock-backed Playwright suite passed: 18 tests.
- H5/PC live backend smoke passed: 2 tests.

## Scope Audit

| Requirement | Code/UI path | Evidence | Verdict | Downgrade/shortcut |
|---|---|---|---|---|
| PC item upload/create | `frontend/pc-console/src/main.tsx`, `backend/internal/gateway/auction_handlers.go` | PC route-mocked E2E plus live PC smoke | PASS | File upload remains mock E2E; live PC smoke uses image URL |
| PC auction creation | `frontend/pc-console/src/main.tsx`, `backend/internal/auction/repository.go` | PC route-mocked E2E plus backend route | PASS | Fixed local demo room `room_main` |
| PC full rule fields and freeze | `frontend/pc-console/src/main.tsx`, `backend/internal/auction/repository.go` | Go tests plus route-mocked E2E plus live PC smoke | PASS | None for P0 |
| PC schedule/start/cancel/narrate | `frontend/pc-console/src/main.tsx`, `backend/internal/gateway/router.go` | Route-mocked E2E plus backend tests | PASS/PARTIAL | No visible unschedule control; narrate conflicts rely on backend authority |
| PC orders | `frontend/pc-console/src/main.tsx`, `backend/internal/auction/bid.go` | Route-mocked E2E plus live PC smoke | PASS | None for P0 |
| PC diagnostics | `frontend/pc-console/src/main.tsx`, `backend/internal/gateway/monitor_handlers.go` | Backend monitor tests plus route-mocked E2E | PASS/PARTIAL | Real API rows, but limited drilldown; not a full diagnostics console |
| H5 room entry and active auction selection | `frontend/mobile-h5/src/main.tsx` | H5 live smoke | PASS WITH LIMIT | Entry room fixed to `room_main`, disclosed |
| H5 WebSocket ticket/connect | `frontend/mobile-h5/src/main.tsx`, `backend/internal/realtime/server.go` | H5 live smoke plus backend tests | PASS | Single-process P0 hub only |
| H5 bid pending/accepted/rejected | `frontend/mobile-h5/src/main.tsx`, `backend/internal/auction/bid.go` | Mock E2E plus live smoke | PASS | No optimistic success observed |
| H5 fat-finger confirm | `frontend/mobile-h5/src/main.tsx`, `backend/internal/auction/bid.go` | Mock E2E plus live smoke | PASS | Server token path exists |
| H5 recovery/gap/stale | `frontend/mobile-h5/src/main.tsx`, `backend/internal/realtime/server.go` | Backend tests plus mock E2E | PASS | Live browser recovery storm not fully proven |
| H5 chat seed/send | `frontend/mobile-h5/src/main.tsx`, `backend/internal/auction/chat.go` | Mock E2E plus live smoke | PASS | Soft social feature, not full moderation/rate-limit product |
| H5 history | `frontend/mobile-h5/src/main.tsx`, `backend/internal/auction/bid.go` | Mock E2E plus live smoke | PASS | Fixed demo room only |
| H5 SOLD winner/payment | `frontend/mobile-h5/src/main.tsx`, `backend/cmd/p0smokeseed/main.go` | Mock E2E plus live smoke | PASS | Cap SOLD generates order, H5 refreshes it, then pays |
| Backend API contract | `backend/internal/gateway/router.go` | Route inspection plus tests | PASS | `pay-mock` validates `confirm: true` |
| Mock auth/room ACL | `backend/internal/gateway/auth.go` | Disclosed known limit | ACCEPTABLE LIMIT/PARTIAL | Real room membership/auth is not implemented |
| Performance claims | README/docs/evidence/perf | Docs review | PASS | No production QPS/P99/fanout number claimed |

## Workflow Walkthrough

- Host workflow: PC can create item, create auction, save full rules, schedule/start/cancel/narrate, view orders, and view diagnostics. Live PC smoke proves browser-to-backend item/auction/rule/lifecycle/diagnostics flow; file upload remains covered by route-mocked E2E.
- Bidder workflow: H5 loads `room_main`, selects active auction from backend response, obtains WS ticket, connects, bids to cap, confirms high bid, receives/loads the generated order, sends chat, views history, and pays mock order in the live smoke.
- Recovery workflow: H5 detects sequence gaps or `outbox_gap_notice`, disables CTA, fetches snapshot, keeps stale state disabled, and resumes on fresh snapshot.
- Diagnostics workflow: Backend monitor APIs query real PostgreSQL tables for auctions, rejects, recovery, anomalies, outbox, and scheduler. PC shows them as tables. Drilldown is shallow.

## Unacceptable Shortcuts

None remaining from this audit.

## Acceptable Disclosed Limits

- Deterministic local room `room_main` is disclosed in `docs/demo/known-limits.md`.
- Mock auth via `X-Mock-Role` and `X-Mock-User-Id` is disclosed.
- Mock payment is disclosed as P0 scope.
- Frontend Playwright route mocks are disclosed as UI contract coverage, not live backend coverage.
- Local load smoke is disclosed as smoke evidence only, with no production QPS/P99/fanout claim.
- Single-process WebSocket hub is P0 scope; multi-instance fanout remains future architecture.

## Underdisclosed Limits

- Real room membership is not implemented. The docs disclose mock auth and now explicitly state any mock user can enter a valid local demo room.

## Test-Only / Mock-Only Coverage

- `tests/e2e/pc-console.spec.ts`: PC host browser behavior is route-mocked.
- `tests/e2e/mobile-h5.spec.ts`: most H5 state matrix, terminal events, recovery snapshots, payment double-click, and chat assertions are route/WebSocket mocked.
- `tests/e2e/mobile-h5-live.spec.ts`: H5 live REST/WS smoke is real backend integration using deterministic seeded `room_main` and `auc_live`; it now generates the payable order via cap SOLD before payment.
- `tests/e2e/pc-console-live.spec.ts`: PC live host workflow is real backend integration for item/auction/rule/lifecycle/diagnostics.
- Backend Go tests prove domain/API invariants, but they are not browser product workflow proof.

## P1 Readiness

May start P1: yes.

Conditions:

1. Do not publish production QPS/P99/fanout numbers until formal 3-run baseline exists.
2. Keep mock auth/payment and fixed demo room disclosed.
3. Do not regress live H5 cap-SOLD payment smoke or PC live host smoke.
