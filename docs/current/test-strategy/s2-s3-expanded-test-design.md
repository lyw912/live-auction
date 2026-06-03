# S2/S3 Expanded Test Design

> Status: governing expansion design, 2026-06-04.
> Scope: split S2 and S3 into judge-defensible workloads with one business
> question per test. New test names use only S2/S3. Legacy L-file names are
> implementation assets only and must not appear as new scenario names.

## 1. Split

| Workload | Primary Tool | Why |
|---|---|---|
| `S2-long-soak` | k6, preferably from an independent ECS | 30-60 min open-model load, cheap and controllable, easy to pair with service-side metrics |
| `S2-convergence-drain` | k6 or PTS | The core evidence is Kafka lag, settlement, Redis pending, and outbox drain time, not the pressure-tool chart |
| `S2-capacity-stair` | k6 first, then PTS RPS for a formal chart | k6 cheaply finds the knee; PTS RPS removes "local generator stole resources" objections |
| `S2-read-interference` | PTS RPS or independent-ECS k6 | Reader traffic is an HTTP RPS problem and should be staged by endpoint and rate |
| `S3-live-only-fanout` | PTS VU mode | WS online-user count is concurrency; PTS multi-IP is better for 2000/5000/10000 connections |
| `S3-mixed-final-burst` | PTS VU/JMeter | WS hold + final bid burst + reader mix is the expensive integration rehearsal, so it runs after isolated gates pass |

This split follows two pressure-model rules:

- S2 HTTP offered load should use an open/RPS model. A closed VU loop can hide
  overload because slow responses reduce the next request rate.
- S3 online viewers should use VU/concurrency. For WebSocket, the business
  variable is how many users are connected at the same time.

Every run must also collect service-side evidence. PTS and k6 provide client
pressure and client-observed latency; they do not by themselves prove Kafka,
Redis, PostgreSQL, outbox, or runtime health.

## 2. `S2-long-soak`

Business story: a normal auction stays live for a long period. A minority of
users bid, most users watch, and the system must not slowly drift.

Recommended shape:

```text
tool             : k6 from a separate same-VPC ECS
duration         : 30-60 min
bid offered rate : 20/s -> 60/s -> 100/s
accepted rate    : measured; expected to be much lower than bid attempts
sampling         : service metrics every 5-10s
```

Evidence:

- bid decision p99/p99.9 by time window;
- `dropped_iterations`;
- delivered rate vs target rate;
- accepted/rejected distribution;
- Kafka lag, Redis pending decisions, settlement gap, outbox unpublished;
- runtime RSS, post-GC heap floor, goroutines, open fds, DB pool stats;
- post-load convergence to all-zero backlog.

Red lines:

- A 2-5 minute stair is not a soak.
- Heap peak is not leak proof; report post-GC floor trend.
- Foreground bid p99 is not payment/finality safety.
- Accepted bids/s is not capacity; decision goodput is accept + reject with
  correctness.

Judge defense:

> "S2-long-soak proves stable operation at the chosen normal-auction rate. We
> report offered rate, delivered rate, dropped iterations, backlog slope, and
> runtime floor together, so a fast p99 cannot hide a growing async backlog."

## 3. `S2-convergence-drain`

Business story: after a hot bidding period or after auction close, payment and
final winner views must wait until the async durability chain is safe.

Recommended shape:

```text
tool              : k6 or PTS
pressure window   : 2-5 min
rate              : known pressure point, e.g. 200/s -> 600/s -> 1000/s
post-run monitor  : continue until all convergence gates are zero
```

Evidence:

- `load_end -> convergence_seconds`;
- Kafka consumer lag samples;
- Redis pending decision samples;
- PostgreSQL settlement total and non-terminal rows;
- outbox unpublished/open deliveries;
- verifier proving durable decisions settled exactly once.

Red lines:

- Do not claim a strict 120.000s bound if the first all-zero sample is later.
- Do not lower correctness gates to make drain look faster.
- Do not open payment/finality while any settlement/outbox/DLQ/backlog remains.

Judge defense:

> "Bidder experience and finance finality are separate clocks. Users can receive
> fast `ENGINE_DURABLE` decisions while payment stays gated until Kafka,
> PostgreSQL settlement, Redis pending state, and outbox are clean."

## 4. `S2-capacity-stair`

Business story: the team knows where the single-auction system bends.

Recommended shape:

```text
tool           : k6 first; PTS RPS for formal evidence if needed
stages         : 100/s -> 200/s -> 400/s -> 600/s -> 1000/s
stage duration : 2-5 min each
```

Evidence:

- p99/p99.9 by stage;
- delivered vs target rate;
- dropped iterations;
- accepted/rejected distribution;
- Kafka/settlement/outbox backlog slope by stage;
- CPU, DB pool wait, Redis latency, Kafka lag, IO.

