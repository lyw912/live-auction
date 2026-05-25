# P4-01 Invariant Verifier

> Date: 2026-05-26 Asia/Shanghai  
> Status: AUTHORITATIVE for P4-R1 verifier coverage and usage.  
> Scope: PostgreSQL truth tables, outbox delivery state, idempotency replay, and P3 runner integration.

## What Changed

Added a durable verifier CLI:

```powershell
cd backend
go run ./cmd/invariantcheck -auction auc_live -format json -max-details 20
go run ./cmd/invariantcheck -auction auc_live -format markdown -out ..\docs\perf\raw\<run>\auc_live-invariants.md
```

The implementation lives in:

- `backend/internal/invariant/`
- `backend/cmd/invariantcheck/`

The P3 local stress runner now runs the verifier after each workload by default and writes:

- `<workload>-invariants.json`
- `<workload>-invariants.md`

Default P3 scope is `-auction auc_live`, configurable with:

- `INVARIANT_AUCTION_ID`
- `INVARIANT_ROOM_ID`
- `P3_INVARIANT_CHECK=0` only for harness debugging.

## Coverage

The verifier checks the P4-R1 exit gate and the v2 money/recovery rules:

| Area | Check |
|---|---|
| seq | `auctions.seq == max(auction_events.seq)` and `auction_events` are contiguous from 1 to max seq. |
| terminal state | At most one terminal event; SOLD/ENDED/CANCELLED statuses match their terminal event. |
| winner and price | `current_winner_id/current_price_cents` match the latest accepted bid; no winner before accepted bids. |
| orders | SOLD has exactly one order matching winner and price; non-SOLD auctions have no order. |
| outbox | Every `auction_event` has matching `outbox_event`; auction outbox rows cannot be extra or payload-drifted from the immutable auction event; every outbox event has delivery; same-auction head-of-line ordering is preserved. |
| outbox audit | payload SHA-256 matches stored JSON; PUBLISHED/unfinished lock fields are consistent; DEAD rows have `OUTBOX_DEAD_LETTER`. |
| idempotency | completed bid/payment records have replay data and expected request hashes; bid idempotency aligns with bid rows. Scoped runs check payment rows that can be joined to scoped orders; unscoped full-database mode also catches payment records whose order row is missing. |
| room isolation | embedded `room_id` payload references must match auction room; one ACTIVE and one narrating auction per room. |

## Evidence

Commands run:

```powershell
cd backend
go test -count=1 ./internal/invariant ./cmd/invariantcheck
cd ..
node --check tests/load/run-p3-local-stress.mjs
pnpm exec node tests/load/validate-k6-suite.mjs
cd backend
go run ./cmd/invariantcheck -auction auc_live -format json -max-details 5
$env:MANAGE_SERVER='1'; $env:WORKLOADS='preflight'; $env:DURATION='1s'; $env:VUS='1'; $env:WORKLOAD_TIMEOUT_MS='120000'; $env:RAW_ROOT='docs/perf/raw/p4-r1-verifier-smoke-20260526-02'; pnpm exec node tests/load/run-p3-local-stress.mjs
```

Results:

- `go test -count=1 ./internal/invariant ./cmd/invariantcheck`: PASS.
- `node --check tests/load/run-p3-local-stress.mjs`: PASS.
- `pnpm exec node tests/load/validate-k6-suite.mjs`: PASS.
- `go run ./cmd/invariantcheck -auction auc_live -format json -max-details 5`: PASS, 20 checks passed, 0 warnings, 0 failures.
- Managed P3 preflight smoke with verifier: PASS, raw root `docs/perf/raw/p4-r1-verifier-smoke-20260526-02/`; `analysis-compact.json` includes invariant status `PASS` and paths to `preflight-invariants.json` / `preflight-invariants.md`.

The integration tests intentionally corrupt fixtures and prove the verifier catches:

- seq gaps;
- sold order amount mismatch;
- outbox delivery gaps;
- outbox payload drift from the corresponding auction event;
- same-auction outbox head-of-line violation;
- bid idempotency hash mismatch;
- cross-room payload leak.

## Full-Database Caveat

An unscoped full-database verifier run on the current Windows-local development database failed on historical rows where previous experiments deliberately left `outbox_events.payload_sha256` as all zeroes. The scoped `auc_live` verifier passes.

Interpretation:

- scoped verifier PASS is the evidence required for a specific stress run or release candidate auction;
- full-database verifier FAIL is useful local data-governance signal, not automatic proof that the current seeded workload failed;
- if final release evidence needs a full database PASS, run on a clean migrated database or clean/archive historical local experiment rows first.

## Review Notes

The verifier is DB-truth focused by design. It does not assert Redis history or browser delivery by itself. Redis/WebSocket recovery still needs the existing realtime tests and P3/P5 load artifacts. This avoids treating projections as money truth.

The P3 runner stores invariant artifacts next to k6/metrics artifacts so every future optimization has both performance data and machine-checkable correctness data.
