# Live-Auction Test Strategy — Governing Spine

> Status: governing test-strategy entry point, 2026-06-02.
> Scope: performance (PTS) + fault/resilience (local k6) for the hot bid path,
> realtime fanout, and recovery. This folder is the single authority for the
> test plan, names, run order, and judge-facing evidence. `tests/pts/MANIFEST.md`
> is only the script/data asset index.

## 0. Read this first

This strategy was rebuilt to fix three concrete problems, not to throw away
working tests:

1. **Sprawl.** Many legacy asset names existed across smoke, component,
   protocol, scenario, and combined tests. They are now asset aliases only; the
   plan names are S0-S5.
2. **PTS friction.** Repeated harness-invalid runs (`TZ9GX7ZG`, `GUA1X7HG`,
   `58A5X7KG`, `3W9CX76G`) burned money producing reports that proved nothing.
3. **Cost blowout.** Single runs cost 18k–52k VUM each, far over the
   ~10 000 VUM/run budget for one 8c32g node.

The fix is **fewer tests, each tied to one business scenario and one headline
number, with the chart coming straight out of the PTS report or Grafana.**
What was already strong — final-second contention and concurrent fault evidence
— is kept and reframed, not rebuilt.

## 1. Design principles (and the research behind them)

| Principle | Why | Source |
|---|---|---|
| **Correctness is the gate, not a bonus.** Every perf run carries a correctness verifier; a latency number without it is not evidence. | Brief weights "工程完整度 + 可用性 + 一致性" at 50%. | brief §评分标准 |
| **Few, precise metrics.** 5 headline numbers total; each maps to exactly one named PTS sampler or one Grafana panel. | Skeptical judges interrogate every number; a wall of metrics dilutes the ones that matter. | [metrics-and-slo.md](metrics-and-slo.md) |
| **Decision goodput, never accepted-TPS, as the bid headline.** In an ascending auction accepted-bids/sec is bounded by the *price ladder*, not the system. | A reject is a correct, fast decision; low accept-rate is the auction rule, not a bottleneck. | AWS Builders' Library (goodput vs throughput) |
| **Open model for sustained load.** Closed VU loops self-throttle under stress and hide overload (coordinated omission). Use local k6 `constant-arrival-rate` / `ramping-arrival-rate` for the required S2 steady soak; report dropped iterations. PTS JMeter steady runs are optional chart artifacts, not the primary open-model proof. | A closed-loop p99 can be ~25× optimistic vs the true tail. | k6 open-vs-closed; Gil Tene |
| **Per-shard capacity + scale-out narrative.** Report single-node numbers as "one shard's worth" and name the horizontal path. | Only the *per-auction single sequencer* is a true architecture ceiling; everything else (Redis pub/sub, Kafka partitions, WS gateways) scales by sharding. | [scale-out-and-architecture-ceilings.md](scale-out-and-architecture-ceilings.md) |
| **Production HA claims stay falsifiable.** S4/S5 local pass is not presented as Kafka RF=3 / Redis HA / multi-gateway proof; those are separate topology tests with quorum, fencing, LB/NAT, mobile, and AZ evidence gates. | Judges will attack the gap between local chaos and production incidents. The answer must be a concrete failure contract and next-test matrix, not "add HA." | [production-ha-expansion-and-judge-defense.md](production-ha-expansion-and-judge-defense.md) |
| **PTS for charts, local k6 for everything cheap.** PTS earns its cost only where distributed source IPs + clean per-sampler p99 export add judge value. | PTS bills `⌈maxVU/500⌉×500×min×(1+sample%)`; soak/fault/reconnect need neither. | [pts-playbook.md](pts-playbook.md) |
| **Keep PTS sampling cheap.** PTS report percentiles are the chart source; sampling logs are for body forensics. Keep sampling at 1% unless debugging. | Higher sampling multiplies VUM cost and does not replace verifier evidence. | Alibaba PTS billing/report docs |

## 1a. Evidence layers: proving pressure reached the target

Every S1-S5 claim must name the evidence layer it comes from. This prevents a
common review mistake: treating a load-generator sampler count, a backend metric,
and a database row count as the same thing.

