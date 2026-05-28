# PTS L1 Admission And Debounce Evidence

> Date: 2026-05-28 Asia/Shanghai<br>
> Slice: PTS-1 L1 Admission/debounce<br>
> Design: `docs/perf/pts/README.md`, `docs/perf/pts/hotspot-redesign-roadmap-2026-05-28.md`, `docs/adr/pts-02-hotspot-bidding-engine-redesign.md`

## Changed

Implemented the L1 layer only. This does not claim L2 PostgreSQL lane or Redis engine work.

- Gateway bid admission now returns explicit retry timing for Redis GCRA user/IP/auction rejects and local auction in-flight saturation.
- `Retry-After` is now sent for both `RATE_LIMITED` and `BID_AUCTION_TOO_HOT`.
- Error JSON now includes `details.retry_after_ms` and `details.retry_after_secs` so H5 can render the same cooldown as the server enforced.
- Completed bid idempotency replay still happens before Redis/local admission, preserving retry correctness under overload.
- Admission reject anomaly payloads now record retry timing for operator diagnosis.
- H5 bid dock keeps a ref-level in-flight guard, reads retry timing from response details/header, disables the CTA during cooldown, and shows a countdown-style retry copy.
- H5 still shows no optimistic bid success before server acceptance.

## Validation

```text
go test ./internal/gateway -run TestBidAdmission -count=1
go test ./internal/gateway -count=1
pnpm --filter mobile-h5 build
pnpm exec playwright test --list --project=mobile-h5
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --grep "H5 bid stays pending|H5 rate-limit rejects|H5 processing retry-later" --workers=1 --reporter=line
```

Result:

- gateway targeted tests: PASS
- gateway package tests: PASS
- H5 TypeScript/Vite build: PASS
- Playwright test discovery: PASS, including `H5 rate-limit rejects enter retry-after cooldown before another bid`
- Targeted H5 Playwright tests: PASS:
  - `H5 bid stays pending until authoritative accepted response`
  - `H5 rate-limit rejects enter retry-after cooldown before another bid`
  - `H5 processing retry-later keeps duplicate-click guidance without cooldown`

Attempted but not counted as passed:

```text
pnpm exec playwright test --project=mobile-h5 --workers=1 --reporter=line
```

This full mobile-H5 project run timed out locally after 364 seconds without test assertion output. The targeted L1 browser gates above did pass.

## Coverage

- Backend proves completed idempotency replay bypasses admission.
- Backend proves Redis-down admission fails open and records an anomaly.
- Backend proves user limit returns `RATE_LIMITED`, `Retry-After`, and retry details.
- Backend proves local auction saturation returns `BID_AUCTION_TOO_HOT`, `Retry-After`, and retry details.
- Backend proves admission anomaly payloads include retry timing for diagnosis.
- Frontend build proves the cooldown/in-flight changes compile and bundle.
- Route-mocked H5 tests prove pending disables optimistic success, retry-after cooldown suppresses duplicate pressure, and `PROCESSING_RETRY_LATER` still shows duplicate-click guidance without cooldown.

## Review

`live-auction-v2-code-review` context checked against:

- `docs/design-v2-industrial/12-engineering-rules.md`
- `docs/design-v2-industrial/10-test-gates.md`
- `docs/design-v2-industrial/05-api-contracts.md`
- `docs/design-v2-industrial/07-frontend-ux.md`

Findings:

- No money-truth drift: PostgreSQL remains bid truth and Redis admission only shapes entry pressure.
- Completed idempotency replay remains before Redis and local admission.
- No optimistic H5 success was introduced; the CTA stays disabled during pending and retry-after cooldown.
- One review issue was found and fixed before commit: the old `PROCESSING_RETRY_LATER` H5 coverage was accidentally replaced by the new cooldown test. A dedicated test now preserves that behavior.

## Known Limits

- This slice does not implement L2 per-auction bounded queue or transaction slimming.
- This slice does not claim improved PTS P99. A before/after PTS run belongs after L2 or a combined L1/L2 run.
- H5 cooldown is driven by server response timing; it does not decide auction state, winner, sold, or final price.
