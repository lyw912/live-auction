# Review Archive Index

> Status: historical/current-adjacent review index, 2026-05-31.

This folder stores hostile reviews, product audits, test attacks, and borrowing
reviews from earlier phases. It is not the current hot-bid architecture
authority. For current bid-path, performance, and PTS-1B claims, read:

- `docs/current/README.md`
- `docs/current/architecture.md`
- `docs/current/performance-correctness-contract.md`
- `docs/current/evidence-policy.md`
- `docs/current/runtime-profiles.md`
- `tests/pts/MANIFEST.md`

## How To Use These Reviews

- Treat each review as scoped to the date, architecture, and implementation it
  names.
- Do not cite a review verdict as current evidence unless the reviewed code path
  was revalidated under `docs/current/evidence-policy.md`.
- Do not cite historical "PostgreSQL remains auction truth" wording as the
  current PTS-1B hot-bid design. The current boundary is Redis live decision
  state, Kafka durable decision WAL/fence, and PostgreSQL settlement/audit/order
  truth.
- Product, UI, observability, runbook, and test-hardening reviews are still
  useful when they do not conflict with the current bid hot path.

## Review Groups

| Group | Files | Current use |
|---|---|---|
| P0 product/test/judge | `p0-*` | product completeness history and local-demo scope boundaries |
| P1 observability/weak network | `p1-*` | metrics, dashboards, toxiproxy, reconciliation, UI trace, and alert-rule background |
| P3 borrowing reviews | `p3-04-*`, `p3-06-*`, `p3-07-*`, `p3-08-*` | engineering references; runtime decisions are historical if they assume PG-lane money truth |
| Rejected-bid realtime policy | `p3-18-*` | still useful for public realtime/noise policy, subject to current engine outcome semantics |
| L4b Kafka judge review | `pts-l4b-kafka-ledger-judge-review-2026-05-29.md` | current-adjacent pre-fix review; not PTS-1B success evidence |

## Red Flags

If a review says any of these, re-check current docs before applying it:

- PostgreSQL is the only current bid decision truth.
- Redis is only a projection/cache for all bid decisions.
- Kafka is absent from the hot-bid correctness contract.
- A local smoke or route-mocked UI test proves cloud PTS capacity.
- HTTP `200` count alone proves accepted-bid correctness.

Use `docs/archive/evidence-era-map.md` to classify old evidence and
`docs/current/evidence-policy.md` to classify new evidence.