| Layer | What it proves | What it does not prove |
|---|---|---|
| PTS/k6 load-generator counts | the offered workload was executed: iterations, sampler `AllCount`, dropped iterations, HTTP failures, assertion failures, fault-window counters | not raw WebSocket frame count; not PostgreSQL row count; not financial finality |
| sampled logs / response markers | forensic detail for a subset of requests: response body, live-message markers, max observed fanout latency, final decision fields | not the authoritative count source when sampling is 1%; not a substitute for service metrics |
| service metrics | pressure reached the intended subsystem: Redis decisions, Kafka lag, settlement/outbox drain, WS publish subscriber fanout, reconnect/recovery counters, queue depth | not necessarily user-visible p99 unless paired with client sampler timing |
| database truth | business effects and finality: accepted/rejected bid rows, settlement rows, orders, outbox rows, duplicate-effect checks | not every WebSocket delivery; fanout to 3000 viewers is not stored as 3000 DB rows |

Scenario-specific pressure proof:

| Scenario | "Pressure reached target" proof |
|---|---|
| S1 | PTS `bid-decision` sampler count and timestamp span prove the final-second arrival; verifier/DB prove all unique bids became final accepted/rejected decisions with winner/reject correctness. |
| S2 | k6 open-arrival decisions, `dropped_iterations=0`, HTTP failures=0, and service-side Kafka/Redis/PG/outbox convergence prove sustained decision pressure actually reached the Redis/Kafka/settlement chain. |
| S3 | PTS viewer/bid sampler counts prove the room workload ran; service `auction_ws_publish_subscribers_sum = accepted publishes * subscribers` proves fanout pressure. For example, `299400` means 100 accepted publishes to 2994 subscribers, not 299400 database rows. |
| S4 | k6 fault-window counters prove traffic overlapped the injected fault; service convergence/verifier gates prove fail-closed, RPO=0, no phantom accepts, and no duplicate settlement effects. |
| S5 | k6 reconnect counters prove stale-`last_seq` clients actually reconnected after missing seqs; service `ws_reconnect`/`ws_recovered` counters and gap/dup/truth checks prove backend recovery, not just socket open success. |

## 2. The spine — six business scenarios

Each scenario is one user story, proves one thing, runs on one tool, and emits
one (occasionally two) headline numbers. IDs are scenario-driven (`S0`–`S5`).
Old `L*` names are not plan stages; they are script/data aliases only.

| ID | Scenario (业务语言) | Proves | Tool | Headline metric | Script/data asset |
|---|---|---|---|---|---|
| **S0** | 单人闭环 — one user completes the whole flow | engineering chain closes (login→join→bid→broadcast→settle→pay) | smoke / local | PASS/FAIL | `tests/pts/L0-smoke/live-auction-pts-business-smoke.jmx` |
| **S1** | 绝杀时刻 — N users bid in the final second on one auction | contention correctness + tail latency | **PTS JMeter** | **bid decision p99 ≤ 50ms** + winner correct + every reject justified | `tests/pts/L1-component/pts-1b-contention-burst-1000vu-1m.jmx`; ladder control `pts-1a-*` |
| **S2** | 正常竞价 — minority bid steadily while viewers poll state | bid-decision long soak, convergence drain, capacity knee, and HTTP read interference | **independent-ECS k6 required** + optional PTS RPS chart | steady decision p99 + read p99 + accepted-update rate + backlog/convergence + flat resources | `tests/load/s2-steady-soak.js`; `tests/load/s2-read-interference.js`; expanded split in `s2-s3-expanded-test-design.md` |
| **S3** | 万人围观 — one room, 10 000 online, price broadcast to all | live-only fanout plus final-burst integration | **PTS VU/JMeter** + local or independent-source k6 | **fanout publish→receive p99 ≤ 1s** + connections held + RAM/conn | `tests/pts/S3-room-fanout/*`; `tests/load/s3-fanout-soak.js`; expanded split in `s2-s3-expanded-test-design.md` |
| **S4** | 故障韧性 — Redis/Kafka/PG/worker fault under live bidding | fail-closed, relay/replay convergence, RTO, RPO=0, no double-charge | **local k6 + Toxiproxy/SIGKILL** | **RTO** + **RPO=0** + zero phantom accepts + zero duplicate settlement | `tests/pts/run-pts-1c-concurrent-fault.sh`; `tests/chaos/*` |
| **S5** | 断连重连 — weak network drops WS, client recovers to current state | WebSocket stability / auto-reconnect / heartbeat | **local k6** | time-to-current-state + no lost/dup notifications | `tests/load/s5-reconnect-recovery.js` |

