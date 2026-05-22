# P0 Judge Review And Repair - 2026-05-23

Skill: `live-auction-v2-tiktok-judge`

## Verdict

JUDGE VERDICT AFTER REPAIR: PASS WITH DAMAGE

The P0 module can enter P1 after the repair set recorded here and in `docs/evidence/p0-36-p0-repair-and-p1-entry-review.md`.

This verdict is intentionally narrow. It validates P0 demo correctness and interview defensibility for the implemented scope. It does not validate production auth, real payment, real room membership, multi-instance realtime fanout, or formal capacity claims.

## Original Judge Findings

The hostile P0 judge review found two material issues that had to be fixed before claiming P0 complete:

- [P0] Outbox poison proof was not reliably reproducible in the dirty local workspace.
  - Why it mattered: a crashed or poisoned outbox relay could leave a same-auction delivery stuck and block recovery proof.
  - A strong evaluator would reject a freeze claim if outbox recovery evidence only passed in a clean/happy local database state.

- [P0] Payment and order-expiry transitions were not part of the auction realtime recovery stream.
  - Why it mattered: SOLD creates a money/order state. If `order_paid` and `order_expired` are not emitted as auction events and outbox rows, H5 reconnect/recovery can show stale payment state.
  - A strong evaluator would ask why bid/order/outbox/realtime are not a single observable state machine.

The judge also carried forward non-blocking P1/P2 limitations:

- mock header auth, not a real account/session system;
- mock payment, not a real payment provider;
- deterministic demo room `room_main`;
- no bid rate limiter;
- single-process WS hub;
- local smoke only, not a formal QPS/P99/fanout capacity baseline.

## Repairs Completed

- Outbox relay now reclaims expired `PUBLISHING` leases.
  - Code: `backend/internal/outbox/relay.go`.
  - Effect: a relay crash after claim no longer permanently blocks head-of-line delivery for that auction.

- Outbox poison regression coverage was hardened.
  - Code/tests: `backend/internal/outbox/relay_integration_test.go`.
  - Proof target: `DEAD`, `OUTBOX_DEAD_LETTER`, and direct `outbox_gap_notice`.

- Outbox tests were adjusted to avoid cross-test interference from shared global delivery state.
  - Code/tests: `backend/internal/outbox/relay_integration_test.go`.

- Mock payment now emits `order_paid` exactly once on first successful transition.
  - Code: `backend/internal/auction/bid.go`.
  - Test: `backend/internal/auction/bid_integration_test.go`.

- Scheduler order expiry now emits `order_expired` inside the order-expiry transaction.
  - Code: `backend/internal/scheduler/scheduler.go`.
  - Test: `backend/internal/scheduler/scheduler_integration_test.go`.

- H5 now consumes `order_paid` and `order_expired` realtime events.
  - Code: `frontend/mobile-h5/src/main.tsx`.
  - Test: `tests/e2e/mobile-h5.spec.ts`.
  - Effect: expired orders become an explicit disabled `ORDER_EXPIRED` state instead of a generic retryable payment failure.

- Evidence docs were updated so older freeze evidence is clearly superseded by the repair record.
  - Docs: `docs/evidence/p0-34-freeze-review.md`, `docs/evidence/p0-36-p0-repair-and-p1-entry-review.md`.

## Verification

Commands recorded by the repair evidence:

```powershell
go test ./internal/outbox -run "TestRelay(PoisonMarksDeadAndWritesAnomaly|ReclaimsExpiredPublishingLease)" -count=3 -v
go test ./internal/auction -run TestPlaceBidCapSoldCreatesOrderAndPaymentIsIdempotent -count=3 -v
go test ./internal/scheduler -run TestOrderExpireMarksPendingOrderOnceAndPaymentRejects -count=3 -v
go test ./...
pnpm build
pnpm test:e2e
pnpm test:e2e:h5-live
pnpm test:load:p0
git diff --check
```

Observed result:

