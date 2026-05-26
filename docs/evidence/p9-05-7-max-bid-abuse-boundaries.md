# P9-S5-7 Max Bid Abuse Boundaries Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S5-7 Max Bid fat-finger/churn/abuse boundaries<br>
> ADR: `docs/adr/p9-04-max-bid-pre-bid-decision.md`

## Changed

Hardened Max Bid intent idempotency and recovery behavior:

- `source` is normalized before hashing and is now part of the Max Bid PUT idempotency identity;
- same key plus changed `MAX_BID`/`PRE_BID` source now rejects as `IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST`;
- H5 Max Bid submit can retry directly after a server-side error state instead of getting stuck until a manual amount change;
- backend tests cover unsafe amount classes, terminal auctions, stuck/expired idempotency records, cancel/recreate churn, and source-change idempotency abuse;
- H5 e2e covers server rejects for too-low, off-grid, above-cap, and processing-retry cases without optimistic success.

## Validation

```text
go test ./internal/auction ./internal/gateway
go test -p 1 ./...
pnpm --filter mobile-h5 exec tsc --noEmit
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5
pnpm build
git diff --check
```

Result: PASS.

Covered:

- Max Bid idempotency replay with same key and same request stays stable;
- same key with changed amount or changed source returns conflict;
- invalid source, too-low amount, increment mismatch, and above-cap amount are rejected;
- terminal auctions reject Max Bid create and cancel;
- stuck `PROCESSING` records return bounded retry-later when the hash matches and conflict when the request differs;
- expired `PROCESSING` records become `IDEMPOTENCY_TIMEOUT`;
- cancel then recreate reactivates the same user intent with version advance;
- H5 does not show committed Max Bid success after server rejects and can retry from error state.

## Review

- No new public realtime fields were added.
- No client-side proxy bidding, winner calculation, or hammer behavior was added.
- Max Bid remains a private current-user REST state plus public real bid rows only when the server settlement path executes.

## Known Limits

- This slice proves correctness and abuse boundaries locally; it does not claim load capacity for Max Bid churn.
- Future P10 no-mock demo still needs live backend evidence across real auctions and monitor APIs.