Red lines:

- Do not describe this as normal user traffic; it is a capacity search.
- Do not compare open-model k6 and closed-loop VU p99 without stating the model.
- Do not stop at "p99 is fine" if backlog grows without bound.

Judge defense:

> "S2-capacity-stair is intentionally adversarial. It finds the knee and names
> the bottleneck; it does not replace the lower-rate long soak."

## 5. `S2-read-interference`

Business story: many viewers refresh room state, leaderboard, and personal bid
history while others bid. Those reads must not steal DB/Redis/CPU from the bid
path.

Recommended shape:

```text
tool           : PTS RPS or independent-ECS k6
bid rate       : 20-100/s
read endpoints : GET auction snapshot, GET leaderboard, GET my bid history
read traffic   : staged RPS per endpoint
duration       : 10-20 min
```

Evidence:

- bid decision p99 under each read stage;
- read p99 per endpoint;
- DB pool acquired/wait, slow queries, Redis command latency, CPU;
- correctness and convergence after the run.

Red lines:

- Do not bury high-frequency read polling inside S3 fanout samplers.
- Do not report only reader VU count; report endpoint RPS and mix.
- Do not accept a read-heavy run if bid correctness or settlement safety fails.

Judge defense:

> "S2-read-interference isolates DB/cache pressure from WebSocket fanout. If a
> leaderboard or my-bids query hurts bid p99, we can attribute it directly
> instead of mislabeling it as a realtime fanout issue."

## 6. `S3-live-only-fanout`

Business story: many users are already watching one room; accepted price updates
must arrive quickly without refresh.

Recommended shape:

```text
tool                  : PTS VU mode for formal chart
viewer tiers           : 1000 -> 2000 -> 5000 -> optional 10000
accepted update source : fixed small source, e.g. 1-5 accepted updates/s
read traffic           : none or minimal
duration per tier      : 2-5 min
metric                 : server published_at_ms -> client receive p99
```

Evidence:

- active WS connections;
- accepted update count and rate;
- receive sample count;
- M2 fanout p99/max;
- server publish subscriber count;
- RSS/fd/goroutine per connection;
- no listener errors, seq gaps, or unexplained churn.

Red lines:

- Do not count snapshot/recovery/history messages as live fanout.
- Do not claim connection count alone as realtime performance.
- Do not claim a rich fanout test from a run with only a dozen public updates.
- Do not let WS hold time contaminate the fanout latency sampler.

Judge defense:

> "S3-live-only-fanout isolates the core realtime problem: viewer count is the
> variable, accepted update rate is controlled, and measured messages are only
> those published after the viewer was online."

## 7. `S3-mixed-final-burst`

Business story: a hot room reaches the final seconds. Viewers are online, some
users read room data, and bidders submit a burst.

Recommended shape:

```text
tool        : PTS VU/JMeter
viewers     : 2000-5000 first; optional 10000 after cost-tier pass
bid burst   : short final window, e.g. 500-1000 bid attempts
reads       : controlled low/medium background, not per-viewer tight polling
duration    : enough for viewer connect, burst, fanout observe, and close
```

Evidence:

- sampler counts and success for join, ticket, WS connect, first message, bid,
  read endpoints, and live fanout;
- bid p99 and correctness verifier;
- accepted update count;
- live fanout p99;
- service metrics for connections, publish subscribers, DB/Redis/Kafka/outbox,
  and settlement convergence.

Red lines:

- Do not run this first; it is too expensive and hard to attribute.
- Do not call it a clean fanout benchmark if reads or settlement dominate.
- Do not use PTS sampling logs as the exact-count ledger; use report details and
  service metrics.

Judge defense:

> "S3-mixed-final-burst is the integration rehearsal. It only becomes meaningful
> after S3-live-only proves fanout and S2-read-interference proves reads do not
> steal the bid path."

## 8. Required Service-Side Collection

For every S2/S3 run, collect:

```text
/metrics before/during/after
Kafka consumer lag and DLQ
Redis pending decisions and hot snapshot state
PostgreSQL settlement coverage and non-terminal rows
outbox unpublished/open deliveries
runtime RSS, heap, goroutines, open fds
DB pool acquired/wait
auction seq, engine_seq, accepted/rejected counts
active WS connections and publish subscriber count for S3
```

For formal evidence, store raw artifacts under
`docs/perf/pts/evidence/incoming/<label>/` and classify the run before citing it.

## 9. Execution Order

1. `S2-long-soak` from a separate same-VPC k6 ECS.
2. `S3-live-only-fanout` PTS cost tier, starting at 2000 WS.
3. `S2-read-interference` with RPS stages.
4. `S2-capacity-stair` to find the knee and attribute bottlenecks.
5. `S3-mixed-final-burst` after the isolated tests are clean.

This order reduces paid debugging and makes each later failure attributable.

