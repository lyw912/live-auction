# P1-05 Redis/DB Reconciliation Checker Evidence

Gate: P1-05 Redis/DB reconciliation checker
Date: 2026-05-23 Asia/Shanghai
Base commit: aa84116

## Design Mapping

- `docs/design-v2-industrial/01-scope-and-roadmap.md`: P1-05 requires a Redis/DB reconciliation checker after snapshot/event schema is stable.
- `docs/design-v2-industrial/04-data-and-storage.md`: PostgreSQL is the authority for auction state, price/winner, bid result, event sequence, order/payment, idempotency, and scheduler truth.
- `docs/design-v2-industrial/06-realtime-and-recovery.md`: Redis history/snapshot are bounded projections for recovery, not audit truth.
- `docs/design-v2-industrial/08-observability-and-ops.md`: real anomaly events back diagnostics; do not add fake diagnostic data.
- `docs/design-v2-industrial/12-engineering-rules.md`: Redis must not be treated as auction authority.

## Implemented

Added checker library:

- `backend/internal/reconcile/checker.go`

Added tests:

- `backend/internal/reconcile/checker_test.go`

Added CLI:

- `backend/cmd/reconcile/main.go`

Added script:

- `package.json` script `test:reconcile:p1`

Checker behavior:

- Reads PostgreSQL auction row and `auction_events` max seq as truth.
- Reads Redis `auction:{id}:snapshot`.
- Reads Redis `auction:{id}:events`.
- Reports:
  - `SNAPSHOT_MISSING`
  - `SNAPSHOT_INVALID_JSON`
  - `SNAPSHOT_SEQ_DRIFT`
  - `SNAPSHOT_FIELD_DRIFT`
  - `HISTORY_MISSING`
  - `HISTORY_INVALID_JSON`
  - `HISTORY_GAP`
  - `HISTORY_LAST_SEQ_DRIFT`
  - `DB_EVENT_SEQ_DRIFT`
- Distinguishes Redis snapshot shapes:
  - full DB snapshot: `event_type=snapshot`
  - relay event envelope: latest committed event, used for seq/history checks
- Does not repair Redis.
- Only writes diagnostics when `--write-anomalies` is explicitly set.

CLI:

```text
cd backend
go run ./cmd/reconcile --limit 20
go run ./cmd/reconcile --auction-id auc_live --write-anomalies
go run ./cmd/reconcile --auction-id auc_live --fail-on-drift
```

## Review Result

`live-auction-v2-code-review` was applied manually against:

- `12-engineering-rules.md`
- `10-test-gates.md`
- `04-data-and-storage.md`
- `06-realtime-and-recovery.md`
- `08-observability-and-ops.md`
- touched reconciliation diff

Findings addressed before evidence:

- Added missing requested-auction error so `--auction-id` cannot silently check zero rows.
- Switched JSON decoding to `UseNumber` so seq/money values are not parsed through float64.
- Kept anomaly writes behind `--write-anomalies` and never repair Redis from the checker.
- Kept the default script non-failing on drift so local dirty projection state remains observable evidence rather than a hidden cleanup requirement.

Current review status: no remaining P0/P1 findings for the P1-05 checker slice.

## Verification

Unit/integration tests:

```text
cd backend
go test ./internal/reconcile ./cmd/reconcile
```

Result: PASS.

Coverage proven by tests:

- A clean relay-published projection returns `drift_count=0`.
- Mutating Redis snapshot seq produces drift.
- Default drift detection remains read-only and writes no anomaly rows.
- `--write-anomalies` path writes one `REDIS_DB_RECONCILIATION_DRIFT` row.
- Full DB snapshot payload comparison catches field drift.
- Missing requested auction ID returns an error.

Real local smoke:

```text
pnpm test:reconcile:p1
```

Result: PASS command execution. The local report checked 20 auctions and found drift in existing dev/test data.

Representative smoke output excerpt:

```json
{
  "auctions_checked": 20,
  "drift_count": 36,
  "results": [
    {
      "auction_id": "auc_2086cd3c-5f1c-4bdc-8fe4-52d814a042b9",
      "status": "ACTIVE",
      "db_seq": 3,
      "db_max_event_seq": 3,
      "redis_history_count": 0,
      "drifts": [
        "SNAPSHOT_MISSING",
        "HISTORY_MISSING"
      ]
    },
    {
      "auction_id": "auc_499d1b43-d49d-4607-8d87-3be9438cf916",
      "status": "ACTIVE",
      "db_seq": 4,
      "db_max_event_seq": 4,
      "redis_snapshot_seq": 4,
      "redis_snapshot_shape": "event_envelope:bid_accepted",
      "redis_history_first_seq": 1,
      "redis_history_last_seq": 4,
      "redis_history_count": 4
    }
  ]
}
```

Interpretation:

- The checker detects drift in old local dev/test rows whose Redis projection has expired, was never published, or was manually mutated during local verification before the cleanup guard was added.
- The exact drift count in local smoke is environment-dependent because prior dev/test rows and Redis TTL state are not reset by this report-only command.
- The smoke is evidence that the checker can report the current local DB/Redis state. It is not a clean-projection release gate.
- The command is intentionally a report by default; use `--fail-on-drift` for CI or release gates.

Known limits:

- The current relay stores latest event envelope as `auction:{id}:snapshot`, while DB rebuild stores a full `event_type=snapshot` payload. The checker supports both shapes, but only the full snapshot shape can prove field-level state equality.
- This checker does not rebuild or repair Redis. Repair must remain an explicit operational action or future tool.
- Existing local dev/test data can contain drift; that is expected and should be interpreted, not hidden.
- The reconcile test now restores the intentionally mutated Redis snapshot during cleanup so future test runs do not add new artificial local drift.

Next action:

- Use `--fail-on-drift` against a freshly seeded demo dataset before final release evidence.
- P1-07 alert rules can use `REDIS_DB_RECONCILIATION_DRIFT` once alert routing is added.
