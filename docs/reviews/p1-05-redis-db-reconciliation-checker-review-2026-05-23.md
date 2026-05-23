# P1-05 Redis/DB Reconciliation Checker Review - 2026-05-23

## Scope

Review target: Redis/DB reconciliation checker library, CLI, tests, package script, and evidence.

Design basis:

- `docs/design-v2-industrial/04-data-and-storage.md`
- `docs/design-v2-industrial/06-realtime-and-recovery.md`
- `docs/design-v2-industrial/08-observability-and-ops.md`
- `docs/design-v2-industrial/10-test-gates.md`
- `docs/design-v2-industrial/12-engineering-rules.md`

## Findings

No remaining P0/P1 findings after fixes.

Fixed during review:

- [P1] Requested auction IDs initially could return an empty report if the ID was wrong. Added an explicit missing-ID error.
- [P1] JSON decoding initially used default `encoding/json`, which parses numbers as `float64`. Switched to `UseNumber` before comparing seq and money fields.
- [P1] Anomaly writes are opt-in only through `--write-anomalies`; default checker mode is read-only.
- [P2] The drift test intentionally mutated Redis snapshot state and originally left that mutation behind. Added test cleanup so repeated local test runs do not create new artificial reconciliation drift.
- [P2] Local smoke reports drift from existing dev/test rows. Evidence now documents that this is expected report behavior, not a failed checker.

## Missing Tests

No blocker for P1-05 readiness. Current tests prove:

- clean relay projection reports zero drift;
- Redis snapshot seq mutation is detected;
- default drift detection stays read-only and writes no anomaly row;
- optional anomaly writing inserts `REDIS_DB_RECONCILIATION_DRIFT`;
- full DB snapshot payload field mismatch is detected;
- missing requested auction ID is rejected.

Still useful before release:

- Run `go run ./cmd/reconcile --fail-on-drift` against a fresh demo seed with relay drained.
- Add a repair runbook if the project wants operational remediation, but keep repair separate from this checker.

## V2 Drift

No drift in the checker.

The checker treats PostgreSQL as source of truth and never repairs Redis automatically.

## Residual Risk

- Relay and DB rebuild currently write different snapshot shapes to the same Redis key. The checker handles both, but relay event envelopes cannot prove field-level equality.
- `REDIS_DB_RECONCILIATION_DRIFT` is not listed in the original anomaly table. This is a P1 checker-specific anomaly and must be wired into P1-07 alert/runbook work before being marketed as an alert.
- The default npm script reports drift but does not fail on drift. Use `--fail-on-drift` for release gates.
