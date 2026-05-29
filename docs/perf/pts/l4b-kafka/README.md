# L4B Kafka PTS Evidence Pack

Date: 2026-05-29

This folder records the Kafka/L4B pressure-test background, decisions, workflow,
and correctness gates after the project moved the hot bid path from PG-lane to
Redis Lua + Apache Kafka ledger.

## Why This Exists

The goal is not to make one PTS report look good. The goal is to prove, under
cloud pressure, that the new hot path improves latency while preserving auction
truth:

```text
HTTP bid
  -> Redis Lua per-auction decision
  -> Kafka durable ledger
  -> PostgreSQL settlement truth
  -> auction_events/outbox/realtime projection
```

A run is useful only if it answers both questions:

- performance: where is the first bottleneck;
- correctness: did Redis/Kafka/PG/outbox converge without gaps, duplicates, or
  stranded accepted work.

## Current Artifacts

| Purpose | Path |
|---|---|
| Workload matrix | `docs/perf/pts/pts-workload-matrix-20260529.md` |
| PTS-1 runbook | `docs/perf/pts/l4b-final-second-1000vu-runbook.md` |
| PTS-1 JMX | `tests/pts/live-auction-l4b-final-second-1000vu.jmx` |
| PTS-1 CSV | `docs/perf/pts/pts_l4b_final_second_1000vu_sessions.csv` |
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
| PTS-1 | Single hot auction one-shot burst | 1000 bidders submit one bid at the same moment | Can Redis Lua + Kafka accept a final burst and settle correctly? |
| PTS-1B | True last-second soft-close sniper | 1000 bidders fire within the final 1-5 seconds | Does soft close/cap/end_at remain atomic at the hammer boundary? |
| PTS-2 | Sustained accepted-bid saturation | A hot room stays busy for minutes | Where is the throughput knee: Redis, Kafka, settlement, PG, outbox, CPU? |
| PTS-3 | Watcher fanout overlay | Many viewers watch bid changes | Does realtime fanout lag or memory grow under real bid events? |
| PTS-4 | Reconnect storm | Weak-network viewers reconnect during pressure | Does history/snapshot recovery converge to server truth? |
| PTS-5 | Hot/cold isolation | One room is hot while other rooms are normal | Does a hot auction affect cold-room latency or leak events? |
| PTS-6 | Admission-on overload | Production protection behavior | Do limits shed load while preserving already accepted work? |

## Current PTS-1 Semantics

The prepared PTS-1 JMX is a one-shot final-burst workload:

- `ThreadGroup.num_threads=1000`;
- `LoopController.loops=1`;
- `burst_wait_ms=330000`;
- each VU sends exactly one bid after the barrier opens;
- snapshot and websocket-ticket background groups are disabled.

This means PTS-1 does **not** create a 30-second continuous loop. The 30 seconds
after the barrier are response/settlement observation margin, not intended bid
traffic.

It also means PTS-1 is not yet a true hammer-boundary soft-close sniper test.
The reset script keeps `auc_live.end_at` far in the future to avoid accidental
auction closure during paid cloud validation. PTS-1B should align `end_at` with
the barrier when the explicit goal is soft-close/cap/end race validation.

## Required PTS-1 UI Settings

| Setting | Value |
|---|---|
| Pressure mode | Virtual users |
| Traffic model | Manual speed or uniform ramp-up |
| Maximum virtual users | 1000 |
| Test duration | 6 minutes |
| Ramp-up duration | 1 minute |
| Specify loop count | No |
| Specified IP count | Default unless PTS quota requires otherwise |

The ramp must finish before the JMX barrier opens. Do not set ramp-up to 6
minutes for PTS-1; that can leave late VUs unstarted when the barrier opens.

## Risk Coverage

PTS-1 can detect these failures after a normal final-burst run:

- Redis pending decisions not reaching Kafka;
- Kafka settlement rows not matching PG accepted bids;
- Kafka offset order diverging from `engine_seq`;
- seq gaps in `bids` or `auction_events`;
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

Those require PTS-1B or dedicated fault-injection workloads.
