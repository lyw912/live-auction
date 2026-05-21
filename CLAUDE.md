# CLAUDE.md

This repository implements a live auction system. Use the design documents in `docs/design-v2-industrial/` as the source of truth.

## Priorities

1. Preserve server-authoritative auction correctness.
2. Keep realtime delivery recoverable through outbox, Redis history/snapshot and authoritative snapshots.
3. Make failures visible through diagnostics and evidence records.
4. Avoid unmeasured performance claims.

## Non-Negotiables

- PostgreSQL is truth for auction state, price, winner, bids, orders, scheduler jobs and idempotency.
- Redis is cache/projection/recovery helper only.
- WebSocket is delivery only.
- Never use client timestamps to decide ordering, close time or winner.
- No direct DB commit followed by direct WebSocket publish without outbox.
- No fake dashboard cards or static diagnostic data.
- No AI in bid, cancel or settlement paths.

## Expected Workflow

1. Read the relevant design section before changing code.
2. Keep changes scoped to the module being implemented.
3. Add or update tests for invariants, races and recovery behavior.
4. For any P0 gate touched, add an evidence record under `docs/evidence/`.
5. For performance claims, add raw output under `docs/perf/` before writing the claim.

## Current Decisions

- Backend: Go modular monolith.
- HTTP: `net/http` with `chi`.
- Migrations: `goose` SQL migrations in `backend/migrations/`.
- Frontend: React + TypeScript + Vite.
- PC UI: Arco Design.
- H5 UI: custom domain UI.
- Local infra: Docker Compose with PostgreSQL, Redis and MinIO.

## Review Checklist

- Does this affect auction truth?
- Does it need idempotency?
- Does it need event/outbox records?
- Can it race with bid, cancel or end jobs?
- Can retry duplicate money state?
- Can reconnect recover this state?
- Can slow clients cause memory growth?
- Is there a test or evidence record for the failure mode?
