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
- PC rule form `保存规则` calls `PATCH /api/auctions/{selected_auction_id}/rules`; the selected DRAFT auction is used instead of a fixed target.
- Request body uses backend contract fields:
  - `start_price_cents`
  - `increment_cents`
  - `cap_price_cents`
  - duration, extension, fat-finger, and deposit fields covered in `p0-26-pc-full-rule-fields.md`
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
- E2E uses mocked backend responses; this slice proves PC browser contract and error surfacing.
- Backend persistence for start price, increment, cap, and full rule fields is covered by Go repository/gateway tests.

Next action:
- Add a dedicated live PC browser smoke if P1 wants browser-to-live-backend host workflow evidence beyond mocked contract coverage.
