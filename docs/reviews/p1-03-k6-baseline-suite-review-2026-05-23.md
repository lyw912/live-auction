# P1-03 k6 Baseline Suite Review - 2026-05-23

## Scope

Review target: k6 baseline workload scripts and validation for P1-03.

Design basis:

- `docs/design-v2-industrial/09-performance-and-benchmark.md`
- `docs/design-v2-industrial/10-test-gates.md`
- `docs/design-v2-industrial/12-engineering-rules.md`

## Findings

No remaining P0/P1 findings after fixes.

Fixed during review:

- [P1] Shared k6 helper initially imported remote `https://jslib.k6.io/...`. That weakens reproducibility and can fail in restricted environments. Replaced with a local helper.
- [P1] Slow-consumer script initially lacked explicit thresholds. Added threshold validation and script threshold.
- [P2] The one-iteration execution smoke used CLI overrides, producing a k6 warning. Documented as smoke-only; formal baseline must run committed script scenarios without CLI shape overrides.

## Missing Tests

No blocker for P1-03 suite readiness. Current evidence proves:

- all five required workload scripts exist;
- scripts inspect successfully with local k6;
- WebSocket scripts use `k6/websockets`;
- one real backend k6 smoke executes and exports raw JSON.

Still required before any performance number:

- 3 raw runs per workload on Linux native or documented equivalent;
- environment capture using `templates/perf-baseline.md`;
- DB/Redis/runtime metrics captured alongside k6 output;
- bottleneck and next action written honestly.

## V2 Drift

No drift in committed suite.

No QPS/P99/fanout/online-user capacity claim was added.

## Residual Risk

- P0 smoke seed is a small deterministic dataset. Formal baseline may need a larger seed plan before claims.
- Mock auth remains a documented P1/P2 product limitation; k6 uses mock headers because real auth is not implemented.
- WS slow-consumer behavior is hard to emulate perfectly inside k6 because JS VU execution differs from browser rendering stalls. Treat slow-consumer k6 output as transport pressure evidence, not UI longtask evidence.
