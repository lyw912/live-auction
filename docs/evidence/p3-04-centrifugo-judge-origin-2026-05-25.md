# P3-04 Centrifugo Judge Origin Review

Date: 2026-05-25 Asia/Shanghai

Status: `ORIGIN_REVIEW_SAVED`

This document preserves the hostile judge review that triggered the Centrifugo-inspired hardening work. Later ADRs and evidence records for realtime recovery, backpressure, stream metadata, outbox wakeup, and diagnostics reference this file as their origin.

## Prompt Context

The project was preparing to enter P3. Centrifugo source had been pulled under `references/oss/sources/centrifugo`. The request was to compare Centrifugo with the current architecture, design, and implementation, then decide whether to borrow, reuse, reimplement, or fully replace the current realtime path.

The review was explicitly constrained by the project design:

- PostgreSQL remains auction truth.
- Redis and WebSocket remain projection/delivery.
- The final answer must map back to official scope, the two core challenges, and scoring terms in `docs/design-v2-industrial/00-project-brief.md`.
- The answer must give a defensible position against competitors who directly use Centrifugo.

## Original Judge Verdict

JUDGE VERDICT: `PASS WITH DAMAGE`

Do not fully rebuild the project around Centrifugo. The current project's strongest competitive point is PostgreSQL-authoritative auction correctness plus transactional outbox plus recoverable realtime plus diagnostic evidence. That is closer to the two core challenges than replacing the self hub with a general realtime messaging server.

Centrifugo should be used as a mature realtime engineering reference, not as a runtime replacement for the auction correctness model.

## Core Finding

Centrifugo is strong as a generic realtime messaging platform:

- channel/pub-sub abstraction;
- multiple transports;
- hot history and recovery;
- presence and connection lifecycle;
- auth and subscription policy surfaces;
- Prometheus/Grafana observability;
- broker/outbox/partition engineering.

The live-auction project is stronger when the hard problem is framed differently:

```text
auction command -> PostgreSQL row lock -> bid/order/idempotency/event/outbox transaction
               -> Redis history/snapshot projection
               -> app-owned WebSocket recovery/backpressure/diagnostics
```

The project should preserve app-owned `auction_id + seq` recovery semantics rather than hide them behind a generic SDK protocol.

## Borrowing Matrix

| Centrifugo Reference Point | Project Landing Point | Value | Priority |
|---|---|---|---|
| Stream position: offset/epoch/history | Add auction stream epoch and snapshot version next to auction seq | Strengthens ordering and history-window explanation | P3/P4 |
| RecoveryMaxPublicationLimit / HistoryMaxPublicationLimit | Bound Redis history recovery reads and snapshot when outside the window | Prevents reconnect-storm unbounded reads | P3 |
| QueueMaxSize by bytes | Add byte budget in addition to message-count queue | Prevents large-payload memory blow-up | P3 |
| Presence / presence stats | Do not copy generic presence; reuse the idea for WS lifecycle and recovery diagnostics | Improves observability and data collection | P3 |
| PG outbox shard/advisory lock/partition/LISTEN notify | Keep current outbox semantics, borrow notify wakeup and evaluate ownership/partition later | Reduces polling latency without making NOTIFY correctness-critical | P3/P4 |
| Metrics taxonomy | Add queue depth, queue bytes, recovery publications, payload bytes, subscriber fanout metrics | Makes performance/stability claims inspectable | P3 |
| Multi-transport / SDK / RPC | Do not adopt | Adds complexity outside official scope | No-go |

## Defensible Interview Line

> I studied Centrifugo but did not replace the auction realtime path with it. Centrifugo solves generic realtime pub/sub very well, but this project is judged on server-authoritative auction correctness and recoverable realtime. I borrowed the mature implementation ideas that match the domain: bounded recovery windows, stream epoch metadata, byte-based backpressure, connection lifecycle diagnostics, and outbox wakeup. I kept PostgreSQL row-lock and transactional outbox as the correctness core so that winner, price, order, and idempotency remain auditable.

## Scoring Mapping

| Official Scoring Term | How This Review Improves It |
|---|---|
| 完整工程链路 | Keep item/rule/bid/order/payment/history/diagnostics in one app-owned flow. |
| 竞拍数据采集 | Add WS reconnect/recovered/slow-close/fanout/recovery source data. |
| 数据治理 | Make seq, stream epoch, snapshot version, history window, and replay boundary explicit. |
| 后端服务 | Keep PG row lock, state machine, outbox; borrow outbox wakeup and future shard-owner rigor. |
| 接口网关 | Do not delegate auction ACL to generic channel names. |
| 前端交互 | Preserve H5 gap detection and snapshot recovery instead of opaque SDK recovery. |
| 系统可用性 | Bound memory and reconnect work; keep snapshot fallback. |
| 性能 | Record small local evidence only; no final capacity claim. |
| 稳定性 | Borrow backpressure and recovery-window discipline. |
| 可观测性 | Metrics and diagnostics must remain producer-backed. |
| 核心挑战优化 | Improve realtime delivery while preserving final-second bid/cap/cancel/end correctness. |
| 独特思考 | Cut mature OSS ideas down to domain-specific correctness instead of outsourcing the core. |

## Competitive Position Against Direct Centrifugo Use

If another project directly uses Centrifugo, this project should not argue "my WebSocket is faster." The defensible claim is:

- Centrifugo proves transport maturity, not auction correctness.
- Direct use still needs a transactionally correct bid/order/idempotency/event/outbox path.
- Direct use must prove gap recovery returns to server-authoritative snapshot.
- Direct use must prove client code does not locally hammer, locally decide winner, or hide order/payment races.
- This project uses mature realtime ideas but keeps the money path and recovery semantics inspectable in project-owned code.

## Required Implementation Follow-Up

- Add an ADR for the Centrifugo borrowing decision.
- Implement bounded recovery reads.
- Implement byte-based queue backpressure.
- Add stream epoch and snapshot version metadata.
- Add outbox LISTEN/NOTIFY wakeup with polling fallback.
- Add realtime metrics and tests for each mechanism.
- Keep self hub as the only runtime transport unless a later clean failure bundle explicitly reopens scope.
