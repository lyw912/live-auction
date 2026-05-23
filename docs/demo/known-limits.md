# P0 Known Limits

Date: 2026-05-23

Commit: pending

## Demo Scope

- The P0 demo uses deterministic local room `room_main`; H5 does not yet provide a full room selector.
- The H5 app selects active auction and payable order IDs from backend API responses, and the live smoke proves a cap SOLD bid generates the payable order before mock payment. The entry room is still fixed for local smoke and demo repeatability.
- PC console supports the P0 host workflow for local demo: item creation, auction creation, selected-auction rule save, schedule/start/cancel/narrate controls, order list, and diagnostics. It is not a complete merchant CMS.
- P2 local auth now uses `POST /api/auth/login` and an HttpOnly `la_session` cookie backed by `auth_sessions`. Mock headers are disabled in normal runtime unless `ALLOW_MOCK_AUTH=true` or `APP_ENV=test`.
- There is still no OAuth, SMS, password policy, or account binding. P2 room membership and host ownership ACL now exist for local users and rooms.
- Payment is mock payment; no external payment provider is integrated.
- Payment success and order expiry are now emitted through the auction event/outbox stream for realtime H5 recovery, but payment provider callbacks, refunds, disputes, and settlement reconciliation are outside P0.

## Correctness Scope

- PostgreSQL is the source of truth for auction state, bids, idempotency, orders, and terminal results.
- Redis and WebSocket are projection and delivery layers only.
- WebSocket browser auth uses Redis-backed one-time tickets. If Redis is unavailable, ticket issue/connect fails closed.
- There is no bid rate limiter in P0. The Redis-down bid-limit gate is explicitly treated as a scope adjustment, not an implemented degradation feature.
- Existing room ACL rejects are recorded as `ACL_FORBIDDEN` anomalies. Rich ACL drilldown/filtering is still P2-06.
- Backend validation remains authoritative. PC rule validation is guardrail coverage for common illegal inputs.

## Evidence Scope

- P0 correctness/demo gates are covered by Go tests, Playwright tests, live H5/PC backend smoke, and committed evidence records under `docs/evidence/`.
- Frontend unit/E2E route mocks prove UI state contracts; live backend coverage is limited to the explicit H5/PC live smoke tests.
- `docs/perf/p0-load-smoke-2026-05-22.md` is a local smoke baseline only.
- No QPS, online-user capacity, production P99/P999, or fanout capacity claim is allowed until a formal native 3-run k6 baseline is recorded.

## Operations Scope

- P0 has diagnostic APIs and panels backed by real producers, but not a full Prometheus/Grafana stack.
- Runbooks are in `docs/design-v2-industrial/08-observability-and-ops.md`; project-specific incident drills beyond the committed evidence are future work.
- Toxiproxy and formal chaos scenarios are reserved for future baselines.
