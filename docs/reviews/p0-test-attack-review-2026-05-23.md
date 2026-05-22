# P0 Test Attack Review - 2026-05-23

Skill: `live-auction-v2-tiktok-test-attacker`

## Verdict

TEST ATTACK VERDICT: PASS WITH GAPS

P0 core accident coverage is strong enough to enter P1 under the documented local demo scope. The money path, concurrency terminal races, scheduler lease/crash handling, outbox poison/reclaim behavior, WebSocket ticket/recovery behavior, and H5 state contract all have concrete code and tests behind them.

This is not a production-readiness verdict. The current system must still be described as local demo scoped: mock auth, mock payment, deterministic room, no bid rate limiter, and local smoke only for load.

## Failed Or Unproven High-Risk Scenarios

- [P1] Mock auth defaults missing `X-Mock-Role` to `host`.
  - Why it matters: a no-header request receives host privileges in the local API surface.
  - Code: `backend/internal/gateway/auth.go:18`, `backend/internal/gateway/router.go:38`.
  - Verdict: documented demo limit; production blocker if exposed beyond local demo.

- [P1] No bid rate limiter.
  - Why it matters: hostile bidders can spam bid endpoints and force PostgreSQL lock/idempotency pressure.
  - Evidence: `docs/demo/known-limits.md:21`, `docs/evidence/p0-32-bid-rate-limit-scope-adjustment.md:17`.
  - Verdict: acceptable only because P0 explicitly documents the scope adjustment.

- [P1] No real room membership ACL.
  - Why it matters: WS validates ticket scope and auction-room relation, but not whether the user belongs to the room.
  - Code: `backend/internal/realtime/server.go:55`.
  - Verdict: valid P1 auth/ACL gap.

- [P1] `client_seen_seq` is not enforced server-side.
  - Why it matters: stale clients are not rejected because of stale sequence; server-side amount/state locking preserves price correctness, but stale-client defense is frontend/recovery only.
  - Code: `backend/internal/auction/bid.go:36`, `backend/internal/auction/bid.go:103`, `backend/internal/auction/bid.go:349`.
  - Verdict: correctness is mostly protected; stale-client contract is unproven.

- [P1] Fat-finger negative cases lack explicit tests.
  - Why it matters: mismatch, expiry, wrong user, and confirm-after-state-change are common incident cases.
  - Code: `backend/internal/auction/bid.go:151`.
  - Verdict: code has checks; tests should pin them down.

- [P1] Payment wrong-user and stale historical order tests are incomplete.
  - Why it matters: payment is the money boundary; wrong-user and stale-browser flows must be explicit.
  - Code: `backend/internal/auction/bid.go:228`, `backend/internal/auction/bid.go:256`.
  - Verdict: code has winner check; missing durable negative tests.

- [P2] Load and capacity evidence is only local smoke.
  - Why it matters: local smoke does not prove QPS, P99, fanout, reconnect storm, or slow consumer capacity.
  - Evidence: `docs/evidence/p0-36-p0-repair-and-p1-entry-review.md:102`, `docs/perf/p0-load-smoke-2026-05-22.md:39`.
  - Verdict: not a P0 blocker, but must not be claimed as capacity proof.

## Scenario Matrix

