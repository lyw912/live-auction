# P3-07 NATS / JetStream Borrowed Hardening Evidence

Date: 2026-05-25 Asia/Shanghai

Commit: pending at evidence creation

## Claim

The project selectively borrowed NATS/JetStream delivery semantics without integrating NATS, JetStream, or a broker-backed runtime.

Implemented borrowings:

- delivery state vocabulary on `/api/monitor/outbox`: `READY`, `ACK_PENDING`, `NAK_RETRY_WAIT`, `ACKED`, `TERM`;
- delivery message identity as `outbox:<outbox_id>` while preserving `auction_id + seq` as domain order;
- redelivery and retry diagnostics: attempts, max attempts, redelivery count, retry age, ack deadline, error class and retriable flag;
- shard-level consumer lag vocabulary on `/api/monitor/outbox/watermarks`: ack-pending, retrying, redelivered, oldest retry age;
- TERM-style poison proof for non-retriable payload failures: immediate `DEAD`, anomaly, and gap notice;
- slow-consumer pending-byte/message reason and queue pressure fields in `user_activity_events`;
- PC diagnostics now load outbox watermarks, snapshot rebuilds, and control signals.

## Files

- `backend/internal/gateway/monitor_handlers.go`
- `backend/internal/outbox/relay.go`
- `backend/internal/realtime/hub.go`
- `backend/internal/realtime/server.go`
- `frontend/pc-console/src/main.tsx`
- `docs/design-v2-industrial/06-realtime-and-recovery.md`
- `docs/reviews/p3-07-nats-jetstream-borrowing-review-2026-05-25.md`

## Verification

Commands:

```text
go test ./...
pnpm exec tsc --noEmit  # frontend/pc-console
pnpm exec tsc --noEmit  # frontend/mobile-h5
```

Result:

```text
go test ./... PASS
pc-console tsc PASS
mobile-h5 tsc PASS
```

Focused tests covering the borrowing:

- `backend/internal/outbox/relay_integration_test.go`
  - envelope includes outbox id as delivery message id;
  - invalid envelope becomes non-retriable DEAD with dead-letter anomaly and gap notice;
  - watermark path exposes ack-pending and redelivery state.
- `backend/internal/gateway/monitor_integration_test.go`
  - monitor APIs return delivery state, ack-pending watermark fields, and slow-consumer queue pressure from real rows.
- `backend/internal/realtime/server_integration_test.go`
  - hub slow-consumer callback signature carries queue-pressure metadata.

## Known Limits

- No NATS server, JetStream stream, backend NATS SDK, or browser NATS client is integrated.
- No NATS/JetStream performance number is claimed.
- Broker adoption remains evidence-gated by `docs/p3-decision-log.md` P3-D13 and would require a new ADR plus broker-down, duplicate, poison, restart, ordering, and backpressure tests.
