# P1-06 UI Performance Trace Review - 2026-05-23

## Scope

Review target: H5 UI performance trace runner, package script, raw output policy, and evidence.

Design basis:

- `docs/design-v2-industrial/07-frontend-ux.md`
- `docs/design-v2-industrial/09-performance-and-benchmark.md`
- `docs/design-v2-industrial/10-test-gates.md`
- `docs/design-v2-industrial/12-engineering-rules.md`

## Findings

No remaining P0/P1 findings after verification.

Fixed during review:

- [P1] Existing P0 longtask test only asserted an E2E invariant and did not persist raw trace output. Added a dedicated P1 runner that writes JSON summary plus Playwright trace zip.
- [P1] UI trace evidence could have been misread as a production performance baseline. Evidence now states the thresholds are local UI safety gates only and makes no QPS/P99/fanout/device claim.

## State Matrix

| State/event | User sees | Host sees | Gap |
|---|---|---|---|
| ACTIVE bidding controls | exercised in H5 state matrix | not part of P1-06 | none for this slice |
| recovering/disconnected safety | recovering state exercised with disabled CTA in existing E2E and trace flow | not part of P1-06 | none for this slice |
| rejected/extended/sold/cancelled | exercised through H5 state matrix | not part of P1-06 | none for this slice |

## Performance Verdict

PASS for local UI safety trace.

No production performance claim is supported by this slice.

## Missing Tests

No blocker for P1-06 readiness.

Still useful before final release:

- Run the same trace on a low-end Android device or emulated throttling profile if the demo adds heavier atmosphere effects.
- Keep `pnpm test:e2e` longtask smoke green as a fast regression gate.

## V2 Drift

No current drift.

The runner measures UI longtask/input/frame stability and stores raw evidence. It does not claim backend throughput or online-user scale.

## Residual Risk

- Local Chromium timing can vary with laptop load and power mode.
- The trace covers state-matrix interactions, not a live backend auction under k6 load.
- Playwright trace zip is useful for debugging but should not be interpreted as a formal production benchmark.
