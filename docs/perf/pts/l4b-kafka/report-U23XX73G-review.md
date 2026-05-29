# PTS Report U23XX73G Review

Date: 2026-05-30

Verdict: `CORRECTNESS_PASS_AND_ACCEPTED_LADDER_PASS`, with PTS report-summary
count caveat.

PTS overview:

- report id: `U23XX73G`;
- report name: `accepted-20260530024416`;
- time: `2026-05-30 02:44:16` to `2026-05-30 02:45:16`;
- `AgentCount=2`;
- `Vum=881`;
- `POST PTS-1 hotspot bid` samples reported by PTS: `211`;
- PTS success rate: `100%`;
- PTS average RT: about `4.69ms`;
- PTS p90: about `14ms`;
- PTS p99: about `32.64ms`.

Server-side business truth:

- DB unique bids: `1000`;
- accepted: `773`;
- rejected: `227`;
- all rejects were `BID_TOO_LOW`;
- DB bid window: `2026-05-29 18:45:05.149584+00` to
  `2026-05-29 18:45:15.250395+00`, about `10.10s`;
- `auc_live.current_price_cents=5010000`;
- `auc_live.current_winner_id=k6_bidder_143_5`;
- `auc_live.accepted_bid_count=773`;
- `auc_live.seq=773`;
- `auc_live.engine_seq=1000`;
- `auction_events=773`, contiguous public seq `1..773`;
- `outbox_events=773`, all published;
- `redis_engine_settlements=1000`, all settled from Kafka;
- DLQ empty;
- Kafka consumer lag zero;
- Redis pending decisions empty;
- `engine_paused=false`;
- all correctness/invariant gates passed.

What it proves:

- The C33 failure mode is fixed for this run: the engine did not pause, Redis
  pending drained to zero, Kafka ledger settled to PostgreSQL, DLQ stayed empty,
  and the post-run P0 gate `engine_not_paused` passed.
- The service processed the complete 1000 unique bid workload exactly once by
  `client_bid_id` and `engine_seq`.
- The winner/price are deterministic: the highest amount `5010000` from
  `k6_bidder_143_5` became the final active winner.
- The public auction stream is consistent: only accepted bids advance public
  `seq`, while rejected low bids advance only `engine_seq`.

PTS count caveat:

- PTS `SamplerMetricsList.AllCount` reports only `211`, while PostgreSQL and
  server metrics both show the full `1000` business bid attempts.
- The server metrics after the run show `redis_lua_script_total=1000`,
  `auction_bid_redis_ledger_total{ENGINE_ACCEPTED}=773`, and
  `auction_bid_redis_ledger_total{ENGINE_REJECTED}=227`.
- `GetJMeterSamplingLogs` returned only `8` sample records for this report.
  Project runbooks already treat this API as sampled detail, not all request
  logs. Therefore the sampled-log export cannot be used to dispute the
  database audit trail.

Performance interpretation:

- Use the server-side throughput for the one-shot accepted ladder: `1000` bid
  decisions in about `10.10s`, roughly `99` decisions/s for this 10ms ladder
  workload.
- The accepted hot path handled `773` accepted bid settlements in the same
  window, with no pending, DLQ, duplicate, gap, or pause.
- Do not claim PTS `AvgTps=3.93` as the real backend throughput. It reflects
  the PTS report-summary count, not the full server-side business workload.

Evidence:

- PTS report details: `docs/perf/pts/evidence/U23XX73G/report-details.json`
- PTS sampled logs: `docs/perf/pts/evidence/U23XX73G/pts-sampling-logs/`
- after snapshot:
  `docs/perf/pts/evidence/after-U23XX73G-l4b-accepted-ladder-10ms-1000vu/`
