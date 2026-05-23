# ADR P2-05 · Payment Provider Boundary

Date: 2026-05-24 Asia/Shanghai

Status: Accepted

## Context

Before P2-05, `pay-mock` directly transitioned an order to `PAID`. That was acceptable for P0 UX, but it did not model a provider order id, signed callback, duplicate callback, invalid signature, late callback, or reconciliation path.

## Decision

- Keep the local fake provider; do not integrate an external payment service.
- Extend orders with provider lifecycle fields: `provider_payment_id`, `payment_initiated_at`, and `payment_succeeded_at`.
- Extend order status with `PAYMENT_INITIATED` and `PAYMENT_SUCCEEDED` as provider boundary states, while preserving the user-facing final `PAID` state.
- Add `payment_events` with provider event id uniqueness, signature validity, processed timestamp, and payload.
- Keep `POST /api/orders/{id}/pay-mock` as the H5-compatible entrypoint, but implement it by initiating a local provider payment and then processing the same signed webhook path used by `POST /api/payments/fake-provider/webhook`.
- Invalid signatures are stored and audited with `PAYMENT_WEBHOOK_INVALID_SIGNATURE` and do not mutate orders.
- Late success for expired orders is rejected and audited with `PAYMENT_RECONCILE_MISMATCH`.
- Add `ReconcileProviderPayments` to repair initiated orders with valid provider success events and audit stale initiated orders without success evidence.

## Consequences

- H5 can keep a one-click local demo payment flow, but backend payment correctness is no longer a direct write shortcut.
- Duplicate provider callbacks are idempotent through `payment_events(provider, provider_event_id)` and existing order state checks.
- The provider secret is local config only and must not be logged.
- Real refunds/disputes/chargebacks remain out of scope.

## Follow-Up Gates

- P2-06 should expose payment anomalies in security/abuse diagnostics.
- P2-07 should include payment callback duplicate/late cases in release evidence.