| Scenario | Code path | Existing proof | Extra run | Verdict |
|---|---|---|---|---|
| Duplicate bid, same and different amount | `PlaceBid` idempotency | backend tests | `go test ./internal/auction ...` | proven |
| Self-leading bid | `evaluateAndApplyBid` | code path and partial tests | package test pass | partially proven |
| Below start, off-grid, above cap, exact cap | `ClassifyBidAmount` and bid apply | backend rule/bid tests | package test pass | proven |
| Cap SOLD creates order | `createOrderForSoldAuction` | integration and H5 live smoke evidence | package test pass | proven |
| Fat-finger replay | `ConfirmBid` | happy path and replay covered | package test pass | proven |
| Fat-finger mismatch, expiry, state changed | `ConfirmBid` | code only | not run | unproven tests |
| Final-second bid storm | concurrency integration | covered | package test pass | proven |
| Cancel vs cap race | concurrency integration | covered | package test pass | proven |
| End job vs extension | scheduler integration | covered | package test pass | proven |
| Stuck PROCESSING idempotency | `upsertProcessing` | degradation test | package test pass | proven |
| WS missing, invalid, reused, wrong-room ticket | `ServeWS` | realtime integration | package test pass | proven |
| Redis history gap to snapshot | `recoveryMessages` | realtime integration | package test pass | proven |
| Snapshot rebuild saturation | `snapshotMessage` | realtime integration | package test pass | proven |
| Outbox poison, DEAD, reclaim | relay tests | focused repeat | focused outbox test | proven |
| Payment double-click | `PayMock` idempotency | integration tests | package test pass | proven |
| Wrong-user payment | `PayMock` winner check | code only | not run | unproven test |
| Mock user calls host-only APIs | `requireHost` | role 403 covered, default-host gap remains | package test pass | partially proven |

## Incident Stories

- Host cancels while user hits cap price: concurrency tests prove there is only one terminal result and order count remains consistent.
- Outbox publishes poison or Redis fails during relay: relay tests prove poison can be marked DEAD, anomaly is written, and expired `PUBLISHING` leases can be reclaimed.
- User reconnects after weak network and misses events: realtime tests prove contiguous Redis history is replayed; gaps fall back to snapshot; saturated rebuild returns stale snapshot or `snapshot_unavailable` with anomaly.
- No-header caller hits host API: mock auth currently treats the caller as host. This is safe only as a documented local demo shortcut.

## Missing Tests To Add

- `TestConfirmBidRejectsMismatchExpiredWrongUserAndStateChanged` in `backend/internal/auction`.
  - Proves fat-finger confirm cannot be abused after token mismatch, expiry, wrong user, cancel, or SOLD transition.

- `TestPayMockRejectsWrongWinnerAndHistoricalStaleOrderFlow` in `backend/internal/auction`.
  - Proves a stale browser or another user cannot pay an order they do not own.

- `TestBidStaleClientSeenSeqContract` in `backend/internal/auction`.
  - Either proves backend rejects stale `client_seen_seq`, or intentionally documents that P0 treats it as frontend recovery metadata only.

- `TestMockAuthNoRoleDefaultsToHostKnownLimit` or an implementation change.
  - If the default remains, tests and evidence should make the demo shortcut explicit.

- P1 load/chaos gates.
  - Redis down, DB lock contention, WS reconnect storm, slow consumer fanout, QPS/P99, and outbox burst should have fixed thresholds and reproducible evidence.

## Abuse And Attack Notes

- Hostile bidder spam: current defense is idempotency, database locks, and reject rules; there is no rate limiter or abuse throttle.
- Forged WS room/auction pair: current defense rejects mismatched room-auction relation and ticket scope mismatch; membership ACL is not present.
- Fake price, fake winner, fake sequence: bid request does not accept client price or winner; server state decides the result. Fake/stale sequence is accepted and ignored.
- Payment double-click: current idempotency handles replay. Wrong-user payment is checked in code but needs a targeted test.
- Auth ticket leakage: HTTP logs record path only, not query; WS ticket is passed via subprotocol rather than query. No immediate log leak found in reviewed path.

## Commands Run

```powershell
go test ./internal/auction ./internal/realtime ./internal/outbox ./internal/scheduler ./internal/gateway -count=1
```

Result: PASS.

```powershell
go test ./internal/outbox -run "TestRelay(PoisonMarksDeadAndWritesAnomaly|ReclaimsExpiredPublishingLease)" -count=3 -v
```

Result: PASS. The Redis connection-refused log lines in that focused run are expected from the poison-path test setup.

