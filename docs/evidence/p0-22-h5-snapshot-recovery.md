# P0-22 H5 Snapshot Recovery

Gate:
- out-of-order-detection
- snapshot-fallback
- recovering CTA disabled

Date:
- 2026-05-22 Asia/Shanghai

Commit:
- pending

Command:
- `pnpm build`
- `pnpm test:e2e`
- `go test ./...`

Environment:
- Windows workspace
- Node v24.9.0
- pnpm 11.2.2
- Playwright Chromium
- Backend tests use local PostgreSQL/Redis test setup

Result:
- H5 client listens for auction realtime events.
- If `seq > last_seq + 1` or `outbox_gap_notice` is observed, the client:
  - switches to recovering state,
  - marks the current price stale,
  - disables the bid CTA,
  - fetches authoritative snapshot from `/api/auctions/auc_live`.
- Fresh snapshot applies server price, leader, and seq, clears recovering, and re-enables bid CTA.
- Stale or failed snapshot keeps the client in recovering state.

Raw output:

```text
pnpm build
mobile-h5: tsc -b && vite build passed
pc-console: tsc -b && vite build passed
warning: PC chunk exceeds 500 kB after minification due Arco bundle; no performance claim made.
```

```text
pnpm test:e2e
Running 9 tests using 1 worker
9 passed (6.8s)
```

```text
go test ./...
passed
```

Known limits:
- Realtime events are simulated through a browser `auction:event` custom event in E2E; real WebSocket connection wiring remains future work.
- Snapshot API is mocked in E2E; this slice proves client recovery state behavior, not backend snapshot rebuild behavior.
- No reconnect backoff UI is implemented yet.

Next action:
- Wire H5 to real WebSocket ticket/connect flow or add PC backend rule save/error surfacing.
