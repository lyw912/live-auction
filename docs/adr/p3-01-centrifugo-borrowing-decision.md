# ADR P3-01 · Centrifugo Borrowing Decision

Date: 2026-05-25 Asia/Shanghai

Status: Accepted

Origin: `docs/evidence/p3-04-centrifugo-judge-origin-2026-05-25.md`

## Context

P3 realtime work started after a hostile judge review compared the app-owned self hub with Centrifugo source under `references/oss/sources/centrifugo`.

Centrifugo is a mature realtime messaging server with channel pub/sub, multiple transports, history recovery, presence, metrics, and PostgreSQL/Redis broker work. The project cannot treat that maturity as irrelevant. At the same time, the live-auction v2 design makes PostgreSQL the authority for price, winner, order, idempotency, and auction state.

The central question:

```text
Should the project replace the self hub with Centrifugo, or borrow selected mechanisms?
```

## Decision

Keep the app-owned self hub as the only runtime browser realtime path.

Borrow and implement these Centrifugo-inspired mechanisms in project-owned code:

1. Bounded history recovery window.
2. Byte-based WebSocket queue backpressure.
3. Stream epoch metadata plus snapshot version.
4. WS lifecycle/recovery/fanout metrics.
5. PostgreSQL LISTEN/NOTIFY outbox wakeup with polling fallback.

Do not adopt Centrifugo as a second runtime transport in P3.

## Rationale

The two core challenges are complex auction rules and millisecond-level recoverable realtime. Centrifugo helps with the second category but does not decide the first. A full replacement would add generic channel protocol, client SDK, subscription policy, and transport operations without proving:

- final-second bid serialization;
- cap/cancel/end race correctness;
- one order per auction;
- idempotent bid/payment outcomes;
- event/outbox commit atomicity;
- H5 snapshot recovery to server truth.

The app must continue to own:

- browser ticket generation and consumption;
- room/auction/user ACL;
- `auction_id + seq` event semantics;
- snapshot fallback and gap behavior;
- diagnostics tied to project events;
- client UI safety states.

## Implementation Mapping

| Borrowed Mechanism | Code Landing |
|---|---|
| Recovery max publications | `backend/internal/realtime/server.go` reads only the last `WS_RECOVERY_MAX_EVENTS` history entries and snapshots on gap. |
| Queue byte budget | `backend/internal/realtime/hub.go` tracks queued bytes per subscription and closes slow consumers on byte overflow. |
| Stream epoch / snapshot version | `backend/internal/outbox/relay.go` writes `stream_epoch` and `snapshot_version`; migration `202605250001_*` stores epoch metadata. |
| Lifecycle/fanout metrics | `backend/internal/realtime/server.go`, `hub.go`, and `observability/metrics.go` expose payload bytes, queue depth/bytes, subscriber fanout, recovery publications, and config limits. |
| Outbox notify wakeup | `backend/internal/outbox/relay.go` listens on `outbox_delivery_ready`; migration trigger emits notification. Polling remains the correctness fallback. |

## Non-Goals

- No Centrifugo runtime process.
- No second browser transport path.
- No generic channel subscription protocol.
- No Redis/transport authority over auction result.
- No performance capacity claim from this decision.

## Failure Semantics

- If PostgreSQL NOTIFY is lost, relay still polls.
- If history is outside the configured recovery window, client receives snapshot instead of unbounded replay.
- If queue message count or byte budget is exceeded, the server closes that client as slow consumer.
- If stream epoch metadata is missing, relay creates it before publishing; snapshot rebuild uses the same stored epoch.

## Tests

Focused tests:

```powershell
go test ./internal/realtime ./internal/outbox
```

Covered gates:

- history-window fallback to snapshot;
- byte-budget slow-consumer close;
- stream epoch stable across event and snapshot;
- outbox notify wakes relay before long polling interval;
- existing poison, head-of-line, lease reclaim, fanout, and ticket recovery gates remain green.

## Review Position

This ADR supports this interview answer:

```text
I did not use Centrifugo as a black-box replacement. I borrowed specific mature
realtime mechanisms and reimplemented them inside the auction-owned correctness
model, so every recovery, backpressure, and ordering decision can be traced to
project code and tests.
```
