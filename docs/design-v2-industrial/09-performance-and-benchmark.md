# 09 · Performance And Benchmark

## Prime Rule

No performance number may appear in README, demo script, slides, or final materials until there is a baseline report with raw output.

Forbidden before baseline:

- QPS claims.
- P99/P999 claims.
- "supports N online users".
- "millisecond latency" as a measured statement.
- comparison with industrial systems by number.

Allowed before baseline:

- architecture invariants;
- workload plan;
- metrics to collect;
- known bottleneck hypotheses.

## Baseline Environment

Final baseline must run on Linux native or clearly documented equivalent. WSL2 is development-only.

Record:

```text
CPU model and cores
RAM
disk
OS/kernel
Go version
PostgreSQL version and settings
Redis version and settings
k6 version
ulimit -n
net.core.somaxconn
ip_local_port_range
CPU governor/power mode
Docker/native boundary
git sha
```

Refuse final baseline if:

- `ulimit -n` too low for target connections;
- CPU power saving mode distorts runs;
- DB/Redis are remote without network latency record;
- scripts changed after raw output.

## Metrics Required

| Layer | Metrics |
|---|---|
| HTTP/bid | throughput, p50/p95/p99/p999, error rate, retry rate |
| DB | lock wait, tx duration, slow query, deadlocks, pool wait |
| Redis | command latency, memory, evictions, blocked clients |
| WS | connection success, fanout latency, dropped/closed, reconnect |
| Runtime | CPU, RSS, heap, GC pause, goroutine, FDs |
| Outbox | backlog, delivery lag, retry, DEAD count |
| Scheduler | drift, stuck jobs, retry |
| UI | longtask count, input blocked, frame stability |

## Workloads

### Final-Second Bid Burst

Purpose: test row lock correctness and latency under contention.

Shape:

- one hot auction.
- many users attempt valid/invalid/duplicate bids in final 1-3 seconds.
- include cap bid and self-leading rejects.

Success invariants:

- seq continuous.
- one winner/order.
- no duplicate bid per idempotency key.
- reject reasons deterministic.

### Watcher Fanout

Purpose: test WS hub memory and fanout.

Shape:

- watcher-heavy room.
- low bid rate.
- event fanout to all connected clients.

Success invariants:

- healthy clients receive events.
- slow clients disconnected.
- RSS bounded.

### Reconnect Storm

Purpose: test history/snapshot protection.

Shape:

- many clients disconnect.
- bids continue.
- clients reconnect with stale `last_seq`.

Success invariants:

- DB snapshot rebuild count <= semaphore.
- clients recover or receive retry-after.
- no wrong active UI during recovery.

### Slow Consumer

Purpose: test backpressure.

Shape:

- some clients stop reading.
- healthy clients remain active.

Success invariants:

- slow clients close.
- healthy fanout unaffected.
- queue depth bounded.

### Outbox Burst

Purpose: test hot table and relay lag.

Shape:

- sustained accepted/rejected bid traffic.
- relay consumes concurrently.

Measure:

- `n_dead_tup / n_live_tup`
- heap/index size
- delivery lag
- claim query explain

### Multi-Room Isolation

Purpose: test hot room does not corrupt cold rooms.

Shape:

- several rooms active.
- one room hot.

Success invariants:

- no cross-room event leak.
- cold room latency does not collapse without diagnosis.

## k6 WebSocket Rule

Use:

```js
import { WebSocket } from 'k6/websockets';
```

Do not use deprecated `k6/experimental/websockets`.

## Baseline Report Template

Use `templates/perf-baseline.md`.

Required sections:

- environment;
- script path and sha;
- dataset;
- workload;
- results table for at least 3 runs;
- raw output link/path;
- errors/timeouts;
- bottleneck;
- next action.

## Bottleneck Triage

If bid latency is high:

1. DB lock wait.
2. transaction duration.
3. connection pool wait.
4. idempotency probe.
5. JSON serialization.
6. outbox writes.
7. runtime GC.

If fanout latency is high:

1. slow-client queue.
2. serialization per client vs once per event.
3. WS write deadline.
4. hub room lock contention.
5. kernel FD/socket limits.
6. client rendering.

If memory grows:

1. connection cleanup.
2. unbounded queues/maps.
3. Redis history retention.
4. goroutine leaks.
5. test client leaks.

If outbox lag grows:

1. delivery shard count.
2. head-of-line blocked event.
3. Redis latency.
4. WS publish blocking.
5. DB update hot tuples.

## Reporting Discipline

Report failed benchmarks honestly:

```text
Target: not claimed.
Observed: X under environment Y.
Bottleneck: Z.
Action: reduce claim / fix / defer.
```

A failed benchmark with diagnosis is stronger than an invented success.
