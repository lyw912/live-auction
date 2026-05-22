# Backend

Go modular monolith for auction truth, order lifecycle, realtime recovery and diagnostics.

## Module Boundaries

- `cmd/server/`: process entrypoint.
- `internal/gateway/`: auth, ACL, schema validation and idempotency probe.
- `internal/auction/`: rules, bid transaction, state machine and auction events.
- `internal/order/`: order, mock payment and deposit lifecycle.
- `internal/realtime/`: WebSocket tickets, room hub, recovery and backpressure.
- `internal/outbox/`: outbox delivery, Redis projection and publish ordering.
- `internal/scheduler/`: durable jobs and retries.
- `internal/observability/`: anomaly scanners, diagnostics and metrics.
- `internal/storage/`: PostgreSQL, Redis and MinIO adapters.
- `internal/config/`: environment/config loader.
- `internal/platform/`: shared errors, logging and low-level helpers.
- `migrations/`: goose SQL migrations.

Do not put auction truth in Redis or WebSocket modules. The bid/cancel/end write path belongs in `internal/auction/` with PostgreSQL transactions and outbox records.
