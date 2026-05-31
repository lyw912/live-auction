# L3 Scenario Realism — Planned

> Status: PLANNED. Not yet implemented. See `tests/pts/MANIFEST.md` for the full framework.

## Purpose

L3 tests bid traffic that matches the real business distribution: not a pure final-second
burst, but a 30-minute auction lifecycle where bids arrive on a realistic curve that
spikes sharply in the last 5 minutes.

L1-C1 proves the engine can handle the worst-case instantaneous spike. L3 proves the system
sustains correctness and latency SLAs over a full auction run, including outbox delivery,
settlement, and snapshot recovery.

## Workloads

### L3-S1 — Full auction lifecycle, realistic bid distribution

**Hypothesis**: outbox delivery lag does not accumulate over 30 minutes; settlement
completes within SLA after auction close; no memory leak over sustained run.

Traffic shape (realistic live auction curve):
```
  0–20 min : ~5 bid/s   (casual browsing phase)
  20–25 min: ~20 bid/s  (competitive phase)
  25–29 min: ~80 bid/s  (final push)
  29–30 min: ~500 bid/s (final 60s burst, approaching L1-C1 intensity)
```

| Dimension | Value |
|---|---|
| Total bids | ~5000–8000 over 30 min |
| Max concurrent VUs | 500 (final minute) |
| WS viewers | 500 long-lived, connected from minute 0 |
| SLA | bid p99 ≤ 60ms in final 60s; outbox lag ≤ 2s at any point |
| Post-run checks | settlement complete, highest valid bid wins, engine_seq gap-free |

JMX file (to be created): `tests/pts/L3-scenario/pts-3s1-full-lifecycle-30min.jmx`

Implementation note: use JMeter Throughput Shaping Timer to model the bid rate curve.
Ramp-up phases: `ConstantThroughputTimer` or `ThroughputController` per phase.

---

### L3-S2 — Multi-room parallel auctions

**Hypothesis**: multiple concurrent hot auctions do not cross-contaminate engine state
or Redis key-space. Room isolation holds under parallel load.

| Dimension | Value |
|---|---|
| Concurrent auctions | 3 (auc_live, auc_side, auc_inv_001) |
| Bid VUs per auction | 300 |
| Total VUs | 900 |
| Duration | 5 min sustained |
| SLA | per-auction p99 ≤ 60ms; no cross-auction winner contamination |

JMX file (to be created): `tests/pts/L3-scenario/pts-3s2-multi-room-isolation.jmx`

## Prerequisites

- L2-P3 must pass before running L3 workloads.
- L3-S1 requires a dedicated session pool (viewer sessions + bidder sessions).
- L3-S1 needs extended auction `end_at` (90+ min) set before run; `reset-l4b-final-second-pressure.sh`
  already supports `P1_LOAD_AUCTION_END_MINUTES`.
- Soak-mode monitoring: collect `/metrics` every 30s; watch `outbox_lag_seconds` histogram.
