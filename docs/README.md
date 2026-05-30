# Documentation Entry Point

> Status: current navigation layer, 2026-05-31.

Read `docs/current/README.md` first for all new architecture, implementation, review, pressure-test, and judge-defense work.

## Authority Order

1. `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md`
   - Immutable official project brief. Do not edit it or the downloaded images under `docs/references/official-brief-images/`.
2. `docs/current/README.md`
   - Current governing project map after the PG-lane to Redis/Kafka architecture reset.
3. `单热点调研.md`
   - Important research source for single-hotspot bidding. It contains multiple possible routes, so it is not by itself a governing design.
4. `docs/current/performance-correctness-contract.md`
   - Current PTS-1B success definition, correctness invariants, failure gates, and evidence rules.
5. `docs/current/document-map.md`
   - Which historical docs remain useful, which are superseded, and how not to confuse old evidence with current claims.
6. `docs/current/pts-run-review-template.md`
   - Required shape for future PTS report reviews.
7. `docs/current/fault-injection-runbook.md`
   - Required Redis/Kafka/PostgreSQL fault gates.
8. `docs/current/pts1b-readiness-checklist.md`
   - Final checklist before paid/current PTS-1B runs.
9. `docs/archive/progress-history-map.md`
   - How to read archived `docs/archive/progress/p*-progress.md` files and `docs/archive/progress/p3-decision-log.md` without treating them as current authority.

## Historical Docs

`docs/design-v2-industrial/`, `docs/adr/`, `docs/evidence/`, `docs/perf/pts/`, `docs/reviews/`, progress docs, and old report reviews are archived or classified as historical/background material. They are not current authority. If they conflict with `docs/current`, use `docs/current` and record the conflict.

Historical indexes:

- `docs/archive/evidence-era-map.md`
- `docs/archive/progress-history-map.md`
- `docs/reviews/README.md`
- `docs/perf/pts/l4b-kafka/report-review-index.md`
- `tests/pts/HISTORICAL.md`

## Current Focus

The current hard target is PTS-1B: final-second 1000 concurrent bids on one hot auction, user-visible bid decision p99 under 50ms, with correctness and failure-injection proof. PostgreSQL-only hot-row serialization is no longer the governing path for this target.
