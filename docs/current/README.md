# Current Live Auction Architecture

> Status: governing entry point, 2026-05-31.
> Purpose: replace the old PG-lane/v2 entry point as the default context for new work.

## Non-Negotiable Sources

- Official brief: `抖音电商AI全栈课题-直播竞拍全栈系统（宣讲版）.md`.
- Official images: `docs/references/official-brief-images/`.
- Single-hotspot research: `单热点调研.md`.

The official brief is immutable. The single-hotspot research is high-value background, but it includes competing routes and must be filtered through the current contract below.

## Current Governing Contract

The hot auction bid path is no longer "PostgreSQL row lock is the synchronous bid truth" for PTS-1B. That design is correct but cannot meet the 1000-concurrent final-second p99 target when many users contend on the same auction row.

Current target design:

```text
HTTP bid
  -> auth / room / ACL / admission / idempotency gate
  -> Redis hot-state decision engine
       - atomic Lua decision per auction
       - monotonic engine_seq
       - request-hash idempotency
       - accept only if amount is valid against the current Redis decision state
       - reject only with decision-time basis
  -> Kafka durable decision WAL / append fence
  -> HTTP returns user-visible ENGINE_* decision
  -> settlement worker applies decisions to PostgreSQL
  -> outbox / WebSocket / snapshots / diagnostics
  -> reconciler verifies Redis, Kafka, PostgreSQL, and outbox
```

PostgreSQL remains the financial settlement, audit, order, and durable query store. Redis is the live hot-state decision engine for the optimized manual-bid path. Kafka is the durable ordered decision log/fence. The system must fail closed when it cannot prove this chain is safe.

## Success Boundary

PTS-1B is successful only when all are true:

- 1000 final-second bid requests reach the intended hot auction workload.
- User-visible bid decision p99 is under 50ms for the current active contract.
- The final winner is the global highest valid bid.
- Every reject has a decision-time basis: the rejected amount is strictly below the current required/accepted price at the engine decision point, or another explicit business rule applies.
- Idempotency is correct under duplicate concurrent requests.
- Redis, Kafka, PostgreSQL, and worker failure-injection tests either recover safely or fail closed with a clear user/system state.
- Evidence identifies environment, git SHA, workload, JMX/CSV, server metrics, correctness verifier output, and raw artifact paths.

## HTTP And User Semantics

Do not treat HTTP status alone as auction outcome.

- `ENGINE_ACCEPTED`, `ENGINE_REJECTED`, `ENGINE_SOLD`, `ENGINE_PAUSED`, and `RECONCILING` are business decision states.
- `settlement_status` tells whether PostgreSQL settlement has completed.
- HTTP `200` means the request completed the configured synchronous boundary.
- HTTP `202` is allowed only when the user already receives a real engine decision and settlement/durability status is explicit. It must not mean vague `PROCESSING_RETRY_LATER` for the normal hot path.
- Large volumes of `409`, `PROCESSING_RETRY_LATER`, or seconds-long pending states fail the PTS-1B user-experience goal even if eventual correctness holds.

## Historical Design Status

`docs/design-v2-industrial/` remains valuable for official scope, UI/UX, recovery principles, diagnostics, engineering guardrails, and many product workflows. It is not the current authority for the hot auction architecture when it says PostgreSQL row-lock truth or Redis Lua as design-only.

`docs/evidence/index.md` and archived P/L/phase progress files are historical maps. Use `docs/current/document-map.md` and `docs/archive/progress-history-map.md` to decide whether a file is current input, bounded background, or superseded.

## Required Reads By Task

| Task | Read first |
|---|---|
| Hot bid architecture | `docs/current/architecture.md`, `docs/current/performance-correctness-contract.md` |
| PTS-1B run/review | `docs/current/performance-correctness-contract.md`, `docs/current/evidence-policy.md`, `tests/pts/MANIFEST.md` |
| Runtime profile choice | `docs/current/runtime-profiles.md` |
| PTS run review writing | `docs/current/pts-run-review-template.md` |
| Fault injection | `docs/current/fault-injection-runbook.md` |
| Final PTS-1B readiness | `docs/current/pts1b-readiness-checklist.md` |
| UI/UX and product scope | `docs/design-v2-industrial/00-project-brief.md`, `07-frontend-ux.md`, `19-extreme-bidding-atmosphere.md`, `20-ui-ux-redesign.md`, then resolve conflicts through `docs/current` |
| Evidence/progress cleanup | `docs/current/document-map.md`, `docs/archive/evidence-era-map.md`, `docs/archive/progress-history-map.md` |
| Skills maintenance | `.codex/skills/*/SKILL.md`, with `docs/current/README.md` as first context |
