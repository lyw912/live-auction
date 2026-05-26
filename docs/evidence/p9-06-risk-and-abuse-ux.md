# P9-S6 Risk And Abuse UX Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S6 Risk and abuse UX<br>
> Design: `docs/design-v2-industrial/07-frontend-ux.md`, `08-observability-and-ops.md`

## Changed

Added user-facing and host-facing risk UX without inventing a new risk engine:

- H5 rejected bid state now includes an actionable risk guidance row for rate limit, auction-hot, processing, timeout, off-grid, too-low, ended, and network cases;
- H5 keeps the bid CTA recoverable after rejects and continues to avoid optimistic success;
- PC Live Assist now renders a `Risk queue` from real monitor rows only: `monitor.rejects`, `monitor.anomalies`, and `monitor.recovery`;
- PC risk rows show source tables/producers so hosts know where to inspect next;
- no fake score, fake moderation status, fake watcher count, or synthetic abuse event was added.

## Validation

```text
pnpm --filter mobile-h5 exec tsc --noEmit
pnpm --filter pc-console exec tsc --noEmit
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console
pnpm build
go test -p 1 ./...
git diff --check
```

Result: PASS.

Covered:

- H5 increment mismatch keeps authoritative price, re-enables CTA, and shows adjustment guidance;
- H5 auction-hot/rate-limit and processing-retry rejects show distinct abuse guidance;
- H5 still does not show optimistic success after a reject;
- PC risk queue renders reject pressure, anomaly, and recovery-pressure rows from route-mocked monitor payloads;
- PC route-mocked coverage remains UI contract coverage, not no-mock demo evidence.

## Review

- Diagnostics remain backed by existing DB/monitor producers.
- No auction truth, outbox, WebSocket, or settlement behavior changed.
- H5 risk guidance is copy/state only; it does not decide winner, close, or throttle policy.

## Known Limits

- There is no automated risk score or moderation workflow; risk queue is an operator triage view over existing diagnostics.
- Final P10 demo must use live backend-created auctions and real monitor APIs for no-mock evidence.
