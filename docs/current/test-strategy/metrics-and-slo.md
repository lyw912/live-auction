# Metrics & SLO — the five headline numbers

> Status: governing metric definitions, 2026-06-02.
> Rule: a number you cannot point at one chart for, and define precisely, is not
> allowed in the judge report. This file is the closed list.

Skeptical judges do not attack a number; they attack the *definition* behind it
("p99 of what, measured from where to where, over which population?"). Every
metric below has a single-sentence definition, an exact measurement boundary, a
target, and the one place the chart comes from.

## The closed list

| # | Metric (判分名) | One-line definition | Target | Chart source |
|---|---|---|---|---|
| **M1** | 出价决策延迟 — bid decision p99 | request start → final `ENGINE_*` visible to bidder; **accepts and rejects both count** | ≤ 50 ms (burst S1); ≤ 100 ms (steady S2) | PTS sampler `出价决策 bid-decision` p99 |
| **M2** | 广播延迟 — fanout p99 | server `published_at_ms` → client receives that public `seq` | ≤ 1000 ms (same region) | PTS `广播接收 ws-fanout-receive` Single-Read p99 + k6 client histogram |
| **M3** | 正确性 — correctness (boolean) | winner = highest valid amount; `engine_seq` gap-free; every reject has decision-time basis; no duplicate accepted/settled | **PASS** every run | `verify-l4b-pts-correctness.sh` + scenario verifier |
| **M4** | 资源稳定性 — stability (soak) | slope of post-GC heap floor, goroutine count, fd count over the soak | slope ≈ 0 (no leak) | Grafana panels (Prometheus) |
| **M5** | 故障恢复 — RTO + RPO | RTO = fault-cleared → steady-state SLI restored over a window; RPO = accepted-but-unsettled count | RTO ≤ 30 s (local); **RPO = 0** | k6 fault harness `recovery-breakdown.json` + reconciler |

Everything else is **supporting context**, reported but never headlined:
offered bid rate, decisions-serialized/sec, accepted-update rate, connections
held, RAM/connection, join-latency components (snapshot/ticket/upgrade),
outbox lag, Kafka consumer lag.

S2 also reports a supporting **settlement convergence** safety signal:
`k6_end -> Kafka lag 0 + Redis pending 0 + PG settlements complete + outbox
drained`. This is measured in seconds, not milliseconds. The current local S2
internal target is <=110s for the 2-minute 200/s -> 600/s -> 1000/s stair, and it
must be proven by a run with that timeout before being marked PASS.

## M1 — bid decision p99 (the signature number)

**Boundary.** Stopwatch starts when the HTTP bid request is sent; stops when the
response carrying the final business decision is parsed. The response is final
only when `durability_status = ENGINE_DURABLE` and `result ∈ {ENGINE_ACCEPTED,
ENGINE_REJECTED, ENGINE_SOLD}`. A `202 PROCESSING_RETRY_LATER` /
`PENDING_DURABILITY` is **acceptance latency, not decision latency** — never cite
it as M1. (Contract: `docs/current/performance-correctness-contract.md`.)

`ENGINE_DURABLE` is the Redis Lua + Redis Stream + idempotency replay boundary.
Kafka relay, PostgreSQL settlement, and outbox/WebSocket delivery are not counted
inside M1; they are mandatory M3/M5 convergence evidence from the same run.

**Why accepts *and* rejects both count.** Under contention most bids are
correctly rejected (below current price). A fast, correct reject is a *successful
decision* and is the bulk of the work. Reporting only accepted-bid latency would
hide the path that actually runs.

**The framing that defeats the "your TPS is tiny" attack.** Report two numbers
together, never accepted-TPS alone:

```
decision goodput   = correct decisions/sec (accept + reject) delivered within p99 ≤ 50ms
decisions/sec      = total bids the single-writer engine adjudicated/sec   ← engine capacity
accepted updates/s = price-ladder progression   ← a BUSINESS property, intentionally bounded
```

> Judge script: *"Accepted bids/sec is low by design — an English ascending
> auction accepts ~one bid per price step. The engine's real capacity is
> decisions/sec (accept + reject), and the user metric is decision p99. We prove
> accept-path capacity separately with the ascending-ladder control (S1-ladder)."*

**Pitfall.** Do not cite a non-existent "native HTTP" script for S1. The current
formal S1 evidence uses the checked-in PTS JMeter asset and the verifier. If a
future native-HTTP PTS script is created, it must first reproduce the same
`ENGINE_DURABLE` assertion and M3 evidence chain before replacing the JMX.

## M2 — fanout p99 (the realtime-sync number)

