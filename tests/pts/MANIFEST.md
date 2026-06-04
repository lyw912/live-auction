# PTS And Load Asset Manifest

> Last updated: 2026-06-04.
> This file is an implementation asset index only. The single test plan,
> scenario names, run order, scale choices, and judge-facing narrative are in
> `docs/current/test-strategy/README.md`.

Do not use old `L*` names as plan stages. They remain only in filenames and
legacy aliases because the scripts already exist. New reports and plans should
say `S0` through `S5`.

Read before running or interpreting any workload:

- `docs/current/performance-correctness-contract.md`
- `docs/current/test-strategy/README.md`
- `docs/current/test-strategy/metrics-and-slo.md`
- `docs/current/test-strategy/pts-playbook.md`
- `docs/current/evidence-policy.md`

## Scenario Assets

| Scenario | Purpose | Tool | Primary asset | Scale / parameters | Evidence gate |
|---|---|---|---|---|---|
| `S0` | Single-user business chain smoke | PTS JMeter or local JMeter | `L0-smoke/live-auction-pts-business-smoke.jmx` | 1 VU, 1 loop, 1s ramp | PASS/FAIL only, not capacity evidence |
| `S1-burst` | Final-second 1000-user contention | PTS JMeter | `L1-component/pts-1b-contention-burst-1000vu-1m.jmx` | 1000 bid VU, one bid each, default `contention_release_window_ms=500` so bids target a short final-second window, 1-2 min, 2 IP, sampling 100% for judge-forensics runs | `verify-l4b-pts-correctness.sh`; `review-s1-pts-run.sh`; M1 p99 <= 50ms; 1000 decisions classified |
| `S1-ladder` | Accept-path control | PTS JMeter | `L1-component/pts-1a-accepted-ladder-1000vu-1m.jmx` | 1000 bid VU, ordered increasing amounts | Control only; do not cite as contention evidence |
| `S2-long-soak` | Steady bid-decision stability and no leak | independent-ECS k6 | `../load/s2-steady-soak.js` | 20/s -> 60/s -> 100/s bid attempts, 30-60 min, report dropped iterations | k6 p99 <= 100ms, dropped iterations bounded, Grafana heap/goroutine/fd flat; this is bid-decision endurance, not read/WS fanout evidence |
| `S2-convergence-drain` | Post-pressure async drain and payment safety | k6 or PTS + server verifier | `run-s2-steady.sh`; `collect-server-evidence.sh` | 2-5 min known pressure point, then monitor until Kafka/Redis/PG/outbox all zero | convergence seconds, verifier PASS, no pending settlement/outbox/DLQ |
| `S2-capacity-stair` | Find the single-auction bid capacity knee | independent-ECS k6 first, optional PTS RPS chart | `../load/s2-steady-soak.js`; `prepare-s2-capacity-stair-pressure.sh`; optional `L2-protocol/pts-2p4-steady-interactive-auction.jmx` | Two explicit profiles. `accepted`: 50/s -> 100/s -> 200/s -> 400/s -> 600/s, 1s ramp plus 90s hold per target, `AMOUNT_MODE=fast_ladder`, `NOISE_PCT=0`, `USER_COUNT=MAX_VUS`, intended to keep prices increasing and stress accepted updates, Kafka, settlement, outbox, and fanout source. `decision`: 100/s -> 200/s -> 400/s -> 600/s -> 1000/s, 1s ramp plus 2m hold per target, `AMOUNT_MODE=time_ladder`, `NOISE_PCT=20`, intentionally includes normal low/self/stale rejects and measures Redis decision throughput. Do not mix the two claims. | k6 exit 0, dropped iterations 0 for clean stair, p99/p99.9 by stage, delivered vs target rate, accepted/rejected distribution, Kafka/Redis/PG/outbox backlog slope, k6 host CPU/RSS/FD/TCP retransmission |
| `S2-read-interference` | Reader RPS impact on bid path | PTS RPS or independent-ECS k6 | `../load/s2-read-interference.js`; `prepare-s2-read-interference-pressure.sh` | 100/s bid attempts plus HTTP reads, 80/15/5 snapshot/leaderboard/my-bids mix; do not reuse S3 fanout samplers. 2026-06-04 10k and 4k profiles are CURRENT_FAILING bottleneck evidence. After the Redis TTL P0 and read-path P1 mitigation, `s2-read-display-postfix-ecs-15m-20260604T140509` is CURRENT_PASS for 1500/s -> 1800/s -> 2000/s reads: dropped 0, bid p99 3.76ms, snapshot p99 11.54ms, leaderboard p99 4.46ms, my-bids p99 0.87ms, verifier P0/P1 PASS. | bid p99 under read load, read p99 by route, DB pool wait, correctness and convergence |
| `S3-live-only-fanout` | Isolated fanout latency vs connection count | PTS VU and/or independent-source k6 | `../load/s3-fanout-soak.js` | 1000 -> 2000 -> 5000 -> optional 10000 WS; controlled accepted updates; no reader polling | active connections, accepted updates, receive samples, fanout p99/max, resource per connection |
| `S3-mixed-final-burst-smoke` | Small PTS harness/network validation before paid mixed run | PTS JMeter | `S3-room-fanout/s3-live-fanout-smoke-30vu-single-branch-20ws-5bid-5read.jmx` | 30 VU, 1 IP, 1 minute, 100% sampling, upload `s3-mixed-smoke-30-sessions.csv` | split labels for join/ticket/handshake/first message/live fanout/read/bid; report-details for exact sampler counts |
| `S3-mixed-final-burst` | Final-window bid burst with online viewers and controlled readers | PTS JMeter | `S3-room-fanout/s3-live-fanout-4500vu-single-branch-3000ws-1000bid-500read.jmx` | 1000 bid + 3000 WS + 500 reader = 4500 VU, 9 IP, one single main Thread Group, upload `s3-mixed-4500-sessions.csv`; set PTS `是否指定循环=是`, `循环次数=1`, and duration long enough for connect + burst + observe | `verify-s3-pts-evidence.sh` with expected counts; live fanout p99 <= 1000ms; first-message p99 <= 1000ms; server metrics for WS/bid/read counts |
| `S4-fault` | Fault resilience, RTO/RPO, no double settlement | local k6 + SIGKILL/Toxiproxy | `run-pts-1c-concurrent-fault.sh`; `L1-component/pts-1c-k6-concurrent-fault.js`; `tests/chaos/*` | default `L1F_PROFILE=rto`, 200 VU, 25s, 1s pacing, 5s fault window | fault gates, RTO, Redis pending/Kafka lag/settlement/outbox convergence |
| `S5-reconnect` | Weak-network reconnect recovery | local k6 + Toxiproxy | `../load/s5-reconnect-recovery.js`; `run-s5-reconnect.sh`; `tests/chaos/s5-toxiproxy-ws-fault.mjs` | clean: 20 -> 100 -> 200 reconnect VU; network: Toxiproxy reset_peer smoke | TTCS p99, zero seq gaps/duplicates, zero recovery errors, server truth |

