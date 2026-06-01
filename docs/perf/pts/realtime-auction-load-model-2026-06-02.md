# Realtime Auction Load Model

> Status: current design note. Use this to explain why L2-P3 alone is not a
> production-realistic realtime auction proof.

## Research Baseline

- k6 distinguishes closed-model VU loops from open-model arrival-rate workloads.
  For sustained request arrival, `constant-arrival-rate` / `ramping-arrival-rate`
  are better because iteration starts are independent of response time. Closed
  loops can hide overload by slowing down when the system slows down.
- k6's WebSocket docs and metrics separate connection establishment from
  message-level behavior. `ws_connecting` is join/upgrade cost; fanout delivery
  latency must be measured at message receipt.
- Google SRE guidance says high percentiles matter because tail latency and
  queuing shape user experience. It also treats overload as a software/system
  design problem, not something to dismiss as "just infrastructure".
- Redis sharded Pub/Sub exists specifically because global Pub/Sub propagation
  is a cluster-scaling concern. That makes Redis fanout topology an architecture
  decision, not an ops footnote.
- Kafka consumer-group parallelism is bounded by partition assignment: each
  partition is consumed by one consumer in a group. More consumers than useful
  partitions do not linearly increase settlement/replay throughput.

## What Existing L2-P3 Proves

`L2-P3` is still useful, but its claim must be narrow:

- about 1000 synchronized bids can coexist with about 5000 WS viewers and about
  1000 read users;
- PTS can show bid p99, join segments, and client fanout receipt p99;
- every sampled WS connection must receive every expected public seq.

It does **not** prove full production realtime-auction behavior, because the bid
shape is a final-window burst rather than sustained human interaction.

## Judge-Facing Load Layers To Add

| ID | Purpose | Connection model | Bid / message model | Minimum duration | Primary proof |
|---|---|---|---|---|---|
| `L2-P3` | Mixed-protocol correctness and observability gate | ~4998 WS, one-loop | 1008 synchronized bids | ~1 min | Exact sample counts, fanout receipt p99 visible in PTS |
| `L2-P4` | Realtime interactive steady auction | 3000-5000 WS held | Open-model accepted/rejected bid arrivals for 5-10 min; active bidders are a minority | 10 min | bid p99 <= 100ms, fanout p99 <= 1000ms, stable heap/goroutines/fd |
| `L2-P5` | Fanout soak and long-connection stability | 10000 WS held | Low/medium accepted bid source, e.g. 1-10 accepted updates/s | 10-30 min | no connection leak, no goroutine/memory climb, fanout tail stable |
| `L2-P6` | Reconnect storm | existing WS cohort plus reconnect wave | reconnect with stale `last_seq` during ongoing updates | 3-5 min | recovery source distribution, no snapshot rebuild saturation, time-to-current-state |
| `L1-F2` | Fault during sustained interaction | 1000-2000 WS held | sustained bid arrivals while Redis/Kafka/PG/backend fault is injected | 5-10 min | RTO from fault clear, no duplicate/lost accepted bids, fanout catches up |
| `INFRA-F` | Architecture scale boundary | room-sharded WS gateways | Kafka/Redis limits varied by partitions/shards/pool size | per probe | documented bottleneck and scale-out path |

## Realistic Bid Mix

Do not blindly set "80% of VUs bid every 2s" for a single auction. That implies
hundreds or thousands of write attempts per second in one room, which is closer
to an adversarial hotspot than normal live bidding.

A defensible first steady model is:

```text
viewers: 3000-5000 connected
active bidder sessions: 5%-15% of viewers
offered bid arrivals: ramp 20/s -> 100/s -> 200/s
accepted update target: measure actual accepted/s from business rules
duration: 10 minutes
```

The workload should still include rejected bids, stale `client_seen_seq`, and
self-leading attempts, because these are normal in realtime contention. But the
claim should report both offered bids and accepted updates; fanout pressure is
driven by accepted public seqs, not by rejected bid attempts.

## Required Metrics

| User journey | SLI |
|---|---|
| Bid click -> decision visible to bidder | HTTP bid decision p95/p99, `ENGINE_*` distribution |
| Accepted bid -> all connected viewers see new seq | client-side publish-to-receive p95/p99, missing seq count |
| Join room | snapshot load, WS ticket, WS upgrade, time-to-first-current-state |
| Reconnect after weak network | reconnect success, recovery source, time-to-current-state |
| Long room watch | active WS count, heartbeat timeout, slow-consumer close, fd, goroutines, RSS/heap, GC |
| Fault recovery | fault-clear-to-safe convergence RTO, outbox lag, Kafka lag, pending Redis decisions |

## Architecture Questions To Answer

These are judge-facing architecture questions, not test-script details:

- **WS scale-out**: current single-node hub is adequate for a one-node evidence
  run. Multi-node production needs room-sharded WS gateways plus a cross-node
  fanout bus such as Redis sharded Pub/Sub, NATS, or a dedicated realtime layer.
- **Redis**: connection count, memory, and Pub/Sub propagation are architecture
  constraints. Redis Cluster/sharded PubSub helps only if keys/channels are
  shardable by room/auction and clients do not create unbounded connections.
- **Kafka**: settlement/replay throughput is bounded by topic partitioning and
  ordering requirements. Keying by `auction_id` preserves per-auction order but
  limits one auction to one partition unless the engine introduces a finer safe
  ordering key.
- **Backpressure**: when fanout or settlement falls behind, the system needs an
  explicit policy: load shed, slow-consumer close, retry-after, pause auction, or
  degrade reads. "Buy a larger server" is not an acceptable answer.

## Infrastructure Saturation Is Not A Blanket Excuse

Use this table when explaining a bottleneck:

| Bottleneck | Can raw machine scale fix it? | Judge-facing answer |
|---|---|---|
| App CPU/RSS exhausted on one node | Sometimes | report per-node limit, then show horizontal WS gateway plan |
| File descriptors exhausted | Partly | raise ulimit for the node, but also bound per-room connections and shard gateways |
| Redis command CPU/latency | Partly | optimize key/data model first; cluster only helps if keys/channels shard by room/auction |
| Redis Pub/Sub propagation | Not by bigger app nodes | use shard channels or a dedicated fanout bus; avoid every app subscribing to global channels |
| Redis connection count | Not safely | use bounded pools and multiplexed/subscription topology; app-node fanout can make this worse |
| Kafka broker disk/network | Often | add brokers/partitions after measuring broker-side saturation |
| Kafka consumer parallelism | Only if partitions/order model allow | consumer group parallelism is bounded by partitions and the auction ordering key |
| One hot auction ordering | No | a single auction often needs a single sequencer; solve with efficient single-writer path and explicit limits |

The correct language is: "this run hit a single-node/one-broker limit; here is
whether that limit is a resource ceiling or an architecture ceiling, and here is
the next falsifiable test."

## Immediate Decision

Do not use the current L2-P3 result as the final judge-facing realtime load
claim. Keep L2-P3 as an instrumented mixed-protocol gate, then add `L2-P4` and
`L2-P5` before making any statement like "the system supports realtime auction
traffic under high concurrency."
