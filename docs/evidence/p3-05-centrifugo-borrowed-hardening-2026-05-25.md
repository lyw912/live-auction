# P3-05 Centrifugo Borrowed Hardening

Date: 2026-05-25 Asia/Shanghai

Status: `IMPLEMENTED_FOCUSED_TESTED`

Origin:

- `docs/evidence/p3-04-centrifugo-judge-origin-2026-05-25.md`
- `docs/adr/p3-01-centrifugo-borrowing-decision.md`

## Scope

This evidence records project-owned hardening inspired by Centrifugo source review. It does not claim Centrifugo runtime adoption and does not claim final capacity.

Implemented mechanisms:

| Mechanism | Implementation | Test Proof |
|---|---|---|
| Bounded history recovery | `backend/internal/realtime/server.go` reads only the last `WS_RECOVERY_MAX_EVENTS`; missing contiguous seq falls back to snapshot. | `TestServeWSRecoveryWindowGapFallsBackToSnapshot` |
| Byte-based slow-consumer backpressure | `backend/internal/realtime/hub.go` tracks queued bytes in addition to message count. | `TestHubClosesSlowConsumerOnByteBudgetOverflow` |
| Stream epoch and snapshot version | `backend/internal/outbox/relay.go` adds `stream_epoch` and `snapshot_version` to events and snapshots; `realtime_stream_epochs` stores per-auction epoch. | `TestRelayStreamEpochStableAcrossEventsAndSnapshot` |
| Outbox LISTEN/NOTIFY wakeup | `202605250001_realtime_stream_epochs_and_outbox_notify.sql` adds trigger; relay listens on `outbox_delivery_ready` while retaining polling fallback. | `TestRelayRunWakesFromPostgresNotify` |
| Realtime metrics | queue bytes/depth, subscriber fanout, payload size, recovery publication count, config limits. | `go test ./internal/realtime ./internal/outbox`; metrics render covered by existing observability tests in full run. |

## Commands

Migration applied locally:

```powershell
goose -dir backend\migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
```

Focused validation:

```powershell
go test ./internal/realtime ./internal/outbox
```

Observed result:

```text
ok   live-auction/backend/internal/realtime
ok   live-auction/backend/internal/outbox
```

## Important Test Environment Note

A stale local `server.exe` from `go run ./cmd/server` was holding outbox shard leases as `server-main` and caused the outbox claim-plan integration test to fail by design. After stopping that stale process, the focused suite passed. This is not a production correctness failure; it proves the shard lease ownership test is sensitive to real competing relay owners.

## Design Claims Allowed

Allowed:

- The self hub now has bounded recovery replay and bounded per-client queued bytes.
- The relay publishes stream metadata that makes history-window and snapshot boundaries explainable.
- Outbox notify is a latency optimization with polling fallback, not a correctness dependency.
- The implementation borrows Centrifugo design logic without introducing a second realtime transport.

Not allowed:

- Any final QPS, p99, or online-user capacity claim from this work.
- Any claim that WebSocket delivery is exactly-once.
- Any claim that NOTIFY is reliable delivery.
- Any claim that Centrifugo was integrated as runtime infrastructure.

## Review Hooks

Questions this evidence is meant to survive:

- Where is replay bounded?
- What happens if the client missed more than the retained history window?
- Is memory bounded by message count only, or by bytes too?
- How do you explain a snapshot relative to an event stream epoch?
- What happens if PostgreSQL NOTIFY is dropped?
- Where is the test proving each borrowed mechanism?

## Next Work

- Run `go test ./...` after broader integration cleanup.
- Run frontend builds/e2e if H5 payload display changes later.
- Add Linux-native P5 capacity calibration before any final realtime number.