## Recommended Run Order

1. `S0`
2. `S1-burst`, then optional `S1-ladder`
3. `S4-fault` P0 set: Redis, settlement worker, PostgreSQL
4. `S2-soak`
5. `S3-live-only-fanout`
6. `S4-fault` P1 set: Kafka, Redis flush, Redis+Kafka
7. `S4-fault` P2 set: Redis partial timeout through Toxiproxy
8. Optional: `S2-capacity-stair` PTS chart, `S3-mixed-final-burst`, `S5-reconnect`

## PTS Upload Data

| File | Purpose |
|---|---|
| `docs/perf/pts/pts-1ab-1000vu-sessions.csv` | S1 1000-user session pool |
| `docs/perf/pts/s3-mixed-4500-sessions.csv` | S3 mixed final burst upload CSV: 3000 viewer + 1000 bidder + 500 reader |
| `docs/perf/pts/s3-mixed-smoke-30-sessions.csv` | S3 smoke upload CSV: 20 viewer + 5 bidder + 5 reader |
| `docs/perf/pts/pts-l2p4-bidder-360-sessions.csv` | S2 optional PTS chart bidders |
| `docs/perf/pts/pts-l2p4-viewer-2400-sessions.csv` | S2 optional PTS chart viewers |
| `docs/perf/pts/pts-l2p4-reader-240-sessions.csv` | S2 optional PTS chart readers |
| `tests/pts/pts_sessions.csv.example` | Example CSV shape only |

