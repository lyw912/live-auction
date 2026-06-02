# PTS And Load Asset Manifest

> Last updated: 2026-06-02.
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
| `S1-burst` | Final-second 1000-user contention | PTS JMeter | `L1-component/pts-1b-contention-burst-1000vu-1m.jmx` | 1000 bid VU, one bid each, 1-2 min, 2 IP, sampling 1% | `verify-l4b-pts-correctness.sh`; M1 p99 <= 50ms; 1000 decisions classified |
| `S1-ladder` | Accept-path control | PTS JMeter | `L1-component/pts-1a-accepted-ladder-1000vu-1m.jmx` | 1000 bid VU, ordered increasing amounts | Control only; do not cite as contention evidence |
| `S2-soak` | Steady auction stability and no leak | local k6 | `../load/s2-steady-soak.js` | 20/s -> 60/s -> 100/s, 30-60 min, report dropped iterations | k6 p99 <= 100ms, dropped iterations bounded, Grafana heap/goroutine/fd flat |
| `S2-pts-chart` | Optional steady realtime PTS PDF | PTS JMeter | `L2-protocol/pts-2p4-steady-interactive-auction.jmx` | 2400 WS + 360 bidders + 240 readers, 10 min, 6 IP | `verify-l2p4-pts-evidence.sh`; bid p99 <= 100ms; fanout p99 <= 1000ms |
| `S3-cost` | Fanout p99 chart + 10k local hold | PTS JMeter + local k6 | `L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx`; `../load/s3-fanout-soak.js` | PTS: 2000 WS + 1000 bid, 5 min, 6 IP. Local: 10000 WS, 10-30 min | PTS M2 chart; local Grafana RAM/conn, fd/goroutine plateau |
| `S3-headline` | One PTS report with 10000 online viewers | PTS JMeter | `L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx` | 10000 WS, 5 min, 20 IP, sampling 1% | M2 p99 <= 1000ms plus connection count and resource evidence |
| `S4-fault` | Fault resilience, RTO/RPO, no double settlement | local k6 + SIGKILL/Toxiproxy | `run-pts-1c-concurrent-fault.sh`; `L1-component/pts-1c-k6-concurrent-fault.js`; `tests/chaos/*` | default `L1F_PROFILE=rto`, 200 VU, 25s, 1s pacing, 5s fault window | fault gates, RTO, Redis pending/Kafka lag/settlement/outbox convergence |
| `S5-reconnect` | Weak-network reconnect recovery | local k6 | `../load/s5-reconnect-recovery.js` | start 20 VU smoke, scale to 100-200 reconnect VU after pass | TTCS p99, zero seq gaps/duplicates, server snapshot truth |

## Recommended Run Order

1. `S0`
2. `S1-burst`, then optional `S1-ladder`
3. `S4-fault` P0 set: Redis, settlement worker, PostgreSQL
4. `S2-soak`
5. `S3-cost`
6. `S4-fault` P1 set: Kafka, Redis flush, Redis+Kafka
7. Optional: `S2-pts-chart`, `S3-headline`, `S5-reconnect`

## PTS Upload Data

| File | Purpose |
|---|---|
| `docs/perf/pts/pts-1ab-1000vu-sessions.csv` | S1 1000-user session pool |
| `docs/perf/pts/pts-l2-bidder-1000-sessions.csv` | S3 bidder session pool |
| `docs/perf/pts/pts-l2-viewer-10000-sessions.csv` | S3 large WebSocket viewer pool |
| `docs/perf/pts/pts-l2-reader-5000-sessions.csv` | Optional read probe pool |
| `docs/perf/pts/pts-l2p4-bidder-360-sessions.csv` | S2 optional PTS chart bidders |
| `docs/perf/pts/pts-l2p4-viewer-2400-sessions.csv` | S2 optional PTS chart viewers |
| `docs/perf/pts/pts-l2p4-reader-240-sessions.csv` | S2 optional PTS chart readers |
| `tests/pts/pts_sessions.csv.example` | Example CSV shape only |

## Utility Scripts

| Script | Role |
|---|---|
| `tests/pts/run-s1-contention.sh` | S1 reset/preflight/post-run helper |
| `tests/pts/run-s2-steady.sh` | S2 k6 soak helper plus optional PTS prompt |
| `tests/pts/run-s3-fanout.sh` | S3 PTS/local fanout helper |
| `tests/pts/run-s4-fault.sh` | S4 fault helper |
| `tests/pts/run-s5-reconnect.sh` | S5 reconnect helper |
| `tests/pts/reset-l4b-final-second-pressure.sh` | Reset/seed for S1 |
| `tests/pts/prepare-l2-protocol-pressure.sh` | Reset/seed and generate S3 bidder/viewer/reader CSVs |
| `tests/pts/prepare-l2p4-steady-pressure.sh` | Reset/seed and generate S2 optional PTS chart CSVs |
| `tests/pts/preflight-l4b-pts-guards.sh` | Preflight guard: Redis Stream/AOF, Kafka, settlement protections |
| `tests/pts/collect-server-evidence.sh` | Post-run server evidence collector |
| `tests/pts/verify-l4b-pts-correctness.sh` | Correctness verifier: ENGINE distribution, seq gaps, DLQ/pending/settlement |
| `tests/pts/verify-l2p3-pts-evidence.sh` | Legacy mixed-protocol verifier; harness/instrumentation only |
| `tests/pts/verify-l2p4-pts-evidence.sh` | S2 optional PTS chart verifier |
| `tests/pts/fetch-pts-sampling-logs.sh` | Optional sampling-log helper |
| `tests/pts/summarize-pts-sampling-logs.sh` | Optional sampling-log summarizer |

## Interpretation Rules

- M1 final bid decision requires `result in ENGINE_*` and `durability_status=ENGINE_DURABLE`.
- `ENGINE_DURABLE` is the Redis decision-log boundary. Kafka relay, PostgreSQL settlement, and outbox delivery must converge before citing correctness.
- HTTP `200` count is not accepted-bid count.
- HTTP `202` / `PROCESSING_RETRY_LATER` RTT is not M1 decision latency.
- Dominant `PROCESSING_RETRY_LATER`, vague `409`, pending Redis decisions, Kafka lag, DLQ, engine pause, settlement gap, or outbox backlog fails current pass evidence.
- A latency number without verifier output is not success evidence.
- A file existing here does not make it current evidence. Historical scripts are indexed in `tests/pts/HISTORICAL.md`.
