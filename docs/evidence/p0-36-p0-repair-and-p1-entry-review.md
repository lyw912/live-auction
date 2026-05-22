# P0 Repair And P1 Entry Review

Feature/Gate: P0 repair after hostile judge review and P1 entry decision

Date: 2026-05-23

Commit: pending

Environment: Windows 11, Go 1.26.3, Node 24.9.0, pnpm 11.2.2, local PostgreSQL/Redis/MinIO from Docker Compose

## Trigger

A hostile P0 review found that the freeze evidence for outbox poison handling was not reliably reproducible in the current workspace. The same review also identified that order payment/expiry status changes were not part of the auction event/outbox recovery stream.

## Fixes

- Outbox relay now reclaims expired `PUBLISHING` leases, so a crashed relay worker does not permanently block same-auction head-of-line delivery.
- Outbox poison regression coverage now verifies `DEAD`, `OUTBOX_DEAD_LETTER`, and direct `outbox_gap_notice`.
- Outbox integration tests no longer mutate global outbox delivery state for all auctions, avoiding cross-test interference in shared local PostgreSQL.
- Mock payment writes an `order_paid` auction event and outbox row exactly once on the first successful payment transition.
- Order expiry writes an `order_expired` auction event and outbox row in the scheduler transaction.
- H5 handles `order_paid` and `order_expired` realtime events. Expired orders become a disabled, explicit `ORDER_EXPIRED` UI state instead of a generic retryable payment failure.

## Commands

```powershell
go test ./internal/outbox -run "TestRelay(PoisonMarksDeadAndWritesAnomaly|ReclaimsExpiredPublishingLease)" -count=3 -v
go test ./internal/auction -run TestPlaceBidCapSoldCreatesOrderAndPaymentIsIdempotent -count=3 -v
go test ./internal/scheduler -run TestOrderExpireMarksPendingOrderOnceAndPaymentRejects -count=3 -v
go test ./...
pnpm build
pnpm test:e2e
pnpm test:e2e:h5-live
pnpm test:load:p0
```

## Result

PASS for P0 correctness and demo readiness.

Observed results:

- `go test ./internal/outbox -run "TestRelay(PoisonMarksDeadAndWritesAnomaly|ReclaimsExpiredPublishingLease)" -count=3 -v`: PASS.
- `go test ./internal/auction -run TestPlaceBidCapSoldCreatesOrderAndPaymentIsIdempotent -count=3 -v`: PASS.
- `go test ./internal/scheduler -run TestOrderExpireMarksPendingOrderOnceAndPaymentRejects -count=3 -v`: PASS.
- `go test ./...`: PASS.
- `pnpm build`: PASS. PC console still emits the existing Vite chunk-size warning.
- `pnpm test:e2e`: PASS, 19 tests.
- `pnpm test:e2e:h5-live`: PASS, 2 live backend browser tests.
- `pnpm test:load:p0`: PASS local smoke; no capacity claim.

## Raw Output Summary

Outbox focused proof:

```text
=== RUN   TestRelayPoisonMarksDeadAndWritesAnomaly
--- PASS: TestRelayPoisonMarksDeadAndWritesAnomaly
=== RUN   TestRelayReclaimsExpiredPublishingLease
--- PASS: TestRelayReclaimsExpiredPublishingLease
PASS
ok   live-auction/backend/internal/outbox
```

Full backend proof:

```text
ok   live-auction/backend/internal/auction
ok   live-auction/backend/internal/gateway
ok   live-auction/backend/internal/outbox
ok   live-auction/backend/internal/realtime
ok   live-auction/backend/internal/scheduler
```

Frontend and live proof:

```text
pnpm test:e2e: 19 passed
pnpm test:e2e:h5-live: 2 passed
```

Local load smoke:

```text
final_second_bid_burst: PASS
outbox_burst: PASS
watcher_fanout_and_slow_consumer: PASS
```

## P1 Entry Decision

P0 may enter P1 with documented limits.

This decision does not authorize production performance claims. P1 remains responsible for formal k6 baselines, metrics, multi-instance realtime design, room selection/membership, and stronger auth.

## Known Limits Carried Forward

- H5/PC still use deterministic demo room `room_main`; full room selector and room membership are P1.
- Auth is mock header auth, not a real account/session system.
- Payment remains mock payment.
- WS hub is single-process in P0; multi-instance fanout remains P1/P2 design work.
- `pnpm test:load:p0` is local smoke only, not a QPS/P99/fanout capacity baseline.

## Next Action

Start P1 only behind these guardrails:

- keep `go test ./...`, `pnpm test:e2e`, and `pnpm test:e2e:h5-live` green after every P1 slice;
- do not publish performance numbers until the formal native 3-run k6 baseline exists;
- do not claim real authentication, room ACL, or multi-instance realtime until implemented and tested.
