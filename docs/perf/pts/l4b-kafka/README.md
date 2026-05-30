# L4B Kafka PTS Evidence Pack

Date: 2026-05-29

Supersession note, 2026-05-30: the final architecture direction for high-value
live auction is now
`docs/perf/pts/l4b-kafka/single-hotspot-redesign-from-first-principles-2026-05-30.md`.
The implementation plan is
`docs/perf/pts/l4b-kafka/route-b-implementation-plan-2026-05-30.md`.
This README records the current/pre-redesign L4B implementation and PTS evidence.
Do not use the Redis-only HTTP success behavior described below as the final
safety contract.

This folder records the Kafka/L4B pressure-test background, decisions, workflow,
and correctness gates after the project moved the hot bid path from PG-lane to
Redis Lua + Apache Kafka ledger.

## Why This Exists

The goal is not to make one PTS report look good. The goal is to prove, under
cloud pressure, that the new hot path improves latency while preserving auction
truth:

```text
current implementation / pre-redesign:
HTTP bid
  -> Redis Lua per-auction decision
  -> Redis pending hash protected by AOF
  -> settlement worker pumps pending decisions to Kafka in engine_seq order
  -> PostgreSQL settlement truth
  -> auction_events/outbox/realtime projection
```

The hot request path intentionally does not wait for Kafka append. Redis Lua
atomically updates the auction state and records the decision in
`bid:{auction_id}:engine:pending`. A settlement worker owns the append step,
uses a per-auction Redis `SETNX` lock with a unique token, writes decisions to
Kafka in `engine_seq` order, and deletes the pending field only after Kafka
append succeeds. This is the current compromise between the business goal
(low-latency accepted bids on one hot auction) and durability (no Redis-accepted
decision may disappear silently before Kafka/PG settlement).

This compromise is no longer the proposed final high-value auction contract.
For final course defense, HTTP success must move behind a Kafka durable decision
ack as defined in the single-hotspot redesign document.

A run is useful only if it answers both questions:

- performance: where is the first bottleneck;
- correctness: did Redis/Kafka/PG/outbox converge without gaps, duplicates, or
  stranded accepted work.

The Redis pending hash is a durable retry buffer, not an immediate failure
state. Reconcile must first drain pending decisions into Kafka and must not pause
an auction while another worker owns the pending-append lock. It fail-closes only
when pending work remains after a recovery attempt and no append worker is
actively appending it. Kafka append stays synchronous with all-acks and a bounded
retry budget; this preserves the ordered ledger contract without converting
short Kafka hiccups into false engine pauses.

## Current Artifacts

| Purpose | Path |
|---|---|
| Authoritative safety redesign | `docs/perf/pts/l4b-kafka/single-hotspot-redesign-from-first-principles-2026-05-30.md` |
| Route B+ implementation plan | `docs/perf/pts/l4b-kafka/route-b-implementation-plan-2026-05-30.md` |
| Workload matrix | `docs/perf/pts/l4b-kafka/workload-matrix-20260529.md` |
| PTS-1 runbook | `docs/perf/pts/l4b-kafka/final-burst-1000vu-runbook.md` |
| PTS-1A accepted ladder JMX | `tests/pts/pts-1a-accepted-ladder-1000vu-1m.jmx` |
| PTS-1B contention burst JMX | `tests/pts/pts-1b-contention-burst-1000vu-1m.jmx` |
| Full-check PTS-1 JMX | `tests/pts/live-auction-l4b-final-burst-1000vu-1m.jmx` |
| Longer PTS-1 JMX | `tests/pts/live-auction-l4b-final-burst-1000vu-2m.jmx` |
| Legacy PTS-1 JMX | `tests/pts/live-auction-l4b-final-second-1000vu.jmx` |
| Current upload JMX | `tests/pts/pts-1a-accepted-ladder-1000vu-1m.jmx` |
| PTS-1 CSV | `docs/perf/pts/pts-1ab-1000vu-sessions.csv` |
| Reset/seed | `tests/pts/reset-l4b-final-second-pressure.sh` |
| Preflight guards | `tests/pts/preflight-l4b-pts-guards.sh` |
| Post-run correctness gates | `tests/pts/verify-l4b-pts-correctness.sh` |

## Two-Layer Gate

Layer 1 checks that the implementation and runtime environment contain the
required protections before pressure starts:

```bash
bash tests/pts/preflight-l4b-pts-guards.sh before-l4b-final-second-1000vu-YYYYMMDD-HHMM
```

