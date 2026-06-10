# CLAUDE.md

This repository implements the live auction system. Start from the current
architecture docs, not the historical PG-lane baseline.

## Required First Reads

1. `docs/README.md`
2. `docs/design/01-architecture.md`
3. `docs/design/02-performance-correctness-contract.md`
4. `docs/s1-s5/00-overview.md`
5. `tests/pts/MANIFEST.md` for PTS work

The committed documentation is intentionally limited to the final judge-facing
design under `docs/`. Treat old local briefs, screenshots, reviews, and research
notes as historical background only; they are not governing design.

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

Historical review drafts, raw evidence, screenshots, and intermediate design
directories have been removed from the committed `docs/` tree. Treat any old
local artifact outside the new `docs/` layout as historical or current-adjacent,
not as governing material.

## Current PTS Target

PTS-1B means 1000 final-second bids against one hot auction, user-visible bid
decision p99 <= 60ms in the current kafka_ack profile, highest valid amount wins, all rejects have decision-time
basis, and fault-injection gates either recover safely or fail closed.

## Development Checklist

- Keep money and bid decisions server-authoritative.
- Preserve idempotency and request-hash conflict behavior.
- Keep outbox/WebSocket/snapshot recovery ordered and replayable.
- Add or update tests for correctness, race, and recovery paths touched.
- Use `docs/design/04-evidence-policy.md` before writing or citing evidence.
