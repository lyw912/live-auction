# P0 Freeze Review

Feature/Gate: Final P0 freeze review before P1

Date: 2026-05-23

Commit: finalized by this record's commit

Environment: Windows 11, Go 1.26.3, Node 24.9.0, pnpm 11.2.2, local PostgreSQL/Redis/MinIO from Docker Compose

Commands:

```powershell
go test ./internal/outbox -run TestRelayPoisonMarksDeadAndWritesAnomaly -count=1 -v
go test ./...
pnpm build
pnpm test:e2e
pnpm test:e2e:h5-live
pnpm test:load:p0
git diff --check
```

## Scope

This record is the P0 freeze gate for moving to P1. It does not replace the detailed per-gate evidence records; it summarizes the final review result and the last P0 code fix.

## Review Result

SUPERSEDED BY `docs/evidence/p0-36-p0-repair-and-p1-entry-review.md`.

This record remains historical freeze evidence. A later hostile review found the outbox poison proof in this file was not reliably reproducible in a dirty shared local database. The fix and updated P1 entry decision are recorded in `p0-36-p0-repair-and-p1-entry-review.md`.

## Final P0 Fix

Outbox poison handling now follows the v2 recovery rule:

- when a delivery reaches `DEAD`, relay writes `OUTBOX_DEAD_LETTER` anomaly;
- relay directly publishes `outbox_gap_notice` to the auction room through the configured publisher;
- the gap notice is not written back into outbox, avoiding recursive poison;
- H5 already treats `outbox_gap_notice` as a snapshot recovery trigger with CTA disabled while recovering.

Focused proof:

```text
go test ./internal/outbox -run TestRelayPoisonMarksDeadAndWritesAnomaly -count=1 -v
=== RUN   TestRelayPoisonMarksDeadAndWritesAnomaly
--- PASS: TestRelayPoisonMarksDeadAndWritesAnomaly
PASS
```

## Ship-Gate Findings

| Area | Verdict | Evidence |
|---|---|---|
| Money truth | PASS | PostgreSQL bid/order/idempotency path tested in backend gates |
| Idempotency | PASS | completed replay, mismatch, timeout, duplicate payment covered |
| State machine | PASS | lifecycle, cancel/cap/end/scheduler gates covered |
| Outbox/recovery | PASS | relay order, poison DEAD + anomaly + gap notice, WS recovery covered |
| Frontend safety | PASS | no optimistic success, disabled recovery CTA, payment double-click, longtask smoke |
| Host workflow | PASS | PC console uses live APIs for item/auction creation, selected-auction rules, lifecycle controls, orders |
| Diagnostics | PASS | monitor pages use real backend producers for auctions, rejects, recovery, anomalies, outbox, scheduler |
| H5 live SOLD/payment | PASS | H5 live smoke proves cap SOLD creates a generated order before mock payment |
| Reproducibility | PASS | setup guide, demo flow, seed command, test commands documented |
| Security hygiene | PASS | `.env` ignored, no committed real secrets, WS tickets not logged in query/path |
| Performance honesty | PASS | local smoke only; no QPS/P99/fanout capacity claim |

## Known Risks Carried Into P1

- H5 enters deterministic demo room `room_main`; a full room selector is outside P0.
- Mock auth and mock payment remain P0 scope.
- No bid rate limiter exists; Redis-down bid-limit is documented as a scope adjustment, not implemented behavior.
- Single-process WS hub is the P0 implementation; multi-instance fanout remains future architecture work.
- Formal Linux/native 3-run k6 baseline is not done because no production performance number is claimed.

## P1 Entry Rule

P1 may start only if new work keeps these P0 constraints intact:

- PostgreSQL remains money truth.
- Redis/WebSocket remain projection and delivery.
- No client-side SOLD/ENDED/winner decisions.
- No direct DB commit then direct WS publish for authoritative auction events.
- No performance number appears without a formal baseline.
