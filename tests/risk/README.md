# P4 Risk Simulator

The P4 risk simulator is a repeatable product-risk verifier. It is not a capacity benchmark.

It resets real seed data, calls real backend APIs, checks user-visible outcomes, then runs the scoped DB invariant verifier for `auc_live`.

Default scenarios:

- `bid-idempotency-replay-and-conflict`: same bid key replays the original result; same key with a different body is rejected.
- `host-only-flight-recorder-acl`: user role cannot access the flight recorder; host role can read the DB-backed forensic timeline.
- `cap-sold-payment-double-click`: cap bid creates one SOLD order; payment double-click with the same key stays idempotent.

Run against an already running backend:

```powershell
pnpm exec node tests/risk/run-p4-risk-simulator.mjs
```

Run with a managed local backend:

```powershell
$env:MANAGE_SERVER='1'
pnpm exec node tests/risk/run-p4-risk-simulator.mjs
```

Run one scenario:

```powershell
$env:SCENARIOS='cap-sold-payment-double-click'
pnpm exec node tests/risk/run-p4-risk-simulator.mjs
```

Outputs are written under `docs/perf/raw/p4-risk-simulator-*` by default:

- `risk-summary.json`
- `risk-summary.md`
- per-scenario seed logs
- per-scenario invariant JSON/Markdown reports

Use this after performance changes to prove the optimization did not break money correctness, ACL, payment idempotency, or diagnostic evidence. Use P3/P5 runners for actual load and capacity claims.
