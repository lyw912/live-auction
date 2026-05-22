# P0 Product Audit

Date: 2026-05-22

Skill: `live-auction-v2-tiktok-product-auditor`

Design baseline: `docs/design-v2-industrial`

## Verdict

PRODUCT VERDICT: PARTIAL

The backend bid/order truth path and H5 live bidding path have substantial implementation. The product-level P0 flow is not complete because the PC host console, chat/social surface, and diagnostics scope still contain shortcuts that prevent a real user from completing the documented P0 workflow.

## Scope Audit

| Requirement | Code/UI path | Evidence | Verdict | Downgrade/shortcut |
|---|---|---|---|---|
| PC item upload/create | `backend/internal/gateway/router.go`, `auction_handlers.go` | Backend API exists | PARTIAL | PC UI has no item create/upload flow |
| PC auction creation | `backend/internal/gateway/router.go`, `backend/internal/auction/repository.go` | Backend API exists | PARTIAL | PC UI has no auction creation flow |
| PC rule save | `frontend/pc-console/src/main.tsx` | `docs/evidence/p0-26-pc-full-rule-fields.md` | PARTIAL | Fixed auction target instead of selected auction |
| PC start/increment/cap rule editing | `backend/internal/auction/model.go`, `repository.go` | `docs/evidence/p0-26-pc-full-rule-fields.md` | FAIL | Frontend submits fields that backend ignores on rule patch |
| PC schedule/start/cancel/narrate | `backend/internal/gateway/router.go` | Backend API exists | FAIL | PC UI lacks real actions |
| PC orders | `frontend/pc-console/src/main.tsx` | none | FAIL | Static order rows |
| PC diagnostics | `backend/internal/gateway/monitor_handlers.go`, `frontend/pc-console/src/main.tsx` | `docs/evidence/p0-14-monitor-diagnostics-apis.md` | PARTIAL | Missing recent rejects, recovery/source mix, and drilldown links |
| H5 active auction/bid/confirm/payment/history | `frontend/mobile-h5/src/main.tsx` | `docs/evidence/p0-29-h5-live-backend-ws-smoke.md`, `p0-30-h5-live-backend-rest-smoke.md` | MOSTLY OK | Live smoke is single room and single browser |
| H5 SOLD winner/loser state | `frontend/mobile-h5/src/main.tsx` | mock E2E | PARTIAL | Some states are manually selected demo tabs rather than server-state driven |
| H5 chat/social | API contract and frontend requirement | migration only | FAIL | No backend route and no H5 chat UI |
| Backend bid/order truth | `backend/internal/auction/bid.go` | backend tests/evidence | OK | PostgreSQL-authoritative path is substantive |
| Demo/evidence honesty | `docs/demo`, `docs/evidence/p0-27-p0-coverage-ledger.md` | documents | PARTIAL | Final freeze review overstates P1 readiness |

## Unacceptable Shortcuts

- `frontend/pc-console/src/main.tsx` used static auction and order data.
- PC rule save targeted a hardcoded auction instead of selected backend data.
- PC rule patch UI exposed start price, increment, and cap edits that were ignored by the backend patch handler.
- PC schedule/start/cancel/narrate controls were missing or inert.
- H5 chat/social was required by P0 but not implemented.
- Diagnostics were real for four backend tables but below the documented P0 product scope.
- Some H5 state matrix coverage was demo-tab driven and should not be represented as fully live state coverage.

## Acceptable Disclosed Limits

- Deterministic `room_main` demo entry is acceptable for P0 when disclosed.
- Mock auth and mock payment are acceptable P0 scope.
- Local load smoke is acceptable only while no QPS/P99/fanout number is claimed.

## Required Fixes Before P1

- Build PC host flow around live backend data: items, auctions, selected auction rules, lifecycle controls, orders, and diagnostics.
- Align backend and frontend rule update contract so no visible field is silently ignored.
- Implement H5 chat seed/send UI and backend routes, or explicitly remove chat from P0 docs. The chosen path is implementation.
- Add diagnostics for recent rejects and reconnect/snapshot source mix, and expose source links/identifiers in the PC UI.
- Update tests to cover the real product behavior rather than only mocked payload shape.
- Update evidence and freeze review after implementation, without overstating live coverage.