Per-scenario specs: [s1](s1-final-second-contention.md) · [s2](s2-steady-auction-and-soak.md) ·
[s3](s3-room-fanout.md) · [s4](s4-fault-resilience.md) · [s5](s5-reconnect-recovery.md).

## 3. Mapping to the official scoring rubric (逐项对应)

The brief's rubric, mapped cell-by-cell to the scenario that earns it:

| Rubric cell (考察要点) | Weight | Earned by |
|---|---|---|
| 完整工程链路闭环（采集→后端校验/状态机→网关→前端交互） | 50% | **S0** (chain) + **S1** (server-authoritative decision) |
| 系统可用性（断连重连、异常兜底） | 50% | **S4** (fail-closed / recovery) + **S5** (reconnect) |
| 性能 | 50% | **S1** (decision p99) + **S3** (fanout p99) |
| 稳定性（缓存防击穿、数据一致性） | 50% | **S2** (soak/no-leak) + **S4** (RPO=0 / no double-charge) + correctness verifier on every run |
| 可观测性（竞拍状态监控、异常告警） | 50% | Grafana/Prometheus panels referenced in **S2/S3/S4** |
| 技术选型适配高并发直播竞拍 | 25% | **S1** (Redis single-writer) + **S3** (room-isolated WS) — see [scale-out](scale-out-and-architecture-ceilings.md) |
| 针对核心挑战的针对性优化（实时同步/高并发/WS不稳定） | 25% | **S1/S2/S3** (latency) + **S4/S5** (WS resilience) |
| 前瞻性（房间级WS路由隔离、出价幂等性、跨端状态同步） | 25% | **S3** (room isolation) + **S4** (idempotency / no double-charge) |
| 加分：单直播间 1000+ 同时在线（超基础 10×） | bonus | **S3** (10 000 WS headline) |
| 加分：分布式锁解决幂等性，绝不一笔扣两次钱 | bonus | **S4** (settlement-replay no-duplicate test) |

## 4. Tooling split & cost model

PTS bills **`VUM = ⌈maxVU/500⌉ × 500 × duration_min × (1 + sample_rate)`**,
public cloud ¥0.003/VUM, 1 pressure IP ≈ 500 VU. Keep sampling at the free 1%.

| Test | Where | Scale | VUM | ≈ ¥ | Why there |
|---|---|---|---|---|---|
| S1 绝杀 | **PTS JMeter** | 1000 bid VU, one bid each, 1-2 min | 1k-2k | 3-6 | signature contention chart + distributed source IPs |
| S2 稳态 soak | **independent ECS k6** | 20/s → 60/s → 100/s bid attempts, 30-60 min | 0 | 0 | open-model bid-decision stability, dropped-iteration visibility, Grafana resource slope |
| S2 optional PTS chart | **PTS JMeter** | 2400 WS + 360 bidders + 240 readers, 10 min | ~30k | ~90 | polished realtime-steady PDF if budget permits |
| S3 cost variant | **PTS JMeter** + local | 2000 WS PTS ×5 min + 10000 WS local soak | ~10k | ~30 | PTS p99 chart plus free 10k hold/leak evidence |
| S3 headline | **PTS JMeter** | 10000 WS ×5 min, 20 IP | ~50k | ~150 | single PTS report for 10k online + fanout p99 |
| S4 / S5 | **local k6 + chaos** | S4: 200 paced bid VU; S5: 20-200 reconnect VU | 0 | 0 | correctness/recovery needs system evidence, not paid distributed IPs |

Cost note: use the S3 cost variant unless you specifically need one PDF showing
10 000 PTS WebSocket users. The minimum credible PTS spend is S1 plus S3 cost
variant; S2 can be local k6 unless a polished PTS steady chart is worth the cost.

## 5. Staging — what to finish first, what is minimum

