# P0-23 PC Rule Save

Gate:
- PC rule form
- backend rule error surfacing

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
- PC rule form `保存规则` calls `PATCH /api/auctions/auc_next/rules`.
- Request body uses backend contract fields:
  - `start_price_cents`
  - `increment_cents`
  - `cap_price_cents`
- Successful response shows saved state.
- Backend `INVALID_AUCTION_RULE_CAP_UNREACHABLE` response is surfaced even when frontend validation passes.
- Backend `details.suggested_caps` are rendered as clickable cap suggestions.
- Backend remains final authority.

Raw output:

```text
pnpm build
mobile-h5: tsc -b && vite build passed
pc-console: tsc -b && vite build passed
warning: PC chunk exceeds 500 kB after minification due Arco bundle; no performance claim made.
```

```text
pnpm test:e2e
Running 10 tests using 1 worker
10 passed (7.1s)
```

```text
go test ./...
passed
```

Known limits:
- E2E uses mocked backend responses; this slice proves PC client contract and error surfacing.
- Full rule schema fields for duration, extension, fat-finger, and deposit are not yet represented.
- Real backend integration against a live dev server remains future work.

Next action:
- Add H5 fat-finger confirm or wire H5/PC to a live backend dev server smoke path.
