# Judge Report Template — how to present the evidence

> Status: governing report structure, 2026-06-02.
> Use this to assemble the exported PTS PDFs + Grafana screenshots + verifier
> output into one narrative. The report's job is not "we hit a big number" — it is
> **"our architecture is trustworthy in production, and here is the proof."**

## Structure (one section each)

Every scenario explanation must follow
[explanation-template.md](explanation-template.md). In particular, each number
must include load scale, per-user behavior/pacing, duration, metric boundary,
user-visible interpretation, safety gates, and claim boundary. Do not publish
bare numbers such as "RTO 19s" or "1000 WS pass" without that context.

### 1. Executive summary — conclusion first
Lead with the verdict and the pass/fail matrix. Example wording:

> "Under a 1000-user final-second contention burst on one hot auction, bid
> decision p99 = __ ms (≤ 50 ms), the winner is the highest valid bid, and every
> reject carries a decision-time basis. Sustained 10-min realtime load holds p99 =
> __ ms with flat heap/goroutine/fd. A single room held __ WS with fanout p99 =
> __ ms (≤ 1 s). Seven fault injections recovered with RTO gates of 2–16 s,
> RPO = 0, and zero duplicate settlements. Per-node capacity is one shard; the horizontal path is
> room-sharded gateways + sharded pub/sub + auction-partitioned Kafka."

| Scenario | 正确性 (M3) | 性能 | 韧性 | Verdict |
|---|---|---|---|---|
| S1 绝杀 (PTS) | PASS | decision p99 = __ ms | — | ✅ |
| S2 稳态 + soak | PASS | p99 = __ ms | no leak | ✅ |
| S3 围观 (PTS) | PASS | fanout p99 = __ ms @ __ WS | — | ✅ / ⚠ ceiling@__ |
| S4 故障 ×7 | PASS | — | RTO gate 2–16 s, RPO=0 | ✅ |
| S5 重连 | PASS | TTCS = __ s | no lost/dup | ✅ |

### 2. Test architecture — what was measured, where
A topology diagram + a sentence on instrumentation:

```
[PTS pressure IPs, same VPC] --HTTP/WS--> [Go Gateway] --Lua--> [Redis single-writer engine]
                                              |                         |
                                              |--append-->[Kafka WAL]   |--Pub/Sub-->[WS fanout]
                                              |                         |
[k6 + Toxiproxy, local] --fault-->            +--replay-->[Settlement]--+--> [PostgreSQL]
[Prometheus + Grafana] scrape the gateway/Redis/Kafka/PG the whole time
```
State: PTS = same-VPC (clean latency, M2 clock valid); server metrics from
Prometheus; correctness from `verify-*.sh`; faults local (0 VUM).

### 3. Per-scenario pages (one per S1/S2/S3)
Each page carries the required explanation template plus these four artifacts:
1. **Config**: VU/RPS, duration, offered rate, connection count, profile.
2. **The latency chart** — the exported PTS per-sampler p99 (the named sampler =
   the metric; see [pts-playbook](pts-playbook.md)). For S2/S3 soak, the Grafana
   time-series.
3. **Resource panel** — Grafana CPU / heap-floor / goroutines / fd (+ Redis ops,
   Kafka lag where relevant).
4. **Conclusion + root cause** — the *why*, in one paragraph. This is what
   separates a credible report from a screenshot dump. Example:
   > "p99 stayed ≤ 50 ms through the burst. Accepted updates = __/s (price-ladder
   > bound, not a system limit); the engine adjudicated __ decisions/s. No outbox
   > backlog, no pending Redis settlement."

### 4. Fault timeline page (highest technical signal)
For each P0 fault, first state the load model and user-visible meaning using
[explanation-template.md](explanation-template.md), then show the four artifacts
from [s4 §8](s4-fault-resilience.md):
RTO timeline with timestamps, RPO=0 reconciliation table, fail-closed statement
(zero phantom accepts), recovery curve. Example timeline:
```
T+0s   SIGKILL settlement worker mid-batch
T+1s   backend restart; Kafka redelivers uncommitted batch
T+__s  settlement drained; 0 duplicate (epoch, engine_seq) rows; accepted==WAL==settled
RTO = __ s   RPO = 0
```

### 5. Scale-out & production recommendation
From [scale-out-and-architecture-ceilings.md](scale-out-and-architecture-ceilings.md):
per-shard capacity, which ceilings are infra vs architecture, the 100k path, the
multi-broker anti-split-brain config. Give a concrete deployment line:
> "One 8c32g node = one shard, safely ~__ WS and __ decisions/s at target p99.
> For N× scale: room-sharded gateways behind LB, sharded pub/sub, Kafka
> partitioned by `auction_id`, settlement workers ≤ partition count."

## Assembling the artifacts
- PTS: 报告导出 → **无水印 PDF** (S1, S2-RPS, S3). 报告对比 for before/after
  (PG-lane vs Redis single-writer) to *show* the optimization.
- Grafana: screenshot the soak panels (S2/S3 M4) and fault recovery curves (S4).
- Verifier: paste `verify-*.sh` PASS output and the reconciliation counts.

## Anti-patterns (what makes judges distrust the report)
- Only QPS/TPS, no correctness — "how do you know nothing was lost?"
- HTTP 200 cited as auction outcome — inspect `ENGINE_*`/durability/settlement.
- A peak number with no duration — "5000 once" ≠ "stable at 5000".
- Guessed RTO, recovery on one good sample.
- Accepted-TPS as the headline (price-ladder artifact).
- "We have HA / it scales" with no test and no scale-out specifics.
- Client-only latency with no server-side metric.