**Boundary.** The server stamps each broadcast with `published_at_ms` at publish
time. The client computes `recv_local_ms − published_at_ms` for each public
`seq`. M2 is the p99 of that delta across all connected clients and all seqs.

**Clock skew is the trap.** `published_at_ms` is the *server* clock and
`recv_local_ms` is the *client/load-generator* clock. Raw subtraction across
unsynced hosts is meaningless. Two acceptable fixes:
1. **Same-region NTP-disciplined hosts** (PTS pressure IPs in the same VPC as the
   ECS) — residual skew ≪ the 1 s target, so the raw delta is usable. State this.
2. **Round-trip echo** to cancel skew if you ever measure cross-clock (advanced;
   not needed for same-region).

**Two complementary sources, on purpose.** PTS gives the polished exportable
per-sampler p99 chart (the `ws-fanout-receive` Single-Read sampler — see
[pts-playbook.md](pts-playbook.md) §websocket). Local k6 gives the cheap 10 000-conn
client-side histogram for the soak. Reference both; they should agree.

**Reference bar.** Centrifugo published p99 ≤ 200 ms at 500k msg/s on commodity
hardware — cite it so "≤ 1 s same-region" reads as conservative, not lucky.

## M3 — correctness (the gate, not a number)

A boolean that must be PASS on **every** run, perf or fault. The verifier asserts:

| Invariant | Check |
|---|---|
| Highest valid amount wins | settled winner == max valid engine decision |
| Reject is justified | each reject row carries `required_min_price`/`current_price` at decision time |
| Monotonic decisions | `engine_seq` gap-free, or gaps explicitly reconciled/fenced |
| Idempotency | same `client_bid_id`+hash → same result; different hash → conflict |
| No duplicate effect | zero duplicate `(epoch, engine_seq)` settlement rows |
| Settlement coverage | every durable decision settled, bounded-pending, or DLQ/paused |

Existing verifiers: `tests/pts/verify-l4b-pts-correctness.sh` (distribution, seq
gaps, DLQ), plus scenario verifiers (`verify-l2p3/p4`). M3 PASS is a precondition
for citing M1/M2 from the same run.

## M4 — resource stability (the leak gate)

**Definition.** Over a steady soak (≥ 30–60 min), the *post-GC heap floor*, the
goroutine count, and the open-fd count must not trend upward. Watch the floor,
not the sawtooth peaks: a healthy heap resets to a stable floor each GC; a leak is
a floor that climbs and does not recover after load stops.

| Signal | No leak | Leak |
|---|---|---|
| RSS / post-GC heap floor | flat plateau | sustained positive slope |
| `go_goroutines` | flat | monotonic climb (classic WS read-loop leak) |
| open fds | flat | climb (unclosed sockets) |
| p95/p99 over time | flat | slow drift up at constant load |

Source: Prometheus → Grafana. Take a `pprof` heap baseline at warm-up and diff at
the end (`go tool pprof -base`).

## M5 — RTO + RPO (the resilience pair)

**RTO.** The harness emits machine timestamps: `fault_injected`, `first_error`,
`first_success_after`, `slo_recovered`. Recovery is declared only when the
steady-state SLI (e.g. *decision success ≥ 99% AND p99 ≤ 50 ms*) holds over a
trailing window — **never** on a single good sample. `RTO = slo_recovered −
fault_injected` (or, for the user-visible bid path, fault-clear → first sustained
DECIDED). Targets: ≤ 10 s excellent, ≤ 30 s acceptable, ≤ 45 s hard local ceiling
(single-container Kafka cold start). Already realized: S4 measured 3–26 s.

**RPO.** A *data-loss* measure, proven by reconciliation, not uptime:

```
RPO = 0  ⇔  count(distinct accepted decisions, client-confirmed)
            == count(Redis Stream engine decisions)
            == count(Kafka WAL entries)
            == count(distinct settled PG rows)
        AND zero phantom accepts during the fault window
```

The response boundary is `ENGINE_DURABLE`; the data-loss boundary is the
reconciled chain from Redis Stream to Kafka WAL to PostgreSQL settlement. Say
this explicitly — it is the sharpest reviewer question under the current
asynchronous-relay design.

## What is deliberately NOT a headline metric

| Excluded | Why |
|---|---|
| accepted-bids/sec (as headline) | price-ladder artifact, not capacity (M1 framing) |
| HTTP 200 count | not accepted-bid count; outcome lives in `ENGINE_*` fields |
| `202` RTT | acceptance latency, not decision latency (M1 boundary) |
| raw WS connection count alone | capacity, not "low latency"; pair it with M2/M4 |
| client-only latency with no server metric | unverifiable; pair with server-core p99 |
| a peak number with no duration | "5000 VU once" ≠ "stable at 5000 VU"; always co-report duration + offered load |