- focused outbox proof: PASS;
- focused auction payment proof: PASS;
- focused scheduler expiry proof: PASS;
- full backend test suite: PASS;
- frontend build: PASS, with the existing Vite chunk-size warning;
- Playwright e2e route-mock suite: PASS, 19 tests;
- H5 live backend browser suite: PASS, 2 tests;
- P0 load smoke: PASS, but local smoke only;
- whitespace check: PASS.

## Claim Audit

| Claim | Code proof | Test/evidence | Verdict | Attack |
|---|---|---|---|---|
| Bid commit is atomic with idempotency and event/outbox | `backend/internal/auction/bid.go` | backend auction tests | proven | Still depends on PostgreSQL as the only money truth; keep it that way. |
| Outbox poison does not silently stall clients forever | `backend/internal/outbox/relay.go` | focused relay tests, p0-36 | proven after repair | Multi-instance fanout remains future work. |
| Payment state reaches realtime clients | `backend/internal/auction/bid.go`, H5 event handler | auction test, H5 e2e | proven after repair | Real provider callbacks are out of scope. |
| Order expiry reaches realtime clients | `backend/internal/scheduler/scheduler.go`, H5 event handler | scheduler test, H5 e2e | proven after repair | Expiry UX is still demo-level but no longer stale. |
| P0 performance is production-grade | none | local smoke only | false if claimed | Do not publish QPS/P99/fanout claims. |
| Auth/room ACL is production-grade | mock middleware | known limits | mocked | P1 must replace mock auth and add membership ACL. |

## Scorecard After Repair

| Dimension | Score / 10 | Reason |
|---|---:|---|
| Official P0 scope | 8 | Core demo flow is implemented and tested; scope limits are documented. |
| Core technical challenge | 8 | Real money-path locking, idempotency, scheduler, outbox, WS recovery, and frontend safety exist. |
| Evidence quality | 8 | Backend, frontend, live smoke, and load smoke are recorded; capacity remains explicitly unproven. |
| Production sense | 6 | Good state-machine discipline, but auth, ACL, rate limiting, multi-instance, and real payment are not done. |
| Interview defensibility | 8 | Strong if presented honestly as P0 demo correctness, weak if overclaimed as production-ready. |

## Mock / Hardcode / Demo-Only Inventory

- `room_main` deterministic demo room.
  - Acceptable for P0 demo repeatability; P1 needs room selection and membership.

- Mock header auth.
  - Acceptable only for local demo; not acceptable for production or external deployment.

- Mock payment.
  - Acceptable for P0; P1/P2 must model callback idempotency, provider failure, reconciliation, and fraud boundaries.

- No bid rate limiter.
  - Documented P0 scope adjustment; P1 should add abuse throttling before any public test.

- Single-process WS hub.
  - Acceptable for P0; multi-instance fanout and recovery semantics remain future architecture work.

- Local load smoke.
  - Useful smoke signal; not a capacity benchmark.

## Required Next Fixes Before Stronger Claims

- [P1] Replace mock auth with a real identity/session boundary and host/user ACL tests.
- [P1] Add room membership authorization for API and WS ticket issuance.
- [P1] Add bid abuse/rate limiting with Redis-down behavior defined and tested.
- [P1] Add explicit fat-finger negative tests: mismatch, expiry, wrong user, confirm after cancel/SOLD.
- [P1] Add payment wrong-user and stale historical order tests.
- [P1] Decide whether `client_seen_seq` is only telemetry/recovery metadata or a backend stale-client guard; test the chosen contract.
- [P1/P2] Produce formal 3-run performance baselines before making capacity claims.

## P1 Entry Decision

P0 may enter P1 with these guardrails:

- keep `go test ./...`, `pnpm test:e2e`, and `pnpm test:e2e:h5-live` green after every P1 slice;
- do not claim real authentication, room ACL, payment, multi-instance realtime, or capacity until implemented and evidenced;
- keep PostgreSQL as money truth and outbox as the only authoritative realtime emission path;
- keep local smoke labeled as smoke, not benchmark.