| Tier | Contents | Story it tells | State |
|---|---|---|---|
| **P0** (minimum credible) | S0 + S1 (PTS) + S4 {Redis fail-closed, settlement no-double-charge, PG-down no-loss} | "Correct under peak contention, resilient, never double-charges." Covers the 50% core. | S1 ✅, S4 P0 ✅ |
| **P1** (completes realtime story) | S2 steady soak (local) + S3 fanout cost variant + S4 {Kafka, redis-flush, both} | "Stable under sustained realtime load; the bonus 1000+ room works." | S4 P1 ✅, S2/S3 evidence tracked separately |
| **P2** (stretch / capacity) | S2 optional PTS chart + S3 10 000-WS PTS headline + S5 reconnect + multi-room | "We know the single-node ceiling and the horizontal path." | optional |

Do not start a tier before the one below it passes. A higher-layer regression is
isolated by re-running the lower layer (the layering that already exists in the
MANIFEST is preserved — only the *names and headline framing* change).

## 6. Document index

| File | Purpose |
|---|---|
| `README.md` (this) | spine: scenarios, metrics map, cost, staging, reading order |
| [`metrics-and-slo.md`](metrics-and-slo.md) | the 5 headline metrics, exact definitions, SLO targets, what is *not* a metric |
| [`pts-playbook.md`](pts-playbook.md) | how to make PTS emit exactly the chart you want, cheaply; sampler naming; pitfalls checklist |
| [`s1-final-second-contention.md`](s1-final-second-contention.md) | 绝杀: burst vs ladder, script logic, PTS config, metric→chart |
| [`s2-steady-auction-and-soak.md`](s2-steady-auction-and-soak.md) | 稳态: open-model arrival, realistic bid mix, leak detection |
| [`s2-settlement-diagnosis-and-judge-defense.md`](s2-settlement-diagnosis-and-judge-defense.md) | S2 settlement bottleneck diagnosis, rejected write amplification, and judge Q&A |
| [`s2-s3-expanded-test-design.md`](s2-s3-expanded-test-design.md) | S2/S3 split into long soak, convergence, capacity, read interference, live-only fanout, and mixed final burst |
| [`independent-k6-runbook.md`](independent-k6-runbook.md) | independent ECS k6 deployment, host monitoring, evidence gates, and S1-S5 tool choice |
| [`s3-room-fanout.md`](s3-room-fanout.md) | 围观: fanout latency measurement, 10k headline + cost variant, RAM/conn |
| [`s4-fault-resilience.md`](s4-fault-resilience.md) | 故障: chaos structure, minimal fault set, RTO/RPO, no-double-charge test |
| [`s4-fault-resilience-judge-defense.md`](s4-fault-resilience-judge-defense.md) | S4 exact workload, user-visible fault meaning, current evidence, and judge Q&A |
| [`s5-reconnect-recovery.md`](s5-reconnect-recovery.md) | 断连重连: time-to-current-state |
| [`s5-reconnect-judge-defense.md`](s5-reconnect-judge-defense.md) | S5 workload, user-visible meaning, current numbers, and judge Q&A |
| [`scale-out-and-architecture-ceilings.md`](scale-out-and-architecture-ceilings.md) | infra-vs-architecture boundary, per-shard framing, judge Q&A prep |
| [`production-ha-expansion-and-judge-defense.md`](production-ha-expansion-and-judge-defense.md) | Kafka RF=3/minISR=2, Redis HA failover, multi-gateway, LB/NAT, mobile weak network, and cross-AZ/region judge-defense expansion plan |
| [`s1-s5-vs-legacy-pts-chaos-audit.md`](s1-s5-vs-legacy-pts-chaos-audit.md) | internal audit: S1-S5 vs legacy L1-L4 assets and chaos-script gaps |
| [`s1-s5-debug-and-system-change-log.md`](s1-s5-debug-and-system-change-log.md) | S1-S5 debugging history, failed attempts, system-code changes, and judge-defense engineering narrative |
| [`judge-report-template.md`](judge-report-template.md) | the report structure to present, with PTS PDFs slotted in |

## 7. Naming rule

Use only S0-S5 in plans, reports, and judge material. Old `L*` names may appear
only as asset aliases in `tests/pts/MANIFEST.md`, old PTS report reviews, or file
names that are already committed. Do not create new plans named L2/L3/L4.

## 8. Reading order by role

- **Implementer (another model/engineer building scripts):** this README → the
  target `sN-*.md` → `pts-playbook.md`.
- **Reviewer writing the judge report:** `judge-report-template.md` →
  `metrics-and-slo.md` → each `sN` "what to show" section.
- **Architect prepping for interrogation:** `scale-out-and-architecture-ceilings.md`.
