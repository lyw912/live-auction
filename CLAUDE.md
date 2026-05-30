# CLAUDE.md

This repository implements the live auction system. Start from the current
architecture docs, not the historical PG-lane baseline.

## Required First Reads

1. `docs/current/README.md`
2. `docs/current/architecture.md`
3. `docs/current/performance-correctness-contract.md`
4. `docs/current/document-map.md`
5. `tests/pts/MANIFEST.md` for PTS work

The official brief `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md` and images under
`docs/references/official-brief-images/` are immutable. `单热点调研.md` is
important research, but it is not by itself the governing design.

## Current Non-Negotiables

- PostgreSQL remains settlement, audit, order, and durable query truth.
- Redis is the live hot-state decision engine only inside the current
  Kafka/fence/reconciliation contract.
- Kafka is the durable ordered decision WAL/fence for hot-engine decisions.
- The hot bid path must fail closed or reconcile when Redis/Kafka/PostgreSQL
  state cannot be proven safe.
- Never trust client timestamps, current price, winner, terminal state, or bid
  success.
- Do not use HTTP status alone as auction outcome; inspect `ENGINE_*`,
  durability, and settlement fields.
- No performance claim without current workload, profile, verifier, and
  evidence classification.

## Historical Material

`docs/design-v2-industrial/`, `docs/evidence/`, `docs/archive/`,
`docs/perf/pts/`, and old report reviews may be useful background. If they
conflict with `docs/current/`, prefer `docs/current/` and label the older source
as historical or current-adjacent.

## Current PTS Target

PTS-1B means 1000 final-second bids against one hot auction, user-visible bid
decision p99 <= 50ms, highest valid amount wins, all rejects have decision-time
basis, and fault-injection gates either recover safely or fail closed.

## Development Checklist

- Keep money and bid decisions server-authoritative.
- Preserve idempotency and request-hash conflict behavior.
- Keep outbox/WebSocket/snapshot recovery ordered and replayable.
- Add or update tests for correctness, race, and recovery paths touched.
- Use `docs/current/evidence-policy.md` before writing or citing evidence.
