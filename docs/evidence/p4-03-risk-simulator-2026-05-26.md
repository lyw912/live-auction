# P4-03 Risk Simulator

> Date: 2026-05-26 Asia/Shanghai  
> Status: AUTHORITATIVE for P4-R3 repeatable product-risk verification.  
> Scope: smoke-sized risk scenarios that exercise real backend APIs, user-visible outcomes, and scoped DB invariants.

## What Changed

Added a P4 risk simulator:

```powershell
pnpm exec node tests/risk/run-p4-risk-simulator.mjs
```

The runner:

- resets `auc_live` / `auc_side` seed data with `backend/cmd/p1loadseed`;
- can run against an existing backend or manage a local backend with `MANAGE_SERVER=1`;
- exercises real HTTP APIs instead of route mocks;
- writes compact evidence under `docs/perf/raw/p4-risk-simulator-*`;
- runs `backend/cmd/invariantcheck -auction auc_live` after each scenario.

## Scenarios

| Scenario | Risk Covered | Expected Outcome |
|---|---|---|
| `bid-idempotency-replay-and-conflict` | Duplicate client retries and malicious key/body reuse. | Same key and body replays the original bid response; same key with different amount returns `IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST`. |
| `host-only-flight-recorder-acl` | Diagnostic data leakage. | User role receives 403; host role receives the DB-backed flight recorder timeline. |
| `cap-sold-payment-double-click` | High-value auction settlement accident. | Cap bid sells the auction, winner sees one pending order, same-key payment double click returns the same paid provider payment. |

## Evidence

Commands run:

```powershell
pnpm exec node tests/risk/validate-p4-risk-simulator.mjs
pnpm exec node tests/risk/run-p4-risk-simulator.mjs
```

Result:

- simulator static validation: PASS;
- risk simulator: PASS;
- raw compact report: `docs/perf/raw/p4-risk-simulator-202605261020/risk-summary.md`;
- machine-readable report: `docs/perf/raw/p4-risk-simulator-202605261020/risk-summary.json`.

## Boundary

This is a correctness and incident-risk verifier, not a load benchmark. It should run after performance optimizations to prove the system still preserves money truth, ACL, payment idempotency, and diagnostic evidence.

Capacity claims still require P3/P5 pressure baselines with raw output, environment, and bottleneck attribution.
