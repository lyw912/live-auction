# P0-19 PC Rule Validation

Gate:
- PC rule form
- cap reachability
- increment grid

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
- PC rule form now mirrors the backend cap reachability rule:
  - `increment_cents > 0`
  - `cap_price_cents >= start_price_cents + increment_cents`
  - `(cap_price_cents - start_price_cents) % increment_cents == 0`
- Unreachable cap disables `保存规则`.
- The cap field shows nearest legal cap suggestions.
- Clicking a legal suggestion updates the cap and re-enables save.
- Backend remains the final authority; this is a frontend guardrail only.

Raw output:

```text
pnpm build
mobile-h5: tsc -b && vite build passed
pc-console: tsc -b && vite build passed
warning: PC chunk exceeds 500 kB after minification due Arco bundle; no performance claim made.
```

```text
pnpm test:e2e
Running 4 tests using 1 worker
4 passed (4.8s)
```

```text
go test ./...
passed
```

Known limits:
- The rule form is still not wired to `PATCH /api/auctions/{id}/rules`.
- Backend validation is already tested separately; this slice only proves frontend blocking and suggestions.
- Duration, extension, fat-finger, and deposit fields are not yet represented in the PC rule form.

Next action:
- Wire PC rule save to backend and surface backend `INVALID_AUCTION_RULE_CAP_UNREACHABLE` responses, or move to H5 real bid/recovery integration.
