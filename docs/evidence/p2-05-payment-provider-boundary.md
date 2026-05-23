# Evidence Record

Feature/Gate: P2-05 payment provider boundary

Date: 2026-05-24 Asia/Shanghai

Commit: included in this change; final hash recorded after commit

Environment: Windows local development machine; PostgreSQL local service; Go package tests, Vite builds, Playwright smoke.

Command:

```text
cd backend && goose -dir migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
cd backend && go test ./internal/auction
cd backend && go test ./internal/gateway
cd backend && go test ./internal/scheduler
cd backend && go test ./...
pnpm run build
pnpm test:e2e:h5-live
pnpm test:e2e
```

Raw Output Path: this evidence file records command output summary; no separate raw log captured for this gate.

## Setup

- Added migration `backend/migrations/202605240001_payment_provider_boundary.sql`.
- Added provider lifecycle columns and `payment_events`.
- Added fake-provider signed webhook handling.
- Reworked `pay-mock` to initiate local provider payment and process signed success callback.
- Added provider reconciliation repair/audit path.

## Expected Invariant

- Duplicate provider callback creates one `PAID` transition and one `order_paid` event.
- Invalid signature is audited and ignored.
- Late success for expired order is audited and rejected.
- Stale initiated provider payment can be reconciled or audited.
- Existing H5 payment flow still ends in `PAID`.

## Result

PASS

## Observed Data

- `TestProviderWebhookDuplicateCreatesOnePaidTransition` covers duplicate callback idempotency.
- `TestProviderWebhookInvalidSignatureAuditedAndIgnored` covers invalid signature audit/no mutation.
- `TestProviderWebhookLateSuccessForExpiredOrderAudited` covers late expired-order callback.
- `TestProviderPaymentReconcileRepairsInitiatedOrderWithSuccessEvent` covers reconciliation repair.
- `TestProviderPaymentReconcileWritesMismatchForStaleInitiatedOrder` covers reconciliation mismatch anomaly.
- `go test ./...` passed.
- `pnpm run build` passed with existing PC chunk-size warning.
- `pnpm test:e2e:h5-live` passed with three live backend tests, including H5 pay-mock through the provider boundary.
- `pnpm test:e2e` passed with 19 mock UI tests.

## Known Limits

- Fake provider is local and synchronous in `pay-mock` for demo compatibility.
- No external PSP, refund, dispute, chargeback, or settlement ledger.
- Reconciliation is a repository method with tests; no scheduled background runner is wired yet.

## Next Action

Implement P2-06 security and abuse diagnostics.
