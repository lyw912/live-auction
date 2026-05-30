# PTS L4b Kafka Ledger Judge Review

Date: 2026-05-29 Asia/Shanghai

Reviewer mode: `live-auction-v2-tiktok-judge`

Scope:

- Design commit: `1d31bf9 docs: add PTS1-Refactoring docs`
- Implementation commit reviewed: `e4bb02c feat: upgrade l4b redis engine to kafka ledger`
- Related docs checked: `docs/adr/pts-02-hotspot-bidding-engine-redesign.md`, `docs/perf/pts/hotspot-industrial-research-2026-05-28.md`, `docs/perf/pts/hotspot-redesign-roadmap-2026-05-28.md`, `单热点调研.md`, `docs/evidence/pts-l4b-redis-ledger-engine-2026-05-29.md`
- Core implementation checked: Redis Lua engine, Kafka ledger writer/reader/DLQ, settlement worker, reconciliation, monitor API, H5 bid state handling.

## Judge Verdict

**BORDERLINE**

L4b is no longer docs-only. The repo contains a real Redis Lua hot state machine, Kafka ledger interface, PostgreSQL settlement worker, fencing by `engine_epoch`/`engine_seq`, DLQ handling, reconciliation, and monitor surface.

It was still not acceptable to claim "fully complete and only PTS remains" before the follow-up fix because:

- Redis state mutation happened before Kafka append, leaving a crash window where Redis had accepted a decision but Kafka had no ledger entry.
- Reconciliation detected pending Redis decisions but only paused; it did not recover the decision into the durable ledger.
- Settlement refused to process while `engine_paused` was true, which made pause useful for stopping new bids but dangerous for already accepted decisions.
- H5 treated `ENGINE_ACCEPTED` as normal accepted/leading state and displayed "server confirmed" instead of pending settlement.
- Kafka writer used `AllowAutoTopicCreation=true`, which can hide unsafe topic topology in non-test environments.

## Required Fixes

- [P0] Recover Redis pending decisions into Kafka during reconciliation, then settle idempotently.
- [P0] Make engine pause block new hot-engine bids but not block settlement of already accepted ledger entries.
- [P0] H5 must render `ENGINE_ACCEPTED` / `ENGINE_SOLD` with `settlement_status=PENDING` as pending settlement, not as DB-settled success.
- [P1] Disable Kafka auto-topic creation by default; tests may opt in explicitly.
- [P1] Evidence must say that pending recovery is a local single-node recovery mechanism and production still requires replicated Kafka, ISR monitoring, DLQ repair, and formal PTS evidence.

## Residual Risk

- No PTS latency number is proven by this review.
- Redis pending recovery depends on Redis durability for the crash window; local Redis uses AOF `appendfsync always`, but production still needs HA/no-eviction/durability policy.
- Kafka topic replication/min-ISR is documented but not programmatically verified in the current Go client.
- Video timeline sync, recommendation traffic allocation, blacklist, and bid request merging remain outside the completed L4b implementation unless separate scoped work implements them.
