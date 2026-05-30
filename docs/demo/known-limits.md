# P0 Known Limits

> 2026-05-31 current-architecture note: this file describes demo/product limits. It is not the current hot-bid architecture contract. For PTS-1B and Redis/Kafka/PostgreSQL correctness, read `docs/current/architecture.md`, `docs/current/performance-correctness-contract.md`, and `docs/current/evidence-policy.md`.

Date: 2026-05-23

Commit: included in current P2 hardening changes; see git history for exact milestone commits

## Demo Scope

- P10 no-mock auction demo policy is documented in `docs/demo/p10-no-mock-auction-demo.md`. The main P10 trunk should create the product and auction during the demo session and use real backend bid/realtime/diagnostic paths.
- The local seed keeps `room_main` as the root fallback sample room and adds `room_side` for multi-room checks.
- The H5 app supports `/rooms/{room_id}` and selects active auction/payable order IDs from backend API responses. The live smoke proves `room_main` and `room_side` do not share auction/chat state.
- PC console can select host rooms before creating or managing auctions. It is still not a complete room management CMS.
- PC console supports the P0 host workflow for local demo: item creation, auction creation, selected-auction rule save, schedule/start/cancel/narrate controls, order list, and diagnostics. It is not a complete merchant CMS.
- P2 local auth now uses `POST /api/auth/login` and an HttpOnly `la_session` cookie backed by `auth_sessions`. Mock headers are disabled in normal runtime unless `ALLOW_MOCK_AUTH=true` or `APP_ENV=test`.
- There is still no OAuth, SMS, password policy, or account binding. P2 room membership and host ownership ACL now exist for local users and rooms.
- P10 does not add public registration/login as a product surface. Deterministic local demo users, host, room, and session setup are allowed prerequisites.
- P10 live smoke may use `/api/test/rooms` while `APP_ENV=test` to create an isolated repeatable demo room. This endpoint is host-only and test-environment-only; it is not part of the production product surface.
- PC Live Assist includes a local/test host-only demo bid driver at `/api/demo/auctions/{id}/competing-bid` for showing another deterministic bidder during manual demos. It is not production product UI. The driver still calls the real bid repository and produces real bids, auction events, outbox rows, and orders when SOLD.
- Real live streaming remains outside scope. P10 may use a local looping product video or product image as the H5 live-stage visual asset.
- Payment uses a local fake-provider boundary with provider IDs, signed webhook handling, provider event idempotency, and reconciliation checks. No external payment provider is integrated.
- P10 main trunk should stop at SOLD/order creation if claiming "no mock" for the auction path. The local fake-provider payment path is optional and must be labeled as local fake-provider payment, not real external payment.
- Payment success and order expiry are emitted through the auction event/outbox stream for realtime H5 recovery. Refunds, disputes, chargebacks, and settlement ledger accounting remain outside current scope.

## Correctness Scope

- For normal product/demo flows, PostgreSQL remains the durable settlement, audit, order, and query store.
- For the current PTS-1B hot manual-bid path, Redis is the live atomic decision state under Kafka WAL/fence and reconciliation; PostgreSQL applies settlement/audit afterward.
- Kafka is required for the current durable decision ledger/fence. A Redis decision that cannot be durably fenced must not be presented as final settled success.
- WebSocket is a delivery/recovery layer only.
- WebSocket browser auth uses Redis-backed one-time tickets. If Redis is unavailable, ticket issue/connect fails closed.
- There is no bid rate limiter in P0. The Redis-down bid-limit gate is explicitly treated as a scope adjustment, not an implemented degradation feature.
- Existing room ACL rejects are recorded as `ACL_FORBIDDEN` anomalies. Rich ACL drilldown/filtering is still P2-06.
- Backend validation remains authoritative. PC rule validation is guardrail coverage for common illegal inputs.

## Evidence Scope

- P0 correctness/demo gates are covered by Go tests, Playwright tests, live H5/PC backend smoke, and committed evidence records under `docs/evidence/`.
- Current PTS-1B claims require `docs/current/evidence-policy.md`, `tests/pts/MANIFEST.md`, `ENGINE_*` distribution, correctness verifier output, and fault-injection evidence when resilience is claimed.
- Frontend unit/E2E route mocks prove UI state contracts; live backend coverage is limited to the explicit H5/PC live smoke tests.
- Route-mocked UI tests are never evidence for the P10 no-mock auction trunk. P10 evidence must separately capture the created auction ID and backend/flight-recorder proof for that created auction.
- H5 route-mocked visual and behavior tests now follow the current feed -> floating product card -> bid panel interaction where bid controls are asserted. The default feed card still shows price, countdown, status, connection, and next-bid entry.
- `docs/perf/p0-load-smoke-2026-05-22.md` is a local smoke baseline only.
- No QPS, online-user capacity, production P99/P999, PTS-1B p99, or fanout capacity claim is allowed without current evidence classified by `docs/current/evidence-policy.md`.

## Operations Scope

- P0 has diagnostic APIs and panels backed by real producers, but not a full Prometheus/Grafana stack.
- Runbooks are in `docs/design-v2-industrial/08-observability-and-ops.md`; project-specific incident drills beyond the committed evidence are future work.
- Toxiproxy and formal chaos scenarios are reserved for future baselines.