Layer 2 checks whether the pressure run exposed any correctness breach:

```bash
bash tests/pts/verify-l4b-pts-correctness.sh after-REPORTID-l4b-final-second-1000vu
```

Both scripts exit non-zero on P0 failures. A PTS report cannot be used as
performance evidence unless both layers pass.

## PTS Sequence

| Round | Scenario | Business Meaning | Primary Question |
|---|---|---|---|
| PTS-1A | Accepted ladder | 1000 bidders submit narrowly spaced increasing bids | Can Redis Lua + Kafka + PG/outbox settle the accepted hot path under real monotonic-price rules? |
| PTS-1B | Contention burst | 1000 bidders submit one bid at the same barrier with high-stride prices | Does engine order, deterministic reject handling, and final highest-price winner remain correct? |
| PTS-2 | Sustained accepted-bid saturation | A hot room stays busy for minutes | Where is the throughput knee: Redis, Kafka, settlement, PG, outbox, CPU? |
| PTS-3 | Watcher fanout overlay | Many viewers watch bid changes | Does realtime fanout lag or memory grow under real bid events? |
| PTS-4 | Reconnect storm | Weak-network viewers reconnect during pressure | Does history/snapshot recovery converge to server truth? |
| PTS-5 | Hot/cold isolation | One room is hot while other rooms are normal | Does a hot auction affect cold-room latency or leak events? |
| PTS-6 | Admission-on overload | Production protection behavior | Do limits shed load while preserving already accepted work? |

## Current PTS-1 Semantics

The prepared PTS-1A/1B JMX files are one-shot final-window workloads:

- `ThreadGroup.num_threads=1000`;
- `LoopController.loops=1`;
- `burst_wait_ms=40000`;
- 10-second wall-clock barrier quantum;
- PTS-1A uses `accepted_ladder_step_ms=10`;
- PTS-1B submits all bids at the shared barrier and derives prices from CSV rank;
- a post-bid hold sampler keeps each VU alive until about 60 seconds;
- each VU sends exactly one bid after the barrier opens;
- snapshot and websocket-ticket background groups are disabled.

This means PTS-1 does **not** create a continuous loop. The hold time after the
barrier is there to keep Alibaba PTS from ending the scene before it has started
the full 1000-VU cohort; it is not intended bid traffic.

It also means PTS-1B is a contention/reject burst, not a true hammer-boundary
soft-close sniper test. The reset script keeps `auc_live.end_at` far in the
future to avoid accidental auction closure during paid cloud validation. A
future soft-close sniper workload should get its own `PTS-1C` artifact that
aligns `end_at` with the barrier.

## Required PTS-1 UI Settings

| Setting | Value |
|---|---|
| Pressure mode | Virtual users |
| Traffic model | Manual speed or uniform ramp-up |
| Maximum virtual users | 1000 |
| Test duration | 1 minute |
| Ramp-up duration | 1 minute |
| Specify loop count | Yes: 1 |
| Specified IP count | Default unless PTS quota requires otherwise |
| CSV data file | Enable Split File / 切分文件 |

Start all 1000 VUs immediately and let the selected JMX barrier release
near the final 15 seconds of the 1-minute scene. The 10-second wall-clock
barrier is intentional: it aligns multiple PTS agents to the same release
boundary. With `burst_wait_ms=40000`, PTS-1B releases at about 40-50 seconds;
PTS-1A then spends about 10 seconds on its 10ms ladder, leaving PTS
sampler/reporting headroom before the 60-second scene ends. The JMX then
holds threads until about 60 seconds so PTS records a stable one-minute scene
instead of auto-ending after the one-shot POSTs finish. This avoids Alibaba PTS
multi-thread-group loop semantics and avoids the earlier 25-second auto-finish
harness gap. If the specific claim is soft-close behavior inside the final 1-5
seconds, create/use the separate PTS-1C soft-close artifact instead.

