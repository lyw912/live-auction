# PTS Report TR3VX7RG Review

Date: 2026-05-30

Verdict: `HARNESS_GAP`

`TR3VX7RG` cannot be used as accepted-ladder performance evidence. The PTS
overview reports a 1-minute run from `2026-05-30 01:28:31` to
`2026-05-30 01:29:31`, `AgentCount=2`, and `Vum=930`, but PTS returned an empty
`SamplerMetricsList` and the sampling-log export returned zero samples.

Server-side evidence confirms that no bid traffic reached the system:

- `bids=0`, `accepted=0`, `rejected=0`;
- `auction_events=0`;
- `outbox_events=0`;
- `redis_engine_settlements=0`;
- `auc_live` stayed at `current_price_cents=10000`, `seq=0`, `engine_seq=0`.

The cause is the old accepted-ladder JMX timing, not backend performance. It
used `burst_wait_ms=54000` and `accepted_barrier_quantum_ms=10000`. In a
60-second PTS scene, a run starting at `01:28:31` can align the first bid to
about `01:29:30`; the 5ms ladder then overlaps scene teardown, so the POST
sampler never produces useful business traffic.

The current corrected JMX uses `burst_wait_ms=40000`,
`accepted_barrier_quantum_ms=10000`, and `accepted_ladder_step_ms=10`. This keeps
the cross-agent wall-clock alignment benefit of the 10-second quantum while
releasing the 1000 one-shot bids early enough to leave response and PTS reporting
headroom before the 60-second scene ends.

Evidence:

- PTS export: `docs/perf/pts/evidence/TR3VX7RG/`
- after snapshot: `docs/perf/pts/evidence/after-TR3VX7RG-l4b-accepted-ladder-5ms-1000vu/`