## Utility Scripts

| Script | Role |
|---|---|
| `tests/pts/run-s1-contention.sh` | S1 reset/preflight/post-run helper |
| `tests/pts/run-s2-steady.sh` | S2 k6 soak helper plus optional PTS prompt |
| `tests/pts/prepare-s2-capacity-stair-pressure.sh` | Reset/seed for independent-k6 S2-capacity-stair, including mock-auth bidder ACL cache sized by `MAX_VUS` |
| `tests/pts/prepare-s2-read-interference-pressure.sh` | Reset/seed for independent-k6 S2-read-interference, including mock-auth bidder/reader ACL caches |
| `tests/pts/run-s3-fanout.sh` | S3 PTS/local fanout helper |
| `tests/pts/run-s4-fault.sh` | S4 fault helper |
| `tests/pts/run-s5-reconnect.sh` | S5 reconnect helper |
| `tests/pts/reset-l4b-final-second-pressure.sh` | Reset/seed for S1 |
| `tests/pts/prepare-l2-protocol-pressure.sh` | Legacy mixed-protocol reset/seed and CSV generation |
| `tests/pts/prepare-s3-room-fanout-pressure.sh` | Reset/seed and generate current S3 single-branch mixed CSVs and Redis session/snapshot warm-up |
| `tests/pts/prepare-l2p4-steady-pressure.sh` | Reset/seed and generate S2 optional PTS chart CSVs |
| `tests/pts/preflight-l4b-pts-guards.sh` | Preflight guard: Redis Stream/AOF, Kafka, settlement protections |
| `tests/pts/collect-server-evidence.sh` | Post-run server evidence collector |
| `tests/pts/verify-l4b-pts-correctness.sh` | Correctness verifier: ENGINE distribution, seq gaps, DLQ/pending/settlement |
| `tests/pts/verify-l2p3-pts-evidence.sh` | Legacy mixed-protocol verifier; harness/instrumentation only |
| `tests/pts/verify-l2p4-pts-evidence.sh` | S2 optional PTS chart verifier |
| `tests/pts/verify-s3-pts-evidence.sh` | Current S3 verifier; use report-details for sampler counts/success/RT, sampling logs for `S3_V6...WS_ONLY` response proof |
| `tests/pts/fetch-pts-sampling-logs.sh` | Optional sampling-log helper |
| `tests/pts/summarize-pts-sampling-logs.sh` | Optional sampling-log summarizer |
| `tests/pts/review-s1-pts-run.sh` | S1 judge-facing evidence recomputation from 100% sampling logs, server metrics, and correctness gates |

## Interpretation Rules

- M1 final bid decision requires `result in ENGINE_*` and `durability_status=ENGINE_DURABLE`.
- S1-burst's default release model is a 500 ms final-second target window, not a
  zero-ms SyncTimer-style wall. To run the old diagnostic microburst, set JMeter
  property `contention_release_window_ms=0` through PTS JMeter environment
  properties; do not compare that diagnostic p99 directly with the default
  judge-facing S1 result.
- `ENGINE_DURABLE` is the Redis decision-log boundary. Kafka relay, PostgreSQL settlement, and outbox delivery must converge before citing correctness.
- HTTP `200` count is not accepted-bid count.
- HTTP `202` / `PROCESSING_RETRY_LATER` RTT is not M1 decision latency.
- Dominant `PROCESSING_RETRY_LATER`, vague `409`, pending Redis decisions, Kafka lag, DLQ, engine pause, settlement gap, or outbox backlog fails current pass evidence.
- A latency number without verifier output is not success evidence.
- A file existing here does not make it current evidence. Historical scripts are indexed in `tests/pts/HISTORICAL.md`.
- The old S3 7000/5000 multi-ThreadGroup JMX variants are not the current default
  because PTS can redistribute separate main Thread Groups and console duration
  can keep reader groups looping. Use the single-branch mixed CSV/JMX unless a
  run review explicitly states a different controlled experiment.