The accepted-dominant JMX is the primary PTS-1A hot-path evidence workload. Its
bid amount and release offset are derived from the CSV `user_id`, not a
JVM-local JMeter property counter, because PTS may run multiple pressure agents
and JVM properties are not a cross-agent atomic counter. The default release
interval is `accepted_ladder_step_ms=10`, selected after `KY3UX7QG` showed that
a 1ms ladder was below the real PTS/JMeter cross-agent scheduling precision and
`913WX7HG` showed that 5ms still produced 229 arrival-order price inversions and
316 correct `BID_TOO_LOW` rejects on two PTS agents. `TR3VX7RG` showed that
`burst_wait_ms=54000` plus a 10-second barrier alignment can push the first bid
to the 60-second scene boundary and produce zero business traffic. `WT3VX7WG`
then showed that a 1-second barrier avoids the scene boundary but lets two PTS
agents drift apart, and that lexical CSV ordering can stretch the selected
`bucket/lane` ranks beyond the intended 1000-rank ladder. The corrected default
is `burst_wait_ms=40000` and `accepted_barrier_quantum_ms=10000`, paired with a
numeric-bucket CSV, so all agents share a wall-clock barrier and the 10ms ladder
completes before scene teardown. Setting the interval to 0ms converts the run
back into simultaneous contention; lower prices that arrive after a higher
accepted price will be correctly rejected. Use `pts-1b-contention-burst` for
that claim; it must not be used to prove accepted-bid throughput.

The L4B PTS evidence is intentionally split into multiple workloads:

- `PTS-1B` final-window contention: simultaneous VU pressure proves engine order,
  deterministic rejects, and final highest-price winner.
- `PTS-1A` accepted ladder: small ordered intervals prove accepted hot-path
  throughput without disabling the real low-price rule.
- `PTS-1C` hammer-boundary sniper: aligns `end_at` with the pressure window to
  prove soft close, cap, and end-time races.
- `PTS-2` RPS capacity: fixed request-rate steps find the sustained throughput
  knee after the business scenarios are correct.

CSV splitting is required for multi-agent PTS runs. If the data file is not
split, each agent starts at the beginning of the same CSV, producing duplicate
users and duplicate `client_bid_id` values. The symptom is a report with about
1000 HTTP samples but only about half that many unique bid/idempotency rows in
PostgreSQL.

The PTS report must show roughly 1000 `POST PTS-1 hotspot bid` samples. A much
larger count means the cloud harness looped the sampler despite the JMX
`LoopController.loops=1`; classify that report as `HARNESS_GAP`, not as PTS-1
capacity evidence.

## Sampling Logs And Percentiles

Alibaba Cloud PTS sampling logs are diagnostic request details. At the default
1% pressure-log sampling rate, a 1000-request run normally yields only about 10
detail rows. That is enough to inspect example request/response bodies, but it
is not enough to compute credible full-run p95/p99.

For paid evidence runs where we need independently calculated HTTP percentiles,
set the PTS advanced pressure-log sampling rate to `100%` and allocate enough
pressure-machine capacity for the extra overhead. The run can then use
`GetJMeterSamplingLogs` as the raw latency ledger only if the pulled row count
matches the expected request count for the sampler.

Pull and summarize after the run:

```bash
PAGE_SIZE=100 bash tests/pts/fetch-pts-sampling-logs.sh REPORT_ID docs/perf/pts/evidence/REPORT_ID/pts-sampling-logs
bash tests/pts/summarize-pts-sampling-logs.sh docs/perf/pts/evidence/REPORT_ID/pts-sampling-logs/sampling-logs.jsonl 1000
```

For a specific sampler, filter by sampler id or label:

```bash
SAMPLER_ID=2 bash tests/pts/summarize-pts-sampling-logs.sh docs/perf/pts/evidence/REPORT_ID/pts-sampling-logs/sampling-logs.jsonl 1000
```

Only cite the script's p50/p90/p95/p99 as full-run percentiles when
`coverage: FULL`. If it prints `coverage: SAMPLE_ONLY`, keep the rows as
diagnostic examples and cite PTS sampler aggregates plus server-side evidence
instead.

## Risk Coverage

PTS-1 can detect these failures after a normal final-burst run:

- Redis pending decisions not reaching Kafka;
- Kafka settlement rows not matching PG accepted bids;
- Kafka offset order diverging from `engine_seq`;
- public `auction_events.seq` gaps and missing accepted/sold settlement event coverage;
- duplicate `client_bid_id` or duplicate `engine_epoch + engine_seq`;
- accepted bids after final `end_at`;
- increment-grid violations;
- duplicate order;
- Redis eviction;
- DLQ messages;
- consumer group lag after the chosen settlement window;
- event payload cross-auction mismatch.

PTS-1 cannot prove failure modes it does not inject:

- process crash after Redis decision and before Kafka append;
- Redis kill/restart during non-empty pending decisions;
- Kafka consumer rebalance under active lag;
- network partition/stale Redis primary;
- proxy/max-bid private-state leak.

Those require PTS-1C or dedicated fault-injection workloads.
