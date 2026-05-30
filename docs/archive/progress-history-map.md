# Progress Archive Map

> Status: historical progress index, 2026-05-31.

The old `p*-progress.md` files and `p3-decision-log.md` have been physically moved out of the docs root into `docs/archive/progress/`. They are historical execution records only. They are not current architecture authority after the Redis/Kafka hot-bid reset.

Start new work from:

- `docs/current/README.md`
- `docs/current/architecture.md`
- `docs/current/performance-correctness-contract.md`
- `docs/current/evidence-policy.md`

## Archived Progress Files

| File | Era | Current use | Caveat |
|---|---|---|---|
| `docs/archive/progress/p1-progress.md` | observability/evidence foundation | historical implementation ledger | no current PTS-1B authority |
| `docs/archive/progress/p2-progress.md` | auth/ACL/security/payment hardening | useful feature/security ledger | no current hot-path authority |
| `docs/archive/progress/p3-progress.md` | PG-lane/outbox/realtime pressure exploration | historical bottleneck narrative | superseded for Redis/Kafka hot bid |
| `docs/archive/progress/p3-decision-log.md` | P3/P4 go/no-go decisions | historical rationale for old no-go decisions | contains decisions now overturned by current contract |
| `docs/archive/progress/p5-progress.md` | UI foundation | UI evidence ledger | route-mocked coverage is not backend evidence |
| `docs/archive/progress/p6-progress.md` | H5 cockpit | UI/product history | old PostgreSQL-truth phrasing is superseded for hot bid |
| `docs/archive/progress/p7-progress.md` | atmosphere/action ranking | UI/realtime presentation history | do not infer backend truth model |
| `docs/archive/progress/p8-progress.md` | PC host console | UI/product history | old PostgreSQL-truth phrasing is superseded for hot bid |
| `docs/archive/progress/p9-progress.md` | trust/max-bid/diagnostics | product and max-bid history | max-bid remains special; do not merge blindly into Redis hot path |
| `docs/archive/progress/p10-progress.md` | accessibility/demo packaging | demo/evidence hygiene history | not capacity evidence |

## How To Cite

Use these files for:

- explaining why PG-lane became insufficient;
- proving UI/product slices were implemented at the time;
- finding old evidence files;
- understanding constraints that may still matter.

Do not use these files for:

- current PTS-1B pass/fail claims;
- current hot-bid architecture;
- Redis/Kafka fault-recovery guarantees;
- deciding which PTS JMX or reset script to run.

## Known Superseded Decisions

The most dangerous old decisions are:

- `P3-D01`: PostgreSQL remains auction money truth.
- `P3-D14`: Redis is projection/cache/admission support only.
- `P3-D16`: release-track architecture stays PostgreSQL bid truth + DB relay + Redis projection/history.

These were reasonable for the old P3/P4 release track, but are not current PTS-1B authority. Use `docs/current/architecture.md` instead.

## Root Directory Rule

Do not create new `docs/p*-progress.md`, `docs/phase-*`, or `docs/l*-progress.md` files in the docs root. New current work belongs under `docs/current/`, and new historical process notes must be placed directly under an explicit archive directory with a scope-bearing name.
