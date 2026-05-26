# P8-S3 Host Prompter Backend Evidence

> Date: 2026-05-27 Asia/Shanghai  
> Slice: P8-S3 Host Prompter Backend  
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/19-extreme-bidding-atmosphere.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

Added host-only advisory prompter API:

```text
GET /api/host/auctions/{id}/prompts
```

The endpoint reads PostgreSQL auction truth, bid rows, auction events, and orders, then returns advisory prompt cards. It does not mutate auction state, write auction events, write outbox events, publish WebSocket messages, or auto-send chat.

Prompt cases implemented:

- `no_bid`: ACTIVE auction has no accepted bids or no accepted bid for at least 15 seconds.
- `last_10_seconds`: ACTIVE auction end time is within the final 10 seconds.
- `extension_triggered`: latest auction event is `auction_extended`.
- `high_bid_frequency`: recent accepted/rejected bid activity in the last 30 seconds is high.
- `sold_unpaid`: SOLD auction has an `ORDER_PENDING` order.

Each prompt includes type, severity, title, body, action, source, auction/room IDs, generated time, metric context, optional event seq, optional next valid bid reference, and optional expiry. The response is host-only and intended for P8-S4 PC rendering.

## Validation

```text
go test ./internal/gateway -run "TestHostPrompter" -count=1
```

Result: PASS.

```text
go test ./internal/gateway ./internal/auction ./internal/observability ./internal/outbox ./internal/realtime ./internal/scheduler
```

Result: PASS.

```text
go test ./...
```

Result: PASS from `backend`.

```text
pnpm build
```

Result: PASS. Vite retained the existing PC bundle-size warning.

```text
pnpm test:e2e
```

Result: PASS, 50 passed.

Port cleanup:

```text
netstat -ano | findstr ":18080 :5173 :5174 :5177" | findstr LISTENING
```

Result: no LISTENING rows.

## Review

- The API is behind `requireHost`; user role requests return 403.
- Prompt generation is read-only and advisory.
- Prompt data is derived from existing truth tables and diagnostics inputs, not UI mocks or fabricated watcher counts.
- The endpoint returns `AUCTION_NOT_FOUND` for missing auctions instead of empty fake prompts.
- `docs/design-v2-industrial/05-api-contracts.md` now lists the endpoint in PC APIs.

## Known Limits

- P8-S4 owns rendering, dismiss/manual action behavior, and any disabled system chat affordance.
- P8-S6 owns room/auction heat aggregation; this endpoint only reports prompt-specific recent bid activity.
- Prompt text is deterministic rule-based copy, not AI-generated and not part of bid/cancel/settlement paths.
