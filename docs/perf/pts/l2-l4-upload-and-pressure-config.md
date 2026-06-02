# Legacy L2-L4 PTS Configuration Reference

> Status: superseded asset reference, 2026-06-02.
> Do not use this file as a test plan. The only current plan, run order, and
> judge-facing naming scheme is `docs/current/test-strategy/README.md`.

This file used to describe a parallel `L2`-`L4` PTS plan. That plan is replaced
by the `S0`-`S5` strategy. Keep this page only to help map old JMX filenames,
CSV pools, and PTS reports to the current scenarios.

## Current Mapping

| Old asset label | Current use | Current authority |
|---|---|---|
| `L2-P1 bid + WS fanout` | S3 fanout PTS asset / cost variant input | `docs/current/test-strategy/s3-room-fanout.md` |
| `L2-P3 bid + WS + reads` | legacy mixed-protocol diagnostic only | `tests/pts/MANIFEST.md` |
| `L2-P4 steady interactive auction` | optional S2 PTS chart asset | `docs/current/test-strategy/s2-steady-auction-and-soak.md` |
| `L2-P5 fanout soak` | S3 local/k6 fanout soak concept | `docs/current/test-strategy/s3-room-fanout.md` |
| `L2-P6 reconnect storm` | S5 reconnect recovery | `docs/current/test-strategy/s5-reconnect-recovery.md` |
| `L3-*` / `L4-*` | historical or future asset aliases, not current plan stages | `tests/pts/HISTORICAL.md` and `tests/pts/MANIFEST.md` |

## Current PTS Rules

- Use S0-S5 in all plans, report titles, and judge material.
- Use PTS JMeter only where the current strategy says it adds value: S1 and S3,
  plus optional S2 charting.
- Use local k6 for S2 soak, S4 fault, and S5 reconnect.
- Keep PTS sampling at 1% unless doing forensic body checks.
- M1 final bid-decision evidence requires `durability_status=ENGINE_DURABLE`.
  Kafka relay, PostgreSQL settlement, and outbox are convergence/fault evidence,
  not the synchronous HTTP response boundary.

## Source-Of-Truth Configuration

Use these current files instead of the old L2-L4 matrix:

| Need | File |
|---|---|
| Scenario names, order, tool split, scale, cost | `docs/current/test-strategy/README.md` |
| M1-M5 definitions and pass/fail boundaries | `docs/current/test-strategy/metrics-and-slo.md` |
| PTS billing, IP count, sampler naming, console settings | `docs/current/test-strategy/pts-playbook.md` |
| Asset index and run order | `tests/pts/MANIFEST.md` |
| S1 contention details | `docs/current/test-strategy/s1-final-second-contention.md` |
| S2 steady soak / optional PTS chart | `docs/current/test-strategy/s2-steady-auction-and-soak.md` |
| S3 fanout / 10k viewers | `docs/current/test-strategy/s3-room-fanout.md` |
| S4 faults | `docs/current/test-strategy/s4-fault-resilience.md` |
| S5 reconnect | `docs/current/test-strategy/s5-reconnect-recovery.md` |

## Legacy CSV Pools Still Used By Assets

| File | Current scenario |
|---|---|
| `docs/perf/pts/pts-1ab-1000vu-sessions.csv` | S1 |
| `docs/perf/pts/pts-l2p4-bidder-360-sessions.csv` | optional S2 PTS chart |
| `docs/perf/pts/pts-l2p4-viewer-2400-sessions.csv` | optional S2 PTS chart |
| `docs/perf/pts/pts-l2p4-reader-240-sessions.csv` | optional S2 PTS chart |
| `docs/perf/pts/pts-l2-bidder-1000-sessions.csv` | S3 |
| `docs/perf/pts/pts-l2-viewer-10000-sessions.csv` | S3 |
| `docs/perf/pts/pts-l2-reader-5000-sessions.csv` | optional diagnostics only |

If a legacy JMX sampler still contains `L2-*` in its sampler label, treat that
as an asset label required for existing verifier scripts and historical report
matching. It is not a current plan name.
