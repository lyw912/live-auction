# PTS Report 913WX7HG Review

Date: 2026-05-30

Verdict: `CORRECTNESS_PASS`, not accepted-hot-path performance evidence.

PTS overview:

- report id: `913WX7HG`;
- time: `2026-05-30 02:01:42` to `2026-05-30 02:02:42`;
- `AgentCount=2`;
- `Vum=916`;
- `POST PTS-1 hotspot bid` samples reported by PTS: `805`;
- PTS success rate: `100%`;
- PTS p99: about `29.9ms`.

Server-side business truth:

- DB unique bids: `1000`;
- accepted: `684`;
- rejected: `316`;
- all rejects were `BID_TOO_LOW`;
- `auc_live.current_price_cents=5010000`;
- `auc_live.accepted_bid_count=684`;
- `auc_live.seq=684`;
- `auc_live.engine_seq=1000`;
- `outbox_events=684`;
- `auction_events=684`;
- DLQ empty;
- Kafka consumer lag zero;
- Redis pending decisions empty;
- all correctness/invariant gates passed.

What it proves:

- The service processed all 1000 unique business bids and settled the full Redis
  ledger through Kafka/PostgreSQL/outbox without gaps or pending work.
- The monotonic-price rule is doing the right thing: lower prices arriving after
  higher prices are rejected deterministically as `BID_TOO_LOW`.

Why it is not accepted-hot-path performance evidence:

- PTS reported only `805` POST samples while PostgreSQL recorded `1000` bids, so
  the PTS latency/TPS numbers do not cover the full one-shot workload.
- The DB bid window was about `7.127s`, from `2026-05-29 18:02:33.202692+00` to
  `2026-05-29 18:02:40.329989+00`.
- Arrival order still had `229` amount inversions under two PTS agents. With a
  strict monotonic-price auction rule, those inversions correctly turn part of
  the run into rejection traffic.

Fix:

- The accepted-ladder JMX default step is now `accepted_ladder_step_ms=10`.
  `5ms` remains useful as a correctness/contention signal, but it is too small
  for accepted-dominant hot-path evidence on this PTS setup.

Evidence:

- PTS export: `docs/perf/pts/evidence/913WX7HG/`
- after snapshot: `docs/perf/pts/evidence/after-913WX7HG-l4b-accepted-ladder-5ms-1000vu/`
